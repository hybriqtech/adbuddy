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
