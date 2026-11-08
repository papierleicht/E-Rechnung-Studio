package fints

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

type tanMockBank struct {
	t            *testing.T
	security     string
	methodName   string
	decoupled    bool
	hktanVersion int

	mu        sync.Mutex
	requests  [][]segment
	pollCount int
}

func (m *tanMockBank) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	if err != nil {
		m.t.Fatal(err)
	}
	raw, err := base64.StdEncoding.DecodeString(string(body))
	if err != nil {
		m.t.Fatal(err)
	}
	segments, err := parseSegments(raw)
	if err != nil {
		m.t.Fatal(err)
	}
	m.mu.Lock()
	m.requests = append(m.requests, segments)
	m.mu.Unlock()

	inner := ""
	switch {
	case len(findSegments(segments, "HKEND")) > 0:
		inner = "HIRMG:2:2+0100::Dialog ended'"
	case len(findSegments(segments, "HKSPA")) > 0:
		inner = "HIRMG:2:2+0010::Accepted'" +
			"HISPA:3:1:3+J:DE00100100100012345678:MARKDEF1100:0012345678::280:10010010'"
	case len(findSegments(segments, "HKTAB")) > 0:
		inner = "HIRMG:2:2+0010::Accepted'" +
			"HITAB:3:5:3+0+1:CARD:SEQ:APP:ACCOUNT:20260101:20300101:LIST:?+49123:?+49***:BestSign Device:0:20260723'"
	case hktanProcess(segments) == "4":
		code := "0030"
		if m.decoupled {
			code = "3955"
		}
		version := m.hktanVersion
		if version == 0 {
			version = 6
		}
		inner = fmt.Sprintf(
			"HIRMG:2:2+9050::Partially erroneous'HIRMS:3:2:%d+%s::Approve request'"+
				"HITAN:4:%d:%d+4++task-reference+Approve in %s'",
			findSegments(segments, "HKTAN")[0].Header.Number, code, version, findSegments(segments, "HKTAN")[0].Header.Number, escape(m.methodName),
		)
	case hktanProcess(segments) == "2":
		inner = authenticatedUPD()
	case hktanProcess(segments) == "S":
		m.mu.Lock()
		m.pollCount++
		poll := m.pollCount
		m.mu.Unlock()
		if poll == 1 {
			version := m.hktanVersion
			if version == 0 {
				version = 6
			}
			inner = "HIRMG:2:2+9050::Partially erroneous'HIRMS:3:2:3+3956::Approval pending'" +
				fmt.Sprintf("HITAN:4:%d:3+S++task-reference+Approve in app'", version)
		} else {
			inner = authenticatedUPD()
		}
	case len(findSegments(segments, "HKIDN")) > 0:
		mediumRequired := "N"
		if strings.Contains(strings.ToLower(m.methodName), "bestsign") {
			mediumRequired = "J"
		}
		version := m.hktanVersion
		if version == 0 {
			version = 6
		}
		inner = fmt.Sprintf(
			"HIRMG:2:2+0010::Accepted+3920::Allowed:%s'"+
				"HIBPA:3:3:2+1+280:10010010+Mock Bank+0+1+300'"+
				"HITANS:4:%d:2+%s:2:TECH:%s:1.0:ID:6:N:N:2048:N:2:2:N:N:N:%s:2'"+
				"HITABS:5:5:2+1+1+0'",
			m.security, version, m.security, escape(m.methodName), mediumRequired,
		)
	default:
		m.t.Fatalf("unexpected FinTS request: %#v", segments)
	}

	payload := responseMessage(dialogIDFor(segments), inner)
	_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
}

func authenticatedUPD() string {
	return "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Authenticated'" +
		"HIUPD:4:6:3+0012345678::280:10010010+DE00100100100012345678+customer+1+EUR+Ada Example++FYRST'"
}

func dialogIDFor(request []segment) string {
	if len(findSegments(request, "HKEND")) > 0 {
		return "dialog-tan"
	}
	return "dialog-tan"
}

func hktanProcess(segments []segment) string {
	matches := findSegments(segments, "HKTAN")
	if len(matches) == 0 {
		return ""
	}
	return component(matches[0], 0, 0)
}

