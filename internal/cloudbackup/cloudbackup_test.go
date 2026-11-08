package cloudbackup

import (
	"bytes"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

func mustKey(t *testing.T) Key {
	t.Helper()
	k, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestKeyFormatAndParsing(t *testing.T) {
	k := mustKey(t)
	s := k.String()
	if groups := strings.Split(s, "-"); len(groups) != 14 || len(groups[13]) != 4 {
		t.Fatalf("format %q", s)
	}
	for _, variant := range []string{s, strings.ToLower(s), strings.ReplaceAll(s, "-", " "), strings.ReplaceAll(s, "-", "")} {
		got, err := ParseKey(variant)
		if err != nil || !got.Equal(k) {
			t.Fatalf("%q: %v", variant, err)
		}
	}
	// A single typo is caught by the checksum.
	typo := []byte(s)
	if typo[0] == 'A' {
		typo[0] = 'B'
	} else {
		typo[0] = 'A'
	}
	for _, bad := range []string{string(typo), s[:len(s)-5], "", "not a key"} {
		if _, err := ParseKey(bad); !errors.Is(err, ErrInvalidKey) {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if mustKey(t).Equal(k) {
		t.Fatal("two random keys are equal")
	}
}

func TestSealOpenRoundTrip(t *testing.T) {
	k := mustKey(t)
	plain := bytes.Repeat([]byte(`{"invoice":"RE-2026-001","gross":"119.00"}`), 2000)
	blob, err := Seal(k, plain)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(blob, []byte("RE-2026-001")) || len(blob) >= len(plain) {
		t.Fatal("ciphertext must be compressed and must not contain plaintext")
	}
	again, _ := Seal(k, plain)
	if bytes.Equal(blob, again) {
		t.Fatal("two seals of the same data must differ (salt and nonce)")
	}
	got, err := Open(k, blob)
	if err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("open: %v", err)
	}
}

func TestOpenRejectsWrongKeyAndTampering(t *testing.T) {
	k := mustKey(t)
	blob, err := Seal(k, []byte("geheime Rechnungsdaten"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Open(mustKey(t), blob); !errors.Is(err, ErrWrongKey) {
		t.Fatalf("wrong key: %v", err)
	}
	// Flip one bit in every region: header fields, ciphertext and tag.
	for _, pos := range []int{5, headerSize - nonceSize - 1, headerSize - 1, headerSize + 2, len(blob) - 1} {
		bad := bytes.Clone(blob)
		bad[pos] ^= 0x01
		_, err := Open(k, bad)
		if pos < 5+keyIDSize {
			if !errors.Is(err, ErrWrongKey) {
				t.Errorf("key id byte %d: %v", pos, err)
			}
			continue
		}
		if !errors.Is(err, ErrTampered) {
			t.Errorf("byte %d: %v", pos, err)
		}
	}
	if _, err := Open(k, blob[:len(blob)-1]); !errors.Is(err, ErrTampered) {
		t.Fatalf("truncated: %v", err)
	}
	if _, err := Open(k, []byte("{\"format\":\"e-rechnung-studio-backup\"}")); !errors.Is(err, ErrNotBackup) {
		t.Fatalf("plain json: %v", err)
	}
	future := bytes.Clone(blob)
	future[4] = FormatVersion + 1
	if _, err := Open(k, future); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("future version: %v", err)
	}
	if _, err := Seal(Key{}, []byte("x")); !errors.Is(err, ErrInvalidKey) {
		t.Fatal("zero key accepted")
	}
}

func TestPackUnpackPreview(t *testing.T) {
	k := mustKey(t)
	now := time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)
	env := Envelope{CreatedAt: now, DeviceName: "MacBook", AppVersion: "1.2.3", Counts: Counts{Customers: 2, Invoices: 1, Received: 1, Expenses: 3},
		Document: []Ref{NewRef([]byte("teil-1"))}, Blobs: []Ref{NewRef([]byte("beleg")), NewRef([]byte("foto")), NewRef([]byte("teil-1"))}}
	blob, err := Pack(k, env)
	if err != nil {
		t.Fatal(err)
	}
	got, err := Unpack(k, blob)
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceName != "MacBook" || got.Version != EnvelopeVersion {
		t.Fatalf("envelope %+v", got)
	}
	p := got.Preview()
	if p.Customers != 2 || p.Invoices != 1 || p.Received != 1 || p.Expenses != 3 || p.Documents != 3 || p.Bytes != 15 || p.CreatedAt != "2026-10-26T12:00:00Z" {
		t.Fatalf("preview %+v", p)
	}
	if refs := got.Refs(); len(refs) != 3 || refs[0].Sum != NewRef([]byte("teil-1")).Sum {
		t.Fatalf("refs %+v", refs)
	}
	// A sealed file that is not an index is rejected after decryption.
	other, _ := Seal(k, []byte(`{"format":"other"}`))
	if _, err := Unpack(k, other); !errors.Is(err, ErrNotBackup) {
		t.Fatalf("foreign payload: %v", err)
	}
	future, _ := Seal(k, []byte(`{"format":"papierleicht-cloud-backup","version":99,"document":[{"sum":"x"}]}`))
	if _, err := Unpack(k, future); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("future version: %v", err)
	}
}

func TestRefCheck(t *testing.T) {
	ref := NewRef([]byte("original"))
	if err := ref.Check([]byte("original")); err != nil {
		t.Fatal(err)
	}
	if err := ref.Check([]byte("verändert")); !errors.Is(err, ErrTampered) {
		t.Fatalf("changed data: %v", err)
	}
}

func TestObjectIDHidesTheHash(t *testing.T) {
	k := mustKey(t)
	sum := NewRef([]byte("beleg")).Sum
	id := k.ObjectID(sum)
	if id != k.ObjectID(sum) || len(id) != 64 {
		t.Fatal("object ids must be stable hex")
	}
	if id == sum || id == mustKey(t).ObjectID(sum) {
		t.Fatal("object id reveals the document or links two customers")
	}
}

func TestSplitKeepsUnchangedParts(t *testing.T) {
	k := mustKey(t)
	data := make([]byte, 20<<20)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	parts := k.Split(data)
	if !bytes.Equal(bytes.Join(parts, nil), data) {
		t.Fatal("parts do not add up to the document")
	}
	for i, part := range parts {
		if len(part) > maxChunk || (len(part) < minChunk && i != len(parts)-1) {
			t.Fatalf("part %d has %d bytes", i, len(part))
		}
	}
	if len(parts) < 5 {
		t.Fatalf("%d parts", len(parts))
	}
	// An insert near the start changes the parts around it, not the rest.
	changed := append(append(bytes.Clone(data[:1<<20]), "neue Rechnung"...), data[1<<20:]...)
	known := map[string]bool{}
	for _, part := range parts {
		known[NewRef(part).Sum] = true
	}
	var fresh int
	after := k.Split(changed)
	for _, part := range after {
		if !known[NewRef(part).Sum] {
			fresh += len(part)
		}
	}
	if fresh > 8<<20 {
		t.Fatalf("%d of %d bytes changed after a small insert", fresh, len(changed))
	}
	if len(k.Split(nil)) != 0 {
		t.Fatal("empty document has parts")
	}
	if !bytes.Equal(bytes.Join(k.Split([]byte("klein")), nil), []byte("klein")) {
		t.Fatal("small document changed")
	}
}
