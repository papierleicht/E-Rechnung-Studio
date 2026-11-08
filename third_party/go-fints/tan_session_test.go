package fints

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func matrixPayload(mime string, image []byte) []byte {
	result := []byte{byte(len(mime) >> 8), byte(len(mime))}
	result = append(result, mime...)
	result = append(result, byte(len(image)>>8), byte(len(image)))
	return append(result, image...)
}

func TestHITANMatrixChallengePreservesBinaryAndText(t *testing.T) {
	image := []byte("\x89PNG\r\n\x1a\n+:'@\x00test")
	data := matrixPayload("image/png", image)
	response, err := parseSegments([]byte("HIRMS:3:2:4+0030::TAN required'HITAN:4:7:4+4++task+Scan the image+" + binary(data) + "'"))
	if err != nil {
		t.Fatal(err)
	}
	challenge, _, required := tanChallenge(response, "910", "phone")
	if !required || challenge.ImageMIMEType != "image/png" || !bytes.Equal(challenge.Image, image) || !bytes.Equal(challenge.ChallengeHHD, data) || challenge.Challenge != "Scan the image" {
		t.Fatalf("unexpected challenge: %#v", challenge)
	}
	// Mutating caller-owned presentation must not alter the parsed response.
	challenge.ChallengeHHD[0] = 99
	if findSegments(response, "HITAN")[0].Fields[4].raw[0] != 0 {
		t.Fatal("challenge aliases response bytes")
	}
}

func TestChallengePresentationRejectsMalformedAndActiveImages(t *testing.T) {
	cases := [][]byte{{0, 100, 0, 1}, matrixPayload("image/svg+xml", []byte("<svg onload='test'/>")), {0, 3, 'p', 'n', 'g', 255, 255}}
	for _, data := range cases {
		challenge := TANChallenge{ChallengeHHD: data, Challenge: "Read this"}
		decodeChallengePresentation(&challenge)
		if len(challenge.Image) > 0 || challenge.ImageMIMEType != "" {
			t.Fatalf("unexpected image from %q", data)
		}
		if !bytes.Equal(challenge.ChallengeHHD, data) {
			t.Fatal("unknown HHD data discarded")
		}
	}
}

func TestLegacyChallengeTextAndImageEnvelope(t *testing.T) {
	image := []byte("\x89PNG\r\n\x1a\nexample")
	encoded := base64.StdEncoding.EncodeToString(image)
	challenge := TANChallenge{Challenge: fmt.Sprintf("CHLGUC  %04d%sCHLGTEXT0007Confirm", len(encoded), encoded)}
	decodeChallengePresentation(&challenge)
	if challenge.Challenge != "Confirm" || challenge.ImageMIMEType != "image/png" || !bytes.Equal(challenge.Image, image) {
		t.Fatalf("challenge=%#v", challenge)
	}
	malformed := TANChallenge{Challenge: "CHLGUC  9999short"}
	decodeChallengePresentation(&malformed)
	if malformed.Challenge != "CHLGUC  9999short" {
		t.Fatal("malformed text silently truncated")
	}
}

func TestDecoupledProgressAndCancellation(t *testing.T) {
	bank := &tanMockBank{t: t, security: "921", methodName: "pushTAN", decoupled: true, hktanVersion: 7}
	config := testConfig(mockHTTPClient(bank))
	config.SystemID = "system-123"
	config.AutoPoll = true
	config.PollInterval = time.Millisecond
	var attempts []int
	config.OnTANPoll = func(_ context.Context, challenge TANChallenge) error {
		attempts = append(attempts, challenge.PollAttempt)
		return nil
	}
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = client.Accounts(context.Background()); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(attempts) != "[1 2]" {
		t.Fatalf("attempts=%v", attempts)
	}

	ctx, cancel := context.WithCancel(context.Background())
	client.config.OnTANPoll = func(context.Context, TANChallenge) error { cancel(); return ctx.Err() }
	before := len(bank.requests)
	if _, err = client.Accounts(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v", err)
	}
	for _, request := range bank.requests[before:] {
		if hktanProcess(request) == "S" || len(findSegments(request, "HKEND")) > 0 {
			t.Fatal("sent poll/cleanup after cancellation")
		}
	}
}

