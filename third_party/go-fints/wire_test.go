package fints

import (
	"bytes"
	"testing"
)

func TestParseSegmentsEscapesAndBinaryPayload(t *testing.T) {
	inner := []byte("HIKAZ:2:7+@19@abc+'?:?@payload??x'")
	wire := []byte("HNHBK:1:3+000000000099+300+dialog+1'HNVSD:999:1+" + binary(inner) + "'HNHBS:3:1+1'")

	segments, err := parseSegments(wire)
	if err != nil {
		t.Fatalf("parseSegments() error = %v", err)
	}
	hikaz := findSegments(segments, "HIKAZ")
	if len(hikaz) != 1 {
		t.Fatalf("got %d HIKAZ segments, want 1", len(hikaz))
	}
	if got, want := hikaz[0].Fields[0].raw, []byte("abc+'?:?@payload??x"); !bytes.Equal(got, want) {
		t.Fatalf("binary field = %q, want %q", got, want)
	}
}

func TestEscapeRoundTrip(t *testing.T) {
	value := "a+b:c'd?e@f"
	wire := []byte("TEST:1:1+" + escape(value) + "'")
	segments, err := parseSegments(wire)
	if err != nil {
		t.Fatalf("parseSegments() error = %v", err)
	}
	if got := component(segments[0], 0, 0); got != value {
		t.Fatalf("round trip = %q, want %q", got, value)
	}
}

func TestParseSegmentsRejectsTruncatedBinary(t *testing.T) {
	_, err := parseSegments([]byte("TEST:1:1+@10@short'"))
	if err == nil {
		t.Fatal("parseSegments() error = nil, want truncated binary error")
	}
}

func TestMalformedWireLengthsAndHeaders(t *testing.T) {
	for _, raw := range []string{"HNVSD:1:1+@-1@'", "HNVSD:1:1+@999999999999999999999999999@'", "HNVSD:1:1+@3@x'", "HNVSD:1:1+@x@'", "HNHBK:1:3+trailing?", "HNHBK:secret:3'"} {
		if _, err := parseSegments([]byte(raw)); err == nil {
			t.Errorf("malformed input accepted")
		}
	}
}

func TestNestedWireLimit(t *testing.T) {
	data := []byte("HIRMG:1:2+0010::OK'")
	for i := 0; i < 8; i++ {
		data = []byte("HNVSD:1:1+" + binary(data) + "'")
	}
	if _, err := parseSegments(data); err == nil {
		t.Fatal("deep nesting accepted")
	}
}

func FuzzParseSegments(f *testing.F) {
	f.Add([]byte("HIRMG:1:2+0010::OK'"))
	f.Add([]byte("HNVSD:1:1+@-2@'"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 65536 {
			t.Skip()
		}
		_, _ = parseSegments(data)
	})
}
