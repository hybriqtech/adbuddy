# ADBuddy M6a · Pipeline de releases · Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que una etiqueta `vX.Y.Z` produzca una release en borrador con los binarios del host y el APK firmado, y que el host solo instale el APK descargado si está firmado con el certificado de hybriqtech.

**Architecture:** Un parser Go del APK Signing Block en `host/internal/phoneapp` (sin verificar la criptografía, que verifica Android) da el SHA-256 del certificado de cada firmante. `Download` lo compara con una constante fijada. goreleaser compila el host. Un workflow compila el APK con la versión de la etiqueta, lo firma con `apksigner` y comprueba con el mismo parser que el certificado sea el fijado. Otro workflow lo prueba todo en cada PR, sin secretos.

**Tech Stack:** Go 1.27 (stdlib: `archive/zip`, `crypto/x509`, `encoding/binary`), goreleaser v2, GitHub Actions, AGP 9 + `apksigner`/`zipalign` (build-tools 37.0.0), POSIX `sh`.

**Spec:** `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md`

## Global Constraints

- Rama `m6a`, en el worktree `C:\Users\Jose\orca\workspaces\adbuddy\skimmer`. Identidad git local hybriqtech. Cada commit acaba con el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
- Solo se tocan estas rutas, acordadas con el supervisor (`adbuddy-e5`): `.goreleaser.yaml`, `.github/workflows/release.yml`, `.github/workflows/release-check.yml`, `scripts/install.sh`, `scripts/install_test.sh`, `scripts/release-key.sh`, `scripts/release-version.sh`, `scripts/release-version_test.sh`, `scripts/release-cert.sh`, `host/internal/phoneapp/**`, `host/cmd/apkcert/**`, `host/internal/cli/appsetup.go`, `host/internal/cli/appsetup_test.go`, la línea de ayuda de `--apk` en `host/internal/cli/setup.go`, las dos líneas `versionCode`/`versionName` de `android/app/build.gradle.kts` (en un commit propio), `LICENSE`, `docs/releasing.md`, la sección Install de `host/README.md` y la sección M6 de `docs/superpowers/plans/pendientes-m3-m5.md`. **Nada más.** No se tocan `emulator.yml`, `host.yml` ni `android.yml`.
- Nada de push, PR ni merge: los autoriza el supervisor al final.
- Etiquetas: `^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})(-rc\.([1-9][0-9]?))?$`, con X ≤ 20 y N ≤ 98. `versionCode = ((X·1000 + Y)·1000 + Z)·100 + S` (S = N en una rc, 99 en la final).
- IDs del Signing Block: v2 `0x7109871a`, v3 `0xf05368c0`, v3.1 `0x1b93ad61`. Atributo de rotación `0x3ba06f8c`. Permitidos que no son de firma: `0x42726577` (relleno), `0x2b09189e` y `0x6dff800d` (sello de origen v1 y v2), `0x504b4453` (dependencias de AGP) y `0x2146444e` (frosting de Play). Cualquier otro ID rechaza el APK.
- El APK se firma con v2 y v3, sin v1. El alias de la clave es siempre `adbuddy`. Hay dos secretos: `ADBUDDY_KEYSTORE_B64` y `ADBUDDY_KEYSTORE_PASSWORD`. Las contraseñas nunca van en la línea de órdenes (`--ks-pass env:…`).
- La release se crea siempre en **borrador**; la publica el usuario.
- Binarios con `CGO_ENABLED=0`, `-trimpath`, `-s -w` y `-X github.com/hybriqtech/adbuddy/host/internal/cli.Version={{.Version}}`.
- Los mensajes al usuario van en inglés, como el resto del host. Los comentarios del código siguen el estilo del paquete (frases cortas en inglés).
- Tests de Go con límite: `go test -timeout 10m ./...` desde `host/`. En este PC no hay JDK ni goreleaser: lo que necesita `apksigner`, `keytool`, Gradle o goreleaser se prueba en la CI (sección "Validaciones manuales pendientes").
- Los scripts son POSIX `sh` (en el PC de desarrollo se ejecutan con Git Bash) y se llaman siempre con `sh scripts/…`, sin depender del bit de ejecución.

Ledger: `.superpowers/sdd/2026-10-01-adbuddy-m6a-releases/progress.md`.

## Mapa de ficheros

| Fichero | Responsabilidad |
|---|---|
| `host/internal/phoneapp/apksig.go` | `Signers` (SHA-256 del certificado de cada firmante) y `CheckSigner` |
| `host/internal/phoneapp/apktest/apktest.go` | Construye APK falsos con Signing Block para los tests (`APK`, `Cert`, `SHA256`, `LP`) |
| `host/internal/phoneapp/apksig_test.go` | Tabla de casos y `FuzzSigners` |
| `host/internal/phoneapp/release.go` | `SigningCertSHA256`, `ErrUnpinned`, `Download(…, cert)` |
| `host/internal/phoneapp/phoneapp.go` | `ErrOtherSigner` en `Install` |
| `host/internal/cli/appsetup.go` | `releaseCert` y los mensajes nuevos de `setupApp` |
| `host/cmd/apkcert/main.go` | Herramienta de la CI: imprime los certificados de un APK o el fijado |
| `android/app/build.gradle.kts` | Versión de la app desde propiedades de Gradle |
| `scripts/release-version.sh` | Etiqueta → `name`, `code` y `prerelease` |
| `scripts/install.sh` | Instalador `curl … \| sh` |
| `scripts/release-cert.sh` | Compara `apksigner` con `apkcert` y, opcionalmente, con el certificado fijado |
| `scripts/release-key.sh` | Genera la clave de release (lo ejecuta el usuario) |
| `.goreleaser.yaml`, `.github/workflows/release.yml`, `.github/workflows/release-check.yml` | Pipeline |
| `docs/releasing.md`, `LICENSE`, `host/README.md` | Documentación |

---

### Task 1: Parser del APK Signing Block

**Files:**
- Create: `host/internal/phoneapp/apksig.go`
- Create: `host/internal/phoneapp/apktest/apktest.go`
- Test: `host/internal/phoneapp/apksig_test.go`

**Interfaces:**
- Consumes: nada.
- Produces:
  - `phoneapp.Signers(apk []byte) ([]string, error)`: SHA-256 en hexadecimal minúsculo del primer certificado de cada firmante, en el orden de los bloques y de los firmantes;
  - `phoneapp.CheckSigner(apk []byte, cert string) error`: nil, o un error que envuelve `phoneapp.ErrSigner`;
  - `phoneapp.ErrSigner`;
  - paquete `apktest`, con las constantes `V2`, `V3`, `V31`, `Padding`, `StampV1`, `StampV2`, `DependencyInfo`, `Frosting` y `Rotation`, los tipos `Signer{Cert []byte; Rotation bool}` y `Block{ID uint32; Signers []Signer; Raw []byte}`, y las funciones `Cert(cn string) []byte`, `SHA256(der []byte) string`, `APK(blocks ...Block) []byte`, `SigningBlock(blocks ...Block) []byte` y `LP(parts ...[]byte) []byte`.

- [ ] **Step 1: Escribir el constructor de APK de prueba**

`host/internal/phoneapp/apktest/apktest.go`:

```go
// Package apktest builds fake APKs for tests: a small zip with an APK
// Signing Block whose signers carry the certificates the test picks.
// Nothing is really signed (no digests, signatures or public keys): the
// host only reads who signed (phoneapp.Signers); Android verifies the rest.
package apktest

import (
	"archive/zip"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"time"
)

// IDs of the APK Signing Block (source.android.com/docs/security/features/apksigning).
const (
	V2             uint32 = 0x7109871a
	V3             uint32 = 0xf05368c0
	V31            uint32 = 0x1b93ad61
	Padding        uint32 = 0x42726577
	StampV1        uint32 = 0x2b09189e
	StampV2        uint32 = 0x6dff800d
	DependencyInfo uint32 = 0x504b4453
	Frosting       uint32 = 0x2146444e
	// Rotation is the v3 signed attribute with the proof of key rotation.
	Rotation uint32 = 0x3ba06f8c
)

// Signer is one signer of a V2, V3 or V31 block.
type Signer struct {
	Cert     []byte // DER of its certificate, the first of its list
	Rotation bool   // V3 and V31 only: add a proof-of-rotation attribute
}

// Block is one ID-value pair of the Signing Block.
type Block struct {
	ID      uint32
	Signers []Signer // encoded as a V2 block when ID is V2, as V3 otherwise
	Raw     []byte   // the value as is, instead of Signers
}

const comment = "apktest"

// Cert returns the DER of a new self-signed certificate for cn.
func Cert(cn string) []byte {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		panic(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Unix(0, 0),
		NotAfter:     time.Unix(1<<32, 0),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		panic(err)
	}
	return der
}

// SHA256 is the hex SHA-256 of der, as phoneapp.Signers reports it.
func SHA256(der []byte) string {
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:])
}

// APK returns a zip with one entry and a comment and, when there are
// blocks, an APK Signing Block with them right before the central directory.
func APK(blocks ...Block) []byte {
	var z bytes.Buffer
	w := zip.NewWriter(&z)
	f, err := w.Create("AndroidManifest.xml")
	if err != nil {
		panic(err)
	}
	f.Write([]byte("<manifest/>"))
	w.SetComment(comment)
	if err := w.Close(); err != nil {
		panic(err)
	}
	b := z.Bytes()
	if len(blocks) == 0 {
		return b
	}
	eocd := len(b) - 22 - len(comment)
	cd := int(binary.LittleEndian.Uint32(b[eocd+16:]))
	block := SigningBlock(blocks...)
	out := make([]byte, 0, len(b)+len(block))
	out = append(out, b[:cd]...)
	out = append(out, block...)
	out = append(out, b[cd:]...)
	binary.LittleEndian.PutUint32(out[eocd+len(block)+16:], uint32(cd+len(block)))
	return out
}

// SigningBlock encodes an APK Signing Block holding blocks.
func SigningBlock(blocks ...Block) []byte {
	var pairs []byte
	for _, bl := range blocks {
		v := bl.Raw
		if v == nil {
			v = scheme(bl.ID != V2, bl.Signers)
		}
		pairs = binary.LittleEndian.AppendUint64(pairs, uint64(4+len(v)))
		pairs = binary.LittleEndian.AppendUint32(pairs, bl.ID)
		pairs = append(pairs, v...)
	}
	size := uint64(len(pairs) + 8 + 16)
	out := binary.LittleEndian.AppendUint64(nil, size)
	out = append(out, pairs...)
	out = binary.LittleEndian.AppendUint64(out, size)
	return append(out, "APK Sig Block 42"...)
}

// scheme encodes the value of a v2 block, or of a v3 one when v3.
func scheme(v3 bool, signers []Signer) []byte {
	sdks := func(b []byte) []byte {
		b = binary.LittleEndian.AppendUint32(b, 30)
		return binary.LittleEndian.AppendUint32(b, 0x7fffffff)
	}
	var seq []byte
	for _, s := range signers {
		var attrs []byte
		if s.Rotation {
			attrs = LP(binary.LittleEndian.AppendUint32(nil, Rotation), []byte("lineage"))
		}
		signed := LP() // digests: none
		signed = append(signed, LP(LP(s.Cert))...)
		if v3 {
			signed = sdks(signed)
		}
		signed = append(signed, LP(attrs)...)
		signer := LP(signed)
		if v3 {
			signer = sdks(signer)
		}
		signer = append(signer, LP()...) // signatures: none
		signer = append(signer, LP()...) // public key: none
		seq = append(seq, LP(signer)...)
	}
	return LP(seq)
}

// LP returns parts joined, after their total length as a little-endian uint32.
func LP(parts ...[]byte) []byte {
	var body []byte
	for _, p := range parts {
		body = append(body, p...)
	}
	return append(binary.LittleEndian.AppendUint32(nil, uint32(len(body))), body...)
}
```

