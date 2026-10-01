# ADBuddy M6a · Pipeline de releases y app de confianza · Plan de implementación

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Que el host solo dé el token y los permisos a una app `tech.hybriq.adbuddy` firmada con un certificado de confianza, y que una etiqueta `vX.Y.Z` produzca una release en borrador con los binarios del host y el APK firmado con ese certificado.

**Architecture:**
- **Parser:** un parser Go del APK Signing Block en `host/internal/phoneapp`, que encuentra el mismo bloque que Android o rechaza, da el SHA-256 del certificado de cada firmante.
- **App de confianza:** `Verifier.Verify` lee la app instalada por adb (`pm path`, `stat`, `base64`) y devuelve un `Verified`, que no se puede construir fuera del paquete. Las funciones que dan algo a la app lo exigen. `Download` exige el certificado fijado.
- **Releases:** goreleaser compila el host; `assembleRelease` compila el APK en un job, otro job aislado y sin código de terceros lo firma, y un tercero comprueba el certificado con el parser antes de subirlo a la release en borrador.

**Tech Stack:** Go 1.27 (stdlib: `archive/zip`, `crypto/x509`, `encoding/binary`, `encoding/base64`), goreleaser v2, GitHub Actions (acciones fijadas por SHA), AGP 9 + `apksigner`/`zipalign` (build-tools 37.0.0), POSIX `sh`.

**Spec:** `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md` (commit 0edf31e o posterior)

## Global Constraints

- **Rama y commits:**
  - Rama `m6a`, worktree `C:\Users\Jose\orca\workspaces\adbuddy\skimmer`, identidad git local hybriqtech.
  - Cada commit acaba con el trailer exacto `Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>`.
  - Nada de push, PR ni merge: los autoriza el supervisor (`adbuddy-e5`) al final.
- **Rutas permitidas.** Solo estas (acordadas con el supervisor):
  - host: `host/internal/phoneapp/**`, `host/cmd/apkcert/**`, `host/internal/cli/appsetup.go`, `host/internal/cli/appsetup_test.go` y la línea de ayuda de `--apk` en `host/internal/cli/setup.go`;
  - en `host/internal/cli/phone.go`: `connect()`, el `Verifier` (los campos `verifier` y `recheck` de `phoneLink`), el gancho `Admit`, el `select` del ticker y el aviso de arranque de `trusted-dev-apk`, **en un commit propio** (con el cambio de `Handshake`); además `host/internal/cli/phone_test.go` y `host/cmd/adbuddy/phone_e2e_test.go`;
  - en `host/cmd/adbuddy/e2e_test.go` (fichero de tern), solo `startWithData(t, cfg, prepare func(data string), rules...)`, con `startWith` delegando en él con `prepare` nil, **en un commit propio**: solo prepara el arnés (escribe la confianza de desarrollo en el directorio de datos antes de arrancar serve) y no cambia las aserciones de ningún test (ruling del supervisor);
  - canal: `host/internal/bridge/server.go` (solo `Options.Admit`, `admitTimeout` y su llamada en `handle`, **en un commit propio** tras m3-bridge) y el nuevo `host/internal/bridge/admit_test.go`;
  - clasificador: `host/internal/classify/special.go` y `host/internal/classify/testdata/habitual.jsonl`;
  - en `android/app/build.gradle.kts`, las dos líneas `versionCode`/`versionName`, **en un commit propio**;
  - scripts: `scripts/install.sh`, `scripts/install_test.sh`, `scripts/release-key.sh`, `scripts/release-version.sh`, `scripts/release-version_test.sh` y `scripts/release-cert.sh`;
  - CI: `.goreleaser.yaml`, `.github/workflows/release.yml` y `.github/workflows/release-check.yml`;
  - docs: `LICENSE`, `docs/releasing.md`, la sección Install de `host/README.md` y la sección `## M6 · Publicación` de `docs/superpowers/plans/pendientes-m3-m5.md`.

  **Nada más.** No se tocan `emulator.yml`, `host.yml`, `android.yml`, `serve.go`, `connect.go`, `config/` ni `testutil/`.
- **Etiquetas:** `^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})(-rc\.[1-9][0-9]?)?$`, con X ≤ 20 y N ≤ 98. `versionCode = ((X·1000 + Y)·1000 + Z)·100 + S`, donde S = N en una rc y 99 en la final.
- **IDs del Signing Block:**
  - de firma: v2 `0x7109871a`, v3 `0xf05368c0` y v3.1 `0x1b93ad61`; el atributo de rotación es `0x3ba06f8c`;
  - permitidos que no son de firma: `0x42726577`, `0x2b09189e`, `0x6dff800d`, `0x504b4453` y `0x2146444e`;
  - cualquier otro ID, o uno repetido, rechaza el APK.
- **Zip:** el EOCD exactamente en `len-22` y sin comentario; sin localizador ZIP64 (`0x07064b50` en `eocd-20`); `cdOffset+cdSize == eocd`; ninguna entrada `META-INF/*.SF|.RSA|.DSA|.EC` (sin distinguir mayúsculas).
- **Firma:** v2 y v3, sin v1, con el alias `adbuddy`. Dos secretos, **del environment `release`**: `ADBUDDY_KEYSTORE_B64` y `ADBUDDY_KEYSTORE_PASSWORD`. Nunca hay contraseñas en la línea de órdenes (`--ks-pass env:…`, `-storepass:env`).
- **Release:** siempre **en borrador**; la publica el usuario.
- **Binarios:** `CGO_ENABLED=0`, `-trimpath`, `-s -w` y `-X github.com/hybriqtech/adbuddy/host/internal/cli.Version={{ .Version }}`.
- **Acciones fijadas por SHA** (resueltas el 2026-10-01 con `git ls-remote`):

  | Acción | SHA | Versión |
  |---|---|---|
  | `actions/checkout` | `3d3c42e5aac5ba805825da76410c181273ba90b1` | v7.0.1 |
  | `actions/setup-go` | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` | v7.0.0 |
  | `actions/setup-java` | `de7274f081f381c8f8158605e0321c36c376e2e6` | v6.0.1 |
  | `gradle/actions/setup-gradle` | `b9bee63ef481cc545e4517ec730544a4d8b1dfa0` | v6.4.0 |
  | `actions/upload-artifact` | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` | v7.0.1 |
  | `actions/download-artifact` | `3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c` | v8.0.1 |
  | `goreleaser/goreleaser-action` | `f06c13b6b1a9625abc9e6e439d9c05a8f2190e94` | v7.2.3 |

- **Estilo:** mensajes al usuario en inglés, como el resto del host. Comentarios en el estilo del paquete: frases cortas en inglés.
- **Tests de Go con límite:** `go test -timeout 10m ./...`, desde `host/`.
- **Sin herramientas locales:** en este PC no hay JDK, goreleaser, `shellcheck` ni `actionlint`. Lo que los necesita se prueba en la CI (ver "Validaciones manuales pendientes").
- **Scripts:** POSIX `sh`, llamados siempre con `sh scripts/…`. En el PC se ejecutan con Git Bash.

Ledger: `.superpowers/sdd/2026-10-01-adbuddy-m6a-releases/progress.md`.

**Revisores:**
- `adbuddy-reviewer-opus` para las Tasks 1-6 y 10 (parser, app de confianza, gancho `Admit`, clasificador y secretos de la CI);
- `adbuddy-reviewer` para las Tasks 7-9 y 11.

## Mapa de ficheros

| Fichero | Responsabilidad |
|---|---|
| `host/internal/phoneapp/apksig.go` | `Signers` (certificados de los firmantes, con las condiciones de la spec §5) y `CheckSigner` |
| `host/internal/phoneapp/apktest/apktest.go` | APK falsos con Signing Block para los tests (`APK`, `Build`, `Cert`, `SHA256`, `LP`) |
| `host/internal/phoneapp/trust.go` | `Trust`, `LoadTrust`, `TrustDevAPK`, `ForgetDevAPK`, `Verifier`, `Verified` y `MinAppVersionCode` |
| `host/internal/phoneapp/release.go` | `SigningCertSHA256`, `ErrUnpinned`, `Download(…, cert)` |
| `host/internal/phoneapp/phoneapp.go` | `Install` (`--no-incremental`, `ErrOtherSigner`) y funciones que piden `Verified` |
| `host/internal/cli/appsetup.go` | `releaseCert`, `releaseURL` con límites, `chooseAPK` y el flujo de setup con `Verified` |
| `host/internal/cli/phone.go` | `connect()`: `Verify` antes de `Handshake` |
| `host/internal/classify/special.go` | Órdenes que quitan o neutralizan la app: 🔴 especial |
| `host/cmd/apkcert/main.go` | Herramienta de la CI |
| `android/app/build.gradle.kts` | Versión de la app desde propiedades de Gradle |
| `scripts/*.sh` | Versión, instalación, certificado y clave |
| `.goreleaser.yaml`, `.github/workflows/release*.yml` | Pipeline |
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
  - `phoneapp.Signers(apk []byte) ([]string, error)`: SHA-256 en hexadecimal minúsculo del primer certificado de cada firmante, en el orden de bloques y firmantes;
  - `phoneapp.CheckSigner(apk []byte, trusted ...string) error`: nil o un error que envuelve `phoneapp.ErrSigner`;
  - `phoneapp.ErrSigner`;
  - paquete `apktest`:
    - constantes `V2`, `V3`, `V31`, `Padding`, `StampV1`, `StampV2`, `DependencyInfo`, `Frosting` y `Rotation`;
    - tipos `Signer{Cert []byte; Rotation bool}`, `Block{ID uint32; Signers []Signer; Raw []byte}` y `Options{Blocks []Block; Files []string; Comment string; Gap int; Zip64Locator bool}`;
    - funciones `Cert(cn string) []byte`, `SHA256(der []byte) string`, `APK(blocks ...Block) []byte`, `Build(o Options) []byte`, `SigningBlock(blocks ...Block) []byte` y `LP(parts ...[]byte) []byte`.

- [ ] **Step 1: El constructor de APK de prueba**

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

// Options shape the zip around the Signing Block.
type Options struct {
	Blocks       []Block  // none: a plain zip
	Files        []string // entry names; none: AndroidManifest.xml
	Comment      string   // zip comment (APKs have none)
	Gap          int      // junk bytes between the central directory and its end record
	Zip64Locator bool     // a ZIP64 locator right before the end record
}

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

// APK returns a well-formed APK with blocks in its Signing Block.
func APK(blocks ...Block) []byte { return Build(Options{Blocks: blocks}) }

// Build returns a zip shaped by o.
func Build(o Options) []byte {
	var z bytes.Buffer
	w := zip.NewWriter(&z)
	files := o.Files
	if len(files) == 0 {
		files = []string{"AndroidManifest.xml"}
	}
	for _, name := range files {
		f, err := w.Create(name)
		if err != nil {
			panic(err)
		}
		f.Write([]byte("x"))
	}
	if o.Comment != "" {
		w.SetComment(o.Comment)
	}
	if err := w.Close(); err != nil {
		panic(err)
	}
	b := z.Bytes()
	eocd := len(b) - 22 - len(o.Comment)
	cd := int(binary.LittleEndian.Uint32(b[eocd+16:]))
	var block []byte
	if len(o.Blocks) > 0 {
		block = SigningBlock(o.Blocks...)
	}
	mid := make([]byte, o.Gap)
	if o.Zip64Locator {
		mid = binary.LittleEndian.AppendUint32(mid, 0x07064b50)
		mid = append(mid, make([]byte, 16)...)
	}
	var out []byte
	out = append(out, b[:cd]...)
	out = append(out, block...)
	out = append(out, b[cd:eocd]...)
	out = append(out, mid...)
	out = append(out, b[eocd:]...)
	newEOCD := len(out) - 22 - len(o.Comment)
	binary.LittleEndian.PutUint32(out[newEOCD+16:], uint32(cd+len(block)))
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

- [ ] **Step 2: Los tests que fallan**

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
// starts (its first size field) and magic where its magic is.
func mangle(apk []byte, fn func(b []byte, at, magic int)) []byte {
	b := slices.Clone(apk)
	magic := bytes.Index(b, []byte("APK Sig Block 42"))
	size := int(binary.LittleEndian.Uint64(b[magic-8:]))
	fn(b, magic+16-size-8, magic)
	return b
}

func TestSigners(t *testing.T) {
	me, them := apktest.SHA256(ours), apktest.SHA256(other)
	v2 := apktest.Block{ID: apktest.V2, Signers: by(ours)}
	good := apktest.APK(v2)
	eocd := len(good) - 22
	cases := []struct {
		name string
		apk  []byte
		want []string
		err  string // part of the error; "" means no error
	}{
		{name: "v2", apk: good, want: []string{me}},
		{name: "v3", apk: apktest.APK(apktest.Block{ID: apktest.V3, Signers: by(ours)}), want: []string{me}},
		{name: "v3.1", apk: apktest.APK(apktest.Block{ID: apktest.V31, Signers: by(ours)}), want: []string{me}},
		{name: "v2 and v3", apk: apktest.APK(v2, apktest.Block{ID: apktest.V3, Signers: by(ours)}), want: []string{me, me}},
		{name: "v2 ours, v3 theirs", apk: apktest.APK(v2, apktest.Block{ID: apktest.V3, Signers: by(other)}), want: []string{me, them}},
		{name: "two signers", apk: apktest.APK(apktest.Block{ID: apktest.V3, Signers: by(ours, other)}), want: []string{me, them}},
		{name: "blocks that are not signatures", apk: apktest.APK(
			apktest.Block{ID: apktest.Padding, Raw: make([]byte, 16)}, v2,
			apktest.Block{ID: apktest.StampV1, Raw: []byte("stamp")},
			apktest.Block{ID: apktest.StampV2, Raw: []byte("stamp")},
			apktest.Block{ID: apktest.DependencyInfo, Raw: []byte("deps")},
			apktest.Block{ID: apktest.Frosting, Raw: []byte("play")}), want: []string{me}},
		{name: "a manifest is not a v1 signature", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2},
			Files: []string{"AndroidManifest.xml", "META-INF/MANIFEST.MF", "META-INF/com/android/build/gradle/app-metadata.properties"}}), want: []string{me}},

		{name: "not a zip", apk: []byte("hello, world: this is not a zip file"), err: "end of central directory"},
		{name: "too short", apk: []byte("PK"), err: "end of central directory"},
		{name: "zip comment", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Comment: "x"}), err: "end of central directory"},
		{name: "gap before the end record", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Gap: 8}), err: "central directory"},
		{name: "zip64 locator", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Zip64Locator: true}), err: "ZIP64"},
		{name: "zip64 offset", apk: func() []byte {
			b := slices.Clone(good)
			binary.LittleEndian.PutUint32(b[eocd+16:], 0xffffffff)
			return b
		}(), err: "ZIP64"},
		{name: "central directory offset moved", apk: func() []byte {
			b := slices.Clone(good)
			binary.LittleEndian.PutUint32(b[eocd+16:], binary.LittleEndian.Uint32(b[eocd+16:])-1)
			return b
		}(), err: "central directory"},
		{name: "plain zip", apk: apktest.APK(), err: "no APK Signing Block"},
		{name: "only padding", apk: apktest.APK(apktest.Block{ID: apktest.Padding, Raw: make([]byte, 4)}), err: "no APK Signature Scheme v2 or v3 block"},
		{name: "unknown block", apk: apktest.APK(v2, apktest.Block{ID: 0x12345678, Raw: []byte{1}}), err: "unknown block 0x12345678"},
		{name: "repeated block", apk: apktest.APK(v2, apktest.Block{ID: apktest.V2, Signers: by(other)}), err: "repeated block 0x7109871a"},
		{name: "repeated padding", apk: apktest.APK(v2, apktest.Block{ID: apktest.Padding, Raw: []byte{0}}, apktest.Block{ID: apktest.Padding, Raw: []byte{0}}), err: "repeated block"},
		{name: "v1 signature", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Files: []string{"AndroidManifest.xml", "META-INF/CERT.SF", "META-INF/CERT.RSA"}}), err: "v1 signature"},
		{name: "v1 signature, lower case", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Files: []string{"AndroidManifest.xml", "meta-inf/x.ec"}}), err: "v1 signature"},
		{name: "v1 signature in a subfolder", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Files: []string{"AndroidManifest.xml", "META-INF/a/b.SF"}}), err: "v1 signature"},
		{name: "NUL in a name", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Files: []string{"AndroidManifest.xml", "META-INF/A.SF\x00.txt"}}), err: "bad name"},
		{name: "invalid UTF-8 in a name", apk: apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Files: []string{"AndroidManifest.xml", "a\xff"}}), err: "bad name"},
		{name: "entry count too small", apk: func() []byte {
			b := apktest.Build(apktest.Options{Blocks: []apktest.Block{v2}, Files: []string{"AndroidManifest.xml", "classes.dex"}})
			binary.LittleEndian.PutUint16(b[len(b)-22+8:], 1)
			binary.LittleEndian.PutUint16(b[len(b)-22+10:], 1)
			return b
		}(), err: "more than its entry count"},
		{name: "entry count too big", apk: func() []byte {
			b := slices.Clone(good)
			binary.LittleEndian.PutUint16(b[eocd+8:], 2)
			binary.LittleEndian.PutUint16(b[eocd+10:], 2)
			return b
		}(), err: "bad entry"},
		{name: "entry counts that differ", apk: func() []byte {
			b := slices.Clone(good)
			binary.LittleEndian.PutUint16(b[eocd+8:], 2)
			return b
		}(), err: "entry counts"},
		{name: "multi-disk", apk: func() []byte {
			b := slices.Clone(good)
			binary.LittleEndian.PutUint16(b[eocd+4:], 1)
			return b
		}(), err: "multi-disk"},
		{name: "key rotation", apk: apktest.APK(apktest.Block{ID: apktest.V3, Signers: []apktest.Signer{{Cert: ours, Rotation: true}}}), err: "key rotation"},
		{name: "no signers", apk: apktest.APK(apktest.Block{ID: apktest.V2, Raw: apktest.LP()}), err: "no signers"},
		{name: "empty certificate", apk: apktest.APK(apktest.Block{ID: apktest.V2, Signers: by([]byte{})}), err: "empty certificate"},
		{name: "trailing data", apk: apktest.APK(apktest.Block{ID: apktest.V2, Raw: []byte{0, 0, 0, 0, 0}}), err: "trailing data"},
		{name: "truncated", apk: apktest.APK(apktest.Block{ID: apktest.V2, Raw: []byte{9, 0, 0, 0}}), err: "truncated"},
		{name: "block size past the start", apk: mangle(good, func(b []byte, at, magic int) {
			binary.LittleEndian.PutUint64(b[magic-8:], 1<<40)
		}), err: "bad APK Signing Block size"},
		{name: "block size too small", apk: mangle(good, func(b []byte, at, magic int) {
			binary.LittleEndian.PutUint64(b[magic-8:], 8)
		}), err: "bad APK Signing Block size"},
		{name: "sizes that differ", apk: mangle(good, func(b []byte, at, magic int) {
			binary.LittleEndian.PutUint64(b[at:], 7)
		}), err: "sizes do not match"},
		{name: "pair past the block", apk: mangle(good, func(b []byte, at, magic int) {
			binary.LittleEndian.PutUint64(b[at+8:], 1<<32)
		}), err: "bad ID-value pair"},
		{name: "pair shorter than its ID", apk: mangle(good, func(b []byte, at, magic int) {
			binary.LittleEndian.PutUint64(b[at+8:], 3)
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
	me, dev := apktest.SHA256(ours), apktest.SHA256(other)
	both := apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(ours)}, apktest.Block{ID: apktest.V3, Signers: by(ours)})
	if err := CheckSigner(both, me); err != nil {
		t.Fatal(err)
	}
	if err := CheckSigner(both, strings.ToUpper(me)); err != nil {
		t.Fatalf("upper-case pin: %v", err)
	}
	if err := CheckSigner(apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(other)}), me, dev); err != nil {
		t.Fatalf("second trusted certificate: %v", err)
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
	if err := CheckSigner(both); !errors.Is(err, ErrSigner) {
		t.Errorf("nothing trusted: %v", err)
	}
	if err := CheckSigner(both, ""); !errors.Is(err, ErrSigner) {
		t.Errorf("empty trusted certificate: %v", err)
	}
}

