// Package images makes web-sized copies of uploaded pictures. Flyers come
// off phones and design tools at full resolution — the imported archive
// averages 0.7MB and reaches 12MB — which is far more than a page needs.
package images

import (
	"image"
	"image/jpeg"
	_ "image/png" // register the PNG decoder
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/image/draw"
)

// MaxWidth is the widest a web copy gets. The flyer column on an event
// page is far narrower than this even on a desktop display, so this
// leaves room for high-density screens without shipping the original.
const MaxWidth = 1200

// jpegQuality trades a little fidelity for a lot of bytes on the
// photographic and poster-like images flyers tend to be.
const jpegQuality = 82

// WebName is the filename a web copy is stored under. Everything is
// re-encoded as JPEG, so the extension changes.
func WebName(original string) string {
	return strings.TrimSuffix(original, filepath.Ext(original)) + ".jpg"
}

// MakeWeb writes a web-sized copy of src to dst. An image already at or
// below MaxWidth is re-encoded rather than enlarged.
//
// A file this package can't decode — a format without a registered
// decoder, or something corrupt — is copied through unchanged, so a web
// copy always exists at dst and callers never have to handle a gap.
func MakeWeb(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	decoded, _, err := image.Decode(in)
	if err != nil {
		return copyThrough(src, dst)
	}

	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width > MaxWidth {
		height = height * MaxWidth / width
		width = MaxWidth
	}

	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	// CatmullRom is the slowest of the kernels here and the only one that
	// keeps small text on a flyer legible after a big reduction.
	draw.CatmullRom.Scale(resized, resized.Bounds(), decoded, bounds, draw.Over, nil)

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if err := jpeg.Encode(out, resized, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return err
	}
	return out.Close()
}

func copyThrough(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
