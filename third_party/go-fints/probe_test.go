package fints

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func probeTestConfig(client *http.Client) ProbeConfig {
	return ProbeConfig{Endpoint: "https://bank.example.test/fints", BankCode: "76030080", ProductID: "ANONYMOUS-PROBE-TEST", ProductVersion: "1.0", HTTPClient: client}
}
func probeTestResponse(dialogID string, messageNumber int, commands ...string) string {
	message, err := buildProbeMessage(dialogID, messageNumber, commands)
	if err != nil {
		panic(err)
	}
	return string(message)
}
func probeMinimalResponse(commands ...string) string {
	if commands == nil {
		commands = []string{"HIRMG:2:2+0010::Accepted'", "HIBPA:3:3+2+280:76030080+Public Bank+0+1:2+201:210:220:300:400'"}
	}
	return probeTestResponse("anonymous-test-dialog", 1, commands...)
}
func probeHTTPResponse(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Status: "200 OK", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(base64.StdEncoding.EncodeToString([]byte(body))))}
}

func TestProbeAnonymousConsorsParametersAndCleanup(t *testing.T) {
	calls := 0
	jar, _ := cookiejar.New(nil)
	endpoint, _ := url.Parse("https://bank.example.test/fints")
	jar.SetCookies(endpoint, []*http.Cookie{{Name: "private-session", Value: "must-not-be-sent"}})
	client := &http.Client{Jar: jar, Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if request.Header.Get("Cookie") != "" || request.Header.Get("Authorization") != "" {
			t.Fatal("anonymous probe sent ambient credentials")
		}
		encoded, _ := io.ReadAll(request.Body)
		raw, err := base64.StdEncoding.DecodeString(string(encoded))
		if err != nil {
			t.Fatal(err)
		}
		segments, err := parseSegments(raw)
		if err != nil {
			t.Fatal(err)
		}
		if component(segments[0], 0, 0) != fmt.Sprintf("%012d", len(raw)) {
			t.Fatal("wrong request byte size")
		}
		var kinds []string
		for _, segment := range segments {
			kinds = append(kinds, segment.Header.Type)
		}
		if calls == 1 {
			if !reflect.DeepEqual(kinds, []string{"HNHBK", "HKIDN", "HKVVB", "HNHBS"}) {
				t.Fatalf("probe sent non-anonymous segments: %v", kinds)
			}
			id := segments[1]
			if component(id, 0, 0) != "280" || component(id, 0, 1) != "76030080" || component(id, 1, 0) != "9999999999" || component(id, 2, 0) != "0" || component(id, 3, 0) != "0" {
				t.Fatal("probe identity is not anonymous")
			}
			return probeHTTPResponse(consorsAnonymousProbeFixture), nil
		}
		if calls != 2 || !reflect.DeepEqual(kinds, []string{"HNHBK", "HKEND", "HNHBS"}) {
			t.Fatalf("unexpected cleanup: %v", kinds)
		}
		if component(segments[0], 2, 0) != "ANONYMOUS-TEST-DIALOG-00000001" || component(segments[0], 3, 0) != "2" || component(segments[1], 0, 0) != "ANONYMOUS-TEST-DIALOG-00000001" {
			t.Fatal("invalid anonymous cleanup")
		}
		return probeHTTPResponse(probeTestResponse("ANONYMOUS-TEST-DIALOG-00000001", 2, "HIRMG:2:2+0100::Ended'")), nil
	})}
	result, err := Probe(context.Background(), probeTestConfig(client))
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || result.ResponseProtocol != "300" || result.BankName != "Consors" || result.BPDVersion != 2 {
		t.Fatalf("unexpected result %#v, calls %d", result, calls)
	}
	if !reflect.DeepEqual(result.AdvertisedProtocols, []string{"201", "210", "220", "300", "400"}) {
		t.Fatalf("advertised protocols conflated: %v", result.AdvertisedProtocols)
	}
	if len(result.TANMethods) != 2 || result.TANMethods[1].SecurityFunction != "901" || !result.TANMethods[1].Decoupled || result.TANMethods[1].Name != "Consorsbank/myPrivateBank App" {
		t.Fatalf("unexpected TAN methods %#v", result.TANMethods)
	}
	found := false
	tanRequirements := make(map[string]bool)
	for _, capability := range result.Capabilities {
		if capability.Operation == "HKRM" {
			t.Fatal("HIRMS return codes were misreported as a banking capability")
		}
		tanRequirements[capability.Operation] = capability.TANRequired
		if capability.Operation == "HKKAZ" {
			found = reflect.DeepEqual(capability.Versions, []int{3, 4, 5, 6, 7})
		}
	}
	if !found {
		t.Fatal("missing public statement capabilities")
	}
	for operation, expected := range map[string]bool{"HKCCS": true, "HKIPZ": true, "HKKAZ": false, "HKSAL": false} {
		if required, exists := tanRequirements[operation]; !exists || required != expected {
			t.Fatalf("public HIPINS TAN requirement for %s: got %v (present %v), want %v", operation, required, exists, expected)
		}
	}
	if !reflect.DeepEqual(result.ResponseCodes, []string{"0010", "3060", "0020", "3050"}) {
		t.Fatalf("unexpected codes %v", result.ResponseCodes)
	}
	encoded, _ := json.Marshal(result)
	for _, forbidden := range []string{"ANONYMOUS-TEST-DIALOG", "999999999999999999999999999999", "raw", "dialog_id", "customer_id", "user_id"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("result exposed %q", forbidden)
		}
	}
	if client.Jar != jar || client.CheckRedirect != nil {
		t.Fatal("caller HTTP client mutated")
	}
}

