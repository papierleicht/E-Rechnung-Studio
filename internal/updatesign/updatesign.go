// Package updatesign signs release packages and lets the Wails updater check
// them before it installs anything.
//
// Every package of a release carries a sidecar "<package>.sig" with the
// base64 Ed25519ph signature of the package's SHA-512 digest. The private key
// lives only in the release workflow's secret store; the app holds the public
// key, so a release that someone else uploaded, or a package exchanged after
// the build, is rejected even when it comes from the right repository.
package updatesign

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ed25519"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// PublicKey is the base64 Ed25519 public key that official releases are
// signed with. A fork that publishes its own updates sets its own key, here or
// with -ldflags="-X papierleicht.de/e-rechnung-studio/internal/updatesign.PublicKey=<base64>".
var PublicKey = "voC3EoAUPVqae/ZTyexCClqBVhhkyLawrpv4i0ytu7Q="

// Algorithm is the Wails updater name of the signature scheme: Ed25519 over
// the SHA-512 digest of the whole package, which the updater computes while
// downloading.
const Algorithm = "ed25519ph"

// Suffix names the signature file next to a package.
const Suffix = ".sig"

// maxSignatureFile bounds the download of a signature file; a signature is
// 88 base64 characters.
const maxSignatureFile = 1024

// ParsePublicKey decodes a base64 Ed25519 public key.
func ParsePublicKey(text string) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil {
		return nil, fmt.Errorf("update key: %w", err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("update key: %d bytes, want %d", len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// ParsePrivateKey decodes the base64 32-byte seed of an Ed25519 private key.
func ParsePrivateKey(text string) (ed25519.PrivateKey, error) {
	seed, err := base64.StdEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil {
		return nil, errors.New("signing key: not base64")
	}
	if len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("signing key: %d bytes, want a %d-byte seed", len(seed), ed25519.SeedSize)
	}
	return ed25519.NewKeyFromSeed(seed), nil
}

// EncodePrivateKey returns the base64 seed of key, the form ParsePrivateKey reads.
func EncodePrivateKey(key ed25519.PrivateKey) string {
	return base64.StdEncoding.EncodeToString(key.Seed())
}

// EncodePublicKey returns key in base64, the form ParsePublicKey reads.
func EncodePublicKey(key ed25519.PublicKey) string {
	return base64.StdEncoding.EncodeToString(key)
}

// Sign reads a package and returns the content of its signature file.
func Sign(key ed25519.PrivateKey, pkg io.Reader) (string, error) {
	digest, err := sha512Of(pkg)
	if err != nil {
		return "", err
	}
	sig, err := key.Sign(nil, digest, &ed25519.Options{Hash: crypto.SHA512})
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig) + "\n", nil
}

// Verify checks a package against the content of its signature file, the way
// the Wails updater does after a download.
func Verify(key ed25519.PublicKey, pkg io.Reader, signature string) error {
	sig, err := decodeSignature([]byte(signature))
	if err != nil {
		return err
	}
	digest, err := sha512Of(pkg)
	if err != nil {
		return err
	}
	if err := ed25519.VerifyWithOptions(key, digest, sig, &ed25519.Options{Hash: crypto.SHA512}); err != nil {
		return errors.New("update signature does not match the package")
	}
	return nil
}

func sha512Of(r io.Reader) ([]byte, error) {
	h := sha512.New()
	if _, err := io.Copy(h, r); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

func decodeSignature(text []byte) ([]byte, error) {
	sig, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(text)))
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, errors.New("update signature is not a base64 Ed25519 signature")
	}
	return sig, nil
}

// Provider wraps the Wails GitHub provider: it finds the release and package
// as before and adds the package's signature, which the updater then checks
// against Config.PublicKey. A release without a signature for the package is
// reported as an error and never offered.
type Provider struct {
	// Inner is the github provider that finds the release and downloads it.
	Inner updater.Provider
	// Repository is "owner/repo", the same as for the github provider.
	Repository string
	// BaseURL serves "/<owner>/<repo>/releases/download/<tag>/<file>";
	// empty means https://github.com.
	BaseURL string
	// Client fetches the signature file; nil uses a client with a timeout.
	Client *http.Client
}

// Name keeps the inner provider's name, so downloads route back to it.
func (p *Provider) Name() string { return p.Inner.Name() }

// Check returns the inner provider's release with the package signature as
// the only verification. The SHA-256 checksum is not needed: the signature
// covers the SHA-512 digest of the whole package.
func (p *Provider) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	rel, err := p.Inner.Check(ctx, req)
	if err != nil || rel == nil {
		return rel, err
	}
	tag, _ := rel.Metadata["github.release.tag"].(string)
	if tag == "" || rel.Artifact.Filename == "" {
		return nil, errors.New("update signature: release without tag or package name")
	}
	sig, err := p.signature(ctx, tag, rel.Artifact.Filename)
	if err != nil {
		return nil, err
	}
	rel.Verification = &updater.Verification{SignatureAlgo: Algorithm, Signature: sig}
	return rel, nil
}

// Download streams the package through the inner provider.
func (p *Provider) Download(ctx context.Context, r *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	return p.Inner.Download(ctx, r, dst, onProgress)
}

func (p *Provider) signature(ctx context.Context, tag, filename string) ([]byte, error) {
	base := strings.TrimRight(p.BaseURL, "/")
	if base == "" {
		base = "https://github.com"
	}
	address := base + "/" + p.Repository + "/releases/download/" + url.PathEscape(tag) + "/" + url.PathEscape(filename+Suffix)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	client := p.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update signature: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("update signature for %s: HTTP %d", filename, resp.StatusCode)
	}
	text, err := io.ReadAll(io.LimitReader(resp.Body, maxSignatureFile+1))
	if err != nil {
		return nil, fmt.Errorf("update signature: %w", err)
	}
	if len(text) > maxSignatureFile {
		return nil, errors.New("update signature file is too large")
	}
	return decodeSignature(text)
}