- [ ] **Step 2: Escribir los tests que fallan**

`host/internal/phoneapp/apksig_test.go`:

```go
package phoneapp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
)

var (
	ours  = apktest.Cert("hybriqtech")
	other = apktest.Cert("someone else")
)

func by(certs ...[]byte) []apktest.Signer {
	var s []apktest.Signer
	for _, c := range certs {
		s = append(s, apktest.Signer{Cert: c})
	}
	return s
}

// mangle returns a copy of apk changed by fn; at is where the Signing Block
// starts (its first size field).
func mangle(apk []byte, fn func(b []byte, at int)) []byte {
	b := slices.Clone(apk)
	magic := bytes.Index(b, []byte("APK Sig Block 42"))
	size := int(binary.LittleEndian.Uint64(b[magic-8:]))
	fn(b, magic+16-size-8)
	return b
}

func TestSigners(t *testing.T) {
	me, them := apktest.SHA256(ours), apktest.SHA256(other)
	good := apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(ours)})
	cases := []struct {
		name string
		apk  []byte
		want []string
		err  string // part of the error; "" means no error
	}{
		{name: "v2", apk: good, want: []string{me}},
		{name: "v3", apk: apktest.APK(apktest.Block{ID: apktest.V3, Signers: by(ours)}), want: []string{me}},
		{name: "v3.1", apk: apktest.APK(apktest.Block{ID: apktest.V31, Signers: by(ours)}), want: []string{me}},
		{name: "v2 and v3", apk: apktest.APK(
			apktest.Block{ID: apktest.V2, Signers: by(ours)},
			apktest.Block{ID: apktest.V3, Signers: by(ours)}), want: []string{me, me}},
		{name: "v2 ours, v3 theirs", apk: apktest.APK(
			apktest.Block{ID: apktest.V2, Signers: by(ours)},
			apktest.Block{ID: apktest.V3, Signers: by(other)}), want: []string{me, them}},
		{name: "two signers", apk: apktest.APK(apktest.Block{ID: apktest.V3, Signers: by(ours, other)}), want: []string{me, them}},
		{name: "blocks that are not signatures", apk: apktest.APK(
			apktest.Block{ID: apktest.Padding, Raw: make([]byte, 16)},
			apktest.Block{ID: apktest.V2, Signers: by(ours)},
			apktest.Block{ID: apktest.StampV1, Raw: []byte("stamp")},
			apktest.Block{ID: apktest.StampV2, Raw: []byte("stamp")},
			apktest.Block{ID: apktest.DependencyInfo, Raw: []byte("deps")},
			apktest.Block{ID: apktest.Frosting, Raw: []byte("play")}), want: []string{me}},

		{name: "not a zip", apk: []byte("hello, world: this is not a zip file"), err: "end of central directory"},
		{name: "plain zip", apk: apktest.APK(), err: "no APK Signing Block"},
		{name: "only padding", apk: apktest.APK(apktest.Block{ID: apktest.Padding, Raw: make([]byte, 4)}), err: "no APK Signature Scheme v2 or v3 block"},
		{name: "unknown block", apk: apktest.APK(
			apktest.Block{ID: apktest.V2, Signers: by(ours)},
			apktest.Block{ID: 0x12345678, Raw: []byte{1}}), err: "unknown block 0x12345678"},
		{name: "key rotation", apk: apktest.APK(apktest.Block{ID: apktest.V3, Signers: []apktest.Signer{{Cert: ours, Rotation: true}}}), err: "key rotation"},
		{name: "no signers", apk: apktest.APK(apktest.Block{ID: apktest.V2, Raw: apktest.LP()}), err: "no signers"},
		{name: "empty certificate", apk: apktest.APK(apktest.Block{ID: apktest.V2, Signers: by([]byte{})}), err: "empty certificate"},
		{name: "trailing data", apk: apktest.APK(apktest.Block{ID: apktest.V2, Raw: []byte{0, 0, 0, 0, 0}}), err: "trailing data"},
		{name: "truncated", apk: apktest.APK(apktest.Block{ID: apktest.V2, Raw: []byte{9, 0, 0, 0}}), err: "truncated"},
		{name: "central directory offset past the end", apk: func() []byte {
			b := slices.Clone(good)
			binary.LittleEndian.PutUint32(b[len(b)-22-len("apktest")+16:], 0x7ffffff0)
			return b
		}(), err: "central directory"},
		{name: "zip64", apk: func() []byte {
			b := slices.Clone(good)
			binary.LittleEndian.PutUint32(b[len(b)-22-len("apktest")+16:], 0xffffffff)
			return b
		}(), err: "ZIP64"},
		{name: "block size past the start", apk: mangle(good, func(b []byte, at int) {
			magic := bytes.Index(b, []byte("APK Sig Block 42"))
			binary.LittleEndian.PutUint64(b[magic-8:], 1<<40)
		}), err: "bad APK Signing Block size"},
		{name: "sizes that differ", apk: mangle(good, func(b []byte, at int) {
			binary.LittleEndian.PutUint64(b[at:], 7)
		}), err: "sizes do not match"},
		{name: "pair past the block", apk: mangle(good, func(b []byte, at int) {
			binary.LittleEndian.PutUint64(b[at+8:], 1<<32)
		}), err: "bad ID-value pair"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := Signers(c.apk)
			if c.err != "" {
				if err == nil || !strings.Contains(err.Error(), c.err) {
					t.Fatalf("got %v, %v; want an error with %q", got, err, c.err)
				}
				return
			}
			if err != nil || !slices.Equal(got, c.want) {
				t.Fatalf("got %v, %v; want %v", got, err, c.want)
			}
		})
	}
}

func TestCheckSigner(t *testing.T) {
	me := apktest.SHA256(ours)
	if err := CheckSigner(apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(ours)}, apktest.Block{ID: apktest.V3, Signers: by(ours)}), me); err != nil {
		t.Fatal(err)
	}
	if err := CheckSigner(apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(ours)}), strings.ToUpper(me)); err != nil {
		t.Fatalf("upper-case pin: %v", err)
	}
	for name, apk := range map[string][]byte{
		"theirs":             apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(other)}),
		"v2 ours, v3 theirs": apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(ours)}, apktest.Block{ID: apktest.V3, Signers: by(other)}),
		"ours and theirs":    apktest.APK(apktest.Block{ID: apktest.V3, Signers: by(ours, other)}),
		"unsigned":           apktest.APK(),
	} {
		if err := CheckSigner(apk, me); !errors.Is(err, ErrSigner) {
			t.Errorf("%s: %v, want ErrSigner", name, err)
		}
	}
}

// Signers never panics nor reads out of bounds, whatever the bytes.
func FuzzSigners(f *testing.F) {
	f.Add(apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(ours)}))
	f.Add(apktest.APK(apktest.Block{ID: apktest.V3, Signers: by(ours, other)}, apktest.Block{ID: apktest.Padding, Raw: make([]byte, 8)}))
	f.Add(apktest.APK(apktest.Block{ID: apktest.V31, Signers: []apktest.Signer{{Cert: ours, Rotation: true}}}))
	f.Add(apktest.APK())
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		got, err := Signers(b)
		if err == nil && len(got) == 0 {
			t.Fatal("no error and no signers")
		}
	})
}
```

- [ ] **Step 3: Ejecutar los tests y ver que fallan**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/ -run 'TestSigners|TestCheckSigner|FuzzSigners'`
Expected: FAIL de compilación con `undefined: Signers`, `undefined: CheckSigner` y `undefined: ErrSigner`.

- [ ] **Step 4: Escribir el parser**

`host/internal/phoneapp/apksig.go`:

```go
package phoneapp

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

// IDs of the APK Signing Block (source.android.com/docs/security/features/apksigning).
const (
	blockV2      = 0x7109871a
	blockV3      = 0xf05368c0
	blockV31     = 0x1b93ad61
	attrRotation = 0x3ba06f8c // v3 signed attribute: proof of key rotation
)

// notSignatures are the Signing Block IDs that hold no signature. Any other
// ID that is not a scheme block above rejects the APK: Android picks the
// highest scheme it knows, so a scheme the host cannot read could hide
// another signer (spec M6a §5).
var notSignatures = map[uint32]bool{
	0x42726577: true, // verity padding
	0x2b09189e: true, // source stamp v1
	0x6dff800d: true, // source stamp v2
	0x504b4453: true, // AGP dependency info
	0x2146444e: true, // Google Play frosting
}

// ErrSigner means the APK is not signed (only) with the pinned certificate.
var ErrSigner = errors.New("the APK is not signed by hybriqtech")

var errTruncated = errors.New("truncated")

const (
	eocdSize = 22
	maxEOCD  = eocdSize + 0xffff // with the longest comment
)

var sigMagic = []byte("APK Sig Block 42")

// Signers returns the lowercase hex SHA-256 of the certificate of every
// signer in every v2, v3 and v3.1 block of apk. It does not verify any
// signature: Android does when it installs, and refuses an APK whose
// signatures do not match its content. What Android does not check is who
// signed, and that is what this tells.
func Signers(apk []byte) ([]string, error) {
	pairs, err := signingBlock(apk)
	if err != nil {
		return nil, err
	}
	var out []string
	found := false
	for _, p := range pairs {
		switch p.id {
		case blockV2, blockV3, blockV31:
			found = true
			certs, err := schemeSigners(p.value, p.id != blockV2)
			if err != nil {
				return nil, fmt.Errorf("block %#x: %w", p.id, err)
			}
			out = append(out, certs...)
		default:
			if !notSignatures[p.id] {
				return nil, fmt.Errorf("unknown block %#x in the APK Signing Block", p.id)
			}
		}
	}
	if !found {
		return nil, errors.New("no APK Signature Scheme v2 or v3 block")
	}
	return out, nil
}

// CheckSigner returns nil when every signer that Signers finds in apk has
// the certificate whose SHA-256 is cert (hex), and an ErrSigner otherwise.
func CheckSigner(apk []byte, cert string) error {
	got, err := Signers(apk)
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrSigner, err)
	}
	cert = strings.ToLower(cert)
	for _, g := range got {
		if g != cert {
			return fmt.Errorf("%w (signed by the certificate %s)", ErrSigner, g)
		}
	}
	return nil
}

type pair struct {
	id    uint32
	value []byte
}

