package entitlement

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestSignVerifyRoundTrip(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Unix(1_800_000_000, 0)
	token, err := Sign(priv, Claims{Subject: "u1", Device: "d1", Plan: "plus", Features: []string{FeatureFinTS}, IssuedAt: now.Unix(), Expires: now.Add(time.Hour).Unix()})
	if err != nil {
		t.Fatal(err)
	}
	c, err := Verify(pub, token, now)
	if err != nil || c.Subject != "u1" || c.Device != "d1" || !c.Has(FeatureFinTS) || c.Version != Version {
		t.Fatalf("claims = %+v, err = %v", c, err)
	}
	if _, err = Verify(pub, token, now.Add(time.Hour)); !errors.Is(err, ErrExpired) {
		t.Fatalf("expired token: %v", err)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	otherPub, _, _ := ed25519.GenerateKey(rand.Reader)
	now := time.Now()
	token, _ := Sign(priv, Claims{Subject: "u1", Expires: now.Add(time.Hour).Unix()})
	forged, _ := Sign(priv, Claims{Subject: "u2", Expires: now.Add(time.Hour).Unix()})
	payload, sig, _ := strings.Cut(token, ".")
	forgedPayload, _, _ := strings.Cut(forged, ".")

	cases := map[string]struct {
		key   ed25519.PublicKey
		token string
		want  error
	}{
		"swapped payload": {pub, forgedPayload + "." + sig, ErrSignature},
		"other key":       {otherPub, token, ErrSignature},
		"no signature":    {pub, payload, ErrMalformed},
		"garbage":         {pub, "a.b", ErrMalformed},
	}
	for name, tc := range cases {
		if _, err := Verify(tc.key, tc.token, now); !errors.Is(err, tc.want) {
			t.Errorf("%s: got %v, want %v", name, err, tc.want)
		}
	}
}

func TestFeaturesForPlan(t *testing.T) {
	if f := FeaturesForPlan(PlanPlus); len(f) != 2 || f[0] != FeatureFinTS || f[1] != FeatureAudit {
		t.Fatal(f)
	}
	if f := FeaturesForPlan(PlanPlusBackup); len(f) != 3 || f[2] != FeatureBackup {
		t.Fatal(f)
	}
	if f := FeaturesForPlan(PlanPlusTaxBackup); len(f) != 4 || f[2] != FeatureElster || f[3] != FeatureBackup {
		t.Fatal(f)
	}
	if PlanFor(PlanPlusTax, true) != PlanPlusTaxBackup || PlanFor(PlanPlus, false) != PlanPlus || PlanFor("gold", true) != "" {
		t.Fatal("PlanFor")
	}
	if PlanRank(PlanPlusTax) <= PlanRank(PlanPlusBackup) || PlanRank(PlanPlusTaxBackup) <= PlanRank(PlanPlusTax) {
		t.Fatal("rank")
	}
	if FeaturesForPlan("gold") != nil || PlanRank("gold") != 0 || PlanRank(PlanPlusBackup) <= PlanRank(PlanPlus) {
		t.Fatal("unknown plans must grant nothing")
	}
}

func TestParsePrivateKey(t *testing.T) {
	if _, err := ParsePrivateKey("AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="); err != nil {
		t.Fatal(err)
	}
	if _, err := ParsePrivateKey("too-short"); err == nil {
		t.Fatal("accepted invalid seed")
	}
}