func TestPreferredSecurityFunctionsAreApplicationConfigured(t *testing.T) {
	client := &Client{
		config: Config{Compatibility: CompatibilityOptions{
			PreferredSecurityFunctions: []string{"920", "921"},
		}},
		tanMethods: map[string]TANMethod{
			"921": {SecurityFunction: "921", HKTANVersion: 7},
			"920": {SecurityFunction: "920", HKTANVersion: 6},
		},
	}
	if got := client.selectTANMethod(); got != "920" {
		t.Fatalf("selected security function = %q, want configured 920", got)
	}
	if client.config.HKTANVersion != 6 {
		t.Fatalf("selected HKTAN version = %d, want 6", client.config.HKTANVersion)
	}
}

func TestManualTANIsCaseSensitiveAndSentInSignature(t *testing.T) {
	bank := &tanMockBank{
		t:          t,
		security:   "911",
		methodName: "photoTAN",
	}
	config := testConfig(mockHTTPClient(bank))
	config.SystemID = "system-123"
	var gotChallenge TANChallenge
	config.TANHandler = func(_ context.Context, challenge TANChallenge) (string, error) {
		gotChallenge = challenge
		return "AbC123", nil
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := client.Accounts(context.Background())
	if err != nil {
		t.Fatalf("Accounts() error = %v", err)
	}
	if len(accounts) != 1 {
		t.Fatalf("Accounts() count = %d, want 1", len(accounts))
	}
	if gotChallenge.TaskReference != "task-reference" || gotChallenge.Decoupled {
		t.Fatalf("challenge = %#v", gotChallenge)
	}

	var followup []segment
	for _, request := range bank.requests {
		if hktanProcess(request) == "2" {
			followup = request
			break
		}
	}
	if followup == nil {
		t.Fatal("bank received no HKTAN process 2 request")
	}
	signatures := findSegments(followup, "HNSHA")
	if len(signatures) != 1 ||
		component(signatures[0], 2, 0) != "12345" ||
		component(signatures[0], 2, 1) != "AbC123" {
		t.Fatalf("manual TAN signature = %#v", signatures)
	}
}

func TestBestSignDecoupledFlowSelectsMethodMediumAndPolls(t *testing.T) {
	bank := &tanMockBank{
		t:            t,
		security:     "921",
		methodName:   "BestSign",
		decoupled:    true,
		hktanVersion: 7,
	}
	config := testConfig(mockHTTPClient(bank))
	config.SystemID = "system-123"
	config.TANMedium = "BestSign Device"
	config.AutoPoll = true
	config.PollInterval = time.Millisecond
	config.MaxPolls = 3
	handlerCalls := 0
	config.TANHandler = func(_ context.Context, challenge TANChallenge) (string, error) {
		handlerCalls++
		if !challenge.Decoupled || challenge.Method != "921" {
			t.Fatalf("challenge = %#v", challenge)
		}
		return "", nil
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Accounts(context.Background()); err != nil {
		t.Fatalf("Accounts() error = %v", err)
	}
	if handlerCalls != 1 {
		t.Fatalf("TAN handler calls = %d, want 1", handlerCalls)
	}

	var initial []segment
	polls := 0
	for _, request := range bank.requests {
		switch hktanProcess(request) {
		case "4":
			initial = request
		case "S":
			polls++
			status := findSegments(request, "HKTAN")[0]
			if component(status, 4, 0) != "task-reference" ||
				component(status, 5, 0) != "N" {
				t.Fatalf("HKTAN status request = %#v", status)
			}
		}
	}
	if polls != 2 {
		t.Fatalf("poll requests = %d, want 2", polls)
	}
	hktan := findSegments(initial, "HKTAN")
	if len(hktan) != 1 || component(hktan[0], 1, 0) != "HKIDN" ||
		component(hktan[0], 10, 0) != "BestSign Device" {
		t.Fatalf("initial HKTAN = %#v", hktan)
	}
	security := findSegments(initial, "HNSHK")
	encryption := findSegments(initial, "HNVSK")
	if len(security) != 1 || component(security[0], 0, 1) != "2" ||
		component(security[0], 1, 0) != "921" {
		t.Fatalf("HNSHK security function is not 921: %#v", security)
	}
	if len(encryption) != 1 || component(encryption[0], 0, 1) != "2" {
		t.Fatalf("HNVSK profile is not PIN:2: %#v", encryption)
	}
}

func TestDecoupledHKTANVersion6UsesProcess2ByDefault(t *testing.T) {
	bank := &tanMockBank{
		t:            t,
		security:     "920",
		methodName:   "pushTAN",
		decoupled:    true,
		hktanVersion: 6,
	}
	config := testConfig(mockHTTPClient(bank))
	config.SystemID = "system-123"
	config.AutoPoll = true
	config.PollInterval = time.Millisecond
	config.MaxPolls = 3
	config.TANHandler = func(_ context.Context, challenge TANChallenge) (string, error) {
		if !challenge.Decoupled || challenge.HKTANVersion != 6 {
			t.Fatalf("challenge = %#v", challenge)
		}
		return "", nil
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Accounts(context.Background()); err != nil {
		t.Fatalf("Accounts() error = %v", err)
	}

	followups := 0
	for _, request := range bank.requests {
		if hktanProcess(request) != "2" {
			continue
		}
		followups++
		status := findSegments(request, "HKTAN")[0]
		if status.Header.Version != 6 ||
			component(status, 4, 0) != "task-reference" ||
			component(status, 5, 0) != "N" {
			t.Fatalf("HKTAN v6 process-2 request = %#v", status)
		}
	}
	if followups != 1 {
		t.Fatalf("HKTAN v6 follow-ups = %d, want 1", followups)
	}
}

func TestHITANS7PollingParametersControlDecoupledSchedule(t *testing.T) {
	response, err := parseSegments([]byte(
		"HIRMS:3:2:4+3920::Allowed:921'" +
			"HITANS:13:7:4+1+1+0+N:N:0:" +
			"921:2:PushTan1-Push:Decoupled::PushTan1-Push:::" +
			"PushTan1-Push:999:N:1:N:0:2:N:J:00:2:N:9:60:5:7:N:J'",
	))
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{
		config: Config{
			SecurityFunction: "921",
			PollInterval:     2 * time.Second,
			MaxPolls:         100,
		},
		tanMethods: make(map[string]TANMethod),
	}
	client.updateTANParameters(response)

	method := client.tanMethods["921"]
	if method.Name != "PushTan1-Push" || method.HKTANVersion != 7 ||
		!method.MediumRequired {
		t.Fatalf("TAN method = %#v", method)
	}
	firstDelay, nextDelay, maxPolls := client.decoupledPollingSchedule()
	if firstDelay != 5*time.Second || nextDelay != 7*time.Second || maxPolls != 60 {
		t.Fatalf(
			"polling schedule = (%s, %s, %d), want (5s, 7s, 60)",
			firstDelay,
			nextDelay,
			maxPolls,
		)
	}
}

func TestConsorsPublicTANParameters(t *testing.T) {
	// Public HITANS segment from an anonymous Consorsbank parameter query on
	// 2026-09-22. No login, account details or dialog identifiers are included.
	response, err := parseSegments([]byte(
		"HITANS:14:7:3+1+1+0+N:N:1:" +
			"900:2:MS1.0.0:photoTAN::SecurePlus TAN Generator:9:1:TAN:999:J:1:N:0:0:N:J:00:0:J:1::::::" +
			"901:2:MS2.0.0:Decoupled::Consorsbank/myPrivateBank App:::TAN:999:J:1:N:0:0:N:J:00:0:J:1:60:5:5:N:J'",
	))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(testConfig(nil))
	if err != nil {
		t.Fatal(err)
	}
	client.updateTANParameters(response)
	if len(client.tanMethods) != 2 {
		t.Fatalf("TAN method count = %d, want 2", len(client.tanMethods))
	}
	app := client.tanMethods["901"]
	if app.Name != "Consorsbank/myPrivateBank App" || !app.Decoupled || app.MediumRequired ||
		app.HKTANVersion != 7 || app.TANProcess != "2" {
		t.Fatalf("app method = %#v", app)
	}
	generator := client.tanMethods["900"]
	if generator.Name != "SecurePlus TAN Generator" || generator.Decoupled || generator.MediumRequired {
		t.Fatalf("generator method = %#v", generator)
	}
	client.config.SecurityFunction = client.selectTANMethod()
	if client.config.SecurityFunction != "901" {
		t.Fatalf("selected method = %s, want 901", client.config.SecurityFunction)
	}
	firstDelay, nextDelay, maxPolls := client.decoupledPollingSchedule()
	if firstDelay != 5*time.Second || nextDelay != 5*time.Second || maxPolls != 60 ||
		client.tanPolling["901"].automaticPollingDisabled {
		t.Fatalf("unexpected app polling parameters: %#v", client.tanPolling["901"])
	}
}

func TestTANMediaDiscovery(t *testing.T) {
	bank := &tanMockBank{
		t:          t,
		security:   "921",
		methodName: "BestSign",
	}
	config := testConfig(mockHTTPClient(bank))
	config.SystemID = "system-123"
	config.Compatibility.TANMediumRequiredForFunctions = []string{"921"}
	config.TANHandler = func(context.Context, TANChallenge) (string, error) {
		return "123456", nil
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	media, err := client.TANMedia(context.Background())
	if err != nil {
		t.Fatalf("TANMedia() error = %v", err)
	}
	if len(media) != 1 || media[0].Name != "BestSign Device" {
		t.Fatalf("TANMedia() = %#v", media)
	}
	bank.mu.Lock()
	requests := append([][]segment(nil), bank.requests...)
	bank.mu.Unlock()
	var initRequest, mediaRequest []segment
	for _, request := range requests {
		if len(findSegments(request, "HKIDN")) > 0 {
			initRequest = request
		}
		if len(findSegments(request, "HKTAB")) > 0 {
			mediaRequest = request
		}
	}
	if initRequest == nil || mediaRequest == nil {
		t.Fatalf("bank requests lack separate initialization/HKTAB: %#v", requests)
	}
	if len(findSegments(initRequest, "HKTAB")) != 0 ||
		len(findSegments(mediaRequest, "HKIDN")) != 0 {
		t.Fatalf("HKIDN and HKTAB were not sent separately")
	}
	initHKTAN := findSegments(initRequest, "HKTAN")
	if len(initHKTAN) != 1 ||
		component(initHKTAN[0], 0, 0) != "4" ||
		component(initHKTAN[0], 1, 0) != "HKTAB" ||
		component(initHKTAN[0], 10, 0) != "noref" {
		t.Fatalf("TAN-media initialization HKTAN = %#v", initHKTAN)
	}
	signatures := findSegments(mediaRequest, "HNSHK")
	if len(signatures) != 1 || component(signatures[0], 1, 0) != "921" {
		t.Fatalf("HKTAB security signature = %#v, want function 921", signatures)
	}
}

func TestTANMediaDiscoveryStartsWithStrongAuthentication(t *testing.T) {
	var initRequests, hktabRequests, polls int
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		raw, _ := base64.StdEncoding.DecodeString(string(body))
		segments, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}

		var inner string
		switch {
		case len(findSegments(segments, "HKEND")) > 0:
			inner = "HIRMG:2:2+0100::Dialog ended'"
		case len(findSegments(segments, "HKIDN")) > 0:
			initRequests++
			if len(findSegments(segments, "HKTAB")) != 0 {
				t.Fatalf("TAN-media init contains HKTAB: %#v", segments)
			}
			hktan := findSegments(segments, "HKTAN")
			if len(hktan) != 1 ||
				component(hktan[0], 0, 0) != "4" ||
				component(hktan[0], 1, 0) != "HKTAB" ||
				component(hktan[0], 10, 0) != "noref" {
				t.Fatalf("HKTAB HKTAN = %#v", hktan)
			}
			inner = fmt.Sprintf("HIRMG:2:2+9050::Teilweise fehlerhaft'HIRMS:3:2:%d+3955::Freigabe erforderlich.'HITAN:4:7:%d+4++media-task+Approve media lookup'", hktan[0].Header.Number, hktan[0].Header.Number)
		case hktanProcess(segments) == "2":
			polls++
			hktan := findSegments(segments, "HKTAN")
			if len(hktan) != 1 || hktan[0].Header.Version != 7 {
				t.Fatalf("configured HKTAN status request = %#v", hktan)
			}
			inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Approved'"
		case len(findSegments(segments, "HKTAB")) > 0:
			hktabRequests++
			if len(findSegments(segments, "HKIDN")) != 0 ||
				len(findSegments(segments, "HKTAN")) != 0 {
				t.Fatalf("HKTAB order has invalid companion segments: %#v", segments)
			}
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HITAB:4:5:3+0+1:CARD:SEQ:APP:ACCOUNT:20260101:20300101:LIST:?+49123:?+49***:BestSign Phone:0:20260723'"
		default:
			t.Fatalf("unexpected request: %#v", segments)
		}
		payload := responseMessage("dialog-media-sca", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.BankCode = "10010010"
	config.SystemID = "system-123"
	config.SecurityFunction = "921"
	config.HKTANVersion = 7
	config.AutoPoll = true
	config.PollInterval = time.Millisecond
	config.Compatibility.TANMediumRequiredForFunctions = []string{"921"}
	config.Compatibility.DecoupledPollProcess = "2"
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true
	client.hktabVersion = 5

	media, err := client.TANMedia(context.Background())
	if err != nil {
		t.Fatalf("TANMedia() error = %v", err)
	}
	if len(media) != 1 || media[0].Name != "BestSign Phone" {
		t.Fatalf("TANMedia() = %#v", media)
	}
	if initRequests != 1 || hktabRequests != 1 || polls != 1 {
		t.Fatalf(
			"requests: init=%d HKTAB=%d polls=%d",
			initRequests,
			hktabRequests,
			polls,
		)
	}
}

func TestTANMediaDiscoveryFallsBackWhenInitializationSCAIsRejected(t *testing.T) {
	var initRequests int
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		raw, _ := base64.StdEncoding.DecodeString(string(body))
		segments, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}

		var inner string
		switch {
		case len(findSegments(segments, "HKEND")) > 0:
			inner = "HIRMG:2:2+0100::Dialog ended'"
		case len(findSegments(segments, "HKIDN")) > 0:
			initRequests++
			if len(findSegments(segments, "HKTAN")) > 0 {
				inner = "HIRMG:2:2+9800::Dialog aborted+9050::Message contains errors'" +
					"HIRMS:3:2:4+9340::Invalid signature'"
			} else {
				inner = "HIRMG:2:2+0010::Dialog initialized'"
			}
		case len(findSegments(segments, "HKTAB")) > 0:
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HITAB:3:5:3+0+1:CARD:SEQ:APP:ACCOUNT:20260101:20300101:LIST:?+49123:?+49***:Fallback Device:0:20260723'"
		default:
			t.Fatalf("unexpected request: %#v", segments)
		}
		payload := responseMessage("dialog-media-fallback", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "system-123"
	config.SecurityFunction = "921"
	config.HKTANVersion = 7
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true
	client.hktabVersion = 5

	media, err := client.TANMedia(context.Background())
	if err != nil {
		t.Fatalf("TANMedia() error = %v", err)
	}
	if initRequests != 2 {
		t.Fatalf("initialization requests = %d, want HKTAN attempt plus fallback", initRequests)
	}
	if len(media) != 1 || media[0].Name != "Fallback Device" {
		t.Fatalf("TANMedia() = %#v", media)
	}
}

func TestTransactionCommandCanTriggerBestSign(t *testing.T) {
	requestCount := 0
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		raw, _ := base64.StdEncoding.DecodeString(string(body))
		segments, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}
		requestCount++
		inner := ""
		if requestCount == 1 {
			if len(findSegments(segments, "HKKAZ")) != 1 || hktanProcess(segments) != "4" {
				t.Fatalf("transaction request lacks HKKAZ/HKTAN process 4: %#v", segments)
			}
			reference := findSegments(segments, "HKTAN")[0].Header.Number
			inner = fmt.Sprintf("HIRMG:2:2+9050::Partially erroneous'HIRMS:3:2:%d+3955::Approve transaction'HITAN:4:7:%d+4++transaction-task+Approve transaction in BestSign'", reference, reference)
		} else {
			hktan := findSegments(segments, "HKTAN")
			if hktanProcess(segments) != "2" ||
				len(hktan) != 1 ||
				hktan[0].Header.Version != 7 {
				t.Fatalf("configured HKTAN v7 follow-up must use process 2: %#v", segments)
			}
			statement := ":20:START\r\n:25:DE00100100100012345678\r\n:60F:C260101EUR1,00\r\n" +
				":61:2601020102D1,00NTRFREF//BANK\r\n:86:835?20Test\r\n:62F:C260102EUR0,00\r\n-"
			inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Approved'" +
				"HIKAZ:4:6:3+" + binary([]byte(statement)) + "'"
		}
		payload := responseMessage("dialog-business", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.BankCode = "10010010"
	config.SystemID = "system-123"
	config.SecurityFunction = "921"
	config.TANMedium = "BestSign Device"
	config.HKTANVersion = 7
	config.AutoPoll = true
	config.PollInterval = time.Millisecond
	config.Compatibility.ForceTANForOperations = []string{"HKKAZ"}
	config.Compatibility.DecoupledPollProcess = "2"
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	d := &dialog{
		id:          "dialog-business",
		nextMessage: 1,
		systemID:    "system-123",
		security:    "921",
	}
	response, err := client.executeCommand(
		context.Background(),
		d,
		"HKKAZ:{n}:6+0012345678::280:10010010+N++++'",
		"HKKAZ",
	)
	if err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}
	if len(transactionData(response)) == 0 {
		t.Fatal("approved transaction response has no HIKAZ data")
	}
}

func TestTransactionCommandRetries9370WithHKTAN(t *testing.T) {
	requests := 0
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		raw, _ := base64.StdEncoding.DecodeString(string(body))
		segments, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}
		requests++

		var inner string
		switch requests {
		case 1:
			if len(findSegments(segments, "HKKAZ")) != 1 ||
				len(findSegments(segments, "HKTAN")) != 0 {
				t.Fatalf("initial HKKAZ request = %#v", segments)
			}
			inner = "HIRMG:2:2+9050::Message contains errors'" +
				"HIRMS:3:2:3+9370::Insufficient signatures'"
		case 2:
			hktan := findSegments(segments, "HKTAN")
			if len(findSegments(segments, "HKKAZ")) != 1 ||
				len(hktan) != 1 ||
				component(hktan[0], 0, 0) != "4" ||
				component(hktan[0], 1, 0) != "HKKAZ" {
				t.Fatalf("HKKAZ retry lacks process-4 HKTAN: %#v", segments)
			}
			inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Transactions returned'"
		default:
			t.Fatalf("unexpected request %d: %#v", requests, segments)
		}
		payload := responseMessage("dialog-9370", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "system-123"
	config.SecurityFunction = "921"
	config.HKTANVersion = 7
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	dialog := &dialog{
		id:          "dialog-9370",
		nextMessage: 1,
		systemID:    "system-123",
		security:    "921",
	}
	_, err = client.executeCommand(
		context.Background(),
		dialog,
		"HKKAZ:{n}:7+DE00100100100012345678:MARKDEF1100+N++++'",
		"HKKAZ",
	)
	if err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
}

func TestTransactionCommandRefreshesTANMediumAfter9075And9010(t *testing.T) {
	businessRequests := 0
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		raw, _ := base64.StdEncoding.DecodeString(string(body))
		segments, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}

		var inner string
		switch {
		case len(findSegments(segments, "HKIDN")) > 0:
			if len(findSegments(segments, "HKTAB")) != 0 {
				t.Fatalf("TAN-media init contains HKTAB: %#v", segments)
			}
			inner = "HIRMG:2:2+0010::Dialog initialized'"
		case len(findSegments(segments, "HKTAB")) > 0:
			if len(findSegments(segments, "HKIDN")) != 0 {
				t.Fatalf("HKTAB shares a message with HKIDN: %#v", segments)
			}
			inner = "HIRMG:2:2+0010::Accepted'" +
				"HITAB:3:5:3+0+1:CARD:SEQ:APP:ACCOUNT:20260101:20300101:LIST:?+49123:?+49***:BestSign Device:0:20260723'"
		case len(findSegments(segments, "HKEND")) > 0:
			inner = "HIRMG:2:2+0100::Dialog ended'"
		case len(findSegments(segments, "HKKAZ")) > 0:
			businessRequests++
			switch businessRequests {
			case 1:
				if hktanProcess(segments) != "" {
					t.Fatalf("first request unexpectedly contains HKTAN: %#v", segments)
				}
				inner = "HIRMG:2:2+9050::Teilweise fehlerhaft'" +
					"HIRMS:3:2:3+9075::Starke Authentifizierung erforderlich.'"
			case 2:
				if hktanProcess(segments) != "4" ||
					component(findSegments(segments, "HKTAN")[0], 10, 0) != "Stale Device" {
					t.Fatalf("first HKTAN retry is wrong: %#v", segments)
				}
				inner = "HIRMG:2:2+9050::Teilweise fehlerhaft'" +
					"HIRMS:3:2:4+9010::TAN-Gerät nicht bekannt.'"
			default:
				if hktanProcess(segments) != "4" ||
					component(findSegments(segments, "HKTAN")[0], 10, 0) != "BestSign Device" {
					t.Fatalf("refreshed HKTAN retry is wrong: %#v", segments)
				}
				inner = "HIRMG:2:2+0010::Accepted'HIRMS:3:2:3+0020::Transactions returned'"
			}
		default:
			t.Fatalf("unexpected request: %#v", segments)
		}
		payload := responseMessage("dialog-9075", inner)
		_, _ = writer.Write([]byte(base64.StdEncoding.EncodeToString([]byte(payload))))
	})

	config := testConfig(mockHTTPClient(handler))
	config.SystemID = "system-123"
	config.SecurityFunction = "921"
	config.TANMedium = "Stale Device"
	config.HKTANVersion = 6
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	client.parametersReady = true
	client.hktabVersion = 5
	d := &dialog{
		id:          "dialog-9075",
		nextMessage: 1,
		systemID:    "system-123",
		security:    "921",
	}
	_, err = client.executeCommand(
		context.Background(),
		d,
		"HKKAZ:{n}:6+0012345678:EUR:280:10010010+N++++'",
		"HKKAZ",
	)
	if err != nil {
		t.Fatalf("executeCommand() error = %v", err)
	}
	if businessRequests != 3 {
		t.Fatalf("business request count = %d, want 3", businessRequests)
	}
	if client.config.TANMedium != "BestSign Device" {
		t.Fatalf("selected TAN medium = %q", client.config.TANMedium)
	}
}