func TestProbeTANRequirementOperationCodeLengths(t *testing.T) {
	response, err := parseSegments([]byte("HIPINS:1:1+1+1+0+:::::HKCCS:J:HKTEST:J:HKRM:J:HKTOOLONG:J'"))
	if err != nil {
		t.Fatal(err)
	}
	client, err := newProbeClient(probeTestConfig(nil))
	if err != nil {
		t.Fatal(err)
	}
	client.updateParameters(response)
	if !reflect.DeepEqual(client.tanRequirements, map[string]bool{"HKCCS": true, "HKTEST": true}) {
		t.Fatalf("unexpected TAN requirement operation codes: %v", client.tanRequirements)
	}
}

func TestProbeRejectsInvalidConfigBeforeNetwork(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*ProbeConfig)
	}{
		{"missing endpoint", func(c *ProbeConfig) { c.Endpoint = "" }},
		{"http", func(c *ProbeConfig) { c.Endpoint = "http://bank.example.test/fints" }},
		{"credentials", func(c *ProbeConfig) { c.Endpoint = "https://secret:private@bank.example.test/fints" }},
		{"query", func(c *ProbeConfig) { c.Endpoint += "?secret=value" }},
		{"empty query", func(c *ProbeConfig) { c.Endpoint += "?" }},
		{"fragment", func(c *ProbeConfig) { c.Endpoint += "#secret" }},
		{"empty fragment", func(c *ProbeConfig) { c.Endpoint += "#" }},
		{"missing host", func(c *ProbeConfig) { c.Endpoint = "https:///fints" }},
		{"short BLZ", func(c *ProbeConfig) { c.BankCode = "12345" }},
		{"nonnumeric BLZ", func(c *ProbeConfig) { c.BankCode = "1234567x" }},
		{"missing product", func(c *ProbeConfig) { c.ProductID = "" }},
		{"long product", func(c *ProbeConfig) { c.ProductID = strings.Repeat("a", 26) }},
		{"long version", func(c *ProbeConfig) { c.ProductVersion = "123456" }},
		{"unicode product", func(c *ProbeConfig) { c.ProductID = "Probe🚀" }},
		{"negative timeout", func(c *ProbeConfig) { c.Timeout = -time.Second }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := probeTestConfig(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				t.Fatal("invalid config reached network")
				return nil, nil
			})})
			tc.edit(&config)
			if _, err := Probe(context.Background(), config); err == nil {
				t.Fatal("invalid config accepted")
			} else if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), "private") {
				t.Fatalf("credentials leaked: %v", err)
			}
		})
	}
}