func TestSetTANMethodAndCredentialFreeState(t *testing.T) {
	client, err := NewClient(testConfig(&http.Client{}))
	if err != nil {
		t.Fatal(err)
	}
	client.tanMethods["910"] = TANMethod{SecurityFunction: "910", HKTANVersion: 7}
	client.tanMethods["920"] = TANMethod{SecurityFunction: "920", HKTANVersion: 6}
	client.allowedSecurity = []string{"910"}
	client.config.TANMedium = "old-phone"
	client.authenticatedParametersReady = true
	if err = client.SetTANMethod("920"); err == nil {
		t.Fatal("accepted method disallowed for user")
	}
	if err = client.SetTANMethod("unknown"); err == nil {
		t.Fatal("accepted unknown method")
	}
	if err = client.SetTANMethod("910"); err != nil {
		t.Fatal(err)
	}
	state := client.State()
	if state.SecurityFunction != "910" || state.TANMedium != "" || client.config.HKTANVersion != 7 || client.authenticatedParametersReady {
		t.Fatalf("invalid method state=%#v", state)
	}
	encoded, _ := json.Marshal(state)
	if strings.Contains(string(encoded), client.config.PIN) || strings.Contains(string(encoded), "pin") {
		t.Fatal("state contains a PIN")
	}
}

func TestCredentialsNeverFollowHTTPRedirects(t *testing.T) {
	for _, code := range []int{301, 302, 303, 307, 308} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			requests := 0
			httpClient := mockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if requests > 1 {
					t.Fatal("followed redirect with credentials")
				}
				w.Header().Set("Location", "https://other-bank.test/stolen")
				w.WriteHeader(code)
			}))
			config := testConfig(httpClient)
			client, err := NewClient(config)
			if err != nil {
				t.Fatal(err)
			}
			if _, err = client.Accounts(context.Background()); err == nil {
				t.Fatal("accepted redirect")
			}
			if httpClient.CheckRedirect != nil {
				t.Fatal("mutated caller HTTP client")
			}
		})
	}
}

func TestBankErrorsRedactPINAndTAN(t *testing.T) {
	config := testConfig(mockHTTPClient(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		raw := responseMessage("test", "HIRMG:2:2+9050::PIN 12345 rejected'HIRMS:3:2:4+9340::TAN AbC123 rejected:12345:AbC123'")
		_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(raw))))
	})))
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.sendCommandsWithTAN(context.Background(), &dialog{id: "test", nextMessage: 1, systemID: "test", security: "910"}, []string{"HKTAN:{n}:7+2++++task+N'"}, "AbC123")
	var bankError *BankError
	if !errors.As(err, &bankError) {
		t.Fatalf("expected structured BankError: %v", err)
	}
	encoded, _ := json.Marshal(bankError)
	if strings.Contains(string(encoded), "12345") || strings.Contains(string(encoded), "AbC123") || strings.Contains(err.Error(), "12345") {
		t.Fatal("credential in public error")
	}
	if bankError.Code != "9340" {
		t.Fatalf("lost diagnostic code: %s", bankError.Code)
	}
}

func TestTransactionPaginationRejectsIncompleteOrRepeatedResponses(t *testing.T) {
	for _, mode := range []string{"missing", "repeated", "cycle"} {
		t.Run(mode, func(t *testing.T) {
			pages := 0
			bank := &mockBank{t: t}
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				raw, _ := base64.StdEncoding.DecodeString(string(body))
				segments, err := parseSegments(raw)
				if err != nil {
					t.Fatal(err)
				}
				if len(findSegments(segments, "HKKAZ")) == 0 {
					r.Body = io.NopCloser(bytes.NewReader(body))
					bank.ServeHTTP(w, r)
					return
				}
				pages++
				inner := "HIRMG:2:2+0010::Accepted'"
				if mode != "missing" {
					cursor := "page-a"
					if mode == "cycle" && pages == 2 {
						cursor = "page-b"
					}
					inner += "HIRMS:3:2:3+3040::More entries:" + cursor + "'HIKAZ:4:6:3+@0@'"
				}
				_, _ = w.Write([]byte(base64.StdEncoding.EncodeToString([]byte(responseMessage("dialog-1", inner)))))
			})
			client, err := NewClient(testConfig(mockHTTPClient(handler)))
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Transactions(context.Background(), Account{IBAN: "DE0012345678001234567890"}, time.Time{}, time.Time{})
			if err == nil {
				t.Fatal("incomplete transactions reported as successful")
			}
			want := "continuation token"
			if mode == "missing" {
				want = "no HIKAZ"
			}
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error=%v", err)
			}
			if pages > 3 {
				t.Fatal("unbounded repeated pagination")
			}
		})
	}
}

