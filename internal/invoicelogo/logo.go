// Package invoicelogo prepares the optional company logo printed on invoices.
// Uploaded PNG/JPEG files are normalized once into a compact 8-bit PNG so the
// HTML preview and the PDF/A renderer embed identical pixels at identical sizes.
package invoicelogo

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/jpeg"
	"image/png"
	"math"
	"strings"
)

const (
	// MaxFileSize bounds the uploaded original before decoding.
	MaxFileSize = 10 << 20
	// Larger originals are rejected before allocating their pixel buffers.
	maxSourcePixels = 40_000_000
	// Stored pixels: about 500 dpi at the largest printed size, which keeps
	// the base64 value small enough for settings, backups and issued snapshots.
	maxWidth, maxHeight = 900, 360
	// Printed bounding box in the invoice head (right column, above the number).
	BoxWidthMM, BoxHeightMM = 45.0, 20.0
	// Below this resolution a logo printed at full box size looks blurred.
	minPrintDPI = 150
)

// Logo is a normalized logo as stored in the company profile.
type Logo struct {
	PNG           []byte
	Width, Height int
}

// Normalize decodes a PNG or JPEG upload, removes empty transparent or white
// margins, scales it down to the stored maximum and re-encodes it as PNG.
// Metadata, colour profiles and animation chunks of the original are dropped.
func Normalize(data []byte) (Logo, error) {
	if len(data) == 0 {
		return Logo{}, errors.New("Die Logo-Datei ist leer.")
	}
	if len(data) > MaxFileSize {
		return Logo{}, errors.New("Das Logo darf höchstens 10 MB groß sein.")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || (format != "png" && format != "jpeg") {
		return Logo{}, errors.New("Wähle ein Logo im Format PNG oder JPG.")
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > maxSourcePixels {
		return Logo{}, errors.New("Das Logo ist zu groß. Verwende eine Datei mit höchstens 40 Megapixeln.")
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return Logo{}, errors.New("Die Logo-Datei lässt sich nicht lesen. Speichere sie noch einmal als PNG oder JPG.")
	}
	src := image.NewNRGBA(image.Rect(0, 0, decoded.Bounds().Dx(), decoded.Bounds().Dy()))
	draw.Draw(src, src.Bounds(), decoded, decoded.Bounds().Min, draw.Src)
	content := visibleBounds(src)
	if content.Empty() {
		return Logo{}, errors.New("Das Logo enthält keine sichtbaren Inhalte. Ein weißes oder transparentes Logo ist auf der Rechnung nicht zu sehen.")
	}
	src = src.SubImage(content).(*image.NRGBA)
	w, h := content.Dx(), content.Dy()
	if w > maxWidth || h > maxHeight {
		scale := min(float64(maxWidth)/float64(w), float64(maxHeight)/float64(h))
		w, h = max(1, int(float64(w)*scale+0.5)), max(1, int(float64(h)*scale+0.5))
		src = resize(src, w, h)
	} else {
		copied := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.Draw(copied, copied.Bounds(), src, content.Min, draw.Src)
		src = copied
	}
	var out bytes.Buffer
	if err := (&png.Encoder{CompressionLevel: png.BestCompression}).Encode(&out, src); err != nil {
		return Logo{}, err
	}
	return Logo{PNG: out.Bytes(), Width: w, Height: h}, nil
}

// Decode reads a logo stored in the company profile (base64 PNG) and checks
// that it is a normalized logo. An empty value means no logo.
func Decode(encoded string) (*Logo, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, nil
	}
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("Das gespeicherte Logo ist beschädigt. Lade es noch einmal hoch.")
	}
	cfg, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != "png" || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > maxWidth || cfg.Height > maxHeight {
		return nil, errors.New("Das gespeicherte Logo ist ungültig. Lade es noch einmal hoch.")
	}
	return &Logo{PNG: data, Width: cfg.Width, Height: cfg.Height}, nil
}

// Encode returns the profile representation of a normalized logo.
func (l Logo) Encode() string { return base64.StdEncoding.EncodeToString(l.PNG) }

