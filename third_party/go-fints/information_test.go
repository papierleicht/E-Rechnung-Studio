package fints

import (
	"context"
	"testing"
	"time"
)

func TestStatusProtocolNegotiatesAndFollowsEscapedCursor(t *testing.T) {
	calls := 0
	client := readTestClient(t, "HIPROS:20:3+1+1'HIPROS:21:4+1+1+0'HIPROS:22:8+1+1+0'", func(request []segment) string {
		s := findSegments(request, "HKPRO")[0]
		calls++
		if s.Header.Version != 4 || component(s, 0, 0) != "20260901" || component(s, 1, 0) != "20260930" {
			t.Fatalf("HKPRO=%#v", s)
		}
		if calls == 1 {
			return "HIPRO:4:4:3+old-dialog:8+3+20260901+120000+9000:2,1:Historical rejection:param?+one'HIRMS:5:2:3+3040::More:page?+2'"
		}
		if component(s, 3, 0) != "page+2" {
			t.Fatalf("cursor=%q", component(s, 3, 0))
		}
		return "HIPRO:4:4:3+old-dialog:9+3+20260902+120000+0020::Completed'"
	})
	from, _ := time.Parse(time.DateOnly, "2026-09-01")
	to, _ := time.Parse(time.DateOnly, "2026-09-30")
	entries, err := client.StatusProtocol(context.Background(), from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Response.Code != "9000" || entries[0].Response.Params[0] != "param+one" || entries[1].MessageNumber != 9 {
		t.Fatalf("entries=%#v", entries)
	}
}
func TestCommunicationEndpointsAreDataNotAutomaticRedirects(t *testing.T) {
	calls := 0
	client := readTestClient(t, "HIKOMS:20:4+1+1+0'", func(request []segment) string {
		s := findSegments(request, "HKKOM")[0]
		calls++
		if s.Header.Version != 4 {
			t.Fatalf("HKKOM=%#v", s)
		}
		return "HIKOM:4:4:3+280:12345678+1+3:https?://other.invalid/fints::MIM:1+2:legacy.invalid:3000'"
	})
	endpoints, err := client.CommunicationEndpoints(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(endpoints) != 1 || len(endpoints[0].Endpoints) != 2 || endpoints[0].Endpoints[0].Address != "https://other.invalid/fints" || client.config.Endpoint != "http://bank.test/fints" || calls != 1 {
		t.Fatalf("endpoints=%#v calls=%d", endpoints, calls)
	}
}
func TestInformationRequiresSupportedVersionAndRejectsMissingResponse(t *testing.T) {
	for _, op := range []string{"HKPRO", "HKKOM"} {
		t.Run(op, func(t *testing.T) {
			client := readTestClient(t, "", func(request []segment) string { t.Fatal("unadvertised command sent"); return "" })
			var err error
			if op == "HKPRO" {
				_, err = client.StatusProtocol(context.Background(), time.Time{}, time.Time{})
			} else {
				_, err = client.CommunicationEndpoints(context.Background())
			}
			if err == nil {
				t.Fatal("missing capability accepted")
			}
		})
	}
	for _, op := range []string{"HKPRO", "HKKOM"} {
		t.Run(op+"missing", func(t *testing.T) {
			client := readTestClient(t, "HIPROS:20:4+1+1+0'HIKOMS:21:4+1+1+0'", func(request []segment) string { return "HIRMS:3:2:3+0020::Accepted'" })
			var err error
			if op == "HKPRO" {
				_, err = client.StatusProtocol(context.Background(), time.Time{}, time.Time{})
			} else {
				_, err = client.CommunicationEndpoints(context.Background())
			}
			if err == nil {
				t.Fatal("missing response accepted")
			}
		})
	}
}