func TestTANChallengeRequiresMatchingResponseAndRealTask(t *testing.T) {
	tests := []struct {
		name, wire                  string
		wantRequired, wantDecoupled bool
		wantReference               string
	}{
		{"SCA exemption on HKIDN with placeholder HITAN", "HIRMS:2:2:3+3076::No SCA'HIRMS:3:2:3+0030::Other response'HITAN:4:7:5+4++noref+nochallenge'", false, false, ""},
		{"placeholder despite matching challenge code", "HIRMS:2:2:5+3955::Approval'HITAN:3:7:5+4++noref+nochallenge'", false, false, ""},
		{"unrelated order code", "HIRMS:2:2:3+0030::Other order'HITAN:3:7:5+4++task+Approve'", false, false, ""},
		{"global code is not segment authorization", "HIRMG:2:2+3955::Other order'HITAN:3:7:5+4++task+Approve'", false, false, ""},
		{"matching exemption", "HIRMS:2:2:5+3076::No SCA+0030::Other response'HITAN:3:7:5+4++task+Approve'", false, false, ""},
		{"unrelated exemption does not suppress genuine challenge", "HIRMS:2:2:3+3076::No SCA'HIRMS:3:2:5+3955::Approval'HITAN:4:7:5+4++real-task+Approve'", true, true, "real-task"},
		{"manual challenge", "HIRMS:2:2:5+0030::TAN required'HITAN:3:7:5+4++manual-task+Enter TAN'", true, false, "manual-task"},
		{"missing task reference", "HIRMS:2:2:5+3955::Approval'HITAN:3:7:5+4+++Approve'", true, true, ""},
		{"lone placeholder is invalid", "HIRMS:2:2:5+3955::Approval'HITAN:3:7:5+4++noref+Approve'", true, true, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response, err := parseSegments([]byte(test.wire))
			if err != nil {
				t.Fatal(err)
			}
			challenge, _, required := tanChallenge(response, "901", "")
			if required != test.wantRequired || challenge.Decoupled != test.wantDecoupled || challenge.TaskReference != test.wantReference {
				t.Fatalf("tanChallenge = %#v, required=%v", challenge, required)
			}
			if !required || challenge.TaskReference == "" {
				config := testConfig(mockHTTPClient(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("unexpected bank request") })))
				config.SecurityFunction = "901"
				config.TANHandler = func(context.Context, TANChallenge) (string, error) {
					t.Fatal("unexpected approval prompt")
					return "", nil
				}
				client, err := NewClient(config)
				if err != nil {
					t.Fatal(err)
				}
				_, err = client.completeTAN(context.Background(), &dialog{}, response)
				if (err != nil) != required {
					t.Fatalf("completeTAN error=%v, required=%v", err, required)
				}
			}
		})
	}
}
