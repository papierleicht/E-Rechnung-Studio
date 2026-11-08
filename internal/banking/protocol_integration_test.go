package banking

import (
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	fints "github.com/BergerAPI/go-fints"
)

// This transport is the only bank reachable by the test. It exercises the real
// FinTS client, binary framing and MT940 parser without real credentials/network.
type protocolTestTransport struct {
	t                                       *testing.T
	mu                                      sync.Mutex
	syncs, tanSubmissions, transactionPages int
}

var protocolTANProcess = regexp.MustCompile(`HKTAN:([0-9]+):[0-9]+\+([24S])\+`)

func (b *protocolTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	body, e := io.ReadAll(r.Body)
	if e != nil {
		return nil, e
	}
	raw, e := base64.StdEncoding.DecodeString(string(body))
	if e != nil {
		return nil, e
	}
	request := string(raw)
	if r.URL.String() != "https://synthetic-bank.example/fints" || r.Method != http.MethodPost {
		return nil, fmt.Errorf("unexpected synthetic banking endpoint")
	}
	if !strings.Contains(request, "HNSHA:") || !strings.Contains(request, "synthetic-pin") {
		return nil, fmt.Errorf("missing test authentication envelope")
	}
	process := ""
	tanReference := ""
	if match := protocolTANProcess.FindStringSubmatch(request); len(match) > 2 {
		tanReference, process = match[1], match[2]
	}
	parameters := "HIRMG:2:2+0010::Accepted+3920::Allowed:910'" +
		"HIBPA:3:3:2+1+280:12345678+Synthetic Test Bank+0+1+300'" +
		"HITANS:4:7:2+910:2:TECH:chipTAN::Manual TAN:6:1:TAN:6:N:1:N:0:2:N:N:00:0:N:0'" +
		"HIKAZS:5:6:2+1+1+100:N:N'"
	accounts := "HIUPD:6:6:3+1234567890:00:280:12345678+DE0012345678001234567890+synthetic-user+1+EUR+Test Person++Checking'"
	var inner string
	b.mu.Lock()
	defer b.mu.Unlock()
	switch {
	case strings.Contains(request, "HKEND:"):
		inner = "HIRMG:2:2+0100::Dialog ended'"
	case strings.Contains(request, "HKSYN:"):
		b.syncs++
		inner = parameters + "HISYN:7:4:5+synthetic-system+1'"
	case process == "4":
		feedbackReference := tanReference
		if strings.Contains(request, "HKIDN:") {
			// Consors references the protected login order in HIRMS while
			// HITAN references HKTAN. Exercise both through the application.
			feedbackReference = "3"
		}
		inner = fmt.Sprintf("HIRMG:2:2+9050::Partially erroneous'HIRMS:3:2:%s+0030::TAN required'"+
			"HITAN:4:7:%s+4++synthetic-task+Confirm this synthetic read request'", feedbackReference, tanReference)
	case process == "2":
		if !strings.Contains(request, "synthetic-pin:AbC123'") {
			return nil, fmt.Errorf("TAN was not forwarded case-sensitively in signature")
		}
		b.tanSubmissions++
		inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Authenticated'" + accounts
	case strings.Contains(request, "HKSPA:"):
		inner = "HIRMG:2:2+0010::Accepted'HISPA:3:1:3+J:DE0012345678001234567890:MOCKDEFFXXX:1234567890:00:280:12345678'"
	case strings.Contains(request, "HKKAZ:"):
		if !strings.Contains(request, "+20260801+20260831+") {
			return nil, fmt.Errorf("requested date window was not propagated to FinTS")
		}
		b.transactionPages++
		statement := ""
		if strings.Contains(request, "next-page") {
			statement = ":61:2608020802C25,50NTRFREF2//TEST-TWO\r\n:86:835?20Refund?32Test Shop\r\n:62F:C260802EUR115,50\r\n-"
			inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Transactions returned'"
		} else {
			statement = ":20:START\r\n:25:DE0012345678001234567890\r\n:60F:C260801EUR100,00\r\n:61:2608010801D10,00NTRFREF1//TEST-ONE\r\n:86:835?20Coffee?32Test Cafe\r\n"
			inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+3040::More entries:next-page'"
		}
		inner += fmt.Sprintf("HIKAZ:4:6:3+@%d@%s'", len(statement), statement)
	case strings.Contains(request, "HKIDN:"):
		inner = parameters + accounts
	default:
		return nil, fmt.Errorf("unexpected synthetic FinTS operation")
	}
	response := "HNHBK:1:3+000000000000+300+synthetic-dialog+1'" + inner + "HNHBS:99:1+1'"
	response = strings.Replace(response, "000000000000", fmt.Sprintf("%012d", len(response)), 1)
	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "text/plain")
	_, _ = recorder.Write([]byte(base64.StdEncoding.EncodeToString([]byte(response))))
	return recorder.Result(), nil
}

