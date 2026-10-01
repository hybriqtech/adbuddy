package cli

import (
	"bytes"
	"encoding/base64"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSafeName(t *testing.T) {
	long := strings.Repeat("ñ🙂a", 100) + ".txt"
	for _, c := range []struct{ in, want string }{
		{`..\x`, "x"},
		{`../x`, "x"},
		{`C:\dir\sub\file.txt`, "file.txt"},
		{"/etc/passwd", "passwd"},
		{"CON", "_CON"},
		{"con.txt", "_con.txt"},
		{"nul.tar.gz", "_nul.tar.gz"},
		{"COM1", "_COM1"},
		{"lpt9.log", "_lpt9.log"},
		{"console.txt", "console.txt"},
		{"a:b", "a_b"},
		{`a<b>c"d|e?f*g`, "a_b_c_d_e_f_g"},
		{"x. ", "x"},
		{"x...", "x"},
		{"a\x00b\x1b[31m\n.txt", "a_b_[31m_.txt"},
		{"", "fichero"},
		{".", "fichero"},
		{"..", "fichero"},
		{" . ", "fichero"},
		{"/", "fichero"},
		{"foto 1.jpg", "foto 1.jpg"},
		{".bashrc", ".bashrc"},
		{"ñandú.png", "ñandú.png"},
	} {
		if got := safeName(c.in); got != c.want {
			t.Errorf("safeName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	got := safeName(long)
	if len(got) > 100 || !utf8.ValidString(got) || !strings.HasSuffix(got, ".txt") || len(got) < 90 {
		t.Errorf("long name: %d bytes, valid %v, %q", len(got), utf8.ValidString(got), got)
	}
	// A long name without an extension, and one whose "extension" is the rest.
	got = safeName(strings.Repeat("🙂", 80))
	if len(got) > 100 || !utf8.ValidString(got) || got == "" {
		t.Errorf("long emoji name: %d bytes valid %v", len(got), utf8.ValidString(got))
	}
	got = safeName(strings.Repeat("x", 300) + "." + strings.Repeat("e", 150))
	if len(got) > 100 || !utf8.ValidString(got) {
		t.Errorf("long extension: %d bytes", len(got))
	}
}

func TestExclusiveNames(t *testing.T) {
	dir := t.TempDir()
	var got []string
	for range 4 {
		f, name, err := createExclusive(dir, "20260101-000000-foto", ".jpg")
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		got = append(got, name)
	}
	want := []string{"20260101-000000-foto.jpg", "20260101-000000-foto-2.jpg", "20260101-000000-foto-3.jpg", "20260101-000000-foto-4.jpg"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func writeImage(t *testing.T, path string, w, h int, kind string) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), uint8(x ^ y), 255})
		}
	}
	var b bytes.Buffer
	switch kind {
	case "png":
		png.Encode(&b, img)
	case "jpeg":
		jpeg.Encode(&b, img, nil)
	case "gif":
		gif.Encode(&b, img, nil)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestThumbnails(t *testing.T) {
	dir := t.TempDir()
	for _, kind := range []string{"png", "jpeg", "gif"} {
		p := filepath.Join(dir, "img."+kind)
		writeImage(t, p, 800, 400, kind)
		th := thumbnail(p)
		if th == "" {
			t.Fatalf("%s: no thumbnail", kind)
		}
		raw, err := base64.StdEncoding.DecodeString(th)
		if err != nil || len(raw) > 16<<10 {
			t.Fatalf("%s: %d bytes %v", kind, len(raw), err)
		}
		img, err := jpeg.Decode(bytes.NewReader(raw))
		if err != nil {
			t.Fatalf("%s: not a JPEG: %v", kind, err)
		}
		if b := img.Bounds(); b.Dx() != 256 || b.Dy() != 128 {
			t.Fatalf("%s: %v, want 256x128", kind, b)
		}
	}
	// A small image is not enlarged.
	p := filepath.Join(dir, "small.png")
	writeImage(t, p, 40, 30, "png")
	raw, _ := base64.StdEncoding.DecodeString(thumbnail(p))
	if img, err := jpeg.Decode(bytes.NewReader(raw)); err != nil || img.Bounds().Dx() != 40 {
		t.Fatalf("small: %v %v", img, err)
	}
	// Not an image, nor a missing file.
	os.WriteFile(filepath.Join(dir, "x.txt"), []byte("hello"), 0o600)
	if th := thumbnail(filepath.Join(dir, "x.txt")); th != "" {
		t.Fatal("thumbnail of text")
	}
	if th := thumbnail(filepath.Join(dir, "nothing")); th != "" {
		t.Fatal("thumbnail of nothing")
	}
}

// Noisy images do not fit 16 KiB at the first quality: the thumbnail still does.
func TestThumbnailFitsTheLimit(t *testing.T) {
	dir := t.TempDir()
	img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	seed := uint32(1)
	for i := range img.Pix {
		seed = seed*1664525 + 1013904223
		img.Pix[i] = byte(seed >> 24)
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	p := filepath.Join(dir, "noise.png")
	os.WriteFile(p, b.Bytes(), 0o600)
	raw, err := base64.StdEncoding.DecodeString(thumbnail(p))
	if err != nil || len(raw) == 0 || len(raw) > 16<<10 {
		t.Fatalf("%d bytes %v", len(raw), err)
	}
}

// A small PNG can declare enormous dimensions: only its header is read.
func TestThumbnailOfAHugePNGIsNotDecoded(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("\x89PNG\r\n\x1a\n")
	ihdr := []byte{'I', 'H', 'D', 'R', 0, 1, 0x86, 0xa0, 0, 1, 0x86, 0xa0, 8, 6, 0, 0, 0}
	b.Write([]byte{0, 0, 0, 13})
	b.Write(ihdr)
	crc := crc32.ChecksumIEEE(ihdr)
	b.Write([]byte{byte(crc >> 24), byte(crc >> 16), byte(crc >> 8), byte(crc)})
	p := filepath.Join(t.TempDir(), "huge.png")
	os.WriteFile(p, b.Bytes(), 0o600)
	cfg, err := png.DecodeConfig(bytes.NewReader(b.Bytes()))
	if err != nil || cfg.Width != 100000 || cfg.Height != 100000 {
		t.Fatalf("test image: %+v %v", cfg, err)
	}
	setVar(t, &decodeImage, func(io.Reader) (image.Image, string, error) {
		t.Error("the whole image was decoded")
		return nil, "", errors.New("no")
	})
	if th := thumbnail(p); th != "" {
		t.Fatal("a thumbnail of 100000x100000")
	}
}

// The longer side is the one that is reduced: a very tall image does not
// become 256 x N.
func TestThumbnailOfATallImage(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "tall.png")
	writeImage(t, p, 100, 2000, "png")
	raw, err := base64.StdEncoding.DecodeString(thumbnail(p))
	if err != nil || len(raw) == 0 || len(raw) > 16<<10 {
		t.Fatalf("%d bytes %v", len(raw), err)
	}
	img, err := jpeg.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dy() != 256 || b.Dx() != 12 {
		t.Fatalf("%v, want 12x256", b)
	}
}