// signingBlock returns the ID-value pairs of the APK Signing Block, which
// sits right before the zip's central directory.
func signingBlock(apk []byte) ([]pair, error) {
	eocd := -1
	for i := len(apk) - eocdSize; i >= 0 && i >= len(apk)-maxEOCD; i-- {
		if binary.LittleEndian.Uint32(apk[i:]) == 0x06054b50 &&
			int(binary.LittleEndian.Uint16(apk[i+20:])) == len(apk)-i-eocdSize {
			eocd = i
			break
		}
	}
	if eocd < 0 {
		return nil, errors.New("not a zip file (no end of central directory)")
	}
	cdSize := int64(binary.LittleEndian.Uint32(apk[eocd+12:]))
	cdOffset := int64(binary.LittleEndian.Uint32(apk[eocd+16:]))
	if cdOffset == 0xffffffff || cdSize == 0xffffffff {
		return nil, errors.New("ZIP64 APKs are not supported")
	}
	if cdOffset+cdSize != int64(eocd) {
		return nil, errors.New("the central directory is not right before its end record")
	}
	if cdOffset < 32 || !bytes.Equal(apk[cdOffset-16:cdOffset], sigMagic) {
		return nil, errors.New("no APK Signing Block")
	}
	size := binary.LittleEndian.Uint64(apk[cdOffset-24:])
	if size < 24 || size > uint64(cdOffset-8) {
		return nil, errors.New("bad APK Signing Block size")
	}
	start := cdOffset - 8 - int64(size)
	if binary.LittleEndian.Uint64(apk[start:]) != size {
		return nil, errors.New("the APK Signing Block sizes do not match")
	}
	return pairsOf(apk[start+8 : cdOffset-24])
}

func pairsOf(b []byte) ([]pair, error) {
	var out []pair
	for len(b) > 0 {
		if len(b) < 8 {
			return nil, errors.New("bad ID-value pair in the APK Signing Block")
		}
		n := binary.LittleEndian.Uint64(b)
		b = b[8:]
		if n < 4 || n > uint64(len(b)) {
			return nil, errors.New("bad ID-value pair in the APK Signing Block")
		}
		out = append(out, pair{binary.LittleEndian.Uint32(b), b[4:n]})
		b = b[n:]
	}
	return out, nil
}

// lp splits a value with a little-endian uint32 length prefix off b.
func lp(b []byte) (val, rest []byte, err error) {
	if len(b) < 4 {
		return nil, nil, errTruncated
	}
	n := uint64(binary.LittleEndian.Uint32(b))
	if n > uint64(len(b)-4) {
		return nil, nil, errTruncated
	}
	return b[4 : 4+n], b[4+n:], nil
}

// schemeSigners returns the SHA-256 of the first certificate of each signer
// of a v2 block, or of a v3 or v3.1 one when v3 (those also carry SDK
// bounds and must not rotate the key).
func schemeSigners(v []byte, v3 bool) ([]string, error) {
	signers, rest, err := lp(v)
	if err != nil {
		return nil, err
	}
	if len(rest) != 0 {
		return nil, errors.New("trailing data")
	}
	var out []string
	for len(signers) > 0 {
		var signer, signed, certs, first []byte
		if signer, signers, err = lp(signers); err != nil {
			return nil, err
		}
		if signed, _, err = lp(signer); err != nil {
			return nil, err
		}
		if _, signed, err = lp(signed); err != nil { // digests
			return nil, err
		}
		if certs, signed, err = lp(signed); err != nil {
			return nil, err
		}
		if first, _, err = lp(certs); err != nil {
			return nil, err
		}
		if len(first) == 0 {
			return nil, errors.New("empty certificate")
		}
		if v3 {
			if len(signed) < 8 { // min and max SDK
				return nil, errTruncated
			}
			attrs, _, err := lp(signed[8:])
			if err != nil {
				return nil, err
			}
			for len(attrs) > 0 {
				var a []byte
				if a, attrs, err = lp(attrs); err != nil {
					return nil, err
				}
				if len(a) < 4 {
					return nil, errTruncated
				}
				if binary.LittleEndian.Uint32(a) == attrRotation {
					return nil, errors.New("key rotation is not supported")
				}
			}
		}
		sum := sha256.Sum256(first)
		out = append(out, hex.EncodeToString(sum[:]))
	}
	if len(out) == 0 {
		return nil, errors.New("no signers")
	}
	return out, nil
}
```

- [ ] **Step 5: Ejecutar los tests y ver que pasan**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/ -run 'TestSigners|TestCheckSigner|FuzzSigners' -v`
Expected: PASS en todos los subtests.

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/ -run '^$' -fuzz FuzzSigners -fuzztime 60s`
Expected: `PASS` sin `failing input`. Si el fuzzer encuentra una entrada que falla, arregla el parser y añade esa entrada como caso de `TestSigners`. No se versiona `testdata/fuzz`.

- [ ] **Step 6: gofmt, vet y commit**

Run (desde `host/`): `gofmt -l ./internal/phoneapp && go vet ./internal/phoneapp/...`
Expected: sin salida.

```bash
git add host/internal/phoneapp/apksig.go host/internal/phoneapp/apksig_test.go host/internal/phoneapp/apktest/apktest.go
git commit -m "feat(host): leer quién firma un APK en su Signing Block (v2, v3 y v3.1)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: El host solo instala el APK descargado si lo firma hybriqtech

**Files:**
- Modify: `host/internal/phoneapp/release.go` (`SigningCertSHA256`, `ErrUnpinned`, `Download`)
- Modify: `host/internal/phoneapp/phoneapp.go:104-114` (`ErrOtherSigner` en `Install`)
- Modify: `host/internal/phoneapp/phoneapp_test.go` (`TestInstall`, `TestDownloadChecksTheSHA256`, `TestDownloadHasATimeLimit` y tests nuevos)
- Modify: `host/internal/cli/appsetup.go:39-49,72-96` (`releaseCert`, comentario de `releaseURL` y mensajes)
- Modify: `host/internal/cli/setup.go:357` (ayuda de `--apk`)
- Modify: `host/internal/cli/appsetup_test.go:57-77,160-190` (`release` y casos nuevos)

**Interfaces:**
- Consumes: `phoneapp.CheckSigner` y `phoneapp.ErrSigner` (Task 1); `apktest.APK`, `apktest.Cert`, `apktest.SHA256`, `apktest.Block` y `apktest.Signer` (Task 1).
- Produces:
  - `const phoneapp.SigningCertSHA256 string` (vale `""` hasta que exista la clave);
  - `phoneapp.ErrUnpinned`;
  - `phoneapp.ErrOtherSigner`;
  - `func phoneapp.Download(ctx context.Context, client *http.Client, base, dir, cert string) (string, error)`.

- [ ] **Step 1: Escribir los tests de `phoneapp` que fallan**

En `host/internal/phoneapp/phoneapp_test.go`, añade `"sync/atomic"` y `"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"` a los imports. Sustituye `TestInstall` y `TestDownloadChecksTheSHA256` por estas versiones, y añade los tests nuevos:

```go
func TestInstall(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"install -r /tmp/a.apk": {"Performing Streamed Install\nSuccess\n"}}}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err != nil {
		t.Fatal(err)
	}
	f.answers["install -r /tmp/a.apk"] = []string{"Failure [INSTALL_FAILED_OLDER_SDK]\n"}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err == nil || errors.Is(err, ErrOtherSigner) {
		t.Fatalf("older sdk: %v", err)
	}
	f.answers["install -r /tmp/a.apk"] = []string{"Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package tech.hybriq.adbuddy signatures do not match newer version; ignoring!]\n"}
	if err := Install(context.Background(), f, "/tmp/a.apk"); !errors.Is(err, ErrOtherSigner) || !strings.Contains(err.Error(), "signatures do not match") {
		t.Fatalf("other signer: %v", err)
	}
}

// releaseServer serves apk and a .sha256 sidecar (sidecar() decides its
// text), and counts the requests.
func releaseServer(t *testing.T, apk []byte, sidecar func() string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/adbuddy.apk":
			w.Write(apk)
		case "/adbuddy.apk.sha256":
			io.WriteString(w, sidecar())
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func sumOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestDownloadChecksTheSHA256(t *testing.T) {
	cert := apktest.Cert("hybriqtech")
	apk := apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: cert}}})
	sidecar := sumOf(apk) + "  adbuddy.apk\n"
	srv, _ := releaseServer(t, apk, func() string { return sidecar })
	dir := t.TempDir()
	p, err := Download(context.Background(), srv.Client(), srv.URL+"/", dir, apktest.SHA256(cert))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, apk) || p != filepath.Join(dir, "adbuddy.apk") {
		t.Fatalf("%s %d bytes", p, len(b))
	}
	sidecar = strings.Repeat("0", 64)
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", t.TempDir(), apktest.SHA256(cert)); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad sum: %v", err)
	}
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/none/", t.TempDir(), apktest.SHA256(cert)); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("no release: %v", err)
	}
}

// An APK whose checksum matches but that someone else signed is not saved.
func TestDownloadChecksTheSigner(t *testing.T) {
	ours, theirs := apktest.Cert("hybriqtech"), apktest.Cert("someone else")
	apk := apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: theirs}}})
	srv, _ := releaseServer(t, apk, func() string { return sumOf(apk) + "  adbuddy.apk\n" })
	dir := t.TempDir()
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", dir, apktest.SHA256(ours)); !errors.Is(err, ErrSigner) {
		t.Fatalf("got %v, want ErrSigner", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "adbuddy.apk")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the APK was saved: %v", err)
	}
}

// Without a pinned certificate nothing is downloaded at all.
func TestDownloadNeedsAPinnedCertificate(t *testing.T) {
	srv, hits := releaseServer(t, []byte("x"), func() string { return "" })
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", t.TempDir(), ""); !errors.Is(err, ErrUnpinned) {
		t.Fatalf("got %v, want ErrUnpinned", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("%d requests without a pin", n)
	}
}
```

Añade `"bytes"` a los imports si no está. En `TestDownloadHasATimeLimit`, cambia la llamada a `Download(context.Background(), client, srv.URL+"/", t.TempDir(), strings.Repeat("a", 64))`.

- [ ] **Step 2: Ejecutar y ver que fallan**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/`
Expected: FAIL de compilación (`too many arguments in call to Download`, `undefined: ErrOtherSigner`, `undefined: ErrUnpinned`).

- [ ] **Step 3: Implementar en `phoneapp`**

En `host/internal/phoneapp/release.go`, después de `ErrChecksum`:

```go
// SigningCertSHA256 is the SHA-256 (lowercase hex) of the certificate that
// signs the app's releases (spec M6a §5): Download refuses any APK signed
// with another one, wherever it comes from. It is empty until the release
// key exists (docs/releasing.md), and then Download refuses every APK.
const SigningCertSHA256 = ""

