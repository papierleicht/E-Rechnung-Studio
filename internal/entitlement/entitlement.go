// Package entitlement signs and verifies the offline license tokens that the
// desktop app stores in the OS keychain. It only depends on the standard
// library so the app can verify tokens with the same code.
//
// A token is base64url(JSON claims) + "." + base64url(Ed25519 signature over
// the encoded claims). It is signed, not encrypted: it carries no secrets.
//
// This file is kept identical to the entitlement package of the license
// service, which is not a Go module dependency of the app. Change both together.
package entitlement

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"
)

const (
	Version = 1
	// FeatureFinTS unlocks the bank fetch with papierleicht's product ID.
	FeatureFinTS = "fints"
	// FeatureBackup unlocks the end-to-end encrypted cloud backup.
	FeatureBackup = "backup"
	// FeatureElster unlocks transmitting tax returns to ELSTER.
	FeatureElster = "elster"
	// FeatureAudit unlocks the tools for a tax audit and the server's receipts.
	FeatureAudit = "audit"

	// PlanPlus is the bank fetch subscription ("Plus").
	PlanPlus = "plus"
	// PlanPlusBackup is Plus with the backup add-on.
	PlanPlusBackup = "plus_backup"
	// PlanPlusTax adds the ELSTER transmission ("Plus Steuer").
	PlanPlusTax = "plus_tax"
	// PlanPlusTaxBackup is Plus Steuer with the backup add-on.
	PlanPlusTaxBackup = "plus_tax_backup"
)

// PlanFor combines a tier ("plus" or "plus_tax") with the backup add-on.
// It returns "" for an unknown tier.
func PlanFor(tier string, backup bool) string {
	switch {
	case tier == PlanPlus && backup:
		return PlanPlusBackup
	case tier == PlanPlus:
		return PlanPlus
	case tier == PlanPlusTax && backup:
		return PlanPlusTaxBackup
	case tier == PlanPlusTax:
		return PlanPlusTax
	}
	return ""
}

// FeaturesForPlan lists the features a plan grants. Unknown plans grant
// nothing. Tokens of older servers carry Plan "plus" and only FeatureFinTS,
// so clients must decide by feature, not by plan name.
func FeaturesForPlan(plan string) []string {
	switch plan {
	case PlanPlus:
		return []string{FeatureFinTS, FeatureAudit}
	case PlanPlusBackup:
		return []string{FeatureFinTS, FeatureAudit, FeatureBackup}
	case PlanPlusTax:
		return []string{FeatureFinTS, FeatureAudit, FeatureElster}
	case PlanPlusTaxBackup:
		return []string{FeatureFinTS, FeatureAudit, FeatureElster, FeatureBackup}
	}
	return nil
}

// PlanRank orders plans by what they include; unknown plans rank lowest.
func PlanRank(plan string) int {
	switch plan {
	case PlanPlus:
		return 1
	case PlanPlusBackup:
		return 2
	case PlanPlusTax:
		return 3
	case PlanPlusTaxBackup:
		return 4
	}
	return 0
}

var (
	ErrMalformed = errors.New("entitlement: malformed token")
	ErrSignature = errors.New("entitlement: invalid signature")
	ErrExpired   = errors.New("entitlement: expired")
)

type Claims struct {
	Version  int      `json:"v"`
	Subject  string   `json:"sub"`
	Device   string   `json:"dev"`
	Plan     string   `json:"plan"`
	Features []string `json:"features"`
	IssuedAt int64    `json:"iat"`
	Expires  int64    `json:"exp"`
}

func (c Claims) Has(feature string) bool { return slices.Contains(c.Features, feature) }

func (c Claims) ExpiresAt() time.Time { return time.Unix(c.Expires, 0) }

var encoding = base64.RawURLEncoding

func Sign(key ed25519.PrivateKey, c Claims) (string, error) {
	c.Version = Version
	return signed(key, c)
}

func signed(key ed25519.PrivateKey, v any) (string, error) {
	if len(key) != ed25519.PrivateKeySize {
		return "", errors.New("entitlement: invalid signing key")
	}
	payload, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	encoded := encoding.EncodeToString(payload)
	return encoded + "." + encoding.EncodeToString(ed25519.Sign(key, []byte(encoded))), nil
}

// Anchor is a receipt: at this time, a customer's change log ended in this
// hash. The server sees no content, but a log rewritten later no longer matches
// the receipts given earlier or the server's own list of them.
type Anchor struct {
	Subject string `json:"sub"`
	Seq     int64  `json:"seq"`
	Hash    string `json:"hash"`
	At      int64  `json:"at"`
}

// SignAnchor issues a receipt.
func SignAnchor(key ed25519.PrivateKey, a Anchor) (string, error) { return signed(key, a) }

// VerifyAnchor checks a receipt's signature and returns its content.
func VerifyAnchor(key ed25519.PublicKey, token string) (Anchor, error) {
	var a Anchor
	encoded, sig, ok := strings.Cut(token, ".")
	if !ok || len(key) != ed25519.PublicKeySize {
		return a, ErrMalformed
	}
	rawSig, err := encoding.DecodeString(sig)
	if err != nil || len(rawSig) != ed25519.SignatureSize {
		return a, ErrMalformed
	}
	if !ed25519.Verify(key, []byte(encoded), rawSig) {
		return a, ErrSignature
	}
	payload, err := encoding.DecodeString(encoded)
	if err != nil || json.Unmarshal(payload, &a) != nil {
		return a, ErrMalformed
	}
	return a, nil
}

// Verify checks signature, version and expiry. Callers still compare Device
// with the local device ID and check the features they need.
func Verify(key ed25519.PublicKey, token string, now time.Time) (Claims, error) {
	var c Claims
	encoded, sig, ok := strings.Cut(token, ".")
	if !ok || len(key) != ed25519.PublicKeySize {
		return c, ErrMalformed
	}
	rawSig, err := encoding.DecodeString(sig)
	if err != nil || len(rawSig) != ed25519.SignatureSize {
		return c, ErrMalformed
	}
	if !ed25519.Verify(key, []byte(encoded), rawSig) {
		return c, ErrSignature
	}
	payload, err := encoding.DecodeString(encoded)
	if err != nil || json.Unmarshal(payload, &c) != nil || c.Version != Version {
		return Claims{}, ErrMalformed
	}
	if !now.Before(c.ExpiresAt()) {
		return c, ErrExpired
	}
	return c, nil
}

// ParsePrivateKey reads a base64 (std) encoded 32-byte Ed25519 seed.
func ParsePrivateKey(seed string) (ed25519.PrivateKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(seed))
	if err != nil || len(raw) != ed25519.SeedSize {
		return nil, errors.New("entitlement: signing key must be a base64 encoded 32-byte Ed25519 seed")
	}
	return ed25519.NewKeyFromSeed(raw), nil
}
