package updatesign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

const repo = "papierleicht/E-Rechnung-Studio"

func newKey(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return pub, priv
}

func TestEmbeddedPublicKeyParses(t *testing.T) {
	if _, err := ParsePublicKey(PublicKey); err != nil {
		t.Fatal(err)
	}
}

func TestKeysSurviveTheirTextForm(t *testing.T) {
	pub, priv := newKey(t)
	gotPriv, err := ParsePrivateKey(EncodePrivateKey(priv))
	if err != nil || !gotPriv.Equal(priv) {
		t.Fatal("private key changed", err)
	}
	gotPub, err := ParsePublicKey(EncodePublicKey(pub) + "\n")
	if err != nil || !gotPub.Equal(pub) {
		t.Fatal("public key changed", err)
	}
	for _, bad := range []string{"", "kein base64", EncodePublicKey(pub)[:20]} {
		if _, err := ParsePrivateKey(bad); err == nil {
			t.Fatalf("private key %q accepted", bad)
		}
		if _, err := ParsePublicKey(bad); err == nil {
			t.Fatalf("public key %q accepted", bad)
		}
	}
}

func TestSignatureCoversEveryByteAndOnlyTheKey(t *testing.T) {
	pub, priv := newKey(t)
	other, _ := newKey(t)
	pkg := bytes.Repeat([]byte("Rechnung "), 100000)
	sig, err := Sign(priv, bytes.NewReader(pkg))
	if err != nil {
		t.Fatal(err)
	}
	if err := Verify(pub, bytes.NewReader(pkg), sig); err != nil {
		t.Fatal(err)
	}
	changed := bytes.Clone(pkg)
	changed[len(changed)/2] ^= 1
	if Verify(pub, bytes.NewReader(changed), sig) == nil {
		t.Fatal("changed package accepted")
	}
	if Verify(other, bytes.NewReader(pkg), sig) == nil {
		t.Fatal("signature accepted with another key")
	}
	if Verify(pub, bytes.NewReader(pkg), "AAAA") == nil {
		t.Fatal("short signature accepted")
	}
}

// fakeGitHub serves the two GitHub endpoints the update check uses: the
// latest release from the API and release files from the download host.
func fakeGitHub(t *testing.T, tag string, files map[string][]byte) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(nil)
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/"+repo+"/releases/latest":
			type asset struct {
				ID   int64  `json:"id"`
				Name string `json:"name"`
				Size int64  `json:"size"`
				URL  string `json:"url"`
			}
			var assets []asset
			var id int64
			for name, data := range files {
				id++
				assets = append(assets, asset{ID: id, Name: name, Size: int64(len(data)), URL: server.URL + "/assets/" + name})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": tag, "name": "E-Rechnung-Studio " + tag, "assets": assets})
		case strings.HasPrefix(r.URL.Path, "/assets/"):
			data, ok := files[strings.TrimPrefix(r.URL.Path, "/assets/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(data)
		case strings.HasPrefix(r.URL.Path, "/"+repo+"/releases/download/"+tag+"/"):
			data, ok := files[strings.TrimPrefix(r.URL.Path, "/"+repo+"/releases/download/"+tag+"/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			_, _ = w.Write(data)
		default:
			http.NotFound(w, r)
		}
	})
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func provider(t *testing.T, server *httptest.Server) *Provider {
	t.Helper()
	inner, err := github.New(github.Config{Repository: repo, BaseURL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	return &Provider{Inner: inner, Repository: repo, BaseURL: server.URL}
}

var darwinArm = updater.CheckRequest{CurrentVersion: "1.0.0", Platform: "darwin", Arch: "arm64"}

func TestUpdateCarriesTheSignatureTheUpdaterChecks(t *testing.T) {
	pub, priv := newKey(t)
	pkg := []byte("neues App-Bundle")
	sig, err := Sign(priv, bytes.NewReader(pkg))
	if err != nil {
		t.Fatal(err)
	}
	server := fakeGitHub(t, "v1.0.1", map[string][]byte{
		"e-rechnung-studio-darwin-arm64.zip":     pkg,
		"e-rechnung-studio-darwin-arm64.zip.sig": []byte(sig),
		"e-rechnung-studio-windows-amd64.zip":    []byte("andere Plattform"),
		"SHA256SUMS":                             []byte("nicht gebraucht\n"),
	})
	p := provider(t, server)
	rel, err := p.Check(context.Background(), darwinArm)
	if err != nil {
		t.Fatal(err)
	}
	if rel == nil || rel.Version != "1.0.1" || rel.Artifact.Filename != "e-rechnung-studio-darwin-arm64.zip" {
		t.Fatalf("wrong release: %+v", rel)
	}
	v := rel.Verification
	if v == nil || v.SignatureAlgo != "ed25519ph" || len(v.Digest) != 0 || len(v.Signature) != ed25519.SignatureSize {
		t.Fatalf("verification not set for the updater: %+v", v)
	}
	var got bytes.Buffer
	if err := p.Download(context.Background(), rel, &got, func(int64, int64) {}); err != nil {
		t.Fatal(err)
	}
	// The updater hashes the download with SHA-512 and checks Ed25519ph.
	digest := sha512.Sum512(got.Bytes())
	if err := ed25519.VerifyWithOptions(pub, digest[:], v.Signature, &ed25519.Options{Hash: crypto.SHA512}); err != nil {
		t.Fatal("updater would reject the signed package:", err)
	}
	if p.Name() != "github" {
		t.Fatalf("name %q does not route downloads back", p.Name())
	}
}

func TestReleaseWithoutUsableSignatureIsNotOffered(t *testing.T) {
	_, priv := newKey(t)
	pkg := []byte("App-Bundle")
	sig, err := Sign(priv, bytes.NewReader(pkg))
	if err != nil {
		t.Fatal(err)
	}
	for name, files := range map[string]map[string][]byte{
		"missing":  {"e-rechnung-studio-darwin-arm64.zip": pkg},
		"garbled":  {"e-rechnung-studio-darwin-arm64.zip": pkg, "e-rechnung-studio-darwin-arm64.zip.sig": []byte("kein base64!\n")},
		"too long": {"e-rechnung-studio-darwin-arm64.zip": pkg, "e-rechnung-studio-darwin-arm64.zip.sig": []byte(strings.Repeat(sig, 20))},
	} {
		t.Run(name, func(t *testing.T) {
			rel, err := provider(t, fakeGitHub(t, "v1.0.1", files)).Check(context.Background(), darwinArm)
			if err == nil || rel != nil {
				t.Fatalf("release offered without a usable signature: %+v %v", rel, err)
			}
		})
	}
}

func TestUpToDateNeedsNoSignature(t *testing.T) {
	server := fakeGitHub(t, "v1.0.0", map[string][]byte{"e-rechnung-studio-darwin-arm64.zip": []byte("x")})
	rel, err := provider(t, server).Check(context.Background(), darwinArm)
	if err != nil || rel != nil {
		t.Fatalf("current version reported as update: %+v %v", rel, err)
	}
}