// ErrUnpinned means this build pins no signing certificate, so it downloads
// no APK.
var ErrUnpinned = errors.New("this build of adbuddy has no pinned signing certificate for the app")
```

Sustituye el comentario y la cabecera de `Download`, y añade la comprobación de la firma tras la del checksum:

```go
// Download fetches the latest APK into dir, checks it against the published
// SHA-256 and checks that every signer has the certificate whose SHA-256 is
// cert (SigningCertSHA256, or a test's), then returns its path. base is
// ReleaseURL (or a test server). Without cert it downloads nothing
// (ErrUnpinned). Without a client or a deadline in ctx it takes at most
// downloadTimeout.
func Download(ctx context.Context, client *http.Client, base, dir, cert string) (string, error) {
	if cert == "" {
		return "", ErrUnpinned
	}
```

y, justo después de `if hex.EncodeToString(got[:]) != want { return "", ErrChecksum }`:

```go
	if err := CheckSigner(data, cert); err != nil {
		return "", err
	}
```

En `host/internal/phoneapp/phoneapp.go`, junto a las demás variables de error del fichero (o encima de `Install` si no hay ninguna):

```go
// ErrOtherSigner means the app on the phone was signed with another key (a
// development build, say), so Android refuses to update it.
var ErrOtherSigner = errors.New("the app on the phone was signed with another key")
```

y sustituye `Install`:

```go
// Install installs (or updates, keeping data) the APK at path.
func Install(ctx context.Context, a ADB, path string) error {
	out, err := runFor(ctx, a, installTimeout, nil, "install", "-r", path)
	if err == nil && strings.Contains(out, "Success") {
		return nil
	}
	msg := strings.TrimSpace(out)
	if err != nil {
		msg = err.Error()
	}
	if strings.Contains(msg, "INSTALL_FAILED_UPDATE_INCOMPATIBLE") {
		if err != nil {
			return fmt.Errorf("%w: %w", ErrOtherSigner, err)
		}
		return fmt.Errorf("%w: %s", ErrOtherSigner, msg)
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("adb install: %s", msg)
}
```

(`runFor` ya pone el stderr de adb en el error cuando sale con código distinto de 0. Así el caso real, `adb: failed to install …: Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE…]` con salida 1, también se reconoce.)

- [ ] **Step 4: Ejecutar los tests de `phoneapp`**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/...`
Expected: PASS.

- [ ] **Step 5: Escribir los tests de `cli` que fallan**

En `host/internal/cli/appsetup_test.go`, añade el import `"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"` y sustituye `release` por:

```go
// Test certificates: setup pins testCert (release pins it for the test).
var (
	testCert  = apktest.Cert("hybriqtech")
	otherCert = apktest.Cert("someone else")
)

// release serves an APK signed with testCert and pins testCert.
func release(t *testing.T, status int, badSum bool) string {
	t.Helper()
	return releaseSignedBy(t, status, badSum, testCert)
}

// releaseSignedBy serves an APK signed with cert (status != 200: an error;
// badSum: a .sha256 that does not match) and pins testCert.
func releaseSignedBy(t *testing.T, status int, badSum bool, cert []byte) string {
	t.Helper()
	setVar(t, &releaseCert, apktest.SHA256(testCert))
	apk := apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: cert}}})
	sum := sha256.Sum256(apk)
	if badSum {
		sum = sha256.Sum256([]byte("another file"))
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if status != http.StatusOK {
			http.Error(w, "nope", status)
			return
		}
		if strings.HasSuffix(r.URL.Path, ".sha256") {
			w.Write([]byte(hex.EncodeToString(sum[:]) + "  adbuddy.apk\n"))
			return
		}
		w.Write(apk)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/"
}
```

En la tabla de `TestSetupAppBranches`, justo después del caso `"bad checksum"`, añade:

```go
		{name: "other signer", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args: func(t *testing.T) []string {
				t.Setenv("ADBUDDY_RELEASE_URL", releaseSignedBy(t, 200, false, otherCert))
				return nil
			},
			want:   want{consent: "skipped"},
			stderr: []string{"not signed by hybriqtech", "nothing was installed", "--apk <file>"}, noChange: true},
		{name: "unpinned", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args: func(t *testing.T) []string {
				t.Setenv("ADBUDDY_RELEASE_URL", release(t, 200, false))
				setVar(t, &releaseCert, "")
				return nil
			},
			want:   want{consent: "skipped"},
			stderr: []string{"no pinned signing certificate", "--apk <file>"}, noStderr: []string{"⚠", "Getting the ADBuddy app"}, noChange: true},
		{name: "development build on the phone", consent: "granted",
			over: []testutil.FakeRule{notInstalled(0), {Match: `^install -r `, Exit: 1,
				Stderr: "adb: failed to install app.apk: Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package tech.hybriq.adbuddy signatures do not match newer version; ignoring!]\n"}},
			args:   func(t *testing.T) []string { return []string{"--apk", writeAPK(t)} },
			want:   want{consent: "skipped"},
			stderr: []string{"signed with another key", "Uninstall it on the phone", "adbuddy setup"}, noStderr: []string{"Could not install the app"}, noChange: true},
```

- [ ] **Step 6: Ejecutar y ver que fallan**

Run (desde `host/`): `go test -timeout 10m ./internal/cli/ -run TestSetupAppBranches`
Expected: FAIL de compilación con `undefined: releaseCert`.

- [ ] **Step 7: Implementar en `cli`**

En `host/internal/cli/appsetup.go`, sustituye `releaseURL` por:

```go
// releaseCert is the signing certificate a downloaded APK must have (a
// variable so that tests can pin theirs).
var releaseCert = phoneapp.SigningCertSHA256

// releaseURL is where setup downloads the app: ReleaseURL, or
// ADBUDDY_RELEASE_URL (the e2e tests). Changing it is harmless, because
// Download only accepts an APK signed with releaseCert (spec M6a §5).
func releaseURL(app *App) string {
	u := app.Getenv("ADBUDDY_RELEASE_URL")
	if u == "" {
		return phoneapp.ReleaseURL
	}
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}
	return u
}
```

La llamada a `Download` y los casos de su `switch` quedan así:

```go
			say("Getting the ADBuddy app…\n")
			apk, err = phoneapp.Download(ctx, nil, releaseURL(app), filepath.Join(e.dirs.Data, "app"), releaseCert)
			if err != nil {
				res.note(err.Error())
				switch {
				case errors.Is(err, phoneapp.ErrUnpinned):
					warn("This build of adbuddy has no pinned signing certificate for the app, so it does not download it: skipping it (pass --apk <file> to install one). 🔴 confirmations will happen in the terminal.\n")
				case errors.Is(err, phoneapp.ErrNoRelease):
					warn("There is no published version of the ADBuddy app yet: skipping it (pass --apk <file> to install one). 🔴 confirmations will happen in the terminal.\n")
				case errors.Is(err, phoneapp.ErrChecksum):
					warn("⚠ The downloaded ADBuddy app does not match its checksum, so nothing was installed. Try again later, or pass --apk <file>. 🔴 confirmations will happen in the terminal.\n")
				case errors.Is(err, phoneapp.ErrSigner):
					warn("⚠ The downloaded ADBuddy app is not signed by hybriqtech, so nothing was installed. Please report it at https://github.com/hybriqtech/adbuddy/issues, or pass --apk <file>. 🔴 confirmations will happen in the terminal.\n")
				default:
					warn("⚠ Could not get the ADBuddy app (%v): setup goes on without it; 🔴 confirmations will happen in the terminal. Try again later, or pass --apk <file>.\n", err)
				}
				return res
			}
```

El caso `"unpinned"` pide que no salga "Getting the ADBuddy app". Mueve, por tanto, el `say("Getting the ADBuddy app…\n")` dentro de un `if releaseCert != "" { … }` justo antes de la llamada. `Download` sigue devolviendo `ErrUnpinned` sin tocar la red:

```go
			if releaseCert != "" {
				say("Getting the ADBuddy app…\n")
			}
			apk, err = phoneapp.Download(ctx, nil, releaseURL(app), filepath.Join(e.dirs.Data, "app"), releaseCert)
```

El error de `Install`:

```go
		if err := phoneapp.Install(ctx, a, apk); err != nil {
			res.note(err.Error())
			if errors.Is(err, phoneapp.ErrOtherSigner) {
				warn("⚠ The ADBuddy app on the phone was signed with another key (a development build?), so Android will not update it. Uninstall it on the phone (Settings › Apps › ADBuddy › Uninstall; it forgets its pairing with this PC) and run `adbuddy setup` again. Setup goes on without it.\n")
			} else {
				warn("⚠ Could not install the app: %v. Setup goes on without it; try again later or with --apk <file>.\n", err)
			}
			return res
		}
```

En `host/internal/cli/setup.go:357`:

```go
	cmd.Flags().StringVar(&apkFlag, "apk", "", "install this ADBuddy app APK instead of the latest release (a file you choose: its signature is not checked)")
```

- [ ] **Step 8: Ejecutar todos los tests del host**

Run (desde `host/`): `go test -timeout 10m ./...`
Expected: PASS. Si falla un test que compara la ayuda de `setup` (golden o `strings.Contains`), actualízalo al texto nuevo.

- [ ] **Step 9: gofmt, vet y commit**

Run (desde `host/`): `gofmt -l . && go vet ./...`
Expected: sin salida.

```bash
git add host/internal/phoneapp host/internal/cli/appsetup.go host/internal/cli/appsetup_test.go host/internal/cli/setup.go
git commit -m "feat(host): setup solo instala la app descargada si la firma hybriqtech

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Herramienta `apkcert`

**Files:**
- Create: `host/cmd/apkcert/main.go`
- Test: `host/cmd/apkcert/main_test.go`

**Interfaces:**
- Consumes: `phoneapp.Signers` y `phoneapp.SigningCertSHA256` (Tasks 1 y 2), y `apktest` (Task 1).
- Produces: `go run ./cmd/apkcert <file.apk>` imprime, desde `host/`, una línea por certificado distinto (ordenadas), y `go run ./cmd/apkcert -pinned` imprime el certificado fijado. Salidas: 0 bien, 1 error y 2 uso.

- [ ] **Step 1: Escribir el test que falla**

`host/cmd/apkcert/main_test.go`:

```go
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
)

