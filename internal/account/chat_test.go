package account

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestChatUsesTheSessionAndExplainsErrors(t *testing.T) {
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI()+" "+r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/api/chat/preview":
			writeJSON(w, 200, map[string]any{"members": 12, "offers": 3})
		case "/api/chat/status":
			writeJSON(w, 200, ChatState{Access: ChatJoin, Guidelines: "2026-11-02"})
		case "/api/chat/messages":
			if r.Method == http.MethodPost {
				writeJSON(w, 402, map[string]string{"message": "Der Community-Chat gehört zu Community Support und papierleicht Plus."})
				return
			}
			writeJSON(w, 200, map[string]any{"cursor": 7})
		case "/api/market/listings":
			writeJSON(w, 401, map[string]string{"message": "invalid token"})
		}
	}))
	defer srv.Close()
	pub, _, _ := ed25519.GenerateKey(nil)
	vault := &memVault{m: map[string]string{}}
	m, err := New(Config{ServerURL: srv.URL, PublicKey: pub, DataDir: t.TempDir(), Vault: vault})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if st, err := m.ChatState(ctx); err != nil || st.Access != ChatSignedOut {
		t.Fatalf("signed out: %+v %v", st, err)
	}
	if p, err := m.ChatPreview(ctx); err != nil || p.Members != 12 || p.Offers != 3 || p.Categories == nil {
		t.Fatalf("preview: %+v %v", p, err)
	}
	if len(calls) != 1 {
		t.Fatalf("signed-out state asked the server: %v", calls)
	}

	raw, _ := json.Marshal(session{AuthToken: "tok", Email: "kunde@example.com"})
	vault.m[sessionKey] = string(raw)
	m, _ = New(Config{ServerURL: srv.URL, PublicKey: pub, DataDir: t.TempDir(), Vault: vault})
	if st, err := m.ChatState(ctx); err != nil || st.Access != ChatJoin || st.Guidelines != "2026-11-02" {
		t.Fatalf("state: %+v %v", st, err)
	}
	if page, err := m.ChatMessages(ctx, 5, 0); err != nil || page.Cursor != 7 || page.Items == nil {
		t.Fatalf("messages: %+v %v", page, err)
	}
	if _, err := m.SendChatMessage(ctx, "Hallo"); err == nil || err.Error() != "Der Community-Chat gehört zu Community Support und papierleicht Plus." {
		t.Fatalf("send: %v", err)
	}
	if _, err := m.MarketListings(ctx, MarketFilter{Area: "80", Kind: "offer", Page: 2}); err == nil || err.Error() == "invalid token" {
		t.Fatalf("expired session: %v", err)
	}
	want := []string{"GET /api/chat/status tok", "GET /api/chat/messages?after=5 tok", "POST /api/chat/messages tok", "GET /api/market/listings?area=80&kind=offer&page=2 tok"}
	for i, c := range want {
		if calls[i+1] != c {
			t.Errorf("call %d: %q, want %q", i+1, calls[i+1], c)
		}
	}
	srv.Close()
	if _, err := m.ChatMessages(ctx, 0, 0); err != errChatOffline {
		t.Fatalf("offline: %v", err)
	}
}
