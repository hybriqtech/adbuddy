package screenshot

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
	"time"
)

// capture makes a PNG like screencap's: w×h, opaque, left half red and right
// half blue.
func capture(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.NRGBA{R: 255, A: 255}
			if x >= w/2 {
				c = color.NRGBA{B: 255, A: 255}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestScale(t *testing.T) {
	in := capture(t, 1344, 2992)
	start := time.Now()
	out, err := Scale(in, 0.5, PNG)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("scaled in %v", time.Since(start))
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 672 || b.Dy() != 1496 {
		t.Fatalf("size %v", b)
	}
	if r, g, b, a := img.At(10, 10).RGBA(); r>>8 != 255 || g != 0 || b != 0 || a>>8 != 255 {
		t.Fatalf("left pixel %d %d %d %d", r, g, b, a)
	}
	if r, _, b, _ := img.At(660, 10).RGBA(); r != 0 || b>>8 != 255 {
		t.Fatalf("right pixel %d %d", r, b)
	}
}

func TestScaleJPEGAndFullSize(t *testing.T) {
	in := capture(t, 100, 60)
	out, err := Scale(in, 1, JPEG)
	if err != nil {
		t.Fatal(err)
	}
	img, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != 100 || b.Dy() != 60 {
		t.Fatalf("size %v", b)
	}
	// Tiny scales keep at least one pixel.
	out, err = Scale(in, 0.001, PNG)
	if err != nil {
		t.Fatal(err)
	}
	if img, _ := png.Decode(bytes.NewReader(out)); img.Bounds().Dx() != 1 || img.Bounds().Dy() != 1 {
		t.Fatalf("size %v", img.Bounds())
	}
}

func TestScaleRejects(t *testing.T) {
	in := capture(t, 10, 10)
	for _, c := range []struct {
		in     []byte
		scale  float64
		format string
	}{
		{in, 0, PNG}, {in, 1.5, PNG}, {in, -1, PNG}, {in, 0.5, "gif"},
		{[]byte("not a png"), 0.5, PNG},
	} {
		if _, err := Scale(c.in, c.scale, c.format); err == nil {
			t.Errorf("Scale(%q…, %v, %q): no error", c.in[:4], c.scale, c.format)
		}
	}
	if _, err := Scale(nil, 0.5, PNG); !errors.Is(err, ErrEmpty) {
		t.Errorf("empty: %v", err)
	}
}