func TestPrintsEachCertificateOnce(t *testing.T) {
	a, b := apktest.Cert("a"), apktest.Cert("b")
	p := filepath.Join(t.TempDir(), "x.apk")
	apk := apktest.APK(
		apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: a}}},
		apktest.Block{ID: apktest.V3, Signers: []apktest.Signer{{Cert: a}, {Cert: b}}})
	if err := os.WriteFile(p, apk, 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	if code := run([]string{p}, &out, &errb); code != 0 {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	want := []string{apktest.SHA256(a), apktest.SHA256(b)}
	if want[0] > want[1] {
		want[0], want[1] = want[1], want[0]
	}
	if got := strings.Fields(out.String()); strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFailsOnABadAPK(t *testing.T) {
	p := filepath.Join(t.TempDir(), "x.apk")
	os.WriteFile(p, apktest.APK(), 0o600)
	var out, errb bytes.Buffer
	if code := run([]string{p}, &out, &errb); code != 1 || out.Len() != 0 || !strings.Contains(errb.String(), "no APK Signing Block") {
		t.Fatalf("exit %d out %q err %q", code, out.String(), errb.String())
	}
	if code := run([]string{filepath.Join(t.TempDir(), "none.apk")}, &out, &errb); code != 1 {
		t.Fatalf("missing file: exit %d", code)
	}
}

func TestPinned(t *testing.T) {
	var out, errb bytes.Buffer
	code := run([]string{"-pinned"}, &out, &errb)
	if phoneapp.SigningCertSHA256 == "" {
		if code != 1 || !strings.Contains(errb.String(), "pins no signing certificate") {
			t.Fatalf("exit %d err %q", code, errb.String())
		}
		return
	}
	if code != 0 || strings.TrimSpace(out.String()) != phoneapp.SigningCertSHA256 {
		t.Fatalf("exit %d out %q", code, out.String())
	}
}

func TestUsage(t *testing.T) {
	for _, args := range [][]string{nil, {"a", "b"}} {
		var out, errb bytes.Buffer
		if code := run(args, &out, &errb); code != 2 || !strings.Contains(errb.String(), "usage") {
			t.Fatalf("%q: exit %d", args, code)
		}
	}
}
```

- [ ] **Step 2: Ejecutar y ver que falla**

Run (desde `host/`): `go test -timeout 5m ./cmd/apkcert/`
Expected: FAIL de compilación (`undefined: run`).

- [ ] **Step 3: Implementar**

`host/cmd/apkcert/main.go`:

```go
// Command apkcert prints the SHA-256 of the signing certificates of an APK,
// with the parser the host uses before it installs the app (spec M6a §5).
// The release workflows use it; it is not shipped.
//
//	apkcert <file.apk>   each distinct signer certificate, one per line
//	apkcert -pinned      the certificate this host pins (fails if none)
package main

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(stderr, "usage: apkcert <file.apk> | apkcert -pinned")
		return 2
	}
	if args[0] == "-pinned" {
		if phoneapp.SigningCertSHA256 == "" {
			fmt.Fprintln(stderr, "apkcert: this host pins no signing certificate (phoneapp.SigningCertSHA256 is empty; see docs/releasing.md)")
			return 1
		}
		fmt.Fprintln(stdout, phoneapp.SigningCertSHA256)
		return 0
	}
	data, err := os.ReadFile(args[0])
	if err != nil {
		fmt.Fprintln(stderr, "apkcert:", err)
		return 1
	}
	certs, err := phoneapp.Signers(data)
	if err != nil {
		fmt.Fprintf(stderr, "apkcert: %s: %v\n", args[0], err)
		return 1
	}
	slices.Sort(certs)
	for _, c := range slices.Compact(certs) {
		fmt.Fprintln(stdout, c)
	}
	return 0
}
```

- [ ] **Step 4: Ejecutar y ver que pasa**

Run (desde `host/`): `go test -timeout 5m ./cmd/apkcert/ -v && gofmt -l ./cmd && go vet ./cmd/...`
Expected: PASS y sin salida de gofmt ni de vet.

- [ ] **Step 5: Commit**

```bash
git add host/cmd/apkcert
git commit -m "feat(host): herramienta apkcert para comprobar el certificado del APK en la CI

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Versión desde la etiqueta

**Files:**
- Create: `scripts/release-version.sh`
- Test: `scripts/release-version_test.sh`
- Modify: `android/app/build.gradle.kts:16-17` (commit propio)

**Interfaces:**
- Consumes: nada.
- Produces: `sh scripts/release-version.sh <tag>` imprime exactamente tres líneas, `name=<X.Y.Z[-rc.N]>`, `code=<versionCode>` y `prerelease=<true|false>`, con salida 0. Con una etiqueta inválida sale con 1 y con un número de argumentos distinto de 1, con 2. Gradle lee `-Padbuddy.versionName=…` y `-Padbuddy.versionCode=…`.

- [ ] **Step 1: Escribir el test que falla**

`scripts/release-version_test.sh`:

```sh
#!/bin/sh
# Tests scripts/release-version.sh. Run: sh scripts/release-version_test.sh
set -eu

here=$(dirname "$0")
fail=0
nl='
'

ok() { # tag name code prerelease
	if ! got=$(sh "$here/release-version.sh" "$1" 2>&1); then
		echo "FAIL $1: refused: $got"
		fail=1
		return
	fi
	want="name=$2${nl}code=$3${nl}prerelease=$4"
	if [ "$got" != "$want" ]; then
		echo "FAIL $1: got"
		echo "$got"
		echo "want"
		echo "$want"
		fail=1
	fi
}

bad() { # tag
	if out=$(sh "$here/release-version.sh" "$1" 2>&1); then
		echo "FAIL $1: accepted: $out"
		fail=1
	fi
}

code() { sh "$here/release-version.sh" "$1" | sed -n 's/^code=//p'; }

ok v0.3.0 0.3.0 300099 false
ok v0.3.0-rc.2 0.3.0-rc.2 300002 true
ok v1.2.3 1.2.3 100200399 false
ok v0.0.0-rc.1 0.0.0-rc.1 1 true
ok v20.999.999 20.999.999 2099999999 false
ok v10.0.0-rc.98 10.0.0-rc.98 1000000098 true

for t in 0.3.0 v1.2 v1.2.3.4 v01.2.3 v1.02.3 v1.2.03 v1000.0.0 v21.0.0 \
	v1.2.3-rc.0 v1.2.3-rc.99 v1.2.3-rc.01 v1.2.3-beta.1 v1.2.3-rc.1x v1.2.3-rc \
	"v1.2.3 " "" "v1.2.3
v1.2.4"; do
	bad "$t"
done

if sh "$here/release-version.sh" >/dev/null 2>&1; then
	echo "FAIL no argument accepted"
	fail=1
fi
set +e
sh "$here/release-version.sh" a b >/dev/null 2>&1
[ $? -eq 2 ] || { echo "FAIL two arguments: exit is not 2"; fail=1; }
set -e

# An rc comes before its release, which comes before the next rc.
a=$(code v0.3.0-rc.1) b=$(code v0.3.0-rc.98) c=$(code v0.3.0) d=$(code v0.3.1-rc.1) e=$(code v0.4.0)
if ! { [ "$a" -lt "$b" ] && [ "$b" -lt "$c" ] && [ "$c" -lt "$d" ] && [ "$d" -lt "$e" ]; }; then
	echo "FAIL order: $a $b $c $d $e"
	fail=1
fi

[ "$fail" -eq 0 ] && echo "release-version: all tests pass"
exit "$fail"
```

- [ ] **Step 2: Ejecutar y ver que falla**

Run (desde la raíz del worktree, en Git Bash): `sh scripts/release-version_test.sh`
Expected: varias líneas `FAIL` (el script no existe) y salida 1.

- [ ] **Step 3: Implementar**

`scripts/release-version.sh`:

```sh
#!/bin/sh
# Turns a release tag into the app's versionName and versionCode
# (spec M6a §3):
#
#   sh scripts/release-version.sh v0.3.0
#   name=0.3.0
#   code=300099
#   prerelease=false
#
# Tags: vX.Y.Z or vX.Y.Z-rc.N, with X <= 20, Y and Z <= 999 and 1 <= N <= 98.
# versionCode = ((X*1000 + Y)*1000 + Z)*100 + S, where S is N for an rc and 99
# for the release, so an rc always comes before its release.
set -eu

main() {
	if [ $# -ne 1 ]; then
		echo "usage: release-version.sh vX.Y.Z[-rc.N]" >&2
		exit 2
	fi
	tag=$1
	re='^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})(-rc\.[1-9][0-9]?)?$'
	# grep matches line by line: refuse a tag with a newline first.
	case $tag in
	*"
"*) bad "$tag" ;;
	esac
	printf '%s\n' "$tag" | grep -Eq "$re" || bad "$tag"
	v=${tag#v}
	core=${v%%-*}
	x=${core%%.*}
	rest=${core#*.}
	y=${rest%%.*}
	z=${rest#*.}
	case $v in
	*-rc.*)
		s=${v##*-rc.}
		pre=true
		;;
	*)
		s=99
		pre=false
		;;
	esac
	[ "$x" -le 20 ] || bad "$tag" "the major version is over 20, so the versionCode would not fit"
	[ "$s" -le 98 ] || bad "$tag" "rc numbers go from 1 to 98"
	code=$((((x * 1000 + y) * 1000 + z) * 100 + s))
	printf 'name=%s\ncode=%s\nprerelease=%s\n' "$v" "$code" "$pre"
}

bad() {
	echo "release-version: \"$1\" is not a release tag (vX.Y.Z or vX.Y.Z-rc.N)${2:+: $2}" >&2
	exit 1
}

main "$@"
```

- [ ] **Step 4: Ejecutar y ver que pasa**

Run: `sh scripts/release-version_test.sh`
Expected: `release-version: all tests pass` y salida 0.

- [ ] **Step 5: Commit de los scripts**

```bash
git add scripts/release-version.sh scripts/release-version_test.sh
git commit -m "feat(release): versionName y versionCode desde la etiqueta

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 6: Leer la versión de la app desde Gradle**

En `android/app/build.gradle.kts`, sustituye **solo** estas dos líneas del bloque `defaultConfig`:

```kotlin
        versionCode = 1
        versionName = "0.2.0"
```

por:

```kotlin
        // Releases pass both from the tag (scripts/release-version.sh, spec M6a §3).
        versionCode = providers.gradleProperty("adbuddy.versionCode").map(String::toInt).getOrElse(1)
        versionName = providers.gradleProperty("adbuddy.versionName").getOrElse("0.2.0")
```

En este PC no hay JDK: no compiles. La comprobación la hacen `android.yml` (build de debug, con los valores por defecto) y `release-check.yml` (Task 6, con `-P…`). Comprueba a mano que `git diff android/` solo muestra esas líneas.

- [ ] **Step 7: Commit propio de Gradle**

```bash
git add android/app/build.gradle.kts
git commit -m "build(app): versionName y versionCode desde propiedades de Gradle

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 5: Script de instalación

**Files:**
- Create: `scripts/install.sh`
- Test: `scripts/install_test.sh`

**Interfaces:**
- Consumes: el nombre de los archivos que da goreleaser (Task 6): `adbuddy_<versión>_<os>_<arch>.tar.gz`, donde `<os>_<arch>` es `linux_amd64`, `linux_arm64` o `darwin_all`; y `checksums.txt` con líneas `<hex>  <fichero>`. Cada `.tar.gz` lleva `adbuddy` en su raíz.
- Produces: `scripts/install.sh`, que usa las variables `ADBUDDY_VERSION`, `ADBUDDY_INSTALL_DIR` y `ADBUDDY_DOWNLOAD_URL`, como en la spec §6.

- [ ] **Step 1: Escribir el test que falla**

`scripts/install_test.sh`:

```sh
#!/bin/sh
# Tests scripts/install.sh against a fake release in a temporary folder
# (file:// URLs) and a fake uname. Run: sh scripts/install_test.sh
set -eu

here=$(cd "$(dirname "$0")" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM
fail=0

sha() { if command -v sha256sum >/dev/null 2>&1; then sha256sum "$@"; else shasum -a 256 "$@"; fi; }

# url turns a folder into a file:// URL that this system's curl understands.
url() {
	if command -v cygpath >/dev/null 2>&1; then
		printf 'file:///%s' "$(cygpath -m "$1")"
	else
		printf 'file://%s' "$1"
	fi
}

# A release with an archive for each target; each fake adbuddy says its target.
rel=$work/release
mkdir -p "$rel"
for target in linux_amd64 linux_arm64 darwin_all; do
	mkdir -p "$work/pkg/$target"
	printf '#!/bin/sh\necho "adbuddy 9.9.9 %s"\n' "$target" >"$work/pkg/$target/adbuddy"
	chmod 755 "$work/pkg/$target/adbuddy"
	tar -czf "$rel/adbuddy_9.9.9_${target}.tar.gz" -C "$work/pkg/$target" adbuddy
done
printf 'zip\n' >"$rel/adbuddy_9.9.9_windows_amd64.zip"
(cd "$rel" && sha adbuddy_9.9.9_*) >"$rel/checksums.txt"

# fakeuname makes uname answer $1 to -s and $2 to -m.
fakeuname() {
	mkdir -p "$work/uname-$1-$2"
	printf '#!/bin/sh\ncase "$1" in -s) echo %s ;; -m) echo %s ;; esac\n' "$1" "$2" >"$work/uname-$1-$2/uname"
	chmod 755 "$work/uname-$1-$2/uname"
	printf '%s' "$work/uname-$1-$2"
}

# try os arch base dir: runs install.sh; its output goes to $work/out.
try() {
	PATH="$(fakeuname "$1" "$2"):$PATH" ADBUDDY_DOWNLOAD_URL="$3" ADBUDDY_INSTALL_DIR="$4" \
		sh "$here/install.sh" >"$work/out" 2>&1
}

check() { # name condition...
	name=$1
	shift
	if ! "$@"; then
		echo "FAIL $name"
		sed 's/^/    /' "$work/out"
		fail=1
	fi
}

has() { grep -q "$1" "$work/out"; }

for case in "Linux x86_64 linux_amd64" "Linux aarch64 linux_arm64" "Darwin arm64 darwin_all" "Darwin x86_64 darwin_all"; do
	set -- $case
	dir=$work/bin-$1-$2
	check "$1 $2 installs" try "$1" "$2" "$(url "$rel")" "$dir"
	check "$1 $2 runs" sh -c '"$1/adbuddy" | grep -q "$2"' _ "$dir" "$3"
done

check "warns when the folder is not in PATH" has "not in your PATH"

check "refuses another architecture" sh -c '! try() { :; }; true'
if try Linux riscv64 "$(url "$rel")" "$work/bin-riscv"; then
	echo "FAIL riscv64 accepted"
	fail=1
fi
check "says why for riscv64" has "no build for Linux riscv64"
if try MINGW64_NT-10.0 x86_64 "$(url "$rel")" "$work/bin-win"; then
	echo "FAIL Windows accepted"
	fail=1
fi
check "points Windows to the zip" has "Windows"

# A tampered archive is not installed.
bad=$work/bad
cp -r "$rel" "$bad"
printf 'evil\n' >>"$bad/adbuddy_9.9.9_linux_amd64.tar.gz"
if try Linux x86_64 "$(url "$bad")" "$work/bin-bad"; then
	echo "FAIL tampered archive accepted"
	fail=1
fi
check "says the checksum is wrong" has "does not match its SHA-256"
check "installs nothing when tampered" test ! -e "$work/bin-bad/adbuddy"

# No line for the target in checksums.txt.
none=$work/none
mkdir -p "$none"
cp "$rel"/adbuddy_9.9.9_linux_amd64.tar.gz "$none/"
: >"$none/checksums.txt"
if try Linux x86_64 "$(url "$none")" "$work/bin-none"; then
	echo "FAIL missing checksum accepted"
	fail=1
fi
check "says there is no archive" has "no adbuddy archive for linux_amd64"

[ "$fail" -eq 0 ] && echo "install: all tests pass"
exit "$fail"
```

Quita del test la línea `check "refuses another architecture" sh -c '! try() { :; }; true'`: es un no-op que sobra. El caso de verdad son las líneas siguientes.

- [ ] **Step 2: Ejecutar y ver que falla**

Run (Git Bash, desde la raíz): `sh scripts/install_test.sh`
Expected: `FAIL` en cada caso y salida 1.

- [ ] **Step 3: Implementar**

`scripts/install.sh`:

```sh
#!/bin/sh
# Installs the adbuddy host on Linux or macOS (spec M6a §6):
#
#   curl -fsSL https://raw.githubusercontent.com/hybriqtech/adbuddy/main/scripts/install.sh | sh
#
# ADBUDDY_VERSION=v0.3.0 installs that release instead of the latest one.
# ADBUDDY_INSTALL_DIR picks the folder (default: ~/.local/bin; no sudo).
# ADBUDDY_DOWNLOAD_URL replaces the release URL, for tests only: the
# checksums come from the same place as the archive.
#
# Everything runs from main, called on the last line, so a cut download
# runs nothing.
set -eu

say() { printf 'adbuddy install: %s\n' "$*" >&2; }
die() {
	say "$*"
	exit 1
}
have() { command -v "$1" >/dev/null 2>&1; }

fetch() { # url file
	if have curl; then
		curl -fsSL -o "$2" "$1"
	else
		wget -q -O "$2" "$1"
	fi
}

main() {
	have curl || have wget || die "needs curl or wget"
	have tar || die "needs tar"
	if have sha256sum; then
		sum="sha256sum"
	elif have shasum; then
		sum="shasum -a 256"
	else
		die "needs sha256sum or shasum"
	fi

	os=$(uname -s)
	arch=$(uname -m)
	case $os in
	Linux)
		case $arch in
		x86_64 | amd64) target=linux_amd64 ;;
		aarch64 | arm64) target=linux_arm64 ;;
		*) die "no build for Linux $arch" ;;
		esac
		;;
	Darwin) target=darwin_all ;;
	MINGW* | MSYS* | CYGWIN*) die "on Windows, download the .zip from https://github.com/hybriqtech/adbuddy/releases (see host/README.md)" ;;
	*) die "no build for $os" ;;
	esac

	version=${ADBUDDY_VERSION:-latest}
	if [ -n "${ADBUDDY_DOWNLOAD_URL:-}" ]; then
		base=${ADBUDDY_DOWNLOAD_URL%/}
	elif [ "$version" = latest ]; then
		base=https://github.com/hybriqtech/adbuddy/releases/latest/download
	else
		base=https://github.com/hybriqtech/adbuddy/releases/download/$version
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM
	fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "could not download $base/checksums.txt"
	line=$(grep -E "^[0-9a-f]{64}  adbuddy_[^ /]*_${target}\.tar\.gz\$" "$tmp/checksums.txt" || true)
	if [ -z "$line" ] || [ "$(printf '%s\n' "$line" | wc -l)" -ne 1 ]; then
		die "no adbuddy archive for $target in $base/checksums.txt"
	fi
	want=${line%% *}
	name=${line##* }
	fetch "$base/$name" "$tmp/$name" || die "could not download $base/$name"
	got=$(cd "$tmp" && $sum "$name")
	got=${got%% *}
	[ "$got" = "$want" ] || die "$name does not match its SHA-256 in checksums.txt: not installing it"

	mkdir "$tmp/x"
	tar -xzf "$tmp/$name" -C "$tmp/x" adbuddy || die "$name has no adbuddy inside"
	dir=${ADBUDDY_INSTALL_DIR:-$HOME/.local/bin}
	mkdir -p "$dir"
	cp "$tmp/x/adbuddy" "$dir/.adbuddy.new"
	chmod 755 "$dir/.adbuddy.new"
	mv -f "$dir/.adbuddy.new" "$dir/adbuddy"
	say "installed $("$dir/adbuddy" --version 2>/dev/null || echo adbuddy) in $dir"
	case ":$PATH:" in
	*":$dir:"*) ;;
	*) say "$dir is not in your PATH: add it to run adbuddy" ;;
	esac
}

main "$@"
```

- [ ] **Step 4: Ejecutar y ver que pasa**

Run: `sh scripts/install_test.sh`
Expected: `install: all tests pass` y salida 0.

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "feat(release): script de instalación para Linux y macOS con checksum

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 6: goreleaser y los workflows de release

**Files:**
- Create: `.goreleaser.yaml`
- Create: `scripts/release-cert.sh`
- Create: `.github/workflows/release.yml`
- Create: `.github/workflows/release-check.yml`

**Interfaces:**
- Consumes: `sh scripts/release-version.sh` (Task 4), `sh scripts/install.sh` y `sh scripts/install_test.sh` (Task 5), `go run ./cmd/apkcert` (Task 3) y las propiedades `adbuddy.versionName` y `adbuddy.versionCode` (Task 4).
- Produces: `sh scripts/release-cert.sh <apk> [--pinned]` sale con 0 cuando `apksigner` y `apkcert` dan un único certificado y el mismo (y, con `--pinned`, el fijado). Necesita `APKSIGNER`, la ruta del ejecutable.

- [ ] **Step 1: `scripts/release-cert.sh`**

```sh
#!/bin/sh
# Checks the signing certificate of an APK two ways, apksigner and the
# host's own parser (host/cmd/apkcert), and with --pinned also against the
# certificate the host pins (spec M6a §4, §7):
#
#   APKSIGNER=$ANDROID_HOME/build-tools/37.0.0/apksigner sh scripts/release-cert.sh app.apk [--pinned]
set -eu

die() {
	echo "release-cert: $*" >&2
	exit 1
}

main() {
	[ $# -ge 1 ] && [ $# -le 2 ] || die "usage: release-cert.sh <file.apk> [--pinned]"
	[ -n "${APKSIGNER:-}" ] || die "set APKSIGNER to the apksigner of the build-tools"
	apk=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
	root=$(cd "$(dirname "$0")/.." && pwd)

	"$APKSIGNER" verify --verbose --print-certs "$apk" >"$apk.verify" || die "apksigner does not verify $1"
	grep -q '^Verified using v2 scheme (APK Signature Scheme v2): true' "$apk.verify" || die "$1 has no v2 signature"
	grep -q '^Verified using v3 scheme (APK Signature Scheme v3): true' "$apk.verify" || die "$1 has no v3 signature"
	theirs=$(sed -n 's/^Signer #1 certificate SHA-256 digest: //p' "$apk.verify")
	signers=$(grep -c '^Signer #[0-9]* certificate SHA-256 digest: ' "$apk.verify" || true)
	rm -f "$apk.verify"
	[ "$signers" -eq 1 ] || die "$1 has $signers signers, not 1"
	ours=$(cd "$root/host" && go run ./cmd/apkcert "$apk") || die "the host refuses $1"
	[ "$ours" = "$theirs" ] || die "apksigner says $theirs, the host says $ours"
	if [ "${2:-}" = --pinned ]; then
		pinned=$(cd "$root/host" && go run ./cmd/apkcert -pinned) || die "the host pins no certificate: see docs/releasing.md"
		[ "$ours" = "$pinned" ] || die "$1 is signed by $ours, but the host pins $pinned"
	elif [ -n "${2:-}" ]; then
		die "unknown option $2"
	fi
	echo "release-cert: $1 is signed by $ours"
}

main "$@"
```

(`apksigner verify --print-certs` imprime `Signer #1 certificate SHA-256 digest: <hex>` en minúsculas y sin dos puntos, y `--verbose` añade las líneas `Verified using vN scheme…`.)

- [ ] **Step 2: `.goreleaser.yaml`**

