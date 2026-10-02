package cli

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	_ "image/gif" // registered for image.DecodeConfig and image.Decode
	"image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Helpers of the files of the chat, shared by uploads and captures
// (phone_files.go) and by the files the agent sends (task 9).

const (
	maxNameBytes  = 100       // of a saved name, without the date
	thumbSide     = 256       // px, the longer side
	maxThumbBytes = 16 << 10  // protocol spec §4.2
	maxThumbPixel = 40 << 20  // 40 Mpx: more is not decoded at all
	fallbackName  = "fichero" // what an empty or dotted name becomes
)

// reservedNames are the device names of Windows, which cannot be a file name
// there, with or without an extension.
var reservedNames = func() map[string]bool {
	m := map[string]bool{"CON": true, "PRN": true, "AUX": true, "NUL": true}
	for i := 1; i <= 9; i++ {
		m[fmt.Sprintf("COM%d", i)] = true
		m[fmt.Sprintf("LPT%d", i)] = true
	}
	return m
}()

// safeName turns a name the phone or the agent chose into one that is safe to
// create on any system (spec M3 §6.1): only the base name, no control nor
// <>:"|?* characters (a:b is a stream of a file on Windows), no dots or
// spaces at the end, no reserved names of Windows, at most maxNameBytes bytes
// without breaking an UTF-8 sequence nor the extension; an empty name, "." or
// ".." becomes "fichero". It does not make the name unique: createExclusive
// does.
func safeName(name string) string {
	if i := strings.LastIndexAny(name, `/\`); i >= 0 {
		name = name[i+1:]
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r == utf8.RuneError, unicode.IsControl(r), unicode.In(r, unicode.Cf, unicode.Zl, unicode.Zp), strings.ContainsRune(`<>:"|?*`, r):
			b.WriteByte('_')
		default:
			b.WriteRune(r)
		}
	}
	name = trimName(b.String())
	if name == "" {
		return fallbackName
	}
	stem, _, _ := strings.Cut(name, ".")
	if reservedNames[strings.ToUpper(stem)] {
		name = "_" + name
	}
	if len(name) > maxNameBytes {
		name = cutName(name)
		if name == "" {
			return fallbackName
		}
	}
	return name
}

func trimName(s string) string {
	return strings.TrimLeft(strings.TrimRight(s, ". "), " ")
}

// cutName shortens name to maxNameBytes keeping a short extension.
func cutName(name string) string {
	ext := filepath.Ext(name)
	if len(ext) > 20 || ext == name {
		ext = ""
	}
	stem := cutRunes(name[:len(name)-len(ext)], maxNameBytes-len(ext))
	return trimName(stem) + ext
}

// cutRunes is s cut to at most n bytes at a character boundary.
func cutRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// candidates calls try with stem+ext, then stem-2+ext, stem-3+ext… until it
// returns something other than "exists".
func candidates(stem, ext string, try func(name string) error) (string, error) {
	for i := 1; i <= 10000; i++ {
		name := stem + ext
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		err := try(name)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return name, err
	}
	return "", fs.ErrExist
}

// createExclusive creates dir/stem+ext, or with -2, -3… before the extension
// if that name exists: the name is taken by creating it (O_EXCL), so two at
// once never get the same one.
func createExclusive(dir, stem, ext string) (*os.File, string, error) {
	var f *os.File
	name, err := candidates(stem, ext, func(name string) (err error) {
		f, err = os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return f, name, nil
}

// createExclusiveIn is createExclusive in the folder of root.
func createExclusiveIn(root *os.Root, stem, ext string) (*os.File, string, error) {
	var f *os.File
	name, err := candidates(stem, ext, func(name string) (err error) {
		f, err = root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		return err
	})
	if err != nil {
		return nil, "", err
	}
	return f, name, nil
}

var errMoveChanged = errors.New("the file to move was replaced")

// moveExclusive makes src (relative to root) the file stem+ext of root (or
// its -2, -3… variant) and removes src. A hard link takes the name atomically
// and never overwrites (os.Rename does, on Unix); where it is not possible
// the file is copied into a name created with O_EXCL. want is the file that
// must be moved: a src replaced meanwhile (by another file, or by a link,
// which the hard link would take as it is) is not moved.
func moveExclusive(root *os.Root, src, stem, ext string, want fs.FileInfo) (string, error) {
	name, err := candidates(stem, ext, func(name string) error {
		return root.Link(src, name)
	})
	if err == nil {
		if st, err := root.Lstat(name); err != nil || !os.SameFile(want, st) {
			root.Remove(name)
			return "", errMoveChanged
		}
		return name, root.Remove(src)
	}
	if errors.Is(err, fs.ErrExist) {
		return "", err
	}
	in, err := root.Open(src)
	if err != nil {
		return "", err
	}
	defer in.Close()
	if st, err := in.Stat(); err != nil || !os.SameFile(want, st) {
		return "", errMoveChanged
	}
	out, name, err := createExclusiveIn(root, stem, ext)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(out, in)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		root.Remove(name)
		return "", err
	}
	return name, root.Remove(src)
}

// decodeImage decodes the whole image: a variable so that a test can see it
// is not called for what is too large.
var decodeImage = image.Decode

// thumbnail returns the base64 JPEG thumbnail (at most 16 KiB, 256 px wide)
// of the PNG, JPEG or GIF at path, or "" when it is none of them, cannot be
// read or declares more than 40 Mpx: the header is read first, so a small
// file that declares enormous dimensions is never decoded.
func thumbnail(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	cfg, format, err := image.DecodeConfig(f)
	if err != nil || (format != "png" && format != "jpeg" && format != "gif") {
		return ""
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > maxThumbPixel {
		return ""
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return ""
	}
	src, _, err := decodeImage(f)
	if err != nil {
		return ""
	}
	side := min(max(src.Bounds().Dx(), src.Bounds().Dy()), thumbSide)
	for range 6 {
		small := scaleTo(src, side)
		for _, q := range []int{80, 60, 40, 25, 10} {
			var b bytes.Buffer
			if jpeg.Encode(&b, small, &jpeg.Options{Quality: q}) == nil && b.Len() <= maxThumbBytes {
				return base64.StdEncoding.EncodeToString(b.Bytes())
			}
		}
		side /= 2
		if side < 8 {
			break
		}
	}
	return ""
}

// scaleTo is src reduced so that its longer side is side pixels (a smaller
// image is not enlarged), over white (a JPEG has no transparency), sampling a
// small grid of points of src for each pixel.
func scaleTo(src image.Image, side int) *image.RGBA {
	sb := src.Bounds()
	w, h := sb.Dx(), sb.Dy()
	if long := max(w, h); long > side {
		w, h = max(1, w*side/long), max(1, h*side/long)
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(dst, dst.Bounds(), image.NewUniform(color.White), image.Point{}, draw.Src)
	const grid = 3
	for y := range h {
		for x := range w {
			var r, g, b, n uint32
			for j := range grid {
				for i := range grid {
					sx := sb.Min.X + (x*grid+i)*sb.Dx()/(w*grid)
					sy := sb.Min.Y + (y*grid+j)*sb.Dy()/(h*grid)
					cr, cg, cb, ca := src.At(sx, sy).RGBA()
					// over white: c + (1-a)·white, all in 16 bits
					r += cr + (0xffff - ca)
					g += cg + (0xffff - ca)
					b += cb + (0xffff - ca)
					n++
				}
			}
			dst.SetRGBA(x, y, color.RGBA{uint8(r / n >> 8), uint8(g / n >> 8), uint8(b / n >> 8), 255})
		}
	}
	return dst
}
