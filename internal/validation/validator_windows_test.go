//go:build windows

package validation

import (
	"net/url"
	"strings"
	"testing"
)

func TestFileURIWindowsPath(t *testing.T) {
	uri := fileURI(`C:\tmp\rules\facturx\sheet.xslt`)
	if !strings.HasPrefix(uri, "file:///") {
		t.Fatalf("unexpected URI prefix: %s", uri)
	}
	parsed, err := url.Parse(uri)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != "" {
		t.Fatalf("unexpected host in file URI: %q", parsed.Host)
	}
	if !strings.HasPrefix(parsed.Path, "/C:/tmp/rules/facturx/") {
		t.Fatalf("unexpected URI path: %q", parsed.Path)
	}
}