```yaml
# Releases of the adbuddy host (spec M6a §4). .github/workflows/release.yml
# runs it on a vX.Y.Z tag and then adds the signed APK to the same draft.
version: 2

project_name: adbuddy
dist: host/dist

builds:
  - id: adbuddy
    dir: host
    main: ./cmd/adbuddy
    binary: adbuddy
    env:
      - CGO_ENABLED=0
    flags:
      - -trimpath
    ldflags:
      - -s -w -X github.com/hybriqtech/adbuddy/host/internal/cli.Version={{ .Version }}
    mod_timestamp: "{{ .CommitTimestamp }}"
    goos: [linux, windows, darwin]
    goarch: [amd64, arm64]

universal_binaries:
  - id: adbuddy
    replace: true
    mod_timestamp: "{{ .CommitTimestamp }}"

archives:
  - id: adbuddy
    formats: [tar.gz]
    format_overrides:
      - goos: windows
        formats: [zip]
    name_template: "{{ .ProjectName }}_{{ .Version }}_{{ .Os }}_{{ .Arch }}"
    files:
      - LICENSE
      - README.md

checksum:
  name_template: checksums.txt

snapshot:
  version_template: "{{ incpatch .Version }}-snapshot"

changelog:
  sort: asc

release:
  github:
    owner: hybriqtech
    name: adbuddy
  draft: true
  prerelease: auto
  mode: keep-existing
```

(El binario universal de macOS tiene `.Arch` = `all`, así que su archivo es `adbuddy_<v>_darwin_all.tar.gz`, el nombre que espera `install.sh`. `LICENSE` lo añade la Task 7. Hasta entonces, `goreleaser check` avisa de que falta, pero el job no lo ejecuta antes de que exista.)

- [ ] **Step 3: `.github/workflows/release.yml`**

```yaml
name: release

# A tag vX.Y.Z or vX.Y.Z-rc.N builds a DRAFT release: the host binaries
# (goreleaser) and the signed APK. The maintainer publishes it by hand
# (docs/releasing.md). Spec: docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md.
on:
  push:
    tags: ["v*"]

permissions:
  contents: read

concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false

env:
  BUILD_TOOLS: "37.0.0"

jobs:
  version:
    runs-on: ubuntu-latest
    outputs:
      name: ${{ steps.v.outputs.name }}
      code: ${{ steps.v.outputs.code }}
    steps:
      - uses: actions/checkout@v5
      - id: v
        run: sh scripts/release-version.sh "$GITHUB_REF_NAME" >> "$GITHUB_OUTPUT"

  test:
    needs: version
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: host/go.mod
          cache-dependency-path: host/go.sum
      - name: Host tests
        working-directory: host
        run: |
          go vet ./...
          go test -race -timeout 20m ./...
      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: "17"
      - uses: gradle/actions/setup-gradle@v4
        with:
          gradle-version: "9.8.0"
      - name: Install Android SDK platform 37
        run: yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" "platforms;android-37.0"
      - name: App unit tests
        working-directory: android
        run: gradle --no-daemon :app:testDebugUnitTest

  host:
    needs: test
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v6
        with:
          go-version-file: host/go.mod
          cache-dependency-path: host/go.sum
      - uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  apk:
    needs: [version, host]
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@v5
      - name: Check the signing secrets
        env:
          KS: ${{ secrets.ADBUDDY_KEYSTORE_B64 }}
          KS_PASS: ${{ secrets.ADBUDDY_KEYSTORE_PASSWORD }}
        run: |
          if [ -z "$KS" ] || [ -z "$KS_PASS" ]; then
            echo "::error::The secrets ADBUDDY_KEYSTORE_B64 and ADBUDDY_KEYSTORE_PASSWORD are missing: see docs/releasing.md. The draft release has no APK; delete it and push the tag again."
            exit 1
          fi
      - uses: actions/setup-go@v6
        with:
          go-version-file: host/go.mod
          cache-dependency-path: host/go.sum
      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: "17"
      - uses: gradle/actions/setup-gradle@v4
        with:
          gradle-version: "9.8.0"
      - name: Install Android SDK platform and build-tools
        run: yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" "platforms;android-37.0" "build-tools;$BUILD_TOOLS"
      - name: Build the release APK
        working-directory: android
        env:
          NAME: ${{ needs.version.outputs.name }}
          CODE: ${{ needs.version.outputs.code }}
        run: gradle --no-daemon :app:assembleRelease -Padbuddy.versionName="$NAME" -Padbuddy.versionCode="$CODE"
      - name: Sign
        env:
          KS: ${{ secrets.ADBUDDY_KEYSTORE_B64 }}
          KS_PASS: ${{ secrets.ADBUDDY_KEYSTORE_PASSWORD }}
        run: |
          bt="$ANDROID_HOME/build-tools/$BUILD_TOOLS"
          printf '%s' "$KS" | base64 -d > "$RUNNER_TEMP/release.p12"
          "$bt/zipalign" -f 4 android/app/build/outputs/apk/release/app-release-unsigned.apk "$RUNNER_TEMP/aligned.apk"
          "$bt/apksigner" sign --ks "$RUNNER_TEMP/release.p12" --ks-type PKCS12 --ks-pass env:KS_PASS \
            --ks-key-alias adbuddy --v1-signing-enabled false --v2-signing-enabled true --v3-signing-enabled true \
            --out "$RUNNER_TEMP/adbuddy.apk" "$RUNNER_TEMP/aligned.apk"
      - name: Remove the keystore
        if: always()
        run: rm -f "$RUNNER_TEMP/release.p12"
      - name: Check the certificate against the host's pin
        run: APKSIGNER="$ANDROID_HOME/build-tools/$BUILD_TOOLS/apksigner" sh scripts/release-cert.sh "$RUNNER_TEMP/adbuddy.apk" --pinned
      - name: Upload the APK to the draft release
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          cd "$RUNNER_TEMP"
          sha256sum adbuddy.apk > adbuddy.apk.sha256
          gh release upload "$GITHUB_REF_NAME" adbuddy.apk adbuddy.apk.sha256 --repo "$GITHUB_REPOSITORY"
```

- [ ] **Step 4: `.github/workflows/release-check.yml`**

```yaml
name: release-check

# Builds everything the release workflow builds, without publishing and
# without secrets (spec M6a §7). Linux only.
on:
  pull_request:
    paths: &paths
      - ".goreleaser.yaml"
      - ".github/workflows/release.yml"
      - ".github/workflows/release-check.yml"
      - "scripts/install*.sh"
      - "scripts/release-*.sh"
      - "host/internal/phoneapp/**"
      - "host/cmd/apkcert/**"
      - "android/app/build.gradle.kts"
      - "LICENSE"
  push:
    branches: [main]
    paths: *paths

permissions:
  contents: read

env:
  BUILD_TOOLS: "37.0.0"

jobs:
  scripts:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - run: shellcheck -s sh scripts/install.sh scripts/install_test.sh scripts/release-*.sh
      - run: sh scripts/release-version_test.sh
      - run: sh scripts/install_test.sh

  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
        with:
          fetch-depth: 0
      - uses: actions/setup-go@v6
        with:
          go-version-file: host/go.mod
          cache-dependency-path: host/go.sum
      - uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: check
      - uses: goreleaser/goreleaser-action@v6
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --snapshot --clean
      - name: The snapshot has every archive
        run: |
          cd host/dist
          for t in linux_amd64 linux_arm64 darwin_all; do ls adbuddy_*_"$t".tar.gz; done
          for t in windows_amd64 windows_arm64; do ls adbuddy_*_"$t".zip; done
          test -s checksums.txt
      - name: install.sh installs the snapshot
        run: |
          ADBUDDY_DOWNLOAD_URL="file://$PWD/host/dist" ADBUDDY_INSTALL_DIR="$RUNNER_TEMP/bin" sh scripts/install.sh
          "$RUNNER_TEMP/bin/adbuddy" --version | grep -q '^adbuddy .*-snapshot'

  sign:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v5
      - uses: actions/setup-go@v6
        with:
          go-version-file: host/go.mod
          cache-dependency-path: host/go.sum
      - uses: actions/setup-java@v4
        with:
          distribution: temurin
          java-version: "17"
      - uses: gradle/actions/setup-gradle@v4
        with:
          gradle-version: "9.8.0"
      - name: Install Android SDK platform and build-tools
        run: yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" "platforms;android-37.0" "build-tools;$BUILD_TOOLS"
      - name: Build a release APK with a test version
        working-directory: android
        run: gradle --no-daemon :app:assembleRelease -Padbuddy.versionName=0.0.1-rc.1 -Padbuddy.versionCode=101
      - name: Sign with a throwaway key
        env:
          KS_PASS: throwaway-${{ github.run_id }}
        run: |
          bt="$ANDROID_HOME/build-tools/$BUILD_TOOLS"
          keytool -genkeypair -keystore "$RUNNER_TEMP/test.p12" -storetype PKCS12 -storepass:env KS_PASS \
            -alias adbuddy -keyalg RSA -keysize 2048 -validity 1 -dname "CN=release-check"
          "$bt/zipalign" -f 4 android/app/build/outputs/apk/release/app-release-unsigned.apk "$RUNNER_TEMP/aligned.apk"
          "$bt/apksigner" sign --ks "$RUNNER_TEMP/test.p12" --ks-type PKCS12 --ks-pass env:KS_PASS \
            --ks-key-alias adbuddy --v1-signing-enabled false --v2-signing-enabled true --v3-signing-enabled true \
            --out "$RUNNER_TEMP/adbuddy.apk" "$RUNNER_TEMP/aligned.apk"
      - name: The host reads the same certificate as apksigner
        run: APKSIGNER="$ANDROID_HOME/build-tools/$BUILD_TOOLS/apksigner" sh scripts/release-cert.sh "$RUNNER_TEMP/adbuddy.apk"
      - name: The APK has the test version
        run: |
          "$ANDROID_HOME/build-tools/$BUILD_TOOLS/aapt2" dump badging "$RUNNER_TEMP/adbuddy.apk" | tee "$RUNNER_TEMP/badging"
          grep -q "versionCode='101' versionName='0.0.1-rc.1'" "$RUNNER_TEMP/badging"
```

(GitHub Actions admite anclas YAML desde 2025. Si `actionlint` o el runner las rechazan, repite la lista de `paths` en `push`.)

- [ ] **Step 5: Comprobaciones locales**

En este PC no hay goreleaser, `actionlint`, `shellcheck` ni JDK. Comprueba lo que se puede:

Run: `sh -n scripts/release-cert.sh && sh scripts/release-version_test.sh && sh scripts/install_test.sh`
Expected: sin errores de sintaxis y los dos tests pasan.

Run (desde `host/`): `go run github.com/goreleaser/goreleaser/v2@latest check -f ../.goreleaser.yaml`
Expected: `config is valid` (el aviso de que falta `LICENSE` es aceptable hasta la Task 7). Si `go run` no puede descargar goreleaser, apúntalo en el ledger y sigue: la CI lo comprueba.

Run: `python -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .goreleaser.yaml .github/workflows/release.yml .github/workflows/release-check.yml`
Expected: sin salida (YAML válido). Si `yaml` no está instalado, usa `uv run --with pyyaml python -c …`.

- [ ] **Step 6: Commit**

```bash
git add .goreleaser.yaml scripts/release-cert.sh .github/workflows/release.yml .github/workflows/release-check.yml
git commit -m "feat(release): goreleaser y workflows de release (borrador) y de comprobación

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 7: Clave de release, LICENSE y documentación

**Files:**
- Create: `scripts/release-key.sh`
- Create: `LICENSE`
- Create: `docs/releasing.md`
- Modify: `host/README.md` (sección `## Build`, que pasa a `## Install`)
- Modify: `docs/superpowers/plans/pendientes-m3-m5.md` (solo la sección `## M6 · Publicación`)

