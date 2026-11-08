package account

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPricesComeFromTheServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/billing/mollie/prices" || r.Header.Get("Authorization") != "" {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		writeJSON(w, http.StatusOK, map[string]any{"prices": []Price{{Plan: "support", Interval: "monthly", Currency: "EUR", Value: "4.90"}}})
	}))
	defer srv.Close()
	m, err := New(Config{ServerURL: srv.URL, DataDir: t.TempDir(), Vault: &memVault{m: map[string]string{}}})
	if err != nil {
		t.Fatal(err)
	}
	prices, err := m.Prices(context.Background())
	if err != nil || len(prices) != 1 || prices[0].Plan != "support" || prices[0].Value != "4.90" {
		t.Fatalf("prices %+v %v", prices, err)
	}
	none, err := New(Config{DataDir: t.TempDir(), Vault: &memVault{m: map[string]string{}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = none.Prices(context.Background()); err == nil {
		t.Fatal("prices without a server")
	}
}