// Signers never panics nor reads out of bounds, whatever the bytes.
func FuzzSigners(f *testing.F) {
	f.Add(apktest.APK(apktest.Block{ID: apktest.V2, Signers: by(ours)}))
	f.Add(apktest.APK(apktest.Block{ID: apktest.V3, Signers: by(ours, other)}, apktest.Block{ID: apktest.Padding, Raw: make([]byte, 8)}))
	f.Add(apktest.APK(apktest.Block{ID: apktest.V31, Signers: []apktest.Signer{{Cert: ours, Rotation: true}}}))
	f.Add(apktest.Build(apktest.Options{Blocks: []apktest.Block{{ID: apktest.V2, Signers: by(ours)}}, Files: []string{"a", "META-INF/A.SF"}}))
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

- [ ] **Step 3: Ejecutarlos y ver que fallan**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/ -run 'TestSigners|TestCheckSigner|FuzzSigners'`
Expected: FAIL de compilación (`undefined: Signers`, `undefined: CheckSigner`, `undefined: ErrSigner`).

- [ ] **Step 4: El parser**

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
	"unicode/utf8"
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

// ErrSigner means the APK is not signed only with trusted certificates.
var ErrSigner = errors.New("the APK is not signed by hybriqtech")

var errTruncated = errors.New("truncated")

const eocdSize = 22

var sigMagic = []byte("APK Sig Block 42")

// Signers returns the lowercase hex SHA-256 of the certificate of every
// signer in every v2, v3 and v3.1 block of apk. It does not verify any
// signature: Android does when it installs, and refuses an APK whose
// signatures do not match its content. What Android does not check is who
// signed, and that is what this tells.
//
// It finds the Signing Block the way Android does, more strictly: whatever
// Android might read differently is an error (spec M6a §5), or a v1
// signature Android falls back to could hide behind a block only the host
// sees.
func Signers(apk []byte) ([]string, error) {
	pairs, names, err := signingBlock(apk)
	if err != nil {
		return nil, err
	}
	if err := noV1(names); err != nil {
		return nil, err
	}
	var out []string
	seen := map[uint32]bool{}
	for _, p := range pairs {
		if seen[p.id] {
			return nil, fmt.Errorf("repeated block %#x in the APK Signing Block", p.id)
		}
		seen[p.id] = true
		switch p.id {
		case blockV2, blockV3, blockV31:
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
	if !seen[blockV2] && !seen[blockV3] && !seen[blockV31] {
		return nil, errors.New("no APK Signature Scheme v2 or v3 block")
	}
	return out, nil
}

// CheckSigner returns nil when every signer that Signers finds in apk has
// one of the trusted certificates (hex SHA-256), and an ErrSigner otherwise.
func CheckSigner(apk []byte, trusted ...string) error {
	got, err := Signers(apk)
	if err != nil {
		return fmt.Errorf("%w (%v)", ErrSigner, err)
	}
	for _, g := range got {
		ok := false
		for _, c := range trusted {
			if c != "" && strings.ToLower(c) == g {
				ok = true
			}
		}
		if !ok {
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
// sits right before the zip's central directory, and the names of the
// zip's entries.
func signingBlock(apk []byte) ([]pair, []string, error) {
	if len(apk) < eocdSize {
		return nil, nil, errors.New("not a zip file (no end of central directory)")
	}
	eocd := len(apk) - eocdSize
	if binary.LittleEndian.Uint32(apk[eocd:]) != 0x06054b50 || binary.LittleEndian.Uint16(apk[eocd+20:]) != 0 {
		return nil, nil, errors.New("no end of central directory at the end of the file (an APK has no zip comment)")
	}
	if eocd >= 20 && binary.LittleEndian.Uint32(apk[eocd-20:]) == 0x07064b50 {
		return nil, nil, errors.New("ZIP64 APKs are not supported")
	}
	if binary.LittleEndian.Uint16(apk[eocd+4:]) != 0 || binary.LittleEndian.Uint16(apk[eocd+6:]) != 0 {
		return nil, nil, errors.New("multi-disk zips are not supported")
	}
	count := binary.LittleEndian.Uint16(apk[eocd+10:])
	if binary.LittleEndian.Uint16(apk[eocd+8:]) != count {
		return nil, nil, errors.New("multi-disk zips are not supported (the entry counts of the end record differ)")
	}
	cdSize := uint64(binary.LittleEndian.Uint32(apk[eocd+12:]))
	cdOffset := uint64(binary.LittleEndian.Uint32(apk[eocd+16:]))
	if cdOffset == 0xffffffff || cdSize == 0xffffffff || count == 0xffff {
		return nil, nil, errors.New("ZIP64 APKs are not supported")
	}
	if cdOffset+cdSize != uint64(eocd) {
		return nil, nil, errors.New("the central directory is not right before its end record")
	}
	names, err := centralNames(apk[cdOffset:eocd], int(count))
	if err != nil {
		return nil, nil, err
	}
	if cdOffset < 32 || !bytes.Equal(apk[cdOffset-16:cdOffset], sigMagic) {
		return nil, nil, errors.New("no APK Signing Block")
	}
	size := binary.LittleEndian.Uint64(apk[cdOffset-24:])
	if size < 24 || size > cdOffset-8 {
		return nil, nil, errors.New("bad APK Signing Block size")
	}
	start := cdOffset - 8 - size
	if binary.LittleEndian.Uint64(apk[start:]) != size {
		return nil, nil, errors.New("the APK Signing Block sizes do not match")
	}
	pairs, err := pairsOf(apk[start+8 : cdOffset-24])
	return pairs, names, err
}

// centralNames walks the central directory cd entry by entry (archive/zip
// accepts counts modulo 65536): exactly count entries that fill it, and no
// name with a NUL or invalid UTF-8, as libziparchive demands.
func centralNames(cd []byte, count int) ([]string, error) {
	var names []string
	for i := 0; i < count; i++ {
		if len(cd) < 46 || binary.LittleEndian.Uint32(cd) != 0x02014b50 {
			return nil, errors.New("bad entry in the central directory")
		}
		n := int(binary.LittleEndian.Uint16(cd[28:]))
		m := int(binary.LittleEndian.Uint16(cd[30:]))
		k := int(binary.LittleEndian.Uint16(cd[32:]))
		if 46+n+m+k > len(cd) {
			return nil, errors.New("bad entry in the central directory")
		}
		name := cd[46 : 46+n]
		if bytes.IndexByte(name, 0) >= 0 || !utf8.Valid(name) {
			return nil, errors.New("bad name in the central directory")
		}
		names = append(names, string(name))
		cd = cd[46+n+m+k:]
	}
	if len(cd) != 0 {
		return nil, errors.New("the central directory holds more than its entry count")
	}
	return names, nil
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

// noV1 fails when an entry is part of a v1 (JAR) signature, at any depth
// under META-INF/: ours never have one (minSdk 30), and Android falls back
// to v1 when it misses the block.
func noV1(names []string) error {
	for _, n := range names {
		name := strings.ToUpper(n)
		if !strings.HasPrefix(name, "META-INF/") {
			continue
		}
		for _, ext := range []string{".SF", ".RSA", ".DSA", ".EC"} {
			if strings.HasSuffix(name, ext) {
				return fmt.Errorf("the APK has a v1 signature (%s)", n)
			}
		}
	}
	return nil
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
			signed = signed[8:]
		}
		attrs, _, err := lp(signed)
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
			if v3 && binary.LittleEndian.Uint32(a) == attrRotation {
				return nil, errors.New("key rotation is not supported")
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

(El directorio central se recorre a mano y no con `archive/zip`, que acepta recuentos módulo 65536 y nombres que libziparchive rechaza. Así se ven las mismas entradas que el verificador v1 de Android.)

- [ ] **Step 5: Ejecutarlos y ver que pasan**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/ -run 'TestSigners|TestCheckSigner|FuzzSigners' -v`
Expected: PASS en todos los subtests.

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/ -run '^$' -fuzz FuzzSigners -fuzztime 60s`
Expected: `PASS`, sin `failing input`. Si el fuzzer encuentra una entrada que falla, arregla el parser y añade esa entrada como caso de `TestSigners`. No se versiona `testdata/fuzz`.

- [ ] **Step 6: gofmt, vet y commit**

Run (desde `host/`): `gofmt -l ./internal/phoneapp && go vet ./internal/phoneapp/...`
Expected: sin salida.

```bash
git add host/internal/phoneapp/apksig.go host/internal/phoneapp/apksig_test.go host/internal/phoneapp/apktest/apktest.go
git commit -m "feat(host): leer quién firma un APK como lo lee Android (v2, v3 y v3.1)

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 2: App de confianza en `phoneapp`, descarga con certificado fijado e `Install`

**Files:**
- Create: `host/internal/phoneapp/trust.go`
- Test: `host/internal/phoneapp/trust_test.go`
- Modify: `host/internal/phoneapp/release.go` (`SigningCertSHA256`, `ErrUnpinned`, `Download`)
- Modify: `host/internal/phoneapp/phoneapp.go` (`ErrOtherSigner`, `Install`)
- Modify: `host/internal/phoneapp/phoneapp_test.go` (`TestInstall`, `TestDownloadChecksTheSHA256`, `TestDownloadHasATimeLimit`, `TestCommandsHaveATimeLimit`, `TestEachCommandHasItsOwnDeadline` y tests nuevos)
- Modify: `host/internal/cli/appsetup.go` (`releaseCert`, `releaseURL` y los casos de `Download` e `Install`)
- Modify: `host/internal/cli/appsetup_test.go` (`release`, `writeAPK` y casos nuevos)
- Modify: `host/internal/cli/setup.go:357` (ayuda de `--apk`)

**Interfaces:**
- Consumes: `Signers`, `CheckSigner` y `ErrSigner`, y `apktest` (Task 1).
- Produces:
  - `const phoneapp.SigningCertSHA256 = ""` y `const phoneapp.MinAppVersionCode = 0`;
  - `phoneapp.ErrUnpinned`, `phoneapp.ErrNoTrustedApp`, `phoneapp.ErrAppTooOld` y `phoneapp.ErrOtherSigner`;
  - `type phoneapp.Trust struct{ Release, DevAPK string }`, `func phoneapp.LoadTrust(release, dataDir string) Trust`, `func phoneapp.TrustDevAPK(dataDir, digest string) (path string, err error)`, `func phoneapp.ForgetDevAPK(dataDir string) (removed bool, err error)` y `func phoneapp.DevTrustPath(dataDir string) string`;
  - `type phoneapp.Verified` (campos no exportados), con `func (Verified) Cert() string`, `func (Verified) Dev() bool` y `func (Verified) still(ctx, a) error` (no exportada, para la Task 3);
  - `type phoneapp.Verifier`, con `func phoneapp.NewVerifier(load func() Trust, minCode int) *Verifier`, `func (*Verifier) Verify(ctx context.Context, a ADB) (Verified, error)` y `func (*Verifier) Same(ctx context.Context, a ADB) error`;
  - `func phoneapp.WritePrivate(p string, data []byte) error` y `const phoneapp.MaxAPK`;
  - `func phoneapp.Download(ctx context.Context, client *http.Client, base, dir, cert string) (string, error)`;
  - en `cli`: `var releaseCert = phoneapp.SigningCertSHA256`, `func releaseURL(app *App) (string, error)`, y en los tests `testCert`, `otherCert`, `testAPK(cert []byte) []byte`, `release`, `releaseSignedBy` y `writeAPKSignedBy`.

- [ ] **Step 1: Los tests de `trust.go` que fallan**

`host/internal/phoneapp/trust_test.go`:

```go
package phoneapp

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"
	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

const testPath = "/data/app/~~AbC1-_==/tech.hybriq.adbuddy-XyZ2+/=/base.apk"

// dumpsysOf is `dumpsys package` for the app with versionCode code, after a
// hidden system copy with a higher one that must not count.
func dumpsysOf(code int) string {
	return fmt.Sprintf("Packages:\n  Package [tech.hybriq.adbuddy] (1a2b3c):\n    versionCode=%d minSdk=30 targetSdk=37\n    versionName=0.2.0\n"+
		"Hidden system packages:\n  Package [tech.hybriq.adbuddy] (4d5e6f):\n    versionCode=999999 minSdk=30 targetSdk=37\n", code)
}

// installed makes f answer like a phone with apk installed at path with
// versionCode code.
func installed(f *fakeADB, path string, apk []byte, code int) {
	if f.answers == nil {
		f.answers = map[string][]string{}
	}
	f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:" + path + "\n"}
	f.answers["shell "+shq.Join("stat", "-c", "%s %i %Y", path)] = []string{strconv.Itoa(len(apk)) + " 4242 1700000000\n"}
	enc := base64.StdEncoding.EncodeToString(apk)
	var wrapped strings.Builder
	for len(enc) > 76 {
		wrapped.WriteString(enc[:76] + "\n")
		enc = enc[76:]
	}
	wrapped.WriteString(enc + "\n")
	f.answers["shell "+shq.Join("base64", path)] = []string{wrapped.String()}
	f.answers["shell dumpsys package tech.hybriq.adbuddy"] = []string{dumpsysOf(code)}
}

func signedBy(cert []byte) []byte {
	return apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: cert}}}, apktest.Block{ID: apktest.V3, Signers: []apktest.Signer{{Cert: cert}}})
}

func digest(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

func TestVerify(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 5)
	v, err := NewVerifier(fixed(Trust{Release: rel}), 5).Verify(context.Background(), f)
	if err != nil || v.Cert() != rel || v.Dev() || v.path != testPath {
		t.Fatalf("release: %+v %v", v, err)
	}
	// A development build is trusted for its exact bytes, not its key.
	dev := signedBy(other)
	f = &fakeADB{}
	installed(f, testPath, dev, 1)
	v, err = NewVerifier(fixed(Trust{Release: rel, DevAPK: digest(dev)}), 0).Verify(context.Background(), f)
	if err != nil || v.Cert() != apktest.SHA256(other) || !v.Dev() {
		t.Fatalf("development: %+v %v", v, err)
	}
}

// Every way the check can go wrong gives no Verified (fail closed).
func TestVerifyFailsClosed(t *testing.T) {
	rel := apktest.SHA256(ours)
	good := signedBy(ours)
	dev := signedBy(other)
	// Same key as the trusted development build, other bytes.
	sameKey := apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: other}}})
	stat := "shell " + shq.Join("stat", "-c", "%s %i %Y", testPath)
	b64 := "shell " + shq.Join("base64", testPath)
	cases := []struct {
		name  string
		setup func(f *fakeADB)
		trust *Trust // nil: Trust{Release: rel}
		min   int
		is    error
	}{
		{name: "someone else's app", setup: func(f *fakeADB) { installed(f, testPath, signedBy(other), 1) }, is: ErrSigner},
		{name: "same key as the development build", trust: &Trust{Release: rel, DevAPK: digest(dev)},
			setup: func(f *fakeADB) { installed(f, testPath, sameKey, 1) }, is: ErrSigner},
		{name: "nothing trusted", trust: &Trust{}, setup: func(f *fakeADB) { installed(f, testPath, good, 1) }, is: ErrUnpinned},
		{name: "too old", min: 2, setup: func(f *fakeADB) { installed(f, testPath, good, 1) }, is: ErrAppTooOld},
		{name: "not installed", setup: func(f *fakeADB) { f.answers = map[string][]string{"shell pm path tech.hybriq.adbuddy": {""}} }, is: ErrNoTrustedApp},
		{name: "two paths", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:" + testPath + "\npackage:/data/app/x/split_a.apk\n"}
		}, is: ErrNoTrustedApp},
		{name: "odd path", setup: func(f *fakeADB) { installed(f, "/sdcard/../data/app/x/base.apk", good, 1) }, is: ErrNoTrustedApp},
		{name: "not under /data/app", setup: func(f *fakeADB) { installed(f, "/mnt/expand/u/app/x/base.apk", good, 1) }, is: ErrNoTrustedApp},
		{name: "reinstalled while read", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:" + testPath + "\n", "package:/data/app/~~N==/tech.hybriq.adbuddy-N==/base.apk\n"}
		}, is: ErrNoTrustedApp},
		{name: "too big", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[stat] = []string{strconv.Itoa(maxAPK+1) + " 1 1\n"}
		}, is: ErrNoTrustedApp},
		{name: "size that does not match", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[stat] = []string{"7 4242 1700000000\n"}
		}, is: ErrNoTrustedApp},
		{name: "base64 longer than the size", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[b64] = []string{base64.StdEncoding.EncodeToString(append(good, make([]byte, 4096)...)) + "\n"}
		}, is: ErrNoTrustedApp},
		{name: "unreadable base64", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[b64] = []string{"base64: permission denied\n"}
		}, is: ErrNoTrustedApp},
		{name: "versionCode only in another package", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers["shell dumpsys package tech.hybriq.adbuddy"] = []string{"Packages:\n  Package [tech.hybriq.adbuddyx] (1):\n    versionCode=5\n"}
		}, is: ErrNoTrustedApp},
		{name: "odd stat", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[stat] = []string{"stat: bad format\n"}
		}, is: ErrNoTrustedApp},
		{name: "updated in place while read", setup: func(f *fakeADB) {
			installed(f, testPath, good, 1)
			f.answers[stat] = []string{strconv.Itoa(len(good)) + " 4242 1700000000\n", strconv.Itoa(len(good)) + " 4343 1700000099\n"}
		}, is: ErrNoTrustedApp},
		{name: "adb fails", setup: func(f *fakeADB) {}, is: ErrNoTrustedApp},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeADB{}
			c.setup(f)
			trust := Trust{Release: rel}
			if c.trust != nil {
				trust = *c.trust
			}
			v, err := NewVerifier(fixed(trust), c.min).Verify(context.Background(), f)
			if !errors.Is(err, c.is) || v != (Verified{}) {
				t.Fatalf("got %+v, %v; want %v", v, err, c.is)
			}
		})
	}
}

// The verifier checks a path once: the same app is not read again, a
// reinstalled one (a new path on Android 11+) is.
func TestVerifierRemembersThePath(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(other), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	for i := 0; i < 3; i++ {
		if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrSigner) {
			t.Fatalf("%d: %v", i, err)
		}
	}
	if n := countCalls(f, "shell "+shq.Join("base64", testPath)); n != 1 {
		t.Fatalf("read the app %d times", n)
	}
	newPath := "/data/app/~~New==/tech.hybriq.adbuddy-New==/base.apk"
	installed(f, newPath, signedBy(ours), 1)
	if v, err := vr.Verify(context.Background(), f); err != nil || v.path != newPath {
		t.Fatalf("after reinstall: %+v %v", v, err)
	}
}

// A failure of adb is not remembered: the next call tries again.
func TestVerifierRetriesAfterAnADBFailure(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	key := "shell " + shq.Join("base64", testPath)
	saved := f.answers[key]
	delete(f.answers, key) // the fake exits 1 without an answer
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("first: %v", err)
	}
	f.answers[key] = saved
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatalf("second: %v", err)
	}
}

func TestStill(t *testing.T) {
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	if err := (Verified{}).still(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("zero Verified: %v", err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("a zero Verified ran %q", f.calls)
	}
	v := Verified{path: testPath, key: strconv.Itoa(len(signedBy(ours))) + " 4242 1700000000", cert: "x"}
	if err := v.still(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:/data/app/~~B==/tech.hybriq.adbuddy-C==/base.apk\n"}
	if err := v.still(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("reinstalled: %v", err)
	}
}

func TestDevTrust(t *testing.T) {
	dir := t.TempDir()
	if got := LoadTrust("r", dir); got != (Trust{Release: "r"}) {
		t.Fatalf("empty: %+v", got)
	}
	d := digest([]byte("apk"))
	p, err := TrustDevAPK(dir, d)
	if err != nil || p != DevTrustPath(dir) {
		t.Fatal(p, err)
	}
	if got := LoadTrust("r", dir); got != (Trust{Release: "r", DevAPK: d}) {
		t.Fatalf("saved: %+v", got)
	}
	if _, err := TrustDevAPK(dir, "not hex"); err == nil {
		t.Fatal("bad digest saved")
	}
	if removed, err := ForgetDevAPK(dir); !removed || err != nil {
		t.Fatalf("forget: %v %v", removed, err)
	}
	if removed, err := ForgetDevAPK(dir); removed || err != nil {
		t.Fatalf("forget again: %v %v", removed, err)
	}
	os.MkdirAll(filepath.Dir(DevTrustPath(dir)), 0o700)
	os.WriteFile(DevTrustPath(dir), []byte("garbage\n"), 0o600)
	if got := LoadTrust("r", dir); got.DevAPK != "" {
		t.Fatalf("garbage trusted: %+v", got)
	}
	if runtime.GOOS != "windows" {
		os.WriteFile(DevTrustPath(dir), []byte(d+"\n"), 0o644)
		os.Chmod(DevTrustPath(dir), 0o644)
		if got := LoadTrust("r", dir); got.DevAPK != "" {
			t.Fatalf("world-readable file trusted: %+v", got)
		}
		os.Remove(DevTrustPath(dir))
		target := filepath.Join(dir, "elsewhere")
		os.WriteFile(target, []byte(d+"\n"), 0o600)
		if err := os.Symlink(target, DevTrustPath(dir)); err == nil {
			if got := LoadTrust("r", dir); got.DevAPK != "" {
				t.Fatalf("link trusted: %+v", got)
			}
		}
	}
}

// fixed is a trust loader that always returns t.
func fixed(t Trust) func() Trust { return func() Trust { return t } }

// Same refuses an app updated in place (same path, other stat), any app
// before a good Verify, and a development build no longer trusted.
func TestSame(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if err := vr.Same(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("before Verify: %v", err)
	}
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	if err := vr.Same(context.Background(), f); err != nil {
		t.Fatalf("same app: %v", err)
	}
	f.answers["shell "+shq.Join("stat", "-c", "%s %i %Y", testPath)] = []string{"99 1 2\n"}
	if err := vr.Same(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("updated in place: %v", err)
	}

	dev := signedBy(other)
	trust := Trust{DevAPK: digest(dev)}
	f = &fakeADB{}
	installed(f, testPath, dev, 1)
	vr = NewVerifier(func() Trust { return trust }, 0)
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	trust.DevAPK = "" // trusted-dev-apk deleted while serve runs
	if err := vr.Same(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("development build no longer trusted: %v", err)
	}
	if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrNoTrustedApp) {
		t.Fatalf("Verify after the trust went: %v", err)
	}
}

// An app updated in place (same path, other stat) is read again, not
// answered from the cache.
func TestVerifierReadsAgainWhenTheAppChangesInPlace(t *testing.T) {
	rel := apktest.SHA256(ours)
	f := &fakeADB{}
	installed(f, testPath, signedBy(ours), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if _, err := vr.Verify(context.Background(), f); err != nil {
		t.Fatal(err)
	}
	installed(f, testPath, signedBy(other), 1) // same path, other size
	if _, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrSigner) {
		t.Fatalf("updated in place: %v", err)
	}
	if n := countCalls(f, "shell "+shq.Join("base64", testPath)); n != 2 {
		t.Fatalf("read the app %d times, want 2", n)
	}
}

// With a release certificate set, deleting trusted-dev-apk stops trusting
// the development build at once (the trust is part of the cache key).
func TestForgettingTheDevelopmentBuildTakesEffect(t *testing.T) {
	dev := signedBy(other)
	trust := Trust{Release: apktest.SHA256(ours), DevAPK: digest(dev)}
	f := &fakeADB{}
	installed(f, testPath, dev, 1)
	vr := NewVerifier(func() Trust { return trust }, 0)
	if v, err := vr.Verify(context.Background(), f); err != nil || !v.Dev() {
		t.Fatalf("trusted: %+v %v", v, err)
	}
	trust.DevAPK = ""
	if v, err := vr.Verify(context.Background(), f); !errors.Is(err, ErrSigner) || v != (Verified{}) {
		t.Fatalf("forgotten: %+v %v", v, err)
	}
}

// gateADB holds back the base64 read until release is closed.
type gateADB struct {
	ADB
	started, release chan struct{}
	once             sync.Once
}

func (g *gateADB) Run(ctx context.Context, dir string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if strings.Contains(strings.Join(args, " "), "base64") {
		g.once.Do(func() { close(g.started) })
		<-g.release
	}
	return g.ADB.Run(ctx, dir, args, stdin, stdout, stderr)
}

// Same answers while a Verify is still reading the APK: the channel asks
// it for every connection and must not queue behind a slow read.
func TestSameDoesNotWaitForVerify(t *testing.T) {
	rel := apktest.SHA256(ours)
	current := &fakeADB{}
	installed(current, testPath, signedBy(ours), 1)
	vr := NewVerifier(fixed(Trust{Release: rel}), 0)
	if _, err := vr.Verify(context.Background(), current); err != nil {
		t.Fatal(err)
	}
	updated := &fakeADB{}
	installed(updated, "/data/app/~~New==/tech.hybriq.adbuddy-New==/base.apk", signedBy(ours), 1)
	g := &gateADB{ADB: updated, started: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	open := func() { releaseOnce.Do(func() { close(g.release) }) }
	defer open()
	done := make(chan error, 1)
	go func() {
		_, err := vr.Verify(context.Background(), g)
		done <- err
	}()
	<-g.started // Verify holds its lock, stuck reading the new app
	if err := within(t, 5*time.Second, func() error { return vr.Same(context.Background(), current) }); err != nil {
		t.Fatalf("Same while Verify reads: %v", err)
	}
	open()
	if err := <-done; err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func countCalls(f *fakeADB, key string) int {
	n := 0
	for _, c := range f.calls {
		if c == key {
			n++
		}
	}
	return n
}
```

- [ ] **Step 2: Ejecutarlos y ver que fallan**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/ -run 'TestVerif|TestStill|TestDevTrust|TestSame|TestForgetting'`
Expected: FAIL de compilación (`undefined: NewVerifier`, `Trust`, `Verified`…).

- [ ] **Step 3: `trust.go`**

`host/internal/phoneapp/trust.go`:

```go
package phoneapp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hybriqtech/adbuddy/host/internal/shq"
)