**Interfaces:**
- Consumes: todo lo anterior; los nombres de los secretos y del alias, de los Global Constraints.
- Produces: lo que el usuario sigue a mano.

- [ ] **Step 1: `scripts/release-key.sh`**

```sh
#!/bin/sh
# Creates the app's release signing key, once, OUTSIDE the repo
# (docs/releasing.md). keytool asks for the password; it never goes on a
# command line. Needs keytool: a JDK (winget install EclipseAdoptium.Temurin.21.JDK)
# or Android Studio's jbr/bin.
#
#   sh scripts/release-key.sh
set -eu

die() {
	echo "release-key: $*" >&2
	exit 1
}

main() {
	keytool=keytool
	if ! command -v keytool >/dev/null 2>&1; then
		[ -n "${JAVA_HOME:-}" ] && [ -x "$JAVA_HOME/bin/keytool" ] || die "keytool not found: install a JDK or set JAVA_HOME"
		keytool=$JAVA_HOME/bin/keytool
	fi
	dir=${ADBUDDY_RELEASE_KEY_DIR:-$HOME/.adbuddy-release}
	ks=$dir/release.p12
	root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel 2>/dev/null || true)
	case $dir in
	"$root" | "$root"/*) [ -n "$root" ] && die "$dir is inside the repo: pick a folder outside it" ;;
	esac
	[ -e "$ks" ] && die "$ks already exists: never replace a release key (installed apps could not update)"
	mkdir -p "$dir"
	chmod 700 "$dir"
	"$keytool" -genkeypair -keystore "$ks" -storetype PKCS12 -alias adbuddy \
		-keyalg RSA -keysize 4096 -validity 10950 -dname "CN=hybriqtech ADBuddy, O=hybriqtech"
	chmod 600 "$ks"
	echo
	echo "Key created: $ks"
	echo "Back it up with its password somewhere safe. If it is lost, no release can update the app."
	echo
	echo "Certificate SHA-256 (public: it goes into host/internal/phoneapp/release.go):"
	"$keytool" -list -v -keystore "$ks" -storetype PKCS12 -alias adbuddy |
		sed -n 's/^[[:space:]]*SHA256: //p' | tr -d ':' | tr 'A-F' 'a-f'
	echo
	echo "GitHub secrets (gh asks for each value; nothing is printed):"
	echo "  base64 < \"$ks\" | tr -d '\\n' | gh secret set ADBUDDY_KEYSTORE_B64 --repo hybriqtech/adbuddy"
	echo "  gh secret set ADBUDDY_KEYSTORE_PASSWORD --repo hybriqtech/adbuddy"
}

main "$@"
```

Run: `sh -n scripts/release-key.sh && ADBUDDY_RELEASE_KEY_DIR="$PWD/x" sh scripts/release-key.sh; echo "exit $?"`
Expected: sin errores de sintaxis. La segunda orden sale con 1, con "inside the repo" o, si no hay JDK, con "keytool not found". **No** generes ninguna clave real: eso lo hace el usuario.

- [ ] **Step 2: `LICENSE`**

Descarga el texto oficial y compruébalo:

Run: `curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE && head -3 LICENSE && grep -c "END OF TERMS AND CONDITIONS" LICENSE`
Expected: la cabecera "Apache License / Version 2.0, January 2004" y el número `1`.

El texto oficial no lleva línea de copyright fuera del apéndice. No lo cambies: el copyright va en `README.md` (Step 4).

- [ ] **Step 3: `docs/releasing.md`**

````markdown
# Releasing ADBuddy

A release is a git tag. `.github/workflows/release.yml` builds a **draft** release with the host binaries (goreleaser) and the signed app (`adbuddy.apk` and `adbuddy.apk.sha256`). Nothing is public until a maintainer publishes the draft. Design: `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md`.

## User actions (once)

Only the repository owner can do these. Nothing else in the pipeline waits for them, but there is no APK release until they are done.

1. **Create the signing key** on your PC, outside the repo. It needs `keytool`: install a JDK (`winget install EclipseAdoptium.Temurin.21.JDK`) or use Android Studio's `jbr/bin/keytool`.

   ```sh
   sh scripts/release-key.sh
   ```

   It writes `~/.adbuddy-release/release.p12`, asks for a password and prints the certificate's SHA-256 and the `gh secret set` commands.
2. **Back up** `release.p12` and its password (a password manager, an encrypted drive). If they are lost, no future release can update the app already on phones: Android only updates an app signed with the same key.
3. **Create the two secrets** with the commands it printed:
   - `ADBUDDY_KEYSTORE_B64`: the keystore in base64;
   - `ADBUDDY_KEYSTORE_PASSWORD`: its password.

   The key alias is always `adbuddy`.
4. **Pin the certificate.** Put the SHA-256 it printed (lowercase hex, no colons) in `SigningCertSHA256` in `host/internal/phoneapp/release.go`, in its own commit and PR. Until then, `adbuddy setup` downloads no app (it says so) and the `apk` job fails.
5. **Your phone.** It has a development build of the app, signed with another key. Uninstall ADBuddy on the phone before installing the first release (it forgets its pairing; `adbuddy setup` pairs again). `adbuddy setup` explains this if you forget.

## Cutting a release

1. `main` is green and holds what you want to ship.
2. Tag it: `git tag v0.3.0 && git push origin v0.3.0` (or `v0.3.0-rc.1` for a prerelease). Tags must be `vX.Y.Z` or `vX.Y.Z-rc.N` (`scripts/release-version.sh` explains the limits).
3. Wait for the `release` workflow: `version` → `test` → `host` → `apk`.
4. Open the draft on GitHub, check the assets (5 archives, `checksums.txt`, `adbuddy.apk`, `adbuddy.apk.sha256`) and the notes, and press **Publish release**. Prereleases (`-rc.N`) are never `latest`, so `adbuddy setup` and `install.sh` skip them.

If a job fails, delete the draft and the tag (`git push origin :v0.3.0`), fix the problem and tag again.

## What protects the app download

- `adbuddy setup` checks the APK against `adbuddy.apk.sha256` and then checks that **every** signer in its v2, v3 and v3.1 signature blocks has the pinned certificate. A copy signed with another key is refused, wherever it comes from.
- That is why `ADBUDDY_RELEASE_URL` (used by the e2e tests) is harmless: another origin can only serve APKs that hybriqtech signed.
- `--apk <file>` installs a file you choose, without that check (development builds).
- Key rotation is not supported. Rotating the key needs a new host release that reads the v3 lineage first.

## Later (M6c)

Homebrew tap, winget, IzzyOnDroid and SignPath need accounts or repositories that do not exist yet; see the M6 section of `docs/superpowers/plans/pendientes-m3-m5.md`.
````

- [ ] **Step 4: `host/README.md`**

Sustituye la sección `## Build` (el título y su bloque de código) por:

````markdown
## Install

- **Linux and macOS:**

  ```sh
  curl -fsSL https://raw.githubusercontent.com/hybriqtech/adbuddy/main/scripts/install.sh | sh
  ```

  It installs the latest release in `~/.local/bin` after checking its SHA-256 (`ADBUDDY_INSTALL_DIR` picks another folder, `ADBUDDY_VERSION=v0.3.0` another release).
- **Windows:** download `adbuddy_<version>_windows_amd64.zip` (or `_arm64`) from the [releases](https://github.com/hybriqtech/adbuddy/releases), check it against `checksums.txt` and put `adbuddy.exe` in a folder in your `PATH`. winget comes later.
- **From source:**

  ```sh
  cd host
  go build -o adbuddy ./cmd/adbuddy      # adbuddy.exe on Windows
  ```

Releases: `docs/releasing.md`. Licensed under Apache-2.0 (`LICENSE`), © 2026 hybriqtech.
````

- [ ] **Step 5: `pendientes-m3-m5.md`, solo la sección M6**

En `## M6 · Publicación`, sustituye los dos puntos de `ADBUDDY_RELEASE_URL` y de fijar el SHA-256 del certificado por:

```markdown
- **Hecho en M6a** (`docs/superpowers/plans/2026-10-01-adbuddy-m6a-releases.md`): el host fija el SHA-256 del certificado de firma del APK y `ADBUDDY_RELEASE_URL` deja de ser un riesgo (spec de M6a §5). Falta que el usuario genere la clave y ponga la constante (`docs/releasing.md`, "User actions").
```

y añade al final de la sección:

```markdown
- **M6c, tras la primera release pública:** tap de Homebrew (`hybriqtech/homebrew-tap` y un token con permiso de escritura), manifiesto de winget (`hybriqtech.adbuddy`, PR a `microsoft/winget-pkgs`), IzzyOnDroid y la firma de binarios de Windows con SignPath Foundation. IzzyOnDroid no acepta el bloque de dependencias cifrado de AGP (`0x504b4453`): antes de pedirlo, `dependenciesInfo { includeInApk = false }` en `android/app/build.gradle.kts`, coordinado con quien lleve `android/` entonces.
- **Rotación de la clave de firma:** no está soportada (spec de M6a §9). Si hace falta, el host tiene que leer el linaje de v3 antes de que salga un APK con la clave nueva.
```

- [ ] **Step 6: Comprobación y commit**

Run: `git diff --stat main...HEAD -- . ':!docs/superpowers'` y comprueba que solo aparecen las rutas de los Global Constraints.

```bash
git add scripts/release-key.sh LICENSE docs/releasing.md host/README.md docs/superpowers/plans/pendientes-m3-m5.md
git commit -m "docs(release): clave de firma, licencia Apache-2.0 y guía de releases

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Validaciones manuales pendientes

Lo que no se puede probar en este PC o sin acción del usuario:

1. **CI de `release-check.yml`** (goreleaser snapshot, `shellcheck`, `apksigner` contra `apkcert`, versión en el APK): corre al abrir el PR, cuando el supervisor autorice el push. Si falla, se arregla en la rama antes de integrar. Riesgos que solo se ven ahí:
   - `assembleRelease` con R8 no se había compilado nunca;
   - el nombre exacto de `app-release-unsigned.apk` en AGP 9;
   - las anclas YAML.
2. **La app de release con R8 funciona en un móvil:** el APK de `release-check` (artefacto o build local) en un emulador con imagen válida o en el móvil del usuario. **El móvil, solo con su permiso**, porque obliga a desinstalar el build de debug. Hay que mirar, sobre todo, la serialización de los mensajes del protocolo (kotlinx.serialization con R8).
3. **Acciones del usuario** (`docs/releasing.md`, "User actions"): generar la clave, crear los dos secretos y poner `SigningCertSHA256` en un PR propio. Después, la primera etiqueta: comprobar que el borrador tiene los 9 ficheros y que `adbuddy setup` descarga e instala la app desde la release publicada.
4. **`gh release upload` a un borrador por nombre de etiqueta:** se comprueba en la primera release real (o en un fork con secretos de prueba). Si `gh` no encuentra el borrador, el paso usa `gh release upload "$(gh release list --json tagName,isDraft …)"` o el id que devuelve goreleaser.
````