func TestNoHandlerErrorContainsStructuredChallenge(t *testing.T) {
	client, err := NewClient(testConfig(&http.Client{}))
	if err != nil {
		t.Fatal(err)
	}
	response, err := parseSegments([]byte("HIRMS:3:2:4+0030::TAN required'HITAN:4:7:4+4++task+Read this+" + binary([]byte("123HHD")) + "'"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.completeTAN(context.Background(), &dialog{}, response)
	var required *TANRequiredError
	if !errors.As(err, &required) || required.Details == nil || string(required.Details.ChallengeHHD) != "123HHD" {
		t.Fatalf("error=%#v", err)
	}
}

func TestUPDRecognizesStandardAndCreditCardOperationPermissions(t *testing.T) {
	response, err := parseSegments([]byte("HIUPD:5:6:2+1234567890:00:280:12345678+DE0012345678001234567890+customer+1+EUR+Ada Example++Checking+++HKKAZ:1:0+HKSAL:1:0+DKKKU:1:0'"))
	if err != nil {
		t.Fatal(err)
	}
	accounts := accountsFromUPD(response)
	if len(accounts) != 1 {
		t.Fatalf("accounts=%#v", accounts)
	}
	for _, operation := range []string{"HKKAZ", "HKSAL", "DKKKU"} {
		if !accounts[0].SupportedOperations[operation] {
			t.Fatalf("missing permission for %s", operation)
		}
	}
}

func TestDecoupledPollingHonorsBankProhibition(t *testing.T) {
	client, err := NewClient(testConfig(mockHTTPClient(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("sent forbidden poll") }))))
	if err != nil {
		t.Fatal(err)
	}
	client.config.SecurityFunction = "921"
	response, err := parseSegments([]byte("HIRMS:3:2:4+3920::Allowed:921'HITANS:13:7:4+1+1+0+N:N:0:921:2:PushTan1-Push:Decoupled::PushTan1-Push:::PushTan1-Push:999:N:1:N:0:2:N:J:00:2:N:9:60:5:7:J:N'"))
	if err != nil {
		t.Fatal(err)
	}
	client.updateTANParameters(response)
	if !client.tanMethods["921"].Decoupled {
		t.Fatal("missing decoupled flag")
	}
	_, err = client.pollDecoupled(context.Background(), &dialog{}, TANChallenge{HKTANVersion: 7, TaskReference: "task"})
	if err == nil || !strings.Contains(err.Error(), "does not permit") {
		t.Fatalf("error=%v", err)
	}
}

func TestLatin1CredentialsAndBinaryPayloadRoundTrip(t *testing.T) {
	config := testConfig(&http.Client{})
	config.UserID = "Müller"
	config.PIN = "äß?+@"
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	// UTF-8 XML and arbitrary image bytes must never be transcoded to Latin1.
	payload := []byte("<Nm>Müller €</Nm>\x89PNG\x00\xff?+'@")
	message, err := client.buildMessage("dialog", 1, "system", "910", "TÄN", []string{"HKXYZ:{n}:1+" + binary(payload) + "'"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(message, []byte{'M', 0xfc, 'l', 'l', 'e', 'r'}) {
		t.Fatal("user ID was not encoded as Latin1")
	}
	segments, err := parseSegments(message)
	if err != nil {
		t.Fatal(err)
	}
	signatures := findSegments(segments, "HNSHA")
	if len(signatures) != 1 || component(signatures[0], 2, 0) != config.PIN || component(signatures[0], 2, 1) != "TÄN" {
		t.Fatal("PIN/TAN changed during encoding")
	}
	if !bytes.Equal(findSegments(segments, "HKXYZ")[0].Fields[0].raw, payload) {
		t.Fatal("binary payload transcoded")
	}
	challengeWire, err := encodeWireText([]byte("HIRMS:3:2:4+0030::TAN erforderlich'HITAN:4:7:4+4++task+Prüfen Sie den Betrag'"))
	if err != nil {
		t.Fatal(err)
	}
	response, err := parseSegments(challengeWire)
	if err != nil {
		t.Fatal(err)
	}
	challenge, _, _ := tanChallenge(response, "910", "")
	if challenge.Challenge != "Prüfen Sie den Betrag" {
		t.Fatalf("challenge text=%q", challenge.Challenge)
	}
}

func TestUnrepresentableCredentialsFailWithoutSending(t *testing.T) {
	config := testConfig(mockHTTPClient(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("sent unrepresentable PIN") })))
	config.PIN = "€-secret"
	client, err := NewClient(config)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Accounts(context.Background())
	if err == nil || !strings.Contains(err.Error(), "ISO-8859-1") || strings.Contains(err.Error(), config.PIN) {
		t.Fatalf("error=%v", err)
	}
}
