package paymentqr

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"os"
	"strings"
	"testing"
)

func TestIndependentReferenceMatrices(t *testing.T) {
	data, err := os.ReadFile("testdata/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Name    string
		Payload string
		Version int
		Hashes  []string
	}
	if err := json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			// Every cell, including all Reed–Solomon data, format/version bits,
			// remainder bits, and function patterns, must match an independent
			// implementation for each of the eight mask patterns.
			words := codewords(fixture.Payload, fixture.Version)
			for mask, expected := range fixture.Hashes {
				if got := fingerprint(makeSymbol(words, fixture.Version, mask)); got != expected {
					t.Errorf("version %d mask %d: matrix hash %s, want %s", fixture.Version, mask, got, expected)
				}
			}
			img, err := Encode(fixture.Payload)
			if err != nil {
				t.Fatal(err)
			}
			wantBounds := image.Rect(0, 0, 17+fixture.Version*4, 17+fixture.Version*4)
			if img.Bounds() != wantBounds {
				t.Fatalf("bounds %v, want %v", img.Bounds(), wantBounds)
			}
			got := fingerprint(img)
			for _, expected := range fixture.Hashes {
				if got == expected {
					return
				}
			}
			t.Fatalf("Encode returned a matrix absent from independent references: %s", got)
		})
	}
}

func TestPayloadLimits(t *testing.T) {
	for name, payload := range map[string]string{
		"too many bytes":  strings.Repeat("a", 332),
		"multibyte limit": strings.Repeat("ä", 166),
		"invalid UTF-8":   "a\xffb",
	} {
		t.Run(name, func(t *testing.T) {
			if img, err := Encode(payload); err == nil || img != nil {
				t.Fatalf("expected no image and an error; got image %v, error %v", img, err)
			}
		})
	}
}

func TestImageBounds(t *testing.T) {
	img, err := Encode("EPC payment")
	if err != nil {
		t.Fatal(err)
	}
	for _, point := range []image.Point{{-1, 0}, {0, -1}, {img.Bounds().Dx(), 0}, {0, img.Bounds().Dy()}} {
		r, g, b, a := img.At(point.X, point.Y).RGBA()
		if r != 0xffff || g != 0xffff || b != 0xffff || a != 0xffff {
			t.Errorf("out-of-bounds pixel %v is not white", point)
		}
	}
}

func fingerprint(img image.Image) string {
	var matrix strings.Builder
	for y := 0; y < img.Bounds().Dy(); y++ {
		for x := 0; x < img.Bounds().Dx(); x++ {
			r, _, _, _ := img.At(x, y).RGBA()
			if r == 0 {
				matrix.WriteByte('1')
			} else {
				matrix.WriteByte('0')
			}
		}
	}
	digest := sha256.Sum256([]byte(matrix.String()))
	return hex.EncodeToString(digest[:])
}