func TestProbeRejectsNonBankAndMalformedResponses(t *testing.T) {
	valid := probeMinimalResponse()
	bpd := "HIBPA:3:3+2+280:76030080+Bank+0+1+300'"
	ok := "HIRMG:2:2+0010::Accepted'"
	mismatchReference := strings.Replace(valid, "+1'", "+1+foreign:1'", 1)
	mismatchReference = strings.Replace(mismatchReference, componentMustProbeSize(valid), fmt.Sprintf("%012d", len(mismatchReference)), 1)
	for _, tc := range []struct{ name, body string }{
		{"foreign xml", "<?xml version=\"1.0\"?><FinTSMessage/>"},
		{"html", "<html>Maintenance</html>"},
		{"truncated", valid[:len(valid)-1]},
		{"wrong size", strings.Replace(valid, "+000000000", "+100000000", 1)},
		{"wrong version", strings.Replace(valid, "+300+", "+400+", 1)},
		{"wrong message", strings.ReplaceAll(valid, "+1'", "+2'")},
		{"wrong trailer", strings.Replace(valid, "HNHBS:4:1+1'", "HNHBS:4:1+2'", 1)},
		{"missing bpd", probeTestResponse("test", 1, ok)},
		{"wrong bank", probeMinimalResponse(ok, strings.Replace(bpd, "76030080", "11111111", 1))},
		{"wrong country", probeMinimalResponse(ok, strings.Replace(bpd, "280:", "999:", 1))},
		{"missing global response", probeMinimalResponse("HIRMS:2:2:3+0020::Accepted'", bpd)},
		{"warning only", probeMinimalResponse("HIRMG:2:2+3060::Warning'", bpd)},
		{"error with bpd", probeMinimalResponse("HIRMG:2:2+0010::Accepted+9010::secret-bank-text'", bpd)},
		{"segment error", probeMinimalResponse(ok, "HIRMS:3:2:3+9010::secret-bank-text'", strings.Replace(bpd, "HIBPA:3:", "HIBPA:4:", 1))},
		{"invalid code", probeMinimalResponse("HIRMG:2:2+OK::Accepted'", bpd)},
		{"invalid bpd version", probeMinimalResponse(ok, strings.Replace(bpd, "+2+", "+-1+", 1))},
		{"missing protocols", probeMinimalResponse(ok, strings.Replace(bpd, "+300'", "+'", 1))},
		{"invalid protocols", probeMinimalResponse(ok, strings.Replace(bpd, "+300'", "+4.1'", 1))},
		{"mismatched reference", mismatchReference},
		{"nonsequential segments", probeMinimalResponse("HIRMG:3:2+0010::Accepted'", strings.Replace(bpd, "HIBPA:3:", "HIBPA:2:", 1))},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			config := probeTestConfig(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) { calls++; return probeHTTPResponse(tc.body), nil })})
			result, err := Probe(context.Background(), config)
			if err == nil {
				t.Fatalf("malformed response succeeded: %#v", result)
			}
			if strings.Contains(err.Error(), "secret-bank-text") {
				t.Fatal("bank text leaked")
			}
			if !reflect.DeepEqual(result, ProbeResult{}) || calls > 2 {
				t.Fatal("invalid response produced results or automatic retry")
			}
		})
	}
}
func componentMustProbeSize(message string) string {
	segments, _ := parseSegments([]byte(message))
	return component(segments[0], 0, 0)
}

func TestProbeTimeoutAndRedirectProtection(t *testing.T) {
	t.Run("overall timeout with custom client", func(t *testing.T) {
		calls := 0
		config := probeTestConfig(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			<-request.Context().Done()
			return nil, request.Context().Err()
		})})
		config.Timeout = 10 * time.Millisecond
		if _, err := Probe(context.Background(), config); err == nil || calls != 1 {
			t.Fatalf("timeout not enforced: calls %d, err %v", calls, err)
		}
	})
	t.Run("no redirects", func(t *testing.T) {
		calls := 0
		config := probeTestConfig(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 307, Status: "307 Temporary Redirect", Header: http.Header{"Location": {"https://other.example.test/"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		})})
		if _, err := Probe(context.Background(), config); err == nil || calls != 1 {
			t.Fatalf("redirect not rejected: calls %d, err %v", calls, err)
		}
	})
	t.Run("cleanup cannot extend deadline", func(t *testing.T) {
		calls := 0
		config := probeTestConfig(&http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return probeHTTPResponse(probeMinimalResponse()), nil
			}
			<-request.Context().Done()
			return nil, request.Context().Err()
		})})
		config.Timeout = 10 * time.Millisecond
		start := time.Now()
		if _, err := Probe(context.Background(), config); err != nil || calls != 2 || time.Since(start) > time.Second {
			t.Fatalf("cleanup exceeded budget or discarded public BPD: %v, calls %d", err, calls)
		}
	})
}

