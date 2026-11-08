package validation

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestValidateReusesTheResultForTheSameDocument(t *testing.T) {
	v, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join("..", "einvoice", "testdata", "cii.xml"))
	if err != nil {
		t.Fatal(err)
	}
	first := v.Validate(data)
	if first.State == "error" || len(first.Issues) == 0 {
		t.Fatalf("the sample needs findings to check the copy: %+v", first)
	}
	// What a caller does with its result never reaches the next caller.
	want := v.validate(data)
	first.Issues[0].Severity = "changed"
	v.Validate(data).Issues[0].Severity = "changed"
	if got := v.ValidateImported(data); !reflect.DeepEqual(got, softenLocalIssues(v.validate(data))) {
		t.Fatalf("imported: %+v", got)
	}
	if got := v.Validate(data); !reflect.DeepEqual(got, want) {
		t.Fatalf("reused %+v, want %+v", got, want)
	}
	if len(v.recent) != 1 {
		t.Fatalf("%d results remembered", len(v.recent))
	}
	// Engine errors run again the next time.
	if got := v.Validate([]byte("<kein xml")); got.State != "error" || len(v.recent) != 1 {
		t.Fatalf("%+v, %d remembered", got, len(v.recent))
	}
	for i := 0; i <= recentResults; i++ {
		v.Validate(append(data, []byte("<!-- "+string(rune('a'+i))+" -->")...))
	}
	if len(v.recent) != recentResults {
		t.Fatalf("%d results remembered", len(v.recent))
	}
}
