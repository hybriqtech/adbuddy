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