// MinAppVersionCode is the oldest app this host deals with (spec M6a §6.2):
// raise it when a release fixes a security bug in the app, so that an older
// copy signed by hybriqtech gets nothing either.
const MinAppVersionCode = 0

var (
	// ErrNoTrustedApp means the app on the phone could not be checked or is
	// not one this host trusts: it gets nothing.
	ErrNoTrustedApp = errors.New("the ADBuddy app on the phone is not trusted, so it gets no token or permissions " +
		"(run `adbuddy setup`, or `adbuddy setup --apk <file>` for a development build)")
	// ErrAppTooOld means the app is older than MinAppVersionCode.
	ErrAppTooOld = errors.New("the ADBuddy app on the phone is too old for this host (run `adbuddy setup` to update it)")
)

// pullTimeout bounds reading the installed APK (some MB over Wi-Fi).
var pullTimeout = 2 * time.Minute

// basePathRe is where Android 11+ keeps an installed app: one base.apk in a
// randomised folder under /data/app.
var basePathRe = regexp.MustCompile(`^/data/app/[A-Za-z0-9._~=+/-]+/base\.apk$`)

var versionCodeRe = regexp.MustCompile(`(?m)^\s*versionCode=(\d+)`)

var sha256Re = regexp.MustCompile(`^[0-9a-f]{64}$`)

// statKeyRe is `stat -c '%s %i %Y'`: size, inode and mtime of base.apk.
var statKeyRe = regexp.MustCompile(`^(\d+) \d+ \d+$`)

// Trust is what the host trusts for the app (spec M6a §6.1).
type Trust struct {
	Release string // SHA-256 of the release certificate: SigningCertSHA256, or a test's
	DevAPK  string // SHA-256 of the one development APK `setup --apk` trusted, if any
}

// DevTrustPath is where TrustDevAPK keeps the development APK's SHA-256.
func DevTrustPath(dataDir string) string {
	return filepath.Join(dataDir, "app", "trusted-dev-apk")
}

// LoadTrust returns release and the development APK trusted in dataDir.
// Only a regular file (not a link) holding one SHA-256, and private on
// Unix, counts: anything else trusts no development build.
func LoadTrust(release, dataDir string) Trust {
	t := Trust{Release: release}
	p := DevTrustPath(dataDir)
	before, err := os.Lstat(p)
	if err != nil || !before.Mode().IsRegular() || runtime.GOOS != "windows" && before.Mode().Perm()&0o077 != 0 {
		return t
	}
	f, err := os.Open(p)
	if err != nil {
		return t
	}
	defer f.Close()
	if after, err := f.Stat(); err != nil || !os.SameFile(before, after) {
		return t // swapped for something else between the two looks
	}
	b, err := io.ReadAll(io.LimitReader(f, 128))
	if err != nil {
		return t
	}
	if d := strings.TrimSpace(string(b)); sha256Re.MatchString(d) {
		t.DevAPK = d
	}
	return t
}

// TrustDevAPK saves digest, the SHA-256 of a development APK, as trusted.
func TrustDevAPK(dataDir, digest string) (string, error) {
	if !sha256Re.MatchString(digest) {
		return "", fmt.Errorf("not a SHA-256: %q", digest)
	}
	p := DevTrustPath(dataDir)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return "", err
	}
	return p, WritePrivate(p, []byte(digest+"\n"))
}

// WritePrivate writes data to p (0600) through a new temporary file renamed
// over it, so it never writes through a link someone left at p.
func WritePrivate(p string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(p), ".tmp-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// ForgetDevAPK stops trusting the development APK; removed says whether
// one was trusted.
func ForgetDevAPK(dataDir string) (removed bool, err error) {
	err = os.Remove(DevTrustPath(dataDir))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// Verified is the proof that the app installed on the phone is one this
// host trusts. Only Verifier.Verify makes a non-zero one (its fields are
// not exported); the functions that give the app something (the token,
// consent, permissions) take it and refuse the zero value (spec M6a §6.3).
type Verified struct {
	path   string // the base.apk that was checked
	key    string // its stat (size, inode, mtime): an update in place changes it
	cert   string // its certificate's SHA-256
	digest string // its SHA-256
	dev    bool   // the trusted development APK, not a release
}

// Cert is the SHA-256 of the app's certificate.
func (v Verified) Cert() string { return v.cert }

// Dev reports whether the app is the development build setup --apk trusted.
func (v Verified) Dev() bool { return v.dev }

// still checks, right before acting, that v is not the zero value and that
// the app checked is still the one installed: same path (a reinstall moves
// it to a new folder) and same stat (an update in place would change it).
func (v Verified) still(ctx context.Context, a ADB) error {
	if v.path == "" || v.key == "" || v.cert == "" {
		return fmt.Errorf("%w (not checked)", ErrNoTrustedApp)
	}
	p, err := appPath(ctx, a)
	if err != nil {
		return err
	}
	if p != v.path {
		return fmt.Errorf("%w (the app was reinstalled after it was checked)", ErrNoTrustedApp)
	}
	k, _, err := statKey(ctx, a, p)
	if err != nil {
		return err
	}
	if k != v.key {
		return fmt.Errorf("%w (the app changed after it was checked)", ErrNoTrustedApp)
	}
	return nil
}

// Verifier checks the installed app against the Trust that load returns,
// read again on every call (deleting trusted-dev-apk takes effect at once).
// It remembers the last definite answer for the app's path, stat and trust,
// so serve's reconnections do not read the APK again until something
// changes. Failures of adb are not remembered.
type Verifier struct {
	load    func() Trust
	minCode int

	mu  sync.Mutex
	key string   // path, stat and trust of the remembered answer
	v   Verified // the remembered answer
	err error

	// ok is the last good answer, for Same. Verify holds mu while it reads
	// the APK (up to pullTimeout), and Same, which the channel asks for
	// every connection, must not wait for that: hence an atomic pointer.
	ok atomic.Pointer[Verified]
}

// NewVerifier returns a Verifier for the trust load returns (LoadTrust)
// and minCode (MinAppVersionCode).
func NewVerifier(load func() Trust, minCode int) *Verifier {
	return &Verifier{load: load, minCode: minCode}
}

// Same returns nil when the app installed now is the one the last good
// Verify checked (same path and stat) and its trust still holds. Serve
// asks it for every app that connects with the token (bridge
// Options.Admit): an update with the same key keeps the app's data, the
// token included, but not its path and stat (spec M6a §6.3).
func (vr *Verifier) Same(ctx context.Context, a ADB) error {
	last := vr.ok.Load()
	if last == nil {
		return fmt.Errorf("%w (not checked)", ErrNoTrustedApp)
	}
	if last.dev && vr.load().DevAPK != last.digest {
		return fmt.Errorf("%w (the development build is no longer trusted)", ErrNoTrustedApp)
	}
	return last.still(ctx, a)
}

// Verify reads the installed app (pm path, stat, base64) and checks who
// signed it and its version code (spec M6a §6.2). It is safe without
// verifying any cryptography: Android verified that APK when it installed
// it, and Signers reads the same block Android did. Every failure is an
// ErrNoTrustedApp or an ErrAppTooOld, and gives no Verified.
func (vr *Verifier) Verify(ctx context.Context, a ADB) (Verified, error) {
	path, err := appPath(ctx, a)
	if err != nil {
		return Verified{}, err
	}
	sk, n, err := statKey(ctx, a, path)
	if err != nil {
		return Verified{}, err
	}
	trust := vr.load()
	key := strings.Join([]string{path, sk, trust.Release, trust.DevAPK}, "|")
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if vr.key == key {
		return vr.v, vr.err
	}
	v, err := vr.check(ctx, a, trust, path, sk, n)
	if err == nil || errors.Is(err, ErrSigner) || errors.Is(err, ErrAppTooOld) || errors.Is(err, ErrUnpinned) {
		vr.key, vr.v, vr.err = key, v, err // definite: remembered for this app and trust
	}
	if err == nil {
		vr.ok.Store(&v)
	}
	return v, err
}

func (vr *Verifier) check(ctx context.Context, a ADB, trust Trust, path, sk string, n int64) (Verified, error) {
	if trust.Release == "" && trust.DevAPK == "" {
		return Verified{}, fmt.Errorf("%w: %w", ErrNoTrustedApp, ErrUnpinned)
	}
	data, err := readAPK(ctx, a, path, n)
	if err != nil {
		return Verified{}, fmt.Errorf("%w (reading the app: %w)", ErrNoTrustedApp, err)
	}
	signers, err := Signers(data)
	if err != nil {
		return Verified{}, fmt.Errorf("%w: %w", ErrNoTrustedApp, fmt.Errorf("%w (%v)", ErrSigner, err))
	}
	digest := digestOf(data)
	dev := false
	if err := CheckSigner(data, trust.Release); err != nil {
		if trust.DevAPK == "" || digest != trust.DevAPK {
			return Verified{}, fmt.Errorf("%w: %w", ErrNoTrustedApp, err)
		}
		dev = true
	}
	dump, err := shell(ctx, a, "dumpsys", "package", Package)
	if err != nil {
		return Verified{}, fmt.Errorf("%w (dumpsys: %w)", ErrNoTrustedApp, err)
	}
	code, err := versionCodeOf(dump)
	if err != nil {
		return Verified{}, fmt.Errorf("%w (%v)", ErrNoTrustedApp, err)
	}
	if code < vr.minCode {
		return Verified{}, fmt.Errorf("%w (version code %d, at least %d)", ErrAppTooOld, code, vr.minCode)
	}
	v := Verified{path: path, key: sk, cert: signers[0], digest: digest, dev: dev}
	if err := v.still(ctx, a); err != nil {
		return Verified{}, fmt.Errorf("%w (the app changed while it was checked)", err)
	}
	return v, nil
}

// statKey returns `stat -c '%s %i %Y'` of path (size, inode and mtime) and
// the size, which must be at most maxAPK.
func statKey(ctx context.Context, a ADB, path string) (string, int64, error) {
	out, err := shell(ctx, a, "stat", "-c", "%s %i %Y", path)
	if err != nil {
		return "", 0, fmt.Errorf("%w (stat: %w)", ErrNoTrustedApp, err)
	}
	m := statKeyRe.FindStringSubmatch(out)
	if m == nil {
		return "", 0, fmt.Errorf("%w (unexpected stat %q)", ErrNoTrustedApp, out)
	}
	n, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || n <= 0 || n > maxAPK {
		return "", 0, fmt.Errorf("%w (unexpected size %q)", ErrNoTrustedApp, m[1])
	}
	return out, n, nil
}

// readAPK reads the n bytes of path with base64, its output bounded to
// what n bytes take in base64 with toybox's line breaks.
func readAPK(ctx context.Context, a ADB, path string, n int64) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, pullTimeout)
	defer cancel()
	enc := (n + 2) / 3 * 4
	out := &boundedBuffer{max: int(enc + 2*(enc/76) + 16)} // CR LF after each line at worst
	var errb bytes.Buffer
	code, err := a.Run(ctx, "", []string{"shell", shq.Join("base64", path)}, nil, out, &errb)
	switch {
	case out.over:
		return nil, errors.New("base64 printed more than the app's size")
	case err != nil:
		return nil, err
	case code != 0:
		return nil, fmt.Errorf("base64: exit %d: %s", code, strings.TrimSpace(errb.String()))
	}
	data, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(out.buf.String()), ""))
	if err != nil || int64(len(data)) != n {
		return nil, errors.New("the app read back does not match its size")
	}
	return data, nil
}

// boundedBuffer keeps at most max bytes; past that it refuses the write
// and remembers it.
type boundedBuffer struct {
	buf  bytes.Buffer
	max  int
	over bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.over || b.buf.Len()+len(p) > b.max {
		b.over = true
		return 0, errors.New("output too long")
	}
	return b.buf.Write(p)
}

// versionCodeOf reads the versionCode of the app's own "Package [...]"
// block of `dumpsys package`, not of a hidden system copy.
func versionCodeOf(dump string) (int, error) {
	head := "Package [" + Package + "]"
	i := strings.Index(dump, head)
	if i < 0 {
		return 0, errors.New("no package block in dumpsys")
	}
	block := dump[i+len(head):]
	for _, end := range []string{"Package [", "Hidden system packages:"} {
		if j := strings.Index(block, end); j >= 0 {
			block = block[:j]
		}
	}
	m := versionCodeRe.FindStringSubmatch(block)
	if m == nil {
		return 0, errors.New("no versionCode in dumpsys")
	}
	return strconv.Atoi(m[1])
}