func completeProtocolOperation(t *testing.T, s *Service, id string) Operation {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	submitted := false
	for time.Now().Before(deadline) {
		op, e := s.Status(id)
		if e != nil {
			t.Fatal(e)
		}
		if op.Status == "tan" && op.Challenge != nil {
			if op.Challenge.Text != "Confirm this synthetic read request" {
				t.Fatalf("challenge=%+v", op.Challenge)
			}
			if e = s.SubmitTAN(id, op.Challenge.ID, "AbC123"); e != nil {
				t.Fatal(e)
			}
			submitted = true
		}
		if op.Status == "done" || op.Status == "error" || op.Status == "cancelled" {
			result := awaitTerminal(t, s, id)
			if result.Status != "done" {
				t.Fatalf("protocol operation failed: %+v", result)
			}
			if !submitted {
				t.Fatal("real protocol TAN challenge never reached service")
			}
			return result
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("protocol operation timed out")
	return Operation{}
}

func TestServiceRealFinTSProtocolConnectTANSyncAndDeduplicate(t *testing.T) {
	s, vault := testService(t)
	bank := &protocolTestTransport{t: t}
	s.factory = func(c fints.Config) (readClient, error) {
		c.HTTPClient = &http.Client{Transport: bank}
		return fints.NewClient(c)
	}
	connection := testConnection()
	connection.Endpoint = "https://synthetic-bank.example/fints"
	operation, e := s.Connect(connection, "synthetic-pin", false)
	if e != nil {
		t.Fatal(e)
	}
	result := completeProtocolOperation(t, s, operation.ID)
	if len(result.Methods) != 1 || result.Methods[0].SecurityFunction != "910" {
		t.Fatalf("methods=%+v", result.Methods)
	}
	overview, e := s.Overview()
	if e != nil {
		t.Fatal(e)
	}
	if len(overview.Accounts) != 1 || len(overview.Connections) != 1 {
		t.Fatalf("overview=%+v", overview)
	}
	persisted, e := s.store.connection(operation.ConnectionID)
	if e != nil {
		t.Fatal(e)
	}
	if persisted.SystemID != "synthetic-system" || persisted.SecurityFunction != "910" {
		t.Fatalf("connection state=%+v", persisted)
	}
	account := overview.Accounts[0]
	for index := 0; index < 2; index++ {
		operation, e = s.Sync(account.ID, "synthetic-pin", "2026-08-01", "2026-08-31")
		if e != nil {
			t.Fatal(e)
		}
		result = completeProtocolOperation(t, s, operation.ID)
		expected := 2
		if index > 0 {
			expected = 0
		}
		if result.Added != expected {
			t.Fatalf("sync %d added=%d want=%d", index, result.Added, expected)
		}
	}
	bookings, e := s.Bookings(account.ID)
	if e != nil {
		t.Fatal(e)
	}
	if len(bookings) != 2 {
		t.Fatalf("persisted %d bookings", len(bookings))
	}
	expectedAmounts := map[string]int64{"TEST-ONE": -1000, "TEST-TWO": 2550}
	for _, booking := range bookings {
		want, ok := expectedAmounts[booking.Data.BankReference]
		if !ok || booking.Data.Amount != want || booking.Data.Currency != "EUR" {
			t.Fatalf("booking=%+v", booking)
		}
	}
	account, e = s.store.account(account.ID)
	if e != nil || account.LastSync == "" {
		t.Fatalf("lastSync=%q error=%v", account.LastSync, e)
	}
	if len(vault.pins) != 0 {
		t.Fatal("unsaved PIN persisted")
	}
	bank.mu.Lock()
	defer bank.mu.Unlock()
	if bank.syncs != 1 || bank.tanSubmissions != 3 || bank.transactionPages != 4 {
		t.Fatalf("wire flow: synchronizations=%d TANs=%d pages=%d", bank.syncs, bank.tanSubmissions, bank.transactionPages)
	}
}