// PrintSize fits the logo into the invoice's logo box, preserving its aspect ratio.
func (l Logo) PrintSize() (widthMM, heightMM float64) {
	widthMM = BoxWidthMM
	heightMM = widthMM * float64(l.Height) / float64(l.Width)
	if heightMM > BoxHeightMM {
		heightMM = BoxHeightMM
		widthMM = heightMM * float64(l.Width) / float64(l.Height)
	}
	return widthMM, heightMM
}

// Warning explains a technically valid logo that will not print sharply.
func (l Logo) Warning() string {
	widthMM, _ := l.PrintSize()
	if dpi := float64(l.Width) / (widthMM / 25.4); dpi < minPrintDPI {
		return fmt.Sprintf("Das Logo hat eine geringe Auflösung (%d × %d Pixel) und kann im Druck unscharf wirken. Empfohlen sind mindestens %d Pixel Breite.", l.Width, l.Height, int(math.Ceil(BoxWidthMM/25.4*minPrintDPI/10))*10)
	}
	return ""
}

// visibleBounds is the smallest rectangle containing pixels that are visible
// on white paper. Transparent and near-white margins are padding, not content.
func visibleBounds(img *image.NRGBA) image.Rectangle {
	b := img.Bounds()
	minX, minY, maxX, maxY := b.Max.X, b.Max.Y, b.Min.X, b.Min.Y
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := img.Pix[img.PixOffset(b.Min.X, y):]
		for x := b.Min.X; x < b.Max.X; x++ {
			p := row[(x-b.Min.X)*4:]
			a := int(p[3])
			// Composite over white; background if every channel stays near white.
			visible := false
			for _, c := range p[:3] {
				if (int(c)*a+255*(255-a))/255 < 245 {
					visible = true
					break
				}
			}
			if !visible {
				continue
			}
			minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x+1), max(maxY, y+1)
		}
	}
	if minX >= maxX || minY >= maxY {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX, maxY)
}

// resize downsamples with exact area averaging in premultiplied alpha, so
// edges of transparent logos keep their colour. Source rows are streamed into
// destination accumulators; memory is bounded by the destination size.
func resize(src *image.NRGBA, dw, dh int) *image.NRGBA {
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	xs, ys := float64(sw)/float64(dw), float64(sh)/float64(dh)
	acc := make([]float64, dw*dh*4)
	row := make([]float64, dw*4)
	for sy := 0; sy < sh; sy++ {
		clear(row)
		line := src.Pix[src.PixOffset(b.Min.X, b.Min.Y+sy):]
		for sx := 0; sx < sw; sx++ {
			p := line[sx*4:]
			alpha := float64(p[3])
			r, g, bl := float64(p[0])*alpha, float64(p[1])*alpha, float64(p[2])*alpha
			d0, d1 := float64(sx)/xs, float64(sx+1)/xs
			for dx := int(d0); dx < dw && float64(dx) < d1; dx++ {
				w := min(d1, float64(dx+1)) - max(d0, float64(dx))
				if w <= 0 {
					continue
				}
				o := dx * 4
				row[o] += r * w
				row[o+1] += g * w
				row[o+2] += bl * w
				row[o+3] += alpha * w
			}
		}
		d0, d1 := float64(sy)/ys, float64(sy+1)/ys
		for dy := int(d0); dy < dh && float64(dy) < d1; dy++ {
			w := min(d1, float64(dy+1)) - max(d0, float64(dy))
			if w <= 0 {
				continue
			}
			target := acc[dy*dw*4 : (dy+1)*dw*4]
			for i, v := range row {
				target[i] += v * w
			}
		}
	}
	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))
	for i := 0; i < len(acc); i += 4 {
		alpha := acc[i+3]
		dst.Pix[i+3] = clamp(alpha)
		if alpha > 0 {
			dst.Pix[i] = clamp(acc[i] / alpha)
			dst.Pix[i+1] = clamp(acc[i+1] / alpha)
			dst.Pix[i+2] = clamp(acc[i+2] / alpha)
		}
	}
	return dst
}

func clamp(v float64) uint8 {
	return uint8(min(255, max(0, v+0.5)))
}