func digestOf(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// appPath returns where the app's base.apk is, or an ErrNoTrustedApp.
func appPath(ctx context.Context, a ADB) (string, error) {
	out, err := shell(ctx, a, "pm", "path", Package)
	if err != nil {
		return "", fmt.Errorf("%w (pm path: %w)", ErrNoTrustedApp, err)
	}
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	if len(lines) != 1 {
		return "", fmt.Errorf("%w (pm path gave %d lines)", ErrNoTrustedApp, len(lines))
	}
	p, ok := strings.CutPrefix(lines[0], "package:")
	if !ok || !basePathRe.MatchString(p) || strings.Contains(p, "..") {
		return "", fmt.Errorf("%w (unexpected path %q)", ErrNoTrustedApp, lines[0])
	}
	return p, nil
}
```

(`TestVerifyFailsClosed` compara con `Verified{}` mediante `!=`, así que `Verified` tiene que seguir siendo comparable: sin slices ni maps.)

- [ ] **Step 4: Ejecutar los tests de `trust.go`**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/ -run 'TestVerif|TestStill|TestDevTrust|TestSame|TestForgetting' -v`
Expected: PASS.

- [ ] **Step 5: `Download`, `Install` y sus tests**

En `host/internal/phoneapp/release.go`, después de `ErrChecksum`:

```go
// SigningCertSHA256 is the SHA-256 (lowercase hex) of the certificate that
// signs the app's releases (spec M6a §6.1). It is empty until the release
// key exists (docs/releasing.md); then Download refuses every APK.
const SigningCertSHA256 = ""

// ErrUnpinned means this build pins no signing certificate, so it downloads
// no APK.
var ErrUnpinned = errors.New("this build of adbuddy has no pinned signing certificate for the app")

// MaxAPK bounds every APK the host reads: downloaded, installed or --apk.
const MaxAPK = maxAPK
```

Cambia el comentario y la cabecera de `Download`, y añade la comprobación tras la del checksum:

```go
// Download fetches the latest APK into dir, checks it against the published
// SHA-256 and checks that every signer has the certificate whose SHA-256 is
// cert (the release one: never the development one), then returns its
// path. base is ReleaseURL (or a test server). Without cert it downloads
// nothing (ErrUnpinned). Without a client or a deadline in ctx it takes at
// most downloadTimeout.
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

En `host/internal/phoneapp/phoneapp.go`, encima de `Install`:

```go
// ErrOtherSigner means the app on the phone was signed with another key (a
// development build, say), so Android refuses to update it.
var ErrOtherSigner = errors.New("the app on the phone was signed with another key")
```

y sustituye `Install`:

```go
// Install installs (or updates, keeping data) the APK at path. Never
// incrementally: adb would keep reading the file on the PC after the host
// checked it.
func Install(ctx context.Context, a ADB, path string) error {
	out, err := runFor(ctx, a, installTimeout, nil, "install", "-r", "--no-incremental", path)
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

En `host/internal/phoneapp/phoneapp_test.go`, añade `"bytes"`, `"sync/atomic"` y `"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"` a los imports, y sustituye `TestInstall` y `TestDownloadChecksTheSHA256` por:

```go
func TestInstall(t *testing.T) {
	key := "install -r --no-incremental /tmp/a.apk"
	f := &fakeADB{answers: map[string][]string{key: {"Performing Streamed Install\nSuccess\n"}}}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err != nil {
		t.Fatal(err)
	}
	f.answers[key] = []string{"Failure [INSTALL_FAILED_OLDER_SDK]\n"}
	if err := Install(context.Background(), f, "/tmp/a.apk"); err == nil || errors.Is(err, ErrOtherSigner) {
		t.Fatalf("older sdk: %v", err)
	}
	f.answers[key] = []string{"Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package tech.hybriq.adbuddy signatures do not match newer version; ignoring!]\n"}
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
	apk := signedBy(ours)
	sidecar := sumOf(apk) + "  adbuddy.apk\n"
	srv, _ := releaseServer(t, apk, func() string { return sidecar })
	dir := t.TempDir()
	p, err := Download(context.Background(), srv.Client(), srv.URL+"/", dir, apktest.SHA256(ours))
	if err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !bytes.Equal(b, apk) || p != filepath.Join(dir, "adbuddy.apk") {
		t.Fatalf("%s %d bytes", p, len(b))
	}
	sidecar = strings.Repeat("0", 64)
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/", t.TempDir(), apktest.SHA256(ours)); !errors.Is(err, ErrChecksum) {
		t.Fatalf("bad sum: %v", err)
	}
	if _, err := Download(context.Background(), srv.Client(), srv.URL+"/none/", t.TempDir(), apktest.SHA256(ours)); !errors.Is(err, ErrNoRelease) {
		t.Fatalf("no release: %v", err)
	}
}

// An APK whose checksum matches but that someone else signed is not saved.
func TestDownloadChecksTheSigner(t *testing.T) {
	apk := signedBy(other)
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

En el mismo fichero:
- en `TestDownloadHasATimeLimit`, la llamada pasa a `Download(context.Background(), client, srv.URL+"/", t.TempDir(), strings.Repeat("a", 64))`;
- en `TestEachCommandHasItsOwnDeadline`, la clave `"install -r /tmp/a.apk"` pasa a `"install -r --no-incremental /tmp/a.apk"`, con los valores del mapa alineados como deja `gofmt -w`:

```go
		"shell pm grant tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS": {""},
		"install -r --no-incremental /tmp/a.apk":                                      {"Success\n"},
```

  (`TestCommandsHaveATimeLimit` no cambia: su adb falso se cuelga con cualquier orden.)

- [ ] **Step 6: Los tests de `phoneapp`**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/...`
Expected: FAIL de compilación en `internal/cli` todavía no (no se compila aquí), y PASS en `phoneapp`.

- [ ] **Step 7: `cli`, la descarga y la instalación**

En `host/internal/cli/appsetup_test.go`, añade el import `"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"` (`encoding/base64` va en la Task 3, que es la que lo usa). Después sustituye `release` y `writeAPK` por:

```go
// Test certificates: tests that reach the app pin testCert (pinTestCert).
var (
	testCert  = apktest.Cert("hybriqtech")
	otherCert = apktest.Cert("someone else")
)

// testAPK is an APK signed (v2 and v3) with cert.
func testAPK(cert []byte) []byte {
	return apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: cert}}}, apktest.Block{ID: apktest.V3, Signers: []apktest.Signer{{Cert: cert}}})
}

// pinTestCert makes setup trust testCert as the release certificate.
func pinTestCert(t *testing.T) { setVar(t, &releaseCert, apktest.SHA256(testCert)) }

// release serves an APK signed with testCert and pins testCert.
func release(t *testing.T, status int, badSum bool) string {
	t.Helper()
	return releaseSignedBy(t, status, badSum, testCert)
}

// releaseSignedBy serves an APK signed with cert (status != 200: an error;
// badSum: a .sha256 that does not match) and pins testCert.
func releaseSignedBy(t *testing.T, status int, badSum bool, cert []byte) string {
	t.Helper()
	pinTestCert(t)
	apk := testAPK(cert)
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

// writeAPK writes an APK signed with testCert, as --apk takes it.
func writeAPK(t *testing.T) string { return writeAPKSignedBy(t, testCert) }

func writeAPKSignedBy(t *testing.T, cert []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "app.apk")
	if err := os.WriteFile(p, testAPK(cert), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
```

En `host/internal/cli/appsetup.go`, sustituye `releaseURL` por:

```go
// releaseCert is the signing certificate a downloaded APK must have, and
// the release one the installed app may have (a variable so that tests can
// pin theirs).
var releaseCert = phoneapp.SigningCertSHA256

// releaseURL is where setup downloads the app: ReleaseURL, or
// ADBUDDY_RELEASE_URL for the e2e tests, which must be https:// or a
// loopback http://. Even another origin can only serve APKs signed with
// releaseCert and not older than MinAppVersionCode (spec M6a §6.4).
func releaseURL(app *App) (string, error) {
	u := app.Getenv("ADBUDDY_RELEASE_URL")
	if u == "" {
		return phoneapp.ReleaseURL, nil
	}
	p, err := url.Parse(u)
	loopback := err == nil && (p.Hostname() == "127.0.0.1" || p.Hostname() == "::1" || p.Hostname() == "localhost")
	if err != nil || !(p.Scheme == "https" && p.Host != "" || p.Scheme == "http" && loopback) {
		return "", fmt.Errorf("ADBUDDY_RELEASE_URL must be https:// or http:// to 127.0.0.1, ::1 or localhost, not %q", u)
	}
	if !strings.HasSuffix(u, "/") {
		u += "/"
	}
	return u, nil
}
```

(Añade `"net/url"` a los imports.) La descarga en `setupApp` queda así:

```go
		if apk == "" {
			base, err := releaseURL(app)
			if err != nil {
				res.note(err.Error())
				warn("⚠ %v. Setup goes on without the app; 🔴 confirmations will happen in the terminal.\n", err)
				return res
			}
			if releaseCert != "" {
				say("Getting the ADBuddy app…\n")
			}
			apk, err = phoneapp.Download(ctx, nil, base, filepath.Join(e.dirs.Data, "app"), releaseCert)
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
		} else if _, err := os.Stat(apk); err != nil {
```

(el resto de esa rama queda como está). El error de `Install`:

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
	cmd.Flags().StringVar(&apkFlag, "apk", "", "install this ADBuddy app APK instead of the latest release (a file you choose: if hybriqtech's release certificate did not sign it, exactly this APK becomes the trusted development build)")
```

En `appsetup_test.go`, sustituye `TestReleaseURLEndsWithASlash` (que usa `releaseURL` con un solo resultado) por:

```go
func TestReleaseURL(t *testing.T) {
	get := func(in string) (string, error) {
		return releaseURL(&App{Getenv: func(k string) string {
			if k == "ADBUDDY_RELEASE_URL" {
				return in
			}
			return ""
		}})
	}
	for in, want := range map[string]string{
		"":                        phoneapp.ReleaseURL,
		"http://127.0.0.1:9/x":    "http://127.0.0.1:9/x/",
		"http://127.0.0.1:9/x/":   "http://127.0.0.1:9/x/",
		"http://[::1]:9/x":        "http://[::1]:9/x/",
		"http://localhost:9/x":    "http://localhost:9/x/",
		"https://example.com/dl":  "https://example.com/dl/",
		"https://example.com/dl/": "https://example.com/dl/",
	} {
		if got, err := get(in); err != nil || got != want {
			t.Errorf("%q: %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"example.com",                // no scheme
		"http://example.com/",        // plain http to another host
		"file:///tmp/x",              // not a web URL
		"ftp://127.0.0.1/",           // loopback, but not http
		"http://127.0.0.1.evil.com/", // starts like loopback
		"http://127.0.0.1@evil.com/", // the host is evil.com
		"https://",                   // no host
		"https:///x",                 // no host
	} {
		if got, err := get(in); err == nil {
			t.Errorf("%q accepted as %q", in, got)
		}
	}
}
```

En la tabla de `TestSetupAppBranches` (`appsetup_test.go`), justo después del caso `"bad checksum"`, añade:

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
		{name: "release url not allowed", consent: "granted", over: []testutil.FakeRule{notInstalled(0)},
			args: func(t *testing.T) []string {
				pinTestCert(t)
				t.Setenv("ADBUDDY_RELEASE_URL", "http://example.com/")
				return nil
			},
			want:   want{consent: "skipped"},
			stderr: []string{"ADBUDDY_RELEASE_URL must be https://"}, noChange: true},
		{name: "development build on the phone", consent: "granted",
			over: []testutil.FakeRule{notInstalled(0), {Match: `^install -r `, Exit: 1,
				Stderr: "adb: failed to install app.apk: Failure [INSTALL_FAILED_UPDATE_INCOMPATIBLE: Existing package tech.hybriq.adbuddy signatures do not match newer version; ignoring!]\n"}},
			args:   func(t *testing.T) []string { return []string{"--apk", writeAPK(t)} },
			want:   want{consent: "skipped"},
			stderr: []string{"signed with another key", "Uninstall it on the phone", "adbuddy setup"}, noStderr: []string{"Could not install the app"}, noChange: true},
```

- [ ] **Step 8: Todos los tests del host**

Run (desde `host/`): `go test -timeout 10m ./...`
Expected: PASS. Si falla un test que compara la ayuda de `setup`, actualízalo al texto nuevo.

- [ ] **Step 9: gofmt, vet y commit**

Run (desde `host/`): `gofmt -l . && go vet ./...`
Expected: sin salida.

```bash
git add host/internal/phoneapp host/internal/cli/appsetup.go host/internal/cli/appsetup_test.go host/internal/cli/setup.go
git commit -m "feat(host): comprobar el firmante de la app instalada y del APK descargado

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 3: Setup solo pide consentimiento y concede permisos a la app verificada

**Files:**
- Modify: `host/internal/phoneapp/phoneapp.go` (`AskConsent`, `Grant`, `AllowBackground` y `EnableAccessibility` piden `Verified`)
- Modify: `host/internal/phoneapp/phoneapp_test.go` (sus llamadas)
- Modify: `host/internal/cli/appsetup.go` (`chooseAPK`, `Verify`, `ForgetDevAPK` y el resto del flujo)
- Modify: `host/internal/cli/appsetup_test.go` (`appRules` y casos)

**Interfaces:**
- Consumes: `Verified`, `Verifier`, `LoadTrust`, `TrustDevAPK`, `ForgetDevAPK`, `ErrNoTrustedApp`, `ErrAppTooOld`, `ErrUnpinned` y `Signers` (Tasks 1-2); `testCert`, `otherCert`, `testAPK`, `pinTestCert` y `writeAPKSignedBy` (Task 2).
- Produces:
  - `func phoneapp.AskConsent(ctx context.Context, a ADB, v Verified, hostName string, timeout, every time.Duration) error`;
  - `func phoneapp.Grant(ctx context.Context, a ADB, v Verified) error`;
  - `func phoneapp.AllowBackground(ctx context.Context, a ADB, v Verified) error`;
  - `func phoneapp.EnableAccessibility(ctx context.Context, a ADB, v Verified) (previous string, err error)`;
  - en `cli`: `func chooseAPK(e *env, apk string) (copyPath, devDigest string, err error)` y `func forgetDevBuild(e *env) (bool, error)`, y en los tests `testAppPath`, `appRulesFor(consent string, apk []byte, over ...testutil.FakeRule)`, `appReadRules(apk []byte) []testutil.FakeRule` (las reglas de lectura de la app, que usa también la Task 5) y un `init` que fija `releaseCert` a `apktest.SHA256(testCert)`.

- [ ] **Step 1: Las funciones de `phoneapp` piden `Verified`**

En `host/internal/phoneapp/phoneapp.go`. `AskConsent`: cambia el comentario, la firma y el arranque (el resto del cuerpo, desde `if _, err := Call(ctx, a, "requestSetup", …`, queda igual):

```go
// AskConsent opens the consent screen (spec §3.1 step 6.1) and waits for the
// answer. It takes at most timeout + consentGrace, even if adb hangs. v is
// the proof that the app is ours (Verifier.Verify): nothing is asked of an
// app that is not the one checked.
func AskConsent(ctx context.Context, a ADB, v Verified, hostName string, timeout, every time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout+consentGrace)
	defer cancel()
	if err := v.still(ctx, a); err != nil {
		return err
	}
```

Sustituye `Grant`, `AllowBackground` y `EnableAccessibility` enteras (la lógica es la de antes; solo cambian el comentario, la firma y la primera comprobación):

```go
// Grant gives the app WRITE_SECURE_SETTINGS (for "Cut access"), once v
// still holds.
func Grant(ctx context.Context, a ADB, v Verified) error {
	if err := v.still(ctx, a); err != nil {
		return err
	}
	_, err := shell(ctx, a, "pm", "grant", Package, SecureWrites)
	return err
}

// AllowBackground exempts the app from battery optimisation, so it may start
// its foreground service when the host hands over the token and keep the
// channel open while the screen is off (Android 12+ restricts both), once v
// still holds.
func AllowBackground(ctx context.Context, a ADB, v Verified) error {
	if err := v.still(ctx, a); err != nil {
		return err
	}
	out, err := shell(ctx, a, "dumpsys", "deviceidle", "whitelist", "+"+Package)
	if err != nil {
		return err
	}
	if !strings.Contains(out, "Added:") {
		return fmt.Errorf("dumpsys deviceidle: %s", out)
	}
	return nil
}

// EnableAccessibility adds the app's service to the enabled ones without
// removing any (spec §3.1 step 6.2) and turns accessibility on, once v still
// holds. It returns the previous list so the caller can report it.
func EnableAccessibility(ctx context.Context, a ADB, v Verified) (previous string, err error) {
	if err := v.still(ctx, a); err != nil {
		return "", err
	}
	previous, err = shell(ctx, a, "settings", "get", "secure", "enabled_accessibility_services")
	if err != nil {
		return "", err
	}
	list := MergeServices(previous, A11yService)
	if list != previous {
		if _, err := shell(ctx, a, "settings", "put", "secure", "enabled_accessibility_services", list); err != nil {
			return previous, err
		}
	}
	_, err = shell(ctx, a, "settings", "put", "secure", "accessibility_enabled", "1")
	return previous, err
}
```

Comprueba que ninguna otra función del host pide algo a la app sin un `Verified`: `git grep -n -e requestSetup -e '"am", "start"' -- host/internal ':!*_test.go'` solo debe dar `phoneapp.go`, dentro de `AskConsent` (que llama a `still` antes). `Call` y `Handshake` no cuentan: `Handshake` se arregla en la Task 5.

En `host/internal/phoneapp/phoneapp_test.go`, añade `"github.com/hybriqtech/adbuddy/host/internal/shq"` a los imports y este helper:

```go
// verified makes f answer pm path and stat like a phone with the app at
// testPath, and returns the proof the functions that give the app something
// take (the same app, as Verified.still sees it).
func verified(f *fakeADB) Verified {
	if f.answers == nil {
		f.answers = map[string][]string{}
	}
	f.answers["shell pm path tech.hybriq.adbuddy"] = []string{"package:" + testPath + "\n"}
	f.answers["shell "+shq.Join("stat", "-c", "%s %i %Y", testPath)] = []string{"100 1 1\n"}
	return Verified{path: testPath, key: "100 1 1", cert: "test"}
}
```

En cada llamada a `AskConsent`, `Grant`, `AllowBackground` y `EnableAccessibility` del fichero, pasa `verified(f)` justo después de `f`. Por ejemplo, `AskConsent(context.Background(), f, "My PC", …)` pasa a `AskConsent(context.Background(), f, verified(f), "My PC", …)`. Si `f` se crea en la misma sentencia, `verified(f)` va en la llamada, después de crearlo. Cada una de esas funciones hace ahora dos llamadas más al principio (`shell pm path …` y `shell stat -c …`), así que cambian estos tests:

- `TestEachCommandHasItsOwnDeadline`: `f.left[0]` y `f.left[1]` son el `pm path` y el `stat` de `Grant`, así que `Grant` pasa a ser `f.left[2]` y `Install` `f.left[3]`:

```go
	if l := f.left[2]; l <= 14*time.Second || l > 15*time.Second {
		t.Errorf("Grant: %v left", l)
	}
	if l := f.left[3]; l <= 2*time.Minute || l > 3*time.Minute {
		t.Errorf("Install: %v left", l)
	}
```

- `TestGrant`: sustitúyelo por

```go
func TestGrant(t *testing.T) {
	f := &fakeADB{answers: map[string][]string{"shell pm grant tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS": {""}}}
	if err := Grant(context.Background(), f, verified(f)); err != nil {
		t.Fatal(err, f.calls)
	}
	if len(f.calls) != 3 || !strings.HasPrefix(f.calls[2], "shell pm grant ") {
		t.Fatalf("calls %v", f.calls)
	}
	f = &fakeADB{answers: map[string][]string{}}
	if err := Grant(context.Background(), f, verified(f)); err == nil {
		t.Fatal("a failed grant was accepted")
	}
}
```

- `TestAskConsentHasATimeLimit`: con `hangAll`, `still` se cuelga hasta el límite de su orden y el test sigue viendo el error de tiempo que espera; no cambia nada más.

Añade un test:

```go
// Nothing is given to an app that is not the one checked.
func TestGivingNeedsTheCheckedApp(t *testing.T) {
	f := &fakeADB{}
	v := verified(f)
	pmPath := "shell pm path tech.hybriq.adbuddy"
	stat := "shell " + shq.Join("stat", "-c", "%s %i %Y", testPath)
	check := func(label string, v Verified) {
		t.Helper()
		if err := Grant(context.Background(), f, v); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: Grant: %v", label, err)
		}
		if err := AllowBackground(context.Background(), f, v); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: AllowBackground: %v", label, err)
		}
		if _, err := EnableAccessibility(context.Background(), f, v); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: EnableAccessibility: %v", label, err)
		}
		if err := AskConsent(context.Background(), f, v, "My PC", time.Second, time.Millisecond); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: AskConsent: %v", label, err)
		}
	}
	check("zero Verified", Verified{})
	if len(f.calls) != 0 {
		t.Fatalf("a zero Verified ran %q", f.calls)
	}
	f.answers[pmPath] = []string{"package:/data/app/~~New==/tech.hybriq.adbuddy-New==/base.apk\n"}
	check("reinstalled", v)
	f.answers[pmPath] = []string{"package:" + testPath + "\n"}
	f.answers[stat] = []string{"999 1 1\n"}
	check("updated in place", v)
	for _, c := range f.calls {
		if c != pmPath && c != stat {
			t.Fatalf("ran %q", c)
		}
	}
}
```

- [ ] **Step 2: Ejecutar `phoneapp` (y ver que `cli` aún no compila)**

Run (desde `host/`): `go test -timeout 5m ./internal/phoneapp/... && go vet ./internal/cli/`
Expected: PASS en `phoneapp`. `go vet` falla en `internal/cli` con `not enough arguments in call to phoneapp.AskConsent` (y Grant…).

- [ ] **Step 3: El flujo de setup**

En `host/internal/cli/appsetup.go`, añade `"crypto/sha256"`, `"encoding/hex"` e `"io"` a los imports y estas dos funciones, antes de `setupApp`:

```go
// chooseAPK reads the --apk file once (at most MaxAPK: it could be a FIFO)
// and returns a private copy to install, so nobody swaps it between the
// check and adb install. When the release certificate does not sign it, it
// also returns its SHA-256: that exact APK (not its key: the debug key
// lives on this PC) is trusted as the development build once it installs
// (spec M6a §6.1).
func chooseAPK(e *env, apk string) (copyPath, devDigest string, err error) {
	f, err := os.Open(apk)
	if err != nil {
		return "", "", err
	}
	data, err := io.ReadAll(io.LimitReader(f, phoneapp.MaxAPK+1))
	f.Close()
	if err != nil {
		return "", "", err
	}
	if len(data) > phoneapp.MaxAPK {
		return "", "", fmt.Errorf("%s is larger than %d MiB", apk, phoneapp.MaxAPK>>20)
	}
	if _, err := phoneapp.Signers(data); err != nil {
		return "", "", fmt.Errorf("%s is not a signed APK (%v)", apk, err)
	}
	dir := filepath.Join(e.dirs.Data, "app")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", "", err
	}
	copyPath = filepath.Join(dir, "chosen.apk")
	if err := phoneapp.WritePrivate(copyPath, data); err != nil {
		return "", "", err
	}
	if releaseCert != "" && phoneapp.CheckSigner(data, releaseCert) == nil {
		return copyPath, "", nil
	}
	sum := sha256.Sum256(data)
	return copyPath, hex.EncodeToString(sum[:]), nil
}

// forgetDevBuild stops trusting the development build and removes its copy.
func forgetDevBuild(e *env) (bool, error) {
	os.Remove(filepath.Join(e.dirs.Data, "app", "chosen.apk"))
	return phoneapp.ForgetDevAPK(e.dirs.Data)
}
```

En `setupApp`, justo después de `res := appResult{Consent: "skipped"}`, añade:

```go
	devDigest, devFile := "", "" // the --apk that becomes the trusted development build, and its path
```

La rama de `--apk` (la que empieza en `} else if _, err := os.Stat(apk); err != nil {`) queda así. La copia privada sustituye a `apk`, y la ruta original se guarda para el aviso:

```go
		} else if _, err := os.Stat(apk); err != nil {
			res.note(err.Error())
			warn("⚠ %v: skipping the app. Check the path of --apk, then "+again+"\n", err)
			return res
		} else {
			chosen, digest, err := chooseAPK(e, apk)
			if err != nil {
				res.note(err.Error())
				warn("⚠ %v: skipping the app. Pass a signed APK to --apk, then "+again+"\n", err)
				return res
			}
			devFile, devDigest, apk = apk, digest, chosen
		}
```

Justo después del bloque `if err := phoneapp.Install(ctx, a, apk); err != nil { … }` y antes de `if version, err = phoneapp.InstalledVersion(ctx, a); …`, añade (la confianza se escribe solo cuando la instalación termina bien: un `--apk` que no se instala no deja nada de confianza):

```go
		if devDigest != "" {
			path, err := phoneapp.TrustDevAPK(e.dirs.Data, devDigest)
			if err != nil {
				res.note(err.Error())
				warn("⚠ Could not trust the development build (%v): setup goes on without the app.\n", err)
				return res
			}
			warn("⚠ Trusting this development build of the app (SHA-256 %s of %s): adbuddy serve will hand the phone app's token to exactly this APK. Delete %s to stop.\n", devDigest, devFile, path)
		}
