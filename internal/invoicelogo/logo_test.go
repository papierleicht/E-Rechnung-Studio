package invoicelogo

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"math"
	"strings"
	"testing"
)

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

// A 3000×1600 canvas with a 2400×800 coloured mark and transparent padding.
func paddedLogo() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, 3000, 1600))
	for y := 400; y < 1200; y++ {
		for x := 300; x < 2700; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 57, G: 126, B: 107, A: 255})
		}
	}
	return img
}

func TestNormalizeTrimsAndScalesIntoStoredBounds(t *testing.T) {
	logo, err := Normalize(encodePNG(t, paddedLogo()))
	if err != nil {
		t.Fatal(err)
	}
	// 2400×800 (3:1) fits 900×360 as 900×300.
	if logo.Width != 900 || logo.Height != 300 {
		t.Fatalf("unexpected size %d×%d", logo.Width, logo.Height)
	}
	decoded, err := Decode(logo.Encode())
	if err != nil || decoded.Width != 900 || decoded.Height != 300 {
		t.Fatalf("stored logo not readable: %+v %v", decoded, err)
	}
	img, err := png.Decode(bytes.NewReader(logo.PNG))
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, a := img.At(450, 150).RGBA(); r>>8 != 57 || g>>8 != 126 || b>>8 != 107 || a>>8 != 255 {
		t.Fatalf("colour changed by scaling: %d %d %d %d", r>>8, g>>8, b>>8, a>>8)
	}
	if w, h := logo.PrintSize(); w != BoxWidthMM || math.Abs(h-15) > 0.001 {
		t.Fatalf("unexpected print size %.2f×%.2f", w, h)
	}
	if logo.Warning() != "" {
		t.Fatal("sharp logo reported as low resolution")
	}
}

func TestNormalizeKeepsSmallLogosAndWarns(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 120, 120))
	for y := 10; y < 110; y++ {
		for x := 10; x < 110; x++ {
			img.Set(x, y, color.RGBA{R: 200, A: 255})
		}
	}
	// White JPEG padding is trimmed like transparent padding.
	for y := 0; y < 120; y++ {
		for x := 0; x < 120; x++ {
			if x < 10 || y < 10 || x >= 110 || y >= 110 {
				img.Set(x, y, color.White)
			}
		}
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 100}); err != nil {
		t.Fatal(err)
	}
	logo, err := Normalize(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if logo.Width < 98 || logo.Width > 102 || logo.Height < 98 || logo.Height > 102 {
		t.Fatalf("small logo was rescaled or not trimmed: %d×%d", logo.Width, logo.Height)
	}
	// Square logos are limited by the box height.
	if w, h := logo.PrintSize(); h != BoxHeightMM || math.Abs(w-BoxHeightMM*float64(logo.Width)/float64(logo.Height)) > 0.001 {
		t.Fatalf("unexpected print size %.2f×%.2f", w, h)
	}
	if !strings.Contains(logo.Warning(), "geringe Auflösung") {
		t.Fatal("missing low resolution warning")
	}
}

func TestResizeKeepsTransparentEdgeColour(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 4, 1))
	img.SetNRGBA(0, 0, color.NRGBA{R: 255, A: 255})
	img.SetNRGBA(1, 0, color.NRGBA{G: 255, A: 0}) // invisible colour must not bleed
	got := resize(img, 2, 1).NRGBAAt(0, 0)
	if got.R != 255 || got.G != 0 || got.A != 128 {
		t.Fatalf("premultiplied average wrong: %+v", got)
	}
}

func TestRejectsUnsupportedAndInvisibleLogos(t *testing.T) {
	for name, data := range map[string][]byte{
		"empty":     nil,
		"text":      []byte("<svg></svg>"),
		"invisible": encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 50, 50))),
	} {
		if _, err := Normalize(data); err == nil {
			t.Fatalf("%s logo accepted", name)
		}
	}
	for _, stored := range []string{"%%%", "aGVsbG8="} {
		if _, err := Decode(stored); err == nil {
			t.Fatalf("invalid stored logo %q accepted", stored)
		}
	}
	if logo, err := Decode(" "); logo != nil || err != nil {
		t.Fatal("empty stored logo must mean no logo")
	}
}
