package banking

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	fints "github.com/BergerAPI/go-fints"
)

const subscriptionProductID = "ABO0000000000000000000001"

func productTestService(t *testing.T, buildID string) *Service {
	t.Helper()
	s, e := New(openTestDB(t), t.TempDir(), buildID)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	s.vault = &fakeVault{pins: map[string]string{}}
	return s
}

func usedProductID(t *testing.T, s *Service) string {
	t.Helper()
	used := make(chan string, 1)
	s.factory = func(c fints.Config) (readClient, error) {
		used <- c.ProductID
		return &fakeClient{cfg: c}, nil
	}
	op, e := s.Connect(testConnection(), "pin", false)
	if e != nil {
		t.Fatal(e)
	}
	awaitTerminal(t, s, op.ID)
	return <-used
}

func overviewSource(t *testing.T, s *Service) string {
	t.Helper()
	o, e := s.Overview()
	if e != nil {
		t.Fatal(e)
	}
	return o.ProductSource
}

func TestProductIDSourcesAreConsultedInOrder(t *testing.T) {
	subscription := func() (string, error) { return subscriptionProductID, nil }

	s := productTestService(t, "")
	if got := overviewSource(t, s); got != "" {
		t.Fatalf("source without any ID = %q", got)
	}
	s.SetSubscriptionSource(subscription)
	if got := overviewSource(t, s); got != ProductSourceSubscription {
		t.Fatalf("source = %q", got)
	}
	if got := usedProductID(t, s); got != subscriptionProductID {
		t.Fatalf("used %q", got)
	}
	if e := s.SetProductID("OWN0000000000000000000001"); e != nil {
		t.Fatal(e)
	}
	if got := overviewSource(t, s); got != ProductSourceOwn {
		t.Fatalf("own ID must take precedence, source = %q", got)
	}

	b := productTestService(t, "BUILD000000000000000000001")
	b.SetSubscriptionSource(subscription)
	if got := overviewSource(t, b); got != ProductSourceBuild {
		t.Fatalf("build ID must take precedence, source = %q", got)
	}
	if b.SetProductID("OWN0000000000000000000001") == nil {
		t.Fatal("own ID accepted although the build provides one")
	}
}

func TestOverviewNeverExposesProductID(t *testing.T) {
	s := productTestService(t, "")
	s.SetSubscriptionSource(func() (string, error) { return subscriptionProductID, nil })
	o, e := s.Overview()
	if e != nil {
		t.Fatal(e)
	}
	data, _ := json.Marshal(o)
	if strings.Contains(string(data), subscriptionProductID) {
		t.Fatalf("overview leaks product ID: %s", data)
	}
}

func TestHBCI4JavaTestIDIsNeverUsed(t *testing.T) {
	s := productTestService(t, "")
	if s.SetProductID(hbci4JavaTestProductID) == nil {
		t.Fatal("foreign test ID accepted")
	}
	// Installations that stored it earlier must not keep using it.
	if _, e := s.store.db.Exec("INSERT INTO bank_settings(id,product_id) VALUES(1,?)", hbci4JavaTestProductID); e != nil {
		t.Fatal(e)
	}
	if got := overviewSource(t, s); got != "" {
		t.Fatalf("stored test ID still reported as %q", got)
	}
	if _, e := s.Connect(testConnection(), "pin", false); e == nil {
		t.Fatal("connect without a registered ID must fail")
	}
}

func TestUnavailableSubscriptionKeepsOverviewAndHidesDetails(t *testing.T) {
	s := productTestService(t, "")
	s.SetSubscriptionSource(func() (string, error) {
		return "", errors.New("license server says " + subscriptionProductID)
	})
	if got := overviewSource(t, s); got != "" {
		t.Fatalf("source = %q", got)
	}
	_, e := s.Connect(testConnection(), "pin", false)
	if e == nil || strings.Contains(e.Error(), subscriptionProductID) {
		t.Fatalf("connect error = %v", e)
	}
}

func TestProductIDDoesNotLeakIntoFailureReports(t *testing.T) {
	s := productTestService(t, "")
	s.SetSubscriptionSource(func() (string, error) { return subscriptionProductID, nil })
	s.factory = func(c fints.Config) (readClient, error) {
		return &fakeClient{cfg: c, err: errors.New("rejected product " + c.ProductID)}, nil
	}
	op, e := s.Connect(testConnection(), "pin", false)
	if e != nil {
		t.Fatal(e)
	}
	r := awaitTerminal(t, s, op.ID)
	if r.Status != "error" || strings.Contains(r.Message, subscriptionProductID) {
		t.Fatalf("operation = %+v", r)
	}
	data, e := os.ReadFile(s.diagnosticPath)
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(data), subscriptionProductID) {
		t.Fatalf("diagnostic leaks product ID: %s", data)
	}
}