```

Justo después de `res.Installed, res.Version = true, version`, inserta:

```go
	load := func() phoneapp.Trust { return phoneapp.LoadTrust(releaseCert, e.dirs.Data) }
	v, err := phoneapp.NewVerifier(load, phoneapp.MinAppVersionCode).Verify(ctx, a)
	if err != nil {
		res.note(err.Error())
		switch {
		case errors.Is(err, phoneapp.ErrAppTooOld):
			warn("⚠ The ADBuddy app on the phone is too old for this host: run `adbuddy setup` again to update it. 🔴 confirmations will happen in the terminal.\n")
		case errors.Is(err, phoneapp.ErrUnpinned):
			warn("This build of adbuddy trusts no signing certificate for the app yet, so it skips it: pass --apk <file> to install and trust a development build. 🔴 confirmations will happen in the terminal.\n")
		case errors.Is(err, phoneapp.ErrSigner):
			warn("⚠ The ADBuddy app on the phone is not signed by hybriqtech (a development build?), so ADBuddy will not trust it. Uninstall it on the phone and run `adbuddy setup` again, or pass --apk <that APK> to trust a development build. 🔴 confirmations will happen in the terminal.\n")
		default:
			warn("⚠ Could not check the ADBuddy app on the phone (%v), so ADBuddy will not trust it. "+again+" 🔴 confirmations will happen in the terminal.\n", err)
		}
		return res
	}
	if v.Dev() {
		say("Note: the ADBuddy app on the phone is the development build you trusted (certificate %s).\n", v.Cert())
	} else if removed, err := forgetDevBuild(e); err != nil {
		warn("⚠ Could not stop trusting the development build (%v): delete %s by hand.\n", err, phoneapp.DevTrustPath(e.dirs.Data))
	} else if removed {
		say("The app on the phone is a release: ADBuddy no longer trusts the development build.\n")
	}
```

y pasa `v` a las cuatro funciones: `phoneapp.AskConsent(ctx, a, v, phoneapp.HostName(host), consentWait, time.Second)`, `phoneapp.Grant(ctx, a, v)`, `phoneapp.AllowBackground(ctx, a, v)` y `phoneapp.EnableAccessibility(ctx, a, v)`.

- [ ] **Step 4: Los tests de setup**

En `host/internal/cli/appsetup_test.go`, añade los imports `"encoding/base64"` y `"strconv"`. Sustituye `appRules` por estas dos funciones y la constante. Las reglas de `appRulesFor` son las que `appRules` ya tenía, en el mismo orden; las de lectura de la app (`dumpsys package`, que gana `versionCode`, y las nuevas `pm path`, `stat` y `base64`) pasan a `appReadRules`, que también usa la Task 5:

```go
const testAppPath = "/data/app/~~Ab==/tech.hybriq.adbuddy-Cd==/base.apk"

// appRules answers like a phone whose app (signed with testCert) says
// consent.
func appRules(consent string, over ...testutil.FakeRule) []testutil.FakeRule {
	return appRulesFor(consent, testAPK(testCert), over...)
}

// appRulesFor answers like a phone with apk installed at testAppPath, whose
// app says consent. The first rule that matches answers, so a test puts its
// changes in front (over).
func appRulesFor(consent string, apk []byte, over ...testutil.FakeRule) []testutil.FakeRule {
	provider := `^shell content call --uri content://tech\.hybriq\.adbuddy\.bridge --method `
	rules := append([]testutil.FakeRule{}, over...)
	rules = append(rules, appReadRules(apk)...)
	return append(append(rules, []testutil.FakeRule{
		{Match: `^install -r `, Stdout: "Performing Streamed Install\nSuccess\n"},
		{Match: provider + `requestSetup --extra `, Stdout: "Result: Bundle[{ok=true}]\n"},
		{Match: `^shell am start -n tech\.hybriq\.adbuddy/\.MainActivity -a tech\.hybriq\.adbuddy\.action\.SETUP$`, Stdout: "Starting: Intent\n"},
		{Match: provider + `getSetupConsent$`, Stdout: "Result: Bundle[{consent=" + consent + "}]\n"},
		{Match: `^shell pm grant tech\.hybriq\.adbuddy android\.permission\.WRITE_SECURE_SETTINGS$`},
		{Match: `^shell dumpsys deviceidle whitelist \+tech\.hybriq\.adbuddy$`, Stdout: "Added: tech.hybriq.adbuddy\n"},
		{Match: `^shell settings get secure enabled_accessibility_services$`, Stdout: "com.x/.Svc\n"},
		{Match: `^shell settings put secure `},
	}...), phoneRules()...)
}

// appReadRules answers what the host reads of the installed app: its
// versionCode and versionName, and its path, stat and bytes (pm path, stat
// and base64).
func appReadRules(apk []byte) []testutil.FakeRule {
	return []testutil.FakeRule{
		{Match: `^shell dumpsys package tech\.hybriq\.adbuddy$`, Stdout: "Packages:\n  Package [tech.hybriq.adbuddy] (1a2b):\n    versionCode=1 minSdk=30 targetSdk=37\n    versionName=0.2.0\n"},
		{Match: `^shell pm path tech\.hybriq\.adbuddy$`, Stdout: "package:" + testAppPath + "\n"},
		{Match: `^shell stat -c `, Stdout: strconv.Itoa(len(apk)) + " 1 1\n"},
		{Match: `^shell base64 `, Stdout: base64.StdEncoding.EncodeToString(apk) + "\n"},
	}
}

// The tests that reach the app pin testCert as the release certificate.
func init() { releaseCert = apktest.SHA256(testCert) }
```

(Con `init`, `TestSetupSetsUpTheApp` y el caso de Ctrl+C durante el consentimiento de `setup_test.go` siguen viendo una app de confianza sin tocar `setup_test.go`. `pinTestCert`, de la Task 2, queda para los tests que cambian `releaseCert` y lo restauran con `setVar`.)

En `TestSetupAppBranches`:

- añade a la estructura del caso el campo `apk []byte // the app on the phone (nil: signed with testCert)`;
- el bucle usa ese APK para las reglas. Sustituye `e := newSetupEnv(t, appRules(c.consent, c.over...))` por:

```go
			apk := c.apk
			if apk == nil {
				apk = testAPK(testCert)
			}
			e := newSetupEnv(t, appRulesFor(c.consent, apk, c.over...))
```

- añade estos casos a la tabla, justo antes del caso `"install fails"` (y después de los de la Task 2):

```go
		{name: "installed app signed by someone else", consent: "granted", apk: testAPK(otherCert),
			want:   want{installed: true, version: "0.2.0", consent: "skipped"},
			stderr: []string{"not signed by hybriqtech", "--apk <that APK>"}, noChange: true},
		{name: "nothing trusted", consent: "granted",
			args:   func(t *testing.T) []string { setVar(t, &releaseCert, ""); return nil },
			want:   want{installed: true, version: "0.2.0", consent: "skipped"},
			stderr: []string{"trusts no signing certificate"}, noChange: true},
		{name: "apk of a development build", consent: "granted", apk: testAPK(otherCert), over: []testutil.FakeRule{notInstalled(1)},
			args:   func(t *testing.T) []string { return []string{"--apk", writeAPKSignedBy(t, otherCert)} },
			want:   want{true, true, true, true, "0.2.0", "granted"},
			stderr: []string{"Trusting this development build of the app (SHA-256 ", " of ", "development build you trusted"}, calls: []string{"install -r --no-incremental "}},
		{name: "a release forgets the development build", consent: "granted",
			args: func(t *testing.T) []string {
				if _, err := phoneapp.TrustDevAPK(os.Getenv("ADBUDDY_DATA_DIR"), strings.Repeat("a", 64)); err != nil {
					t.Fatal(err)
				}
				return nil
			},
			want:   want{true, true, true, true, "0.2.0", "granted"},
			stderr: []string{"no longer trusts the development build"}},
		{name: "apk that is not signed", consent: "granted",
			args: func(t *testing.T) []string {
				p := filepath.Join(t.TempDir(), "x.apk")
				os.WriteFile(p, []byte("not an apk"), 0o600)
				return []string{"--apk", p}
			},
			want:   want{consent: "skipped"},
			stderr: []string{"not a signed APK", "skipping the app"}, noChange: true},
```

(El caso `"apk that is not signed"` no reinstala ni concede nada, y como setup se detiene antes de contar la app, `Installed` es `false`. El caso de la app de desarrollo deja `trusted-dev-apk` en el directorio de datos del test, que es un `t.TempDir()`, así que no afecta a los demás casos.)

`noChange` comprueba con `assertUntouched` que no hay `pm grant`, `settings put` ni `deviceidle`; `pm path`, `stat`, `base64` y `dumpsys` son lecturas y no hay que tocarlo.

- [ ] **Step 5: Todos los tests del host**

Run (desde `host/`): `go test -timeout 10m ./...`
Expected: PASS. `setup_test.go` no se toca (no es una ruta de M6a): el `init` de `appsetup_test.go` ya fija el certificado de release de los tests. Si aun así algún otro test que instala la app la ve sin confianza, apúntalo en el ledger y pregunta al controlador.

- [ ] **Step 6: gofmt, vet y commit**

Run (desde `host/`): `gofmt -l . && go vet ./...`
Expected: sin salida.

```bash
git add host/internal/phoneapp/phoneapp.go host/internal/phoneapp/phoneapp_test.go host/internal/cli/appsetup.go host/internal/cli/appsetup_test.go
git commit -m "feat(host): setup solo pide consentimiento y concede permisos a la app verificada

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 4: Gancho `Admit` en el servidor del canal

**Files:**
- Modify: `host/internal/bridge/server.go` (`Options.Admit`, `admitTimeout` y `handle`). **Commit propio**, rebasado sobre `main` con m3-bridge dentro (f36e74d o posterior).
- Test: `host/internal/bridge/admit_test.go`

**Interfaces:**
- Consumes: nada de las tareas anteriores.
- Produces: `bridge.Options.Admit func(ctx context.Context, h Hello) bool`, que se llama en la goroutine de cada conexión tras validar el token y antes de `attach`. `false`, un `ctx` cancelado o `admitTimeout` (20 s) rechazan la conexión con `StatusPolicyViolation` "unauthorized", sin `OnConnect` ni `OnAttach`. Con `nil` se admite a todos.

- [ ] **Step 0: Rebasar**

Run: `git fetch origin && git rebase origin/main`. Expected: sin conflictos (en `main` solo cambió `server.go`, por `SendTo`). Comprueba que `git log origin/main --oneline | grep -c m3-bridge` o el commit f36e74d están en la historia.

- [ ] **Step 1: El test que falla**

`host/internal/bridge/admit_test.go`:

```go
package bridge

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

// Admit is asked after the token and before anything is announced: a
// refused app is closed like one without the token, and neither OnConnect
// nor OnAttach hear of it.
func TestAdmitDecidesAfterTheToken(t *testing.T) {
	var asked, attached atomic.Int32
	var admit atomic.Bool
	var conns calls
	h := start(t, Options{
		Admit:     func(ctx context.Context, hl Hello) bool { asked.Add(1); return admit.Load() },
		OnConnect: conns.add,
		OnAttach:  func() { attached.Add(1) },
	})
	if st := closeStatus(t, h.dial("nope", 0)); st != websocket.StatusPolicyViolation {
		t.Fatalf("wrong token: %v", st)
	}
	if n := asked.Load(); n != 0 {
		t.Fatalf("Admit asked %d times before the token", n)
	}
	if st := closeStatus(t, h.dial(token, 0)); st != websocket.StatusPolicyViolation {
		t.Fatalf("refused: %v", st)
	}
	time.Sleep(50 * time.Millisecond)
	if asked.Load() != 1 || h.srv.Connected() || len(conns.get()) != 0 || attached.Load() != 0 {
		t.Fatalf("refused app: asked %d, connected %v, OnConnect %v, OnAttach %d", asked.Load(), h.srv.Connected(), conns.get(), attached.Load())
	}
	admit.Store(true)
	c := h.dial(token, 0)
	defer c.CloseNow()
	if m := recv(t, c); m.Type != TypeWelcome {
		t.Fatalf("admitted: got %q, want a welcome", m.Type)
	}
	conns.waitFor(t, []bool{true}, 5*time.Second)
	if attached.Load() != 1 {
		t.Fatalf("OnAttach %d times", attached.Load())
	}
}

// An Admit that does not answer in time rejects the app (fail closed).
func TestAdmitThatHangsRejects(t *testing.T) {
	old := admitTimeout
	admitTimeout = 50 * time.Millisecond
	t.Cleanup(func() { admitTimeout = old })
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	h := start(t, Options{Admit: func(ctx context.Context, hl Hello) bool { <-release; return true }})
	if st := closeStatus(t, h.dial(token, 0)); st != websocket.StatusPolicyViolation {
		t.Fatalf("hung Admit: %v", st)
	}
	if h.srv.Connected() {
		t.Fatal("an app counts as connected while Admit hangs")
	}
}

// Without Admit every app with the token gets in, as before.
func TestNoAdmitAdmitsEveryone(t *testing.T) {
	h := start(t, Options{})
	c := h.dial(token, 0)
	defer c.CloseNow()
	if m := recv(t, c); m.Type != TypeWelcome {
		t.Fatalf("got %q, want a welcome", m.Type)
	}
}
```

(Si `closeStatus`, `recv`, `calls`, `start` o `token` se llaman de otra forma en `bridge_test.go` tras m3-bridge, ajusta los nombres. No cambies `bridge_test.go`.)

- [ ] **Step 2: Ejecutarlo y ver que falla**

Run (desde `host/`): `go test -timeout 5m ./internal/bridge/ -run Admit`
Expected: FAIL de compilación (`unknown field Admit`, `undefined: admitTimeout`).

- [ ] **Step 3: El gancho**

En `host/internal/bridge/server.go`, en `Options`, después de `OnReject`:

```go
	// Admit, when set, decides whether an app that has the token may
	// attach: serve checks there that it is still the installation it
	// verified (spec M6a §6.3). It runs in the connection's own goroutine,
	// after the token and the version checks and before OnConnect or
	// OnAttach, so it may do I/O; false, a cancelled ctx or more than
	// admitTimeout reject the app as one without the token. nil admits
	// every app with the token.
	Admit func(ctx context.Context, h Hello) bool
```

Justo después de la variable `helloTimeout`:

```go
// admitTimeout bounds Options.Admit (a variable for the tests).
var admitTimeout = 20 * time.Second
```

En `handle`, justo después del bloque `if hello.Protocol < MinAppProtocol { … }` y antes de `cl := &client{…}`:

```go
	if !s.admit(ctx, hello) {
		c.Close(websocket.StatusPolicyViolation, "unauthorized")
		return
	}
```

y el método, al final del fichero:

```go
// admit asks Options.Admit, failing closed: false, a cancelled ctx or no
// answer within admitTimeout reject the app.
func (s *Server) admit(ctx context.Context, h Hello) bool {
	if s.o.Admit == nil {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, admitTimeout)
	defer cancel()
	done := make(chan bool, 1)
	go func() { done <- s.o.Admit(ctx, h) }()
	select {
	case ok := <-done:
		return ok && ctx.Err() == nil
	case <-ctx.Done():
		return false
	}
}
```

- [ ] **Step 4: Ejecutar los tests del paquete**

Run (desde `host/`): `go test -timeout 5m ./internal/bridge/ && go test -timeout 5m -race ./internal/bridge/ -run Admit`
Expected: PASS. Si `-race` no está disponible en este PC, apúntalo en el ledger: la CI lo pasa en Linux.

- [ ] **Step 5: gofmt, vet y commit propio**

Run (desde `host/`): `gofmt -l ./internal/bridge && go vet ./internal/bridge/`
Expected: sin salida.

```bash
git add host/internal/bridge/server.go host/internal/bridge/admit_test.go
git commit -m "feat(host): gancho Admit en el canal para rechazar una app que no es la verificada

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Avisa al controlador para que se lo diga a tern-51 y a nautilus-19: `server.go` ha cambiado.

---

### Task 5: Serve solo entrega el token a la app verificada

**Files:**
- Modify: `host/internal/phoneapp/phoneapp.go` (`Handshake` pide `Verified`)
- Modify: `host/internal/phoneapp/phoneapp_test.go` (sus llamadas)
- Modify: `host/internal/cli/phone.go` (`connect()`, el `Verifier`, `Admit` y el ticker: **commit propio**, con `Handshake`)
- Modify: `host/cmd/adbuddy/e2e_test.go` (solo `startWithData`: **commit propio**, ruling del supervisor)
- Modify: `host/internal/cli/phone_test.go` (`appPhone`, `linkEnv`, `TestServeInterruptedDuringTheHandshake` y tests nuevos)
- Modify: `host/cmd/adbuddy/phone_e2e_test.go` (`phoneAppRules` y la confianza de desarrollo)

**Interfaces:**
- Consumes: `Verifier` (con `Same`), `Verified`, `LoadTrust`, `DevTrustPath` y `TrustDevAPK` (Task 2); `bridge.Options.Admit` (Task 4); `releaseCert`, `testCert`, `otherCert`, `testAPK`, `pinTestCert`, `testAppPath` y `appReadRules` (Tasks 2-3).
- Produces: `func phoneapp.Handshake(ctx context.Context, a ADB, v Verified, token string, port int, hostName string) error`, y en los tests de `cmd/adbuddy` `startWithData(t, config string, prepare func(data string), extra ...testutil.FakeRule) *harness`.

- [ ] **Step 1: `Handshake` pide `Verified`**

En `host/internal/phoneapp/phoneapp.go`, cambia la firma de `Handshake` y añade el comprobante, justo después de `ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)` y su `defer cancel()` (antes del `content write`):

```go
func Handshake(ctx context.Context, a ADB, v Verified, token string, port int, hostName string) error {
```

```go
	if err := v.still(ctx, a); err != nil {
		return err
	}
```

Añade al comentario de `Handshake` una frase: "v is the proof that the app is ours (Verifier.Verify): the token goes to no other (spec M6a §6.3)."

En `host/internal/phoneapp/phoneapp_test.go`, cada llamada `Handshake(ctx, f, testToken, …)` pasa a `Handshake(ctx, f, verified(f), testToken, …)` (también las de `TestCommandsHaveATimeLimit`, `TestHandshakeChecksItsInputs` y `TestHandshakeHasATimeLimit`). `pm path` y `stat` no llevan stdin, así que lo último que se escribe sigue siendo el `content write`; las entradas inválidas siguen sin llamar a adb, porque se comprueban antes. Añade a la función `check` de `TestGivingNeedsTheCheckedApp`, después de la comprobación de `AskConsent`:

```go
		if err := Handshake(context.Background(), f, v, testToken, 47123, "My PC"); !errors.Is(err, ErrNoTrustedApp) {
			t.Errorf("%s: Handshake: %v", label, err)
		}
```

- [ ] **Step 2: `phone.go`**

Todo esto va en `host/internal/cli/phone.go`, en el mismo commit que el cambio de `Handshake`.

1. Dos campos nuevos en `phoneLink`, después de `agent string` y su línea en blanco:

```go
	// verifier checks the app installed (spec M6a §6.3). recheck is how
	// Admit asks for a new connect() when an app that is not the verified
	// one has connected.
	verifier *phoneapp.Verifier
	recheck  chan struct{}

```

2. En `startPhoneLink`, justo después de `l.conf = bridge.NewConfirmer(noApp{}, fallback)`, el aviso de la confianza de desarrollo. Sale al arrancar siempre que exista el fichero, lleve o no la app ese APK (spec §6.1), una sola vez:

```go
	if trust := phoneapp.LoadTrust(releaseCert, e.dirs.Data); trust.DevAPK != "" {
		fmt.Fprintf(app.Stderr, "⚠ ADBuddy trusts a development build of the phone app (SHA-256 %s, from `adbuddy setup --apk`): it gets the token like a release. Delete %s to stop.\n", trust.DevAPK, phoneapp.DevTrustPath(e.dirs.Data))
	}
```

3. Justo antes del comentario `// No token until connect hands one over: nobody gets in before.`:

```go
	l.verifier = phoneapp.NewVerifier(func() phoneapp.Trust { return phoneapp.LoadTrust(releaseCert, e.dirs.Data) }, phoneapp.MinAppVersionCode)
	l.recheck = make(chan struct{}, 1)
```

4. En las `bridge.Options` de `bridge.New`, justo antes de `OnConnect: func(on bool) {`, el gancho `Admit` (Task 4). Toda app que llegue con el token ha de ser la instalación verificada. Si no lo es, se rechaza y se vuelve a verificar enseguida, con un token nuevo:

```go
		Admit: func(ctx context.Context, _ bridge.Hello) bool {
			if err := l.verifier.Same(ctx, a); err != nil {
				fmt.Fprintf(app.Stderr, "⚠ An app with the token connected, but it is not the app ADBuddy checked (%s): refused, checking the app again.\n", classify.Printable(err.Error()))
				select {
				case l.recheck <- struct{}{}:
				default:
				}
				return false
			}
			return true
		},
```

5. Sustituye el bloque de `connect` (desde el comentario `// connect hands the channel over again…` hasta el final de la función):

