package fints

import (
	"fmt"
	"testing"
	"time"
)

func TestAtruviaPublicTANParametersDoNotInterpretPollingLimitsAsMethods(t *testing.T) {
	// Public GLS/Atruvia HITANS6 and HITANS7 from an anonymous inquiry on
	// 2026-09-22; no customer data or dialog identifiers are included.
	response, err := parseSegments([]byte(
		"HITANS:84:6:3+1+1+1+J:N:0:" +
			"942:2:MTAN2:mobileTAN::mobile TAN:6:1:SMS:2048:J:1:N:0:2:N:J:00:0:N:1:" +
			"944:2:SECUREGO:mobileTAN::SecureGo:6:1:TAN:2048:J:1:N:0:2:N:J:00:0:N:1:" +
			"962:2:HHD1.4:HHD:1.4:Smart-TAN plus manuell:6:1:Challenge:2048:J:1:N:0:2:N:J:00:0:N:1:" +
			"972:2:HHD1.4OPT:HHDOPT1:1.4:Smart-TAN plus optisch / USB:6:1:Challenge:2048:J:1:N:0:2:N:J:00:0:N:1:" +
			"982:2:MS1.0.0:::Smart-TAN photo:6:1:Challenge:2048:J:1:N:0:2:N:J:00:0:N:1'" +
			"HITANS:85:7:3+1+1+1+N:N:0:" +
			"946:2:DECOUPLED:Decoupled::SecureGo plus (Direktfreigabe):::TAN:2048:J:1:N:0:2:N:J:00:0:N:1:150:2:2:J:J'",
	))
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(testConfig(nil))
	if err != nil {
		t.Fatal(err)
	}
	client.updateTANParameters(response)
	if len(client.tanMethods) != 6 {
		t.Fatalf("TAN methods = %#v, want exactly the six advertised mechanisms", client.tanMethods)
	}
	for _, security := range []string{"942", "944", "946", "962", "972", "982"} {
		if _, ok := client.tanMethods[security]; !ok {
			t.Errorf("missing advertised method %s", security)
		}
	}
	method := client.tanMethods["946"]
	if method.Name != "SecureGo plus (Direktfreigabe)" || !method.Decoupled || method.HKTANVersion != 7 {
		t.Fatalf("SecureGo plus = %#v", method)
	}
	client.config.SecurityFunction = "946"
	client.config.MaxPolls = 200
	first, next, maximum := client.decoupledPollingSchedule()
	if maximum != 150 || first != 2*time.Second || next != 2*time.Second {
		t.Fatalf("polling parameters = %d, %s, %s", maximum, first, next)
	}
}

func TestTANPollingLimitsInsideSecurityFunctionRangeCannotOverrideMethodSelection(t *testing.T) {
	for _, allowed := range []string{"", "HIRMS:2:2:3+3920::Allowed:946'"} {
		t.Run(fmt.Sprintf("allowed=%t", allowed != ""), func(t *testing.T) {
			// 997 is a valid mechanism code, but here it is a polling limit.
			// A later mechanism must still be parsed at its proper boundary.
			response, err := parseSegments([]byte(allowed +
				"HITANS:3:7:3+1+1+1+N:N:0:" +
				"946:2:DECOUPLED:Decoupled::SecureGo plus:::TAN:2048:J:1:N:0:2:N:J:00:0:N:1:997:2:2:J:J:" +
				"944:2:SECUREGO:mobileTAN::SecureGo:6:1:TAN:2048:J:1:N:0:2:N:J:00:0:N:1:::::'"))
			if err != nil {
				t.Fatal(err)
			}
			client, err := NewClient(testConfig(nil))
			if err != nil {
				t.Fatal(err)
			}
			client.updateTANParameters(response)
			if _, ok := client.tanMethods["997"]; ok {
				t.Fatal("polling limit was interpreted as a TAN mechanism")
			}
			if selected := client.selectTANMethod(); selected != "946" {
				t.Fatalf("automatically selected %s, want 946", selected)
			}
			if allowed == "" {
				if _, ok := client.tanMethods["944"]; !ok {
					t.Fatal("method following polling parameters was skipped")
				}
			}
		})
	}
}