// Public anonymous BPD response from Consorsbank (2026-09-22). Only the
// temporary dialog reference was replaced; no authenticated account data.
const consorsAnonymousProbeFixture = `HNHBK:1:3+000000001989+300+ANONYMOUS-TEST-DIALOG-00000001+1+ANONYMOUS-TEST-DIALOG-00000001:1'HIRMG:2:2:+0010::Die Nachricht wurde entgegengenommen.+3060::Teilweise liegen Warnungen/Hinweise vor.'HIRMS:3:2:2+0020::Anonymer Benutzer identifiziert.'HIRMS:4:2:3+0020::Informationen fehlerfrei entgegengenommen.+3050::BPD nicht mehr aktuell. Aktuelle Version folgt.'HIBPA:5:3:3+2+280:76030080+Consors+0+1:2+201:210:220:300:400+100'HIKOM:6:4:3+280:76030080+1+3:https?://brokerage-hbci.consorsbank.de/hbci::MIM:1'HISPAS:7:1:3+1+1+0+J:J:J:urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.001.09:urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.001.03:urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.003.03:urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.002.03:urn?:swift?:xsd?:$pain.001.002.02:sepade.pain.001.001.02.xsd'HIKAZS:8:7:3+1+1+0+90:J:N'HISALS:9:3:3+1+1'HIKAZS:10:6:3+1+1+1+90:J:N'HIIPZS:11:1:3+1+1+0+:urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.001.09:urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.001.03:urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.003.03:urn?:iso?:std?:iso?:20022?:tech?:xsd?:pain.001.002.03:urn?:swift?:xsd?:$pain.001.002.02:sepade.pain.001.001.02.xsd'HICCSS:12:1:3+1+1+0'HIKAZS:13:5:3+1+1+90:J:N'HITANS:14:7:3+1+1+0+N:N:1:900:2:MS1.0.0:photoTAN::SecurePlus TAN Generator:9:1:TAN:999:J:1:N:0:0:N:J:00:0:J:1::::::901:2:MS2.0.0:Decoupled::Consorsbank/myPrivateBank App:::TAN:999:J:1:N:0:0:N:J:00:0:J:1:60:5:5:N:J'HIKAZS:15:4:3+1+1+90:J'XIADAS:16:1:3+1+1'HIKAZS:17:3:3+1+1+90:J'HIWPDS:18:2:3+1+1+J'HIWPDS:19:6:3+1+1+0+J:J:J'HIWPDS:20:5:3+1+1+J:J:J'HIPROS:21:4:3+1+1+0'HIWPDS:22:4:3+1+1+J:J:J'HIPROS:23:3:3+1+1'HIWPDS:24:3:3+1+1+J'XIADSS:25:1:3+1+1'HISALS:26:4:3+1+1'HISALS:27:5:3+1+1'HISALS:28:6:3+1+1+0'DIPINS:29:1:3+1+1+HKSPA:N:HKKAZ:N:HKSAL:N:HKIPZ:J:HKCCS:J:HKTAN:N:XKADA:N:HKWPD:N:HKPRO:N:XKADS:N'HIPINS:30:1:3+1+1+0+:::::HKSPA:N:HKKAZ:N:HKSAL:N:HKIPZ:J:HKCCS:J:HKTAN:N:XKADA:N:HKWPD:N:HKPRO:N:XKADS:N'HIUPA:31:3:3+999999999999999999999999999999+0+1'HNHBS:32:1+1'`