```go
	// connect hands the channel over again with a new token (spec §5.2): the
	// server takes only that one and drops an app with the old one before
	// the tunnel is back, so a token captured meanwhile is worth nothing.
	// The token only goes to the app checked right before (spec M6a §6.3):
	// the verifier reads the APK again only after a reinstall or an update.
	connect := func() error {
		// A new token first: whatever app held the old one loses it, even
		// when this check then fails.
		token, err := newToken()
		if err != nil {
			return fmt.Errorf("could not make a token: %w", err)
		}
		l.srv.SetToken(token)
		v, err := l.verifier.Verify(ctx, a)
		if err != nil {
			return err
		}
		if err := phoneapp.Tunnel(ctx, a, port); err != nil {
			return err
		}
		return phoneapp.Handshake(ctx, a, v, token, port, host)
	}
```

6. En la goroutine del ticker, el `select` espera también a `l.recheck`. Cuando llega, `connected = false`, para que vuelva a ejecutarse `connect()`:

```go
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			case <-l.recheck:
				connected = false
			}
```

Si `connect()` falla en `Verify`, el token ya ha rotado: la app que tenía el anterior no puede volver a entrar.

- [ ] **Step 3: El arnés del e2e (`startWithData`)**

En `host/cmd/adbuddy/e2e_test.go` (fichero de tern; ruling del supervisor: solo prepara el arnés, va en su propio commit y no cambia las aserciones de ningún test). `startWith` pasa a delegar en `startWithData` con `prepare` nil, y el cuerpo de `startWith` pasa a ser el de `startWithData`, con una llamada nueva a `prepare` justo después de crear el directorio de datos y antes de arrancar serve:

```go
// startWith starts serve with extra fake adb rules first and, when config is
// not empty, that config.toml.
func startWith(t *testing.T, config string, extra ...testutil.FakeRule) *harness {
	t.Helper()
	return startWithData(t, config, nil, extra...)
}

// startWithData is startWith, and when prepare is not nil it calls it with
// the data folder before serve starts: a test puts there what serve reads
// at start (the trust of a development build of the phone app).
func startWithData(t *testing.T, config string, prepare func(data string), extra ...testutil.FakeRule) *harness {
	t.Helper()
	data, err := os.MkdirTemp("", "e2d")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(data) })
	if prepare != nil {
		prepare(data)
	}
```

(el resto del cuerpo, desde `tmp := t.TempDir()`, queda igual).

- [ ] **Step 4: Los tests de `phone_test.go`**

En `host/internal/cli/phone_test.go`, añade `"crypto/sha256"`, `"encoding/base64"`, `"encoding/hex"` y `"github.com/hybriqtech/adbuddy/host/internal/phoneapp"` y `"github.com/hybriqtech/adbuddy/host/internal/phoneapp/apktest"` a los imports. En `appPhone`, añade los campos después de `fail`:

```go
	// apk is the installed app (nil: signed with testCert); statKey its
	// size, inode and mtime as stat prints them ("": from apk). update
	// changes both, as an update in place does.
	apk     []byte
	statKey string
```

En el `switch` de `Run`, sustituye el caso de `dumpsys package` por estos:

```go
	case strings.Contains(joined, "dumpsys package"):
		io.WriteString(stdout, "Packages:\n  Package [tech.hybriq.adbuddy] (1a2b):\n    versionCode=1 minSdk=30 targetSdk=37\n    versionName=0.2.0\n")
	case joined == "shell pm path tech.hybriq.adbuddy":
		io.WriteString(stdout, "package:"+testAppPath+"\n")
	case strings.HasPrefix(joined, "shell stat -c "):
		_, key := f.app()
		io.WriteString(stdout, key+"\n")
	case strings.HasPrefix(joined, "shell base64 "):
		apk, _ := f.app()
		io.WriteString(stdout, base64.StdEncoding.EncodeToString(apk)+"\n")
```

y añade estas funciones después de `lastReverse`:

```go
// app returns the installed APK and its stat (size, inode, mtime). The
// caller holds f.mu.
func (f *appPhone) app() ([]byte, string) {
	apk := f.apk
	if apk == nil {
		apk = testAPK(testCert)
	}
	if f.statKey != "" {
		return apk, f.statKey
	}
	return apk, strconv.Itoa(len(apk)) + " 1 1"
}

// update replaces the app in place: same path, other bytes and other stat.
func (f *appPhone) update(apk []byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.apk, f.statKey = apk, strconv.Itoa(len(apk))+" 2 2"
}
```

`TestServeInterruptedDuringTheHandshake` necesita ahora las reglas de lectura de la app, o serve no llega al `content write`. Sustituye su regla de `dumpsys package` por `appReadRules`:

```go
	rules := append(append(appReadRules(testAPK(testCert)),
		testutil.FakeRule{Match: `^shell content write `, SleepMs: 20000}),
		remoteRules("AAAA1111")...)
```

(`TestServeWithTheChatPortTaken` no cambia: con el puerto ocupado serve no llega a `connect()`.)

Añade estos tests:

```go
// trustDevBuild makes the host in e trust apk as a development build.
func trustDevBuild(t *testing.T, e *env, apk []byte) string {
	t.Helper()
	sum := sha256.Sum256(apk)
	d := hex.EncodeToString(sum[:])
	if _, err := phoneapp.TrustDevAPK(e.dirs.Data, d); err != nil {
		t.Fatal(err)
	}
	return d
}

// An app that is not ours gets no token, serve says why once, and it does
// not read the app again on every tick.
func TestLinkGivesNoTokenToAnUntrustedApp(t *testing.T) {
	setVar(t, &tunnelCheck, 20*time.Millisecond)
	e := linkEnv(t)
	f := &appPhone{apk: testAPK(otherCert)}
	_, stderr, err := startLink(t, e, f, confirm.DenyAll{})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(300 * time.Millisecond)
	if got := f.handedTokens(); len(got) != 0 {
		t.Fatalf("tokens handed to an untrusted app: %q", got)
	}
	if n := f.count("shell base64 "); n != 1 {
		t.Fatalf("read the app %d times", n)
	}
	if out := stderr.String(); strings.Count(out, "is not trusted") != 1 || strings.Contains(out, "trusts a development build") {
		t.Fatalf("stderr:\n%s", out)
	}
}

// serve says at start that it trusts a development build, once, whether or
// not the app on the phone is that APK.
func TestServeWarnsOnceAboutTheDevelopmentBuild(t *testing.T) {
	setVar(t, &tunnelCheck, 20*time.Millisecond)
	e := linkEnv(t)
	dev := testAPK(otherCert)
	digest := trustDevBuild(t, e, dev)
	f := &appPhone{apk: dev}
	_, stderr, err := startLink(t, e, f, confirm.DenyAll{})
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	time.Sleep(200 * time.Millisecond)
	out := stderr.String()
	if strings.Count(out, "trusts a development build of the phone app") != 1 || !strings.Contains(out, digest) || !strings.Contains(out, phoneapp.DevTrustPath(e.dirs.Data)) {
		t.Fatalf("stderr:\n%s", out)
	}
}

// The app is updated in place (same path, other stat) to a build that is not
// the trusted one: with the token it already has it is refused, serve
// checks the app again with a new token, and the updated app gets none.
func TestAnAppUpdatedInPlaceGetsNoNewToken(t *testing.T) {
	setVar(t, &tunnelCheck, 50*time.Millisecond)
	e := linkEnv(t)
	dev := testAPK(otherCert)
	trustDevBuild(t, e, dev)
	f := &appPhone{apk: dev}
	_, stderr, err := startLink(t, e, f, confirm.DenyAll{})
	if err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "a handshake", func() bool { return len(f.handedTokens()) >= 1 })
	tok := f.handedTokens()[0]
	if typ, _ := dialApp(t, e.cfg.ChatPort, tok); typ != bridge.TypeWelcome {
		t.Fatalf("before the update: %q", typ)
	}
	// Same signer as the development build, other bytes: it keeps its data
	// (the token), but it is not the APK that was trusted.
	f.update(apktest.Build(apktest.Options{
		Blocks: []apktest.Block{{ID: apktest.V2, Signers: []apktest.Signer{{Cert: otherCert}}}},
		Files:  []string{"AndroidManifest.xml", "classes.dex"},
	}))
	if _, code := dialApp(t, e.cfg.ChatPort, tok); code != websocket.StatusPolicyViolation {
		t.Fatalf("the updated app with its token: close %v, want 1008", code)
	}
	waitUntil(t, "the refusal to be said", func() bool { return strings.Contains(stderr.String(), "not the app ADBuddy checked") })
	time.Sleep(300 * time.Millisecond) // several ticks
	if n := len(f.handedTokens()); n != 1 {
		t.Fatalf("%d tokens handed over: the updated app got one", n)
	}
	if _, code := dialApp(t, e.cfg.ChatPort, tok); code != websocket.StatusPolicyViolation {
		t.Fatalf("the old token after the check: close %v, want 1008", code)
	}
}
```

En `linkEnv`, no hace falta cambiar nada: `releaseCert` queda fijado por el `init` de `appsetup_test.go` (Task 3).

- [ ] **Step 5: El e2e (`phone_e2e_test.go`)**

En `host/cmd/adbuddy/phone_e2e_test.go`:

- añade a `phoneAppRules(port)`, antes de las demás, las reglas de `pm path`, `stat` y `base64`, que sirven `e2eAPK`. La respuesta de `dumpsys package` pasa a `"Packages:\n  Package [tech.hybriq.adbuddy] (1a2b):\n    versionCode=1 minSdk=30 targetSdk=37\n    versionName=0.2.0\n"`. Las reglas:

```go
		{Match: `^shell pm path tech\.hybriq\.adbuddy$`, Stdout: "package:/data/app/~~E2e==/tech.hybriq.adbuddy-E2e==/base.apk\n"},
		{Match: `^shell stat -c `, Stdout: strconv.Itoa(len(e2eAPK)) + " 1 1\n"},
		{Match: `^shell base64 `, Stdout: base64.StdEncoding.EncodeToString(e2eAPK) + "\n"},
```

- define, a nivel de paquete:

```go
// e2eAPK is the app the fake phone has installed; each test trusts it as
// the development build (the binary pins no release certificate).
var (
	e2eCert = apktest.Cert("e2e")
	e2eAPK  = apktest.APK(apktest.Block{ID: apktest.V2, Signers: []apktest.Signer{{Cert: e2eCert}}})
)

// trustE2EApp makes the host in dataDir trust e2eAPK.
func trustE2EApp(t *testing.T, dataDir string) {
	t.Helper()
	sum := sha256.Sum256(e2eAPK)
	if _, err := phoneapp.TrustDevAPK(dataDir, hex.EncodeToString(sum[:])); err != nil {
		t.Fatal(err)
	}
}
```

- añade los imports que falten (`crypto/sha256`, `encoding/base64`, `encoding/hex`, `phoneapp` y `phoneapp/apktest`; `strconv` ya está);
- en cada test que use `phoneAppRules`, cambia `startWith(t, cfg, rules…)` por `startWithData(t, cfg, func(data string) { trustE2EApp(t, data) }, rules…)`. Por ejemplo:

```go
	h := startWithData(t, "chat_port = "+port+"\n", func(data string) { trustE2EApp(t, data) }, phoneAppRules(port)...)
```

  y, en `TestPhoneAppConfirmsAndPauses`:

```go
	h := startWithData(t, "chat_port = "+port+"\n", func(data string) { trustE2EApp(t, data) },
		append(phoneAppRules(port), testutil.FakeRule{Match: `^shell reboot$`})...)
```

- [ ] **Step 6: Todos los tests del host**

Run (desde `host/`): `go test -timeout 10m ./...`
Expected: PASS. Repite `go test -timeout 10m -race ./internal/cli/ ./internal/phoneapp/` si hay cgo (en este PC no lo hay: apúntalo, la CI lo pasa en Linux). Si algún otro test de `cli` que arranca serve con la app instalada falla porque ahora serve no entrega el token, arréglalo en `phone_test.go` (que es de M6a en este commit) con `appReadRules`; no toques `serve_test.go`: apúntalo en el ledger y pregunta al controlador.

- [ ] **Step 7: gofmt, vet y tres commits**

Run (desde `host/`): `gofmt -l . && go vet ./...`
Expected: sin salida.

Cada commit tiene que compilar. El cambio de `Handshake` necesita el de `phone.go`, así que el primero lleva `phoneapp` y `phone.go` juntos y es pequeño a propósito:

```bash
git add host/internal/phoneapp/phoneapp.go host/internal/phoneapp/phoneapp_test.go host/internal/cli/phone.go
git commit -m "feat(host): serve solo entrega el token a la app verificada

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
go build ./...
```

(Ejecuta `go build ./...` desde `host/` justo después del primer commit, sin stash: `go build` no compila los ficheros `_test.go`, y todo el código que no es de test ya está en el commit.) Después, el arnés del e2e en su propio commit y luego los tests:

```bash
git add host/cmd/adbuddy/e2e_test.go
git commit -m "test(host): startWithData prepara el directorio de datos antes de arrancar serve

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git add host/internal/cli/phone_test.go host/cmd/adbuddy/phone_e2e_test.go
git commit -m "test(host): el adb falso de serve sirve la app y su certificado

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

Avisa al controlador para que se lo diga a tern-51: `phone.go` ha cambiado más que `connect()` (campos `verifier` y `recheck`, `Admit`, el `select` del ticker y el aviso de arranque), y `e2e_test.go` tiene `startWithData`.

---

### Task 6: Quitar o neutralizar la app de ADBuddy es 🔴 especial

**Files:**
- Modify: `host/internal/classify/testdata/habitual.jsonl`
- Modify (si hace falta): `host/internal/classify/special.go`

**Interfaces:**
- Consumes: nada de las tareas anteriores.
- Produces: el contrato de §7 de la spec en el corpus.

- [ ] **Step 1: Añadir los casos al corpus**

Al final de `host/internal/classify/testdata/habitual.jsonl`, añade:

```json
# ADBuddy's own app: removing, wiping, disabling, stopping or weakening it leaves the phone without its trusted app (spec M6a §7)
{"sh":"pm uninstall tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm uninstall -k tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm uninstall --user 0 tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm uninstall -k --user 0 tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"cmd package uninstall tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"cmd package uninstall -k --user 0 tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm clear tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm clear --user 0 tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm disable tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm disable-user tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm disable-user --user 0 tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm suspend tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm hide tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"pm revoke tech.hybriq.adbuddy android.permission.WRITE_SECURE_SETTINGS","level":"red","special":true}
{"sh":"cmd package disable-user tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"am force-stop tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"am kill tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"am stop-app tech.hybriq.adbuddy","level":"red","special":true}
{"sh":"cmd appops set tech.hybriq.adbuddy RUN_ANY_IN_BACKGROUND ignore","level":"red","special":true}
{"sh":"p=tech.hybriq.adbuddy; pm uninstall $p","level":"red","special":true}
{"sh":"pm list packages | grep hybriq | cut -d: -f2 | xargs pm uninstall","level":"red","special":true}
{"argv":["uninstall","tech.hybriq.adbuddy"],"level":"red","special":true}
{"argv":["uninstall","-k","tech.hybriq.adbuddy"],"level":"red","special":true}
{"argv":["shell","pm","uninstall","tech.hybriq.adbuddy"],"level":"red","special":true}
{"sh":"cmd package uninstall $(pm list packages | grep hybr | cut -c9-)","level":"red","special":true}
{"sh":"pm reset-permissions","level":"red","special":true}
{"sh":"dumpsys deviceidle whitelist reset","level":"red","special":true}
{"sh":"cmd deviceidle whitelist reset","level":"red","special":true}
{"sh":"cmd appops reset","level":"red","special":true}
{"sh":"cmd package reset-permissions","level":"red","special":true}
{"sh":"appops reset","level":"red","special":true}
```

Comprueba antes que `readJSONL` admite líneas `#` en medio del fichero: su comentario dice "lines starting with # are comments".

- [ ] **Step 2: Ejecutar el corpus**

Run (desde `host/`): `go test -timeout 10m ./internal/classify/ -run 'Corpus|Habitual'`. Si ningún test se llama así, ejecuta `go test -timeout 10m ./internal/classify/` y busca en la salida las líneas nuevas.
Expected: o PASS, o FAIL con las líneas que hoy no salen 🔴 especiales. **Apunta en el ledger cuáles fallaron.**

- [ ] **Step 3: Arreglar lo que no salga**

`specialReason` ya tiene la regla general `if level != Green && mentionsADBuddy(v)` (special.go, al final del `switch` de shell), así que las órdenes que nombran el paquete deberían salir ya. Se espera que fallen dos grupos. En `host/internal/classify/special.go`:

1. **`cmd` con argumentos dinámicos.** En la regla de argumentos dinámicos, añade `cmd`:

```go
	if level != Green && (name == "pm" || name == "appops" || name == "am" || name == "cmd") && hasDynamic(ws[1:]) {
		return "arguments only known on the phone could name ADBuddy or a critical app"
	}
```

2. **Órdenes de *reset* que neutralizan la app sin nombrarla.** Justo antes de esa regla:

```go
	if level != Green && resetsEveryApp(name, v) {
		return "resets the permissions, battery exemptions or app ops of every app, ADBuddy's included"
	}
```

con:

```go
// resetsEveryApp: pm reset-permissions without -p, `deviceidle whitelist
// reset` and `cmd appops reset` without a package strip ADBuddy's app of
// WRITE_SECURE_SETTINGS (Cut access), its battery exemption or its app ops
// without naming it (spec M6a §7).
func resetsEveryApp(name string, v []string) bool {
	has := func(i int, s string) bool { return len(v) > i && v[i] == s }
	switch {
	case name == "pm" && has(1, "reset-permissions"):
		return !slices.Contains(v[2:], "-p")
	case name == "cmd" && has(1, "package") && has(2, "reset-permissions"):
		return !slices.Contains(v[3:], "-p")
	case name == "appops" && has(1, "reset"):
		for _, w := range v[2:] {
			if !strings.HasPrefix(w, "-") && w != "0" {
				return false
			}
		}
		return true
	case (name == "dumpsys" || name == "cmd") && has(1, "deviceidle") && has(2, "whitelist") && has(3, "reset"):
		return true
	case name == "cmd" && has(1, "appops") && has(2, "reset"):
		for _, w := range v[3:] {
			if !strings.HasPrefix(w, "-") && w != "0" { // a package: only that one
				return false
			}
		}
		return true
	}
	return false
}
```

(Añade `"slices"` a los imports si falta. `cmd appops reset --user 0` sigue siendo de todas las apps: el `0` es el valor de `--user`.)

Si otra línea nueva sigue sin salir 🔴 especial, sigue el mismo patrón: una condición estrecha sobre `name` y sus subórdenes. No admitas nada nuevo en la lista blanca del evaluador.

Vuelve a ejecutar **todo** el paquete: `go test -timeout 10m ./internal/classify/`. Debe pasar entero. Si una línea de `habitual.jsonl` o de `m2-levels.jsonl` que ya existía sube de nivel, el arreglo es demasiado ancho: estréchalo. Nunca toques esas líneas.

- [ ] **Step 4: gofmt, vet y commit**

Run (desde `host/`): `gofmt -l ./internal/classify && go vet ./internal/classify/`
Expected: sin salida.

```bash
git add host/internal/classify/testdata/habitual.jsonl host/internal/classify/special.go
git commit -m "feat(host): quitar o neutralizar la app de ADBuddy es 🔴 especial

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

(Si `special.go` no cambió, el `git add` solo añade el corpus.)

---

### Task 7: Herramienta `apkcert`

**Files:**
- Create: `host/cmd/apkcert/main.go`
- Test: `host/cmd/apkcert/main_test.go`

**Interfaces:**
- Consumes: `phoneapp.Signers` y `phoneapp.SigningCertSHA256` (Tasks 1-2) y `apktest` (Task 1).
- Produces: desde `host/`, `go run ./cmd/apkcert <file.apk>` imprime una línea por certificado distinto, ordenadas, y `go run ./cmd/apkcert -pinned` imprime el fijado. Salidas: 0 bien, 1 error y 2 uso.

- [ ] **Step 1: El test que falla**

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

- [ ] **Step 2: Ejecutarlo y ver que falla**

Run (desde `host/`): `go test -timeout 5m ./cmd/apkcert/`
Expected: FAIL de compilación (`undefined: run`).

- [ ] **Step 3: La herramienta**

`host/cmd/apkcert/main.go`:

```go
// Command apkcert prints the SHA-256 of the signing certificates of an APK,
// with the parser the host uses before it trusts the app (spec M6a §5),
// every check included. The release workflows use it; it is not shipped.
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

- [ ] **Step 4: Ejecutarlo y ver que pasa**

Run (desde `host/`): `go test -timeout 5m ./cmd/apkcert/ -v && gofmt -l ./cmd && go vet ./cmd/...`
Expected: PASS y sin salida de gofmt ni de vet.

- [ ] **Step 5: Commit**

```bash
git add host/cmd/apkcert
git commit -m "feat(host): herramienta apkcert para comprobar el certificado del APK en la CI

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 8: Versión desde la etiqueta

**Files:**
- Create: `scripts/release-version.sh`
- Test: `scripts/release-version_test.sh`
- Modify: `android/app/build.gradle.kts:16-17` (commit propio)

**Interfaces:**
- Consumes: nada.
- Produces: `sh scripts/release-version.sh <tag>` imprime exactamente tres líneas, `name=<X.Y.Z[-rc.N]>`, `code=<versionCode>` y `prerelease=<true|false>`, y sale con 0. Con una etiqueta inválida sale con 1, y con un número de argumentos distinto de 1, con 2. Gradle lee `-Padbuddy.versionName=…` y `-Padbuddy.versionCode=…`.

- [ ] **Step 1: El test que falla**

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
		printf 'FAIL %s: got\n%s\nwant\n%s\n' "$1" "$got" "$want"
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
	'v1.2.3 ' '' 'v1.2.3$(id)' "v1.2.3${nl}v1.2.4"; do
	bad "$t"
done

set +e
sh "$here/release-version.sh" >/dev/null 2>&1
[ $? -eq 2 ] || { echo "FAIL no argument: exit is not 2"; fail=1; }
sh "$here/release-version.sh" a b >/dev/null 2>&1
[ $? -eq 2 ] || { echo "FAIL two arguments: exit is not 2"; fail=1; }
set -e

# An rc comes before its release, which comes before the next rc.
a=$(code v0.3.0-rc.1)
b=$(code v0.3.0-rc.98)
c=$(code v0.3.0)
d=$(code v0.3.1-rc.1)
e=$(code v0.4.0)
if ! { [ "$a" -lt "$b" ] && [ "$b" -lt "$c" ] && [ "$c" -lt "$d" ] && [ "$d" -lt "$e" ]; }; then
	echo "FAIL order: $a $b $c $d $e"
	fail=1
fi

[ "$fail" -eq 0 ] && echo "release-version: all tests pass"
exit "$fail"
```

- [ ] **Step 2: Ejecutarlo y ver que falla**

Run (desde la raíz del worktree, en Git Bash): `sh scripts/release-version_test.sh`
Expected: líneas `FAIL` (el script no existe) y salida 1.

- [ ] **Step 3: El script**

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
	# grep matches line by line: refuse a tag with a newline first.
	case $tag in
	*"
"*) bad "$tag" ;;
	esac
	printf '%s\n' "$tag" | grep -Eq '^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})(-rc\.[1-9][0-9]?)?$' || bad "$tag"
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

- [ ] **Step 4: Ejecutarlo y ver que pasa**

Run: `sh scripts/release-version_test.sh`
Expected: `release-version: all tests pass` y salida 0.

- [ ] **Step 5: Commit de los scripts**

```bash
git add scripts/release-version.sh scripts/release-version_test.sh
git commit -m "feat(release): versionName y versionCode desde la etiqueta

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

- [ ] **Step 6: La versión de la app, desde Gradle**

En `android/app/build.gradle.kts`, sustituye **solo** estas dos líneas de `defaultConfig`:

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

En este PC no hay JDK, así que no compiles. La comprobación la hacen `android.yml` (build de debug, con los valores por defecto) y `release-check.yml` (Task 10, con `-P…`). Comprueba a mano que `git diff android/` solo muestra esas líneas.

- [ ] **Step 7: Commit propio de Gradle**

```bash
git add android/app/build.gradle.kts
git commit -m "build(app): versionName y versionCode desde propiedades de Gradle

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 9: Script de instalación

**Files:**
- Create: `scripts/install.sh`
- Test: `scripts/install_test.sh`

**Interfaces:**
- Consumes: los nombres de los archivos de goreleaser (Task 10): `adbuddy_<versión>_<os>_<arch>.tar.gz`, donde `<os>_<arch>` es `linux_amd64`, `linux_arm64` o `darwin_all`, y `checksums.txt` con líneas `<hex>  <fichero>`. Cada `.tar.gz` lleva `adbuddy` en su raíz.
- Produces: `scripts/install.sh`, con `ADBUDDY_VERSION`, `ADBUDDY_INSTALL_DIR` y `ADBUDDY_DOWNLOAD_URL` (spec §8).

- [ ] **Step 1: El test que falla**

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
	printf 'extra\n' >"$work/pkg/$target/README.md"
	chmod 755 "$work/pkg/$target/adbuddy"
	tar -czf "$rel/adbuddy_9.9.9_${target}.tar.gz" -C "$work/pkg/$target" adbuddy README.md
done
printf 'zip\n' >"$rel/adbuddy_9.9.9_windows_amd64.zip"
(cd "$rel" && sha adbuddy_9.9.9_*) >"$rel/checksums.txt"

# fakeuname prints a folder whose uname answers $1 to -s and $2 to -m.
fakeuname() {
	d=$work/uname-$1-$2
	mkdir -p "$d"
	printf '#!/bin/sh\ncase "$1" in -s) echo %s ;; -m) echo %s ;; esac\n' "$1" "$2" >"$d/uname"
	chmod 755 "$d/uname"
	printf '%s' "$d"
}

# try os arch base dir [version]: runs install.sh; its output goes to $work/out.
try() {
	PATH="$(fakeuname "$1" "$2"):$PATH" ADBUDDY_DOWNLOAD_URL="$3" ADBUDDY_INSTALL_DIR="$4" ADBUDDY_VERSION="${5:-latest}" \
		sh "$here/install.sh" >"$work/out" 2>&1
}

# refused name os arch base dir [version]: install.sh must fail.
refused() {
	name=$1
	shift
	if try "$@"; then
		echo "FAIL $name: accepted"
		sed 's/^/    /' "$work/out"
		fail=1
	fi
}

says() { # name text
	if ! grep -q "$2" "$work/out"; then
		echo "FAIL $1: no \"$2\" in:"
		sed 's/^/    /' "$work/out"
		fail=1
	fi
}

for c in "Linux x86_64 linux_amd64" "Linux aarch64 linux_arm64" "Darwin arm64 darwin_all" "Darwin x86_64 darwin_all"; do
	set -- $c
	dir=$work/bin-$1-$2
	if ! try "$1" "$2" "$(url "$rel")" "$dir"; then
		echo "FAIL $1 $2: refused"
		sed 's/^/    /' "$work/out"
		fail=1
		continue
	fi
	"$dir/adbuddy" | grep -q "$3" || { echo "FAIL $1 $2: wrong binary"; fail=1; }
	[ ! -e "$dir/README.md" ] || { echo "FAIL $1 $2: extracted more than adbuddy"; fail=1; }
done
says "warns about PATH" "not in your PATH"
says "warns about the origin" "not the official release"

refused "riscv64" Linux riscv64 "$(url "$rel")" "$work/bin-riscv"
says "riscv64" "no build for Linux riscv64"
refused "Windows" MINGW64_NT-10.0 x86_64 "$(url "$rel")" "$work/bin-win"
says "Windows" "Windows"
refused "bad version" Linux x86_64 "$(url "$rel")" "$work/bin-v" 'v1.2.3;rm'
says "bad version" "not a release tag"

# A tampered archive is not installed.
bad=$work/bad
cp -r "$rel" "$bad"
printf 'evil\n' >>"$bad/adbuddy_9.9.9_linux_amd64.tar.gz"
refused "tampered" Linux x86_64 "$(url "$bad")" "$work/bin-bad"
says "tampered" "does not match its SHA-256"
[ ! -e "$work/bin-bad/adbuddy" ] || { echo "FAIL tampered: installed"; fail=1; }

# A checksums line whose name only ends like ours does not count.
odd=$work/odd
mkdir -p "$odd"
cp "$rel"/adbuddy_9.9.9_linux_amd64.tar.gz "$odd/"
(cd "$odd" && sha adbuddy_9.9.9_linux_amd64.tar.gz | sed 's/  /  x/') >"$odd/checksums.txt"
refused "no exact line" Linux x86_64 "$(url "$odd")" "$work/bin-odd"
says "no exact line" "no adbuddy archive for linux_amd64"

[ "$fail" -eq 0 ] && echo "install: all tests pass"
exit "$fail"
```

- [ ] **Step 2: Ejecutarlo y ver que falla**

Run (Git Bash, desde la raíz): `sh scripts/install_test.sh`
Expected: `FAIL` en cada caso y salida 1.

- [ ] **Step 3: El script**

`scripts/install.sh`:

```sh
#!/bin/sh
# Installs the adbuddy host on Linux or macOS (spec M6a §8):
#
#   curl -fsSL https://github.com/hybriqtech/adbuddy/releases/latest/download/install.sh | sh
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

# fetch url file: HTTPS only, unless the URL is a test's.
fetch() {
	if have curl; then
		if [ -n "${ADBUDDY_DOWNLOAD_URL:-}" ]; then
			curl -fsSL -o "$2" "$1"
		else
			curl --proto '=https' --tlsv1.2 -fsSL -o "$2" "$1"
		fi
	elif [ -n "${ADBUDDY_DOWNLOAD_URL:-}" ]; then
		wget -q -O "$2" "$1"
	else
		wget --https-only -q -O "$2" "$1"
	fi
}

main() {
	have curl || have wget || die "needs curl or wget"
	have tar || die "needs tar"
	have awk || die "needs awk"
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
	if [ "$version" != latest ]; then
		printf '%s\n' "$version" | grep -Eq '^v(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})\.(0|[1-9][0-9]{0,2})(-rc\.[1-9][0-9]?)?$' ||
			die "ADBUDDY_VERSION=$version is not a release tag (vX.Y.Z or vX.Y.Z-rc.N)"
	fi
	if [ -n "${ADBUDDY_DOWNLOAD_URL:-}" ]; then
		base=${ADBUDDY_DOWNLOAD_URL%/}
		say "downloading from $base: not the official release"
	elif [ "$version" = latest ]; then
		base=https://github.com/hybriqtech/adbuddy/releases/latest/download
	else
		base=https://github.com/hybriqtech/adbuddy/releases/download/$version
	fi

	tmp=$(mktemp -d)
	trap 'rm -rf "$tmp"' EXIT INT TERM
	fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "could not download $base/checksums.txt"
	name=$(awk -v t="$target" '$2 ~ "^adbuddy_[^/ ]+_" t "\\.tar\\.gz$" { print $2 }' "$tmp/checksums.txt")
	[ -n "$name" ] && [ "$(printf '%s\n' "$name" | wc -l | tr -d ' ')" -eq 1 ] ||
		die "no adbuddy archive for $target in $base/checksums.txt"
	want=$(awk -v f="$name" '$2 == f { print $1 }' "$tmp/checksums.txt")
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

- [ ] **Step 4: Ejecutarlo y ver que pasa**

Run: `sh scripts/install_test.sh`
Expected: `install: all tests pass` y salida 0.

- [ ] **Step 5: Commit**

```bash
git add scripts/install.sh scripts/install_test.sh
git commit -m "feat(release): script de instalación para Linux y macOS con checksum

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 10: goreleaser y los workflows de release

**Files:**
- Create: `.goreleaser.yaml`
- Create: `scripts/release-cert.sh`
- Create: `.github/workflows/release.yml`
- Create: `.github/workflows/release-check.yml`

**Interfaces:**
- Consumes: `sh scripts/release-version.sh` (Task 8), `sh scripts/install.sh` y `sh scripts/install_test.sh` (Task 9), `go run ./cmd/apkcert` (Task 7) y las propiedades `adbuddy.versionName` y `adbuddy.versionCode` (Task 8).
- Produces: `APKSIGNER=… sh scripts/release-cert.sh <apk> [--pinned]` sale con 0 cuando `apksigner` y `apkcert` dan un único certificado y es el mismo (y, con `--pinned`, el fijado).

- [ ] **Step 1: `scripts/release-cert.sh`**

```sh
#!/bin/sh
# Checks the signing certificate of an APK two ways, apksigner and the
# host's own parser (host/cmd/apkcert, every check of spec M6a §5), and
# with --pinned also against the certificate the host pins:
#
#   APKSIGNER=$ANDROID_HOME/build-tools/37.0.0/apksigner sh scripts/release-cert.sh app.apk [--pinned]
set -eu

die() {
	echo "release-cert: $*" >&2
	exit 1
}

main() {
	[ $# -ge 1 ] && [ $# -le 2 ] || die "usage: release-cert.sh <file.apk> [--pinned]"
	case ${2:---} in
	-- | --pinned) ;;
	*) die "unknown option $2" ;;
	esac
	[ -n "${APKSIGNER:-}" ] || die "set APKSIGNER to the apksigner of the build-tools"
	apk=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
	root=$(cd "$(dirname "$0")/.." && pwd)
	out=$(mktemp)
	trap 'rm -f "$out"' EXIT INT TERM

	"$APKSIGNER" verify --verbose --print-certs "$apk" >"$out" || die "apksigner does not verify $1"
	grep -q '^Verified using v1 scheme (JAR signing): false' "$out" || die "$1 has a v1 signature"
	grep -q '^Verified using v2 scheme (APK Signature Scheme v2): true' "$out" || die "$1 has no v2 signature"
	grep -q '^Verified using v3 scheme (APK Signature Scheme v3): true' "$out" || die "$1 has no v3 signature"
	signers=$(grep -c '^Signer #[0-9]* certificate SHA-256 digest: ' "$out" || true)
	[ "$signers" -eq 1 ] || die "$1 has $signers signers, not 1"
	theirs=$(sed -n 's/^Signer #1 certificate SHA-256 digest: //p' "$out")
	ours=$(cd "$root/host" && go run ./cmd/apkcert "$apk") || die "the host refuses $1"
	[ "$ours" = "$theirs" ] || die "apksigner says $theirs, the host says $ours"
	if [ "${2:-}" = --pinned ]; then
		pinned=$(cd "$root/host" && go run ./cmd/apkcert -pinned) || die "the host pins no certificate: see docs/releasing.md"
		[ "$ours" = "$pinned" ] || die "$1 is signed by $ours, but the host pins $pinned"
	fi
	echo "release-cert: $1 is signed by $ours"
}

main "$@"
```

(`apksigner verify --verbose --print-certs` imprime `Verified using vN scheme …: true|false` y `Signer #1 certificate SHA-256 digest: <hex>`, en minúsculas y sin dos puntos.)

- [ ] **Step 2: `.goreleaser.yaml`**

```yaml
# Releases of the adbuddy host (spec M6a §4). .github/workflows/release.yml
# runs it on a vX.Y.Z tag; the signed APK joins the same draft later.
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

# The installer travels with each release: curl … /releases/latest/download/install.sh | sh
# (a change on main reaches nobody until the next release; spec M6a §8).

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
  extra_files:
    - glob: scripts/install.sh
```

(El binario universal de macOS tiene `.Arch` = `all`, así que su archivo es `adbuddy_<v>_darwin_all.tar.gz`, el que espera `install.sh`. `LICENSE` lo añade la Task 11. Si goreleaser falla porque falta, crea antes un `LICENSE` provisional en esta tarea y sustitúyelo en la 11.)

- [ ] **Step 3: `.github/workflows/release.yml`**

```yaml
name: release

# A tag vX.Y.Z or vX.Y.Z-rc.N builds a DRAFT release: the host binaries
# (goreleaser) and the signed APK. The APK is built without secrets, signed
# in a clean job of the "release" environment (a required reviewer, tags v*
# only) with no third-party code, and checked against the host's pin before
# it is uploaded. The maintainer publishes the draft by hand
# (docs/releasing.md). Spec: docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md.
on:
  push:
    tags: ["v*"]

permissions: {}

concurrency:
  group: release-${{ github.ref }}
  cancel-in-progress: false

env:
  BUILD_TOOLS: "37.0.0"

jobs:
  version:
    runs-on: ubuntu-latest
    permissions:
      contents: read
    outputs:
      name: ${{ steps.v.outputs.name }}
      code: ${{ steps.v.outputs.code }}
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
      - id: v
        env:
          TAG: ${{ github.ref_name }}
        run: sh scripts/release-version.sh "$TAG" >> "$GITHUB_OUTPUT"
      - name: The tag is on main
        run: |
          git fetch --no-tags origin main
          git merge-base --is-ancestor "$GITHUB_SHA" origin/main || {
            echo "::error::The tag's commit is not on main: nothing is released."
            exit 1
          }

  test:
    needs: version
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: host/go.mod
          cache: false
      - name: Host tests
        working-directory: host
        run: |
          go vet ./...
          go test -race -timeout 20m ./...
      - uses: actions/setup-java@de7274f081f381c8f8158605e0321c36c376e2e6 # v6.0.1
        with:
          distribution: temurin
          java-version: "17"
      - uses: gradle/actions/setup-gradle@b9bee63ef481cc545e4517ec730544a4d8b1dfa0 # v6.4.0
        with:
          gradle-version: "9.8.0"
          cache-disabled: true
      - name: Install Android SDK platform 37
        run: yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" "platforms;android-37.0"
      - name: App unit tests
        working-directory: android
        run: gradle --no-daemon :app:testDebugUnitTest

  host:
    needs: [version, test]
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: host/go.mod
          cache: false
      - uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3
        with:
          distribution: goreleaser
          version: "~> v2"
          args: release --clean
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  build-apk:
    needs: [version, test]
    runs-on: ubuntu-latest
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-java@de7274f081f381c8f8158605e0321c36c376e2e6 # v6.0.1
        with:
          distribution: temurin
          java-version: "17"
      - uses: gradle/actions/setup-gradle@b9bee63ef481cc545e4517ec730544a4d8b1dfa0 # v6.4.0
        with:
          gradle-version: "9.8.0"
          cache-disabled: true
      - name: Install Android SDK platform 37
        run: yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" "platforms;android-37.0"
      - name: Build the release APK (unsigned)
        working-directory: android
        env:
          NAME: ${{ needs.version.outputs.name }}
          CODE: ${{ needs.version.outputs.code }}
        run: gradle --no-daemon :app:assembleRelease -Padbuddy.versionName="$NAME" -Padbuddy.versionCode="$CODE"
      - uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1
        with:
          name: apk-unsigned
          path: android/app/build/outputs/apk/release/app-release-unsigned.apk
          if-no-files-found: error
          retention-days: 1

  # Signs in a clean VM: no checkout, no Gradle, no Go, no caches and no
  # third-party actions; the secrets only reach the step that signs.
  sign:
    needs: build-apk
    runs-on: ubuntu-latest
    environment: release
    permissions: {}
    steps:
      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1
        with:
          name: apk-unsigned
          path: ${{ runner.temp }}/in
      - name: The artifact is one APK
        run: |
          [ "$(ls -A "$RUNNER_TEMP/in")" = "app-release-unsigned.apk" ] || { echo "::error::unexpected artifact: $(ls -A "$RUNNER_TEMP/in")"; exit 1; }
      - name: Install build-tools
        run: yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" "build-tools;$BUILD_TOOLS"
      - name: Sign
        env:
          KS: ${{ secrets.ADBUDDY_KEYSTORE_B64 }}
          KS_PASS: ${{ secrets.ADBUDDY_KEYSTORE_PASSWORD }}
        run: |
          if [ -z "$KS" ] || [ -z "$KS_PASS" ]; then
            echo "::error::The release environment has no ADBUDDY_KEYSTORE_B64 or ADBUDDY_KEYSTORE_PASSWORD: see docs/releasing.md. The draft release has no APK."
            exit 1
          fi
          umask 077
          ks="$RUNNER_TEMP/release.p12"
          trap 'rm -f "$ks"' EXIT
          printf '%s' "$KS" | base64 -d > "$ks"
          bt="$ANDROID_HOME/build-tools/$BUILD_TOOLS"
          mkdir -p "$RUNNER_TEMP/out"
          "$bt/zipalign" -f 4 "$RUNNER_TEMP/in/app-release-unsigned.apk" "$RUNNER_TEMP/aligned.apk"
          "$bt/apksigner" sign --ks "$ks" --ks-type PKCS12 --ks-pass env:KS_PASS --ks-key-alias adbuddy \
            --v1-signing-enabled false --v2-signing-enabled true --v3-signing-enabled true \
            --out "$RUNNER_TEMP/out/adbuddy.apk" "$RUNNER_TEMP/aligned.apk"
      - name: Remove the keystore
        if: always()
        run: rm -f "$RUNNER_TEMP/release.p12"
      - uses: actions/upload-artifact@043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1
        with:
          name: apk-signed
          path: ${{ runner.temp }}/out/adbuddy.apk
          if-no-files-found: error
          retention-days: 1

  publish-apk:
    needs: [host, sign]
    runs-on: ubuntu-latest
    permissions:
      contents: write
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: host/go.mod
          cache: false
      - uses: actions/download-artifact@3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c # v8.0.1
        with:
          name: apk-signed
          path: signed
      - name: Install build-tools
        run: yes | "$ANDROID_HOME/cmdline-tools/latest/bin/sdkmanager" "build-tools;$BUILD_TOOLS"
      - name: Check the certificate against the host's pin
        run: APKSIGNER="$ANDROID_HOME/build-tools/$BUILD_TOOLS/apksigner" sh scripts/release-cert.sh signed/adbuddy.apk --pinned
      - name: Upload the APK to the draft release
        env:
          GH_TOKEN: ${{ github.token }}
          TAG: ${{ github.ref_name }}
        run: |
          cd signed
          sha256sum adbuddy.apk > adbuddy.apk.sha256
          gh release upload "$TAG" adbuddy.apk adbuddy.apk.sha256 --repo "$GITHUB_REPOSITORY"
```

- [ ] **Step 4: `.github/workflows/release-check.yml`**

```yaml
name: release-check

# Builds everything the release workflow builds, without publishing and
# without secrets (spec M6a §9). pull_request only, never
# pull_request_target. Linux only.
on:
  pull_request:
    paths:
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
    paths:
      - ".goreleaser.yaml"
      - ".github/workflows/release.yml"
      - ".github/workflows/release-check.yml"
      - "scripts/install*.sh"
      - "scripts/release-*.sh"
      - "host/internal/phoneapp/**"
      - "host/cmd/apkcert/**"
      - "android/app/build.gradle.kts"
      - "LICENSE"

permissions:
  contents: read

env:
  BUILD_TOOLS: "37.0.0"

jobs:
  scripts:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - run: shellcheck -s sh scripts/install.sh scripts/install_test.sh scripts/release-*.sh
      - run: sh scripts/release-version_test.sh
      - run: sh scripts/install_test.sh

  goreleaser:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          fetch-depth: 0
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: host/go.mod
          cache-dependency-path: host/go.sum
      - uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3
        with:
          distribution: goreleaser
          version: "~> v2"
          args: check
      - uses: goreleaser/goreleaser-action@f06c13b6b1a9625abc9e6e439d9c05a8f2190e94 # v7.2.3
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
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          persist-credentials: false
      - uses: actions/setup-go@b7ad1dad31e06c5925ef5d2fc7ad053ef454303e # v7.0.0
        with:
          go-version-file: host/go.mod
          cache-dependency-path: host/go.sum
      - uses: actions/setup-java@de7274f081f381c8f8158605e0321c36c376e2e6 # v6.0.1
        with:
          distribution: temurin
          java-version: "17"
      - uses: gradle/actions/setup-gradle@b9bee63ef481cc545e4517ec730544a4d8b1dfa0 # v6.4.0
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
          "$bt/apksigner" sign --ks "$RUNNER_TEMP/test.p12" --ks-type PKCS12 --ks-pass env:KS_PASS --ks-key-alias adbuddy \
            --v1-signing-enabled false --v2-signing-enabled true --v3-signing-enabled true \
            --out "$RUNNER_TEMP/adbuddy.apk" "$RUNNER_TEMP/aligned.apk"
      - name: The host reads the same certificate as apksigner
        run: APKSIGNER="$ANDROID_HOME/build-tools/$BUILD_TOOLS/apksigner" sh scripts/release-cert.sh "$RUNNER_TEMP/adbuddy.apk"
      - name: The APK has the test version
        run: |
          "$ANDROID_HOME/build-tools/$BUILD_TOOLS/aapt2" dump badging "$RUNNER_TEMP/adbuddy.apk" | tee "$RUNNER_TEMP/badging"
          grep -q "versionCode='101' versionName='0.0.1-rc.1'" "$RUNNER_TEMP/badging"
```

- [ ] **Step 5: Comprobaciones locales**

En este PC no hay goreleaser, `actionlint`, `shellcheck` ni JDK. Comprueba lo que se puede:

Run: `sh -n scripts/release-cert.sh && sh scripts/release-version_test.sh && sh scripts/install_test.sh && APKSIGNER=x sh scripts/release-cert.sh a.apk --bad; echo "exit $?"`
Expected: sin errores de sintaxis, los dos tests pasan, y la última orden sale con 1 diciendo "unknown option --bad".

Run (desde `host/`): `go run github.com/goreleaser/goreleaser/v2@latest check -f ../.goreleaser.yaml`
Expected: `config is valid` (un aviso por `LICENSE` es aceptable hasta la Task 11). Si `go run` no puede descargar goreleaser, apúntalo en el ledger y sigue: lo comprueba la CI.

Run: `uv run --with pyyaml python -c "import yaml,sys; [yaml.safe_load(open(f)) for f in sys.argv[1:]]" .goreleaser.yaml .github/workflows/release.yml .github/workflows/release-check.yml`
Expected: sin salida (YAML válido).

Run: `grep -n 'uses:' .github/workflows/release*.yml | grep -v '@[0-9a-f]\{40\} # v'`
Expected: sin salida: todas las acciones están fijadas por SHA.

Run: `grep -n 'secrets\.' .github/workflows/release.yml`
Expected: solo `GITHUB_TOKEN`, en el job `host`, y las dos líneas del paso `Sign` del job `sign`.

Run: `grep -n 'cache' .github/workflows/release.yml`
Expected: solo `cache: false` y `cache-disabled: true`.

- [ ] **Step 6: Commit**

```bash
git add .goreleaser.yaml scripts/release-cert.sh .github/workflows/release.yml .github/workflows/release-check.yml
git commit -m "feat(release): goreleaser, firma aislada en el environment release y workflow de comprobación

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

### Task 11: Clave de release, LICENSE y documentación

**Files:**
- Create: `scripts/release-key.sh`
- Create: `LICENSE`
- Create: `docs/releasing.md`
- Modify: `host/README.md` (la sección `## Build` pasa a `## Install`)
- Modify: `docs/superpowers/plans/pendientes-m3-m5.md` (solo `## M6 · Publicación`)

**Interfaces:**
- Consumes: todo lo anterior; los nombres de los secretos, del environment y del alias, de los Global Constraints.
- Produces: lo que el usuario sigue a mano.

- [ ] **Step 1: `scripts/release-key.sh`**

```sh
#!/bin/sh
# Creates the app's release signing key, once, OUTSIDE the repo
# (docs/releasing.md). Run it yourself in your terminal, never through a
# coding agent. keytool asks for the password: it never goes on a command
# line. Needs keytool: a JDK (winget install EclipseAdoptium.Temurin.21.JDK)
# or Android Studio's jbr/bin.
#
#   sh scripts/release-key.sh
set -eu

die() {
	echo "release-key: $*" >&2
	exit 1
}

main() {
	umask 077
	keytool=keytool
	if ! command -v keytool >/dev/null 2>&1; then
		[ -n "${JAVA_HOME:-}" ] && [ -x "$JAVA_HOME/bin/keytool" ] || die "keytool not found: install a JDK or set JAVA_HOME"
		keytool=$JAVA_HOME/bin/keytool
	fi
	dir=${ADBUDDY_RELEASE_KEY_DIR:-$HOME/.adbuddy-release}
	mkdir -p "$dir"
	dir=$(cd "$dir" && pwd)
	root=$(git -C "$(dirname "$0")" rev-parse --show-toplevel 2>/dev/null || true)
	if [ -n "$root" ]; then
		root=$(cd "$root" && pwd)
		case $dir/ in
		"$root"/*) die "$dir is inside the repo: pick a folder outside it (ADBUDDY_RELEASE_KEY_DIR)" ;;
		esac
	fi
	ks=$dir/release.p12
	[ ! -e "$ks" ] || die "$ks already exists: never replace a release key (installed apps could not update)"
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
	echo "Secrets of the \"release\" environment (create it first, see docs/releasing.md);"
	echo "gh reads each value from stdin or asks for it, and prints nothing:"
	echo "  base64 < \"$ks\" | tr -d '\\n' | gh secret set ADBUDDY_KEYSTORE_B64 --env release --repo hybriqtech/adbuddy"
	echo "  gh secret set ADBUDDY_KEYSTORE_PASSWORD --env release --repo hybriqtech/adbuddy"
}

main "$@"
```

Run: `sh -n scripts/release-key.sh && ADBUDDY_RELEASE_KEY_DIR="$PWD/tmp-key" sh scripts/release-key.sh; echo "exit $?"; rm -rf tmp-key`
Expected: sin errores de sintaxis. La segunda orden sale con 1, con "keytool not found" (no hay JDK) o con "inside the repo". **No** generes ninguna clave real: eso lo hace el usuario.

- [ ] **Step 2: `LICENSE`**

Run: `curl -fsSL https://www.apache.org/licenses/LICENSE-2.0.txt -o LICENSE && head -3 LICENSE && grep -c "END OF TERMS AND CONDITIONS" LICENSE`
Expected: la cabecera "Apache License / Version 2.0, January 2004" y `1`.

El texto no se cambia. El copyright (© 2026 hybriqtech) va en `host/README.md` (Step 4). Si la Task 10 dejó un `LICENSE` provisional, este lo sustituye.

- [ ] **Step 3: `docs/releasing.md`**

````markdown
# Releasing ADBuddy

A release is a git tag. `.github/workflows/release.yml` builds a **draft** release: the host binaries (goreleaser) and the app (`adbuddy.apk` and `adbuddy.apk.sha256`). The app is built without secrets, signed in an isolated job of the `release` environment and checked against the certificate the host pins. Nothing is public until the maintainer approves the signing job and publishes the draft. Design: `docs/superpowers/specs/2026-10-01-adbuddy-m6a-releases-design.md`.

## User actions (once)

Only the repository owner can do these, in their own terminal and on github.com. **Never run them through a coding agent:** the keystore must not end up in a transcript. Nothing else waits for them, but there is no app release until they are done.

1. **Create the signing key**, outside the repo. It needs `keytool`: install a JDK (`winget install EclipseAdoptium.Temurin.21.JDK`) or use Android Studio's `jbr/bin/keytool`.

   ```sh
   sh scripts/release-key.sh
   ```

   It writes `~/.adbuddy-release/release.p12` (private folder), lets `keytool` ask for the password and prints the certificate's SHA-256 and the `gh secret set` commands. It never prints the keystore.
2. **Back up** `release.p12` and its password (a password manager, an encrypted drive). If they are lost, no future release can update the app already on phones: Android only updates an app signed with the same key.
3. **Create the `release` environment** (Settings › Environments › New environment):
   - Deployment branches and tags: *Selected branches and tags*, add the tag rule `v*`.
   - Required reviewers: yourself.
   - Turn off *Allow administrators to bypass configured protection rules*.
4. **Add its two secrets** with the commands the script printed (`--env release`): `ADBUDDY_KEYSTORE_B64` and `ADBUDDY_KEYSTORE_PASSWORD`. Run them in **Git Bash**, not PowerShell: they use `|` and `<` the POSIX way. They are environment secrets, not repository secrets, so a workflow on another branch cannot read them. The key alias is always `adbuddy`.
5. **Rulesets** (Settings › Rules › Rulesets):
   - a tag ruleset on `v*`: restrict creations, updates and deletions; only repository admins bypass;
   - a branch ruleset on `main`: no direct pushes and no force pushes, changes only through pull requests, and **no bypass list** (not even GitHub Actions). The release jobs that may write (`host` and `publish-apk`) then cannot touch `main`.

   With the `main` ruleset on, branches no longer reach `main` with a local fast-forward and `git push`: open a pull request and merge it with **Rebase and merge**. That keeps the history linear, as today.
6. **Pin the certificate:** put the SHA-256 the script printed (lowercase hex, no colons) in `SigningCertSHA256` in `host/internal/phoneapp/release.go`, in its own commit and PR. Until then `adbuddy setup` downloads no app (it says so), and `publish-apk` fails.
7. **Your phone** has a development build signed with another key. Before the first release, uninstall ADBuddy on the phone; it forgets its pairing, and `adbuddy setup` pairs again. Until then, run `adbuddy setup --apk <your build>` once (and again after each new build) so the host trusts that exact APK. `adbuddy serve` says so every time it starts.

## Cutting a release

0. **Before the first public release only:** the emulator checks of validation 3 in `docs/superpowers/plans/2026-10-01-adbuddy-m6a-releases.md` have passed on API 30, 33 and the latest. Do not tag v0.x until they have.
1. `main` is green and holds what you want to ship.
2. If this release fixes a security bug in the app, raise `MinAppVersionCode` in `host/internal/phoneapp/trust.go` to this release's versionCode (`sh scripts/release-version.sh vX.Y.Z`), in the same release.
3. Tag a commit of `main` and push the tag: `git tag v0.3.0 && git push origin v0.3.0`. For a prerelease, use `v0.3.0-rc.1`. Tags must be `vX.Y.Z` or `vX.Y.Z-rc.N`; `scripts/release-version.sh` explains the limits. A tag on a commit that is not on `main` releases nothing.
4. The `release` workflow runs `version`, then `test`, then `host` and `build-apk`, then `sign` (waits for your approval), then `publish-apk`. Approve `sign` when it asks.
5. Open the draft on GitHub and check its assets: 5 archives, `checksums.txt`, `adbuddy.apk` and `adbuddy.apk.sha256`. Check the notes, and press **Publish release**. Prereleases are never `latest`, so `adbuddy setup` and `install.sh` skip them.

If a job fails, delete the draft and the tag (`git push origin :v0.3.0`), fix the problem and tag again.

## What protects the app

- `adbuddy setup` checks a downloaded APK against `adbuddy.apk.sha256`. Then it checks that **every** signer in its v2, v3 and v3.1 blocks has the pinned certificate, and that Android would find the same blocks.
- Before it hands the token to the app, and before it asks for consent or grants a permission, the host reads the **installed** app back from the phone. It checks the same things there, and that its version is not below `MinAppVersionCode`. An app with our name signed by anyone else gets nothing, wherever it came from.
- The installed app must be in `/data/app` (not on adopted storage) and the phone must not be rooted: with root the phone's shell could lie.
- Every app that connects with the token must still be that installation (same path and `stat` of `base.apk`). An update in place keeps the app's data, the token included, so it is refused and the app is checked again before a new token goes out.
- What ADBuddy does not protect against: an agent that can run tools on the PC outside `adbuddy` (it could edit the host's files or run the real adb). The agent environment only pre-approves `adbuddy` commands, and `adbuddy setup` refuses to run from the agent.
- `ADBUDDY_RELEASE_URL` (the e2e tests) only takes `https://` or a loopback `http://`. Another origin could only serve old or current APKs that hybriqtech signed, and `MinAppVersionCode` stops the old ones.
- `adbuddy setup --apk <file>` installs a private copy of a file you choose. If the release key does not sign it, **that exact APK** (its SHA-256) becomes the trusted development build: not its key, because the debug key lives on your PC. `adbuddy serve` says so every time it starts, and a setup that finds a release on the phone stops trusting it.
- Key rotation is not supported: rotating the key needs a host release that reads the v3 lineage first.

## If the key leaks

1. Delete the two secrets of the `release` environment and the leaked keystore.
2. Create a new key (`sh scripts/release-key.sh` with a new `ADBUDDY_RELEASE_KEY_DIR`) and its secrets.
3. Release a host with the new certificate in `SigningCertSHA256` and `MinAppVersionCode` above every release signed with the old key. From then on, new hosts trust nothing signed with the old key.
4. Every user uninstalls the app on the phone and runs `adbuddy setup` again: Android cannot update an app to another key.
5. Say what happened in the README and in the release notes.

## Later (M6c)

Homebrew tap, winget, IzzyOnDroid, SignPath and signed build provenance need accounts, repositories or a public release first: see the M6 section of `docs/superpowers/plans/pendientes-m3-m5.md`.
````

- [ ] **Step 4: `host/README.md`**

Sustituye la sección `## Build` (el título y su bloque de código) por:

````markdown
## Install

- **Linux and macOS:**

  ```sh
  curl -fsSL https://github.com/hybriqtech/adbuddy/releases/latest/download/install.sh | sh
  ```

  It installs the latest release in `~/.local/bin` after checking its SHA-256. `ADBUDDY_INSTALL_DIR` picks another folder, and `ADBUDDY_VERSION=v0.3.0` another release.
- **Windows:** download `adbuddy_<version>_windows_amd64.zip` (or `_arm64`) from the [releases](https://github.com/hybriqtech/adbuddy/releases), check it against `checksums.txt` and put `adbuddy.exe` in a folder in your `PATH`. winget comes later.
- **From source:**

  ```sh
  cd host
  go build -o adbuddy ./cmd/adbuddy      # adbuddy.exe on Windows
  ```

  A build from source pins no signing certificate for the phone app. Install the app with `adbuddy setup --apk <file>`.

Releases: `docs/releasing.md`. Licensed under Apache-2.0 (`LICENSE`), © 2026 hybriqtech.
````

- [ ] **Step 5: `pendientes-m3-m5.md`, solo la sección M6**

En `## M6 · Publicación`, sustituye los dos puntos de `ADBUDDY_RELEASE_URL` y de fijar el SHA-256 del certificado por:

```markdown
- **Hecho en M6a** (`docs/superpowers/plans/2026-10-01-adbuddy-m6a-releases.md`):
  - el host fija el SHA-256 del certificado de firma y lo comprueba en el APK descargado y en la app instalada, antes de cada token y de cada concesión (`phoneapp.Verified`);
  - `ADBUDDY_RELEASE_URL` queda limitado a `https://` o loopback y a `MinAppVersionCode` (spec de M6a §5-§6).

  Falta que el usuario haga lo de `docs/releasing.md`, "User actions".
```

y añade al final de la sección:

```markdown
- **M6c, tras la primera release pública:** tap de Homebrew (`hybriqtech/homebrew-tap` y un token con permiso de escritura), manifiesto de winget (`hybriqtech.adbuddy`, PR a `microsoft/winget-pkgs`), IzzyOnDroid, firma de binarios de Windows con SignPath Foundation y procedencia firmada (`actions/attest-build-provenance`). IzzyOnDroid no acepta el bloque de dependencias cifrado de AGP (`0x504b4453`): antes de pedirlo, `dependenciesInfo { includeInApk = false }` en `android/app/build.gradle.kts`, coordinado con quien lleve `android/` entonces.
- **Rotación de la clave de firma:** no está soportada (spec de M6a §11). Si hace falta, el host tiene que leer el linaje de v3 antes de que salga un APK con la clave nueva.
- **`adbuddy doctor`** no comprueba aún el certificado de la app instalada. Podría decir "app de confianza: release / desarrollo / no" con `phoneapp.Verifier`.
```

- [ ] **Step 6: Comprobación y commit**

Run: `git diff --stat main...HEAD` y comprueba que solo aparecen rutas de los Global Constraints (además de la spec y este plan).

```bash
git add scripts/release-key.sh LICENSE docs/releasing.md host/README.md docs/superpowers/plans/pendientes-m3-m5.md
git commit -m "docs(release): clave de firma, environment release, licencia Apache-2.0 y guía de releases

Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
```

---

## Validaciones manuales pendientes

Lo que no se puede probar en este PC o sin el usuario:

1. **`release-check.yml` en la CI** (goreleaser snapshot, `shellcheck`, `apksigner` contra `apkcert` y versión en el APK). Corre al abrir el PR, cuando el supervisor autorice el push. Si falla, se arregla en la rama antes de integrar. Riesgos que solo se ven ahí:
   - `assembleRelease` con R8 no se había compilado nunca;
   - el nombre `app-release-unsigned.apk` en AGP 9;
   - las entradas que AGP mete en `META-INF/` (no deben ser `.SF`, `.RSA`, `.DSA` ni `.EC`);
   - los bloques que AGP o `apksigner` añaden al Signing Block;
   - las entradas de `setup-java@v6` y `setup-go@v7`.
2. **La app de release (R8) funciona en un móvil:** el APK de `release-check` en un emulador con imagen válida, o en el móvil del usuario **solo con su permiso**, porque obliga a desinstalar el build de debug. Hay que mirar sobre todo la serialización de los mensajes (kotlinx.serialization con R8).
3. **El comportamiento de Android que da por hecho la spec, en un emulador con API 30, 33 y la última, antes de la primera release pública** (ruling de la spec §11; queda como paso que bloquea en `docs/releasing.md`). Hay que comprobar:
   - que los APK de ataque de §5 se comportan como se supone: el hueco antes del EOCD con v1 y `targetSdk` 29, solo v3.1 más v1 en la API 30, y ZIP64. Y que el host los rechaza;
   - la salida de `pm path`, `stat -c '%s %i %Y'` y `base64` de toybox, y el tiempo de leer el APK por Wi-Fi;
   - que una actualización (`install -r`) cambia la ruta, o al menos el `stat`, de `base.apk`;
   - que el `base.apk` instalado sin `--no-incremental` es idéntico byte a byte al fichero;
   - que Android rechaza un provider con la misma autoridad también con el nuestro desactivado.

   En el emulador cuando M2.5b tenga una imagen que funcione; en el Pixel del usuario, solo lo que sea de lectura y con su permiso.
4. **Acciones del usuario** (`docs/releasing.md`, "User actions"): clave, environment `release` con revisor, los dos secretos, ruleset de etiquetas y el PR con `SigningCertSHA256`. Después, la primera etiqueta. Hay que comprobar que el borrador tiene los 9 ficheros y que `adbuddy setup` descarga, verifica e instala la app desde la release publicada.
5. **`gh release upload` a un borrador por nombre de etiqueta:** se ve en la primera release real o en un fork. Si `gh` no encuentra el borrador, se usa el id de la release que crea goreleaser (`gh api repos/{owner}/{repo}/releases --jq '.[] | select(.draft and .tag_name == env.TAG) | .id'` y `gh api … /assets`).
6. **El build de debug del usuario tras M6a:** su próximo `adbuddy serve` no entregará el token a la app de debug que tiene instalada hasta que ejecute `adbuddy setup --apk <ese APK>`. Hay que decírselo al integrar.
