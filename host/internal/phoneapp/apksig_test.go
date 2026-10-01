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
