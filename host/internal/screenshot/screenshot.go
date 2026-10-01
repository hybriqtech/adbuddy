// Package screenshot scales the phone's screen capture for the agent (M5
// spec §3.6): a box filter and PNG or JPEG, with the standard library only.
package screenshot

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
)

// Formats.
const (
	PNG  = "png"
	JPEG = "jpeg"
)

// MaxPixels bounds the decoded capture (a 4K screen is about 8.3 million):
// screencap's output comes from the phone.
const MaxPixels = 40_000_000

// Scale decodes a PNG from screencap -p, scales it by scale (0 < scale <= 1)
// and encodes it as format. A scale of 1 keeps the size.
func Scale(in []byte, scale float64, format string) ([]byte, error) {
	if !(scale > 0 && scale <= 1) {
		return nil, fmt.Errorf("scale must be more than 0 and at most 1, got %v", scale)
	}
	if format != PNG && format != JPEG {
		return nil, fmt.Errorf("format must be png or jpeg, got %q", format)
	}
	if len(in) == 0 {
		return nil, ErrEmpty
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(in))
	if err != nil {
		return nil, fmt.Errorf("the screen capture is not a PNG: %w", err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > MaxPixels {
		return nil, fmt.Errorf("the screen capture is %d×%d", cfg.Width, cfg.Height)
	}
	src, err := png.Decode(bytes.NewReader(in))
	if err != nil {
		return nil, fmt.Errorf("the screen capture is not a PNG: %w", err)
	}
	img := src
	if scale < 1 {
		img = box(src, max(1, int(float64(cfg.Width)*scale+0.5)), max(1, int(float64(cfg.Height)*scale+0.5)))
	}
	var out bytes.Buffer
	if format == JPEG {
		err = jpeg.Encode(&out, img, &jpeg.Options{Quality: 80})
	} else {
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&out, img)
	}
	if err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// ErrEmpty is a capture with no bytes (screencap failed silently).
var ErrEmpty = errors.New("the screen capture is empty")

// box scales src to w×h, averaging the source pixels each target pixel
// covers.
func box(src image.Image, w, h int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	px := pixels(src)
	dst := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		y0, y1 := y*sh/h, max((y+1)*sh/h, y*sh/h+1)
		for x := range w {
			x0, x1 := x*sw/w, max((x+1)*sw/w, x*sw/w+1)
			var r, g, bl, a, n uint64
			for sy := y0; sy < y1; sy++ {
				for sx := x0; sx < x1; sx++ {
					cr, cg, cb, ca := px(b.Min.X+sx, b.Min.Y+sy)
					r, g, bl, a, n = r+uint64(cr), g+uint64(cg), bl+uint64(cb), a+uint64(ca), n+1
				}
			}
			if a == 0 {
				continue // transparent
			}
			// From premultiplied 16-bit sums back to straight 8-bit.
			i := dst.PixOffset(x, y)
			dst.Pix[i+0] = uint8(r * 0xff / a)
			dst.Pix[i+1] = uint8(g * 0xff / a)
			dst.Pix[i+2] = uint8(bl * 0xff / a)
			dst.Pix[i+3] = uint8(a / n >> 8)
		}
	}
	return dst
}

// pixels reads premultiplied 16-bit components, without At's allocation for
// the two kinds png.Decode returns for screencap's output.
func pixels(src image.Image) func(x, y int) (r, g, b, a uint32) {
	switch m := src.(type) {
	case *image.NRGBA:
		return func(x, y int) (uint32, uint32, uint32, uint32) {
			i := m.PixOffset(x, y)
			a := uint32(m.Pix[i+3])
			return uint32(m.Pix[i]) * a * 0x101 / 0xff, uint32(m.Pix[i+1]) * a * 0x101 / 0xff, uint32(m.Pix[i+2]) * a * 0x101 / 0xff, a * 0x101
		}
	case *image.RGBA:
		return func(x, y int) (uint32, uint32, uint32, uint32) {
			i := m.PixOffset(x, y)
			return uint32(m.Pix[i]) * 0x101, uint32(m.Pix[i+1]) * 0x101, uint32(m.Pix[i+2]) * 0x101, uint32(m.Pix[i+3]) * 0x101
		}
	}
	return func(x, y int) (uint32, uint32, uint32, uint32) { return src.At(x, y).RGBA() }
}
