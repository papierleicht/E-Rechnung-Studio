package usagestats

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type recorded struct {
	host, path, usage, version, system string
}

// fakeServer records the statistics headers of every request.
func fakeServer(t *testing.T, status *int) (*httptest.Server, *[]recorded) {
	t.Helper()
	var seen []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, recorded{r.Host, r.URL.Path, r.Header.Get(HeaderUsage), r.Header.Get(HeaderVersion), r.Header.Get(HeaderSystem)})
		w.WriteHeader(*status)
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func check(t *testing.T, client *http.Client, address string) {
	t.Helper()
	resp, err := client.Get(address)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
}

func TestTransportSendsFirstOfPeriodMarksWithoutIdentifier(t *testing.T) {
	status := http.StatusOK
	srv, seen := fakeServer(t, &status)
	host := strings.TrimPrefix(srv.URL, "http://")
	on := true
	clock := time.Date(2026, 11, 18, 9, 0, 0, 0, time.Local) // Wednesday
	file := filepath.Join(t.TempDir(), FileName)
	client := &http.Client{Transport: &Transport{Host: host, Enabled: func() bool { return on }, Version: "1.3.0", File: file, Now: func() time.Time { return clock }}}
	latest := srv.URL + "/api/updates/github/repos/papierleicht/E-Rechnung-Studio/releases/latest"

	check(t, client, latest)
	check(t, client, latest) // same day: no marks
	clock = clock.Add(24 * time.Hour)
	check(t, client, latest) // Thursday: new day, same week and month
	clock = time.Date(2026, 11, 23, 9, 0, 0, 0, time.Local)
	check(t, client, latest) // Monday: new week
	clock = time.Date(2026, 12, 1, 9, 0, 0, 0, time.Local)
	check(t, client, latest) // new month
	check(t, client, srv.URL+"/api/updates/files/x.sig")

	system := runtime.GOOS + "/" + runtime.GOARCH
	want := []string{
		"v=1; neu=tag,woche,monat; seit=2026-11",
		"v=1; neu=; seit=2026-11",
		"v=1; neu=tag; seit=2026-11",
		"v=1; neu=tag,woche; seit=2026-11",
		"v=1; neu=tag,woche,monat; seit=2026-11",
	}
	for i, w := range want {
		got := (*seen)[i]
		if got.usage != w || got.version != "1.3.0" || got.system != system {
			t.Errorf("check %d: %+v, want %q", i, got, w)
		}
	}
	if sig := (*seen)[5]; sig.usage != "" || sig.version != "" || sig.system != "" {
		t.Errorf("signature request carries statistics: %+v", sig)
	}
	raw, _ := os.ReadFile(file)
	var s state
	if err := json.Unmarshal(raw, &s); err != nil || s.Since != "2026-11" {
		t.Fatalf("state %s: %v", raw, err)
	}
	for _, key := range []string{"id", "uuid", "install"} {
		if strings.Contains(strings.ToLower(string(raw)), key) {
			t.Errorf("state file holds %q: %s", key, raw)
		}
	}
}

func TestTransportSendsNothingWhenOff(t *testing.T) {
	status := http.StatusOK
	srv, seen := fakeServer(t, &status)
	file := filepath.Join(t.TempDir(), FileName)
	client := &http.Client{Transport: &Transport{Host: strings.TrimPrefix(srv.URL, "http://"), Enabled: func() bool { return false }, Version: "1.3.0", File: file}}
	check(t, client, srv.URL+"/releases/latest")
	if got := (*seen)[0]; got.usage != "" || got.version != "" || got.system != "" {
		t.Fatalf("statistics sent while off: %+v", got)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("state file written while off: %v", err)
	}
}

func TestTransportOnlyTalksToTheUpdateServer(t *testing.T) {
	status := http.StatusOK
	srv, seen := fakeServer(t, &status)
	client := &http.Client{Transport: &Transport{Host: "lizenz.example", Enabled: func() bool { return true }, Version: "1.3.0", File: filepath.Join(t.TempDir(), FileName)}}
	check(t, client, srv.URL+"/repos/papierleicht/E-Rechnung-Studio/releases/latest")
	if got := (*seen)[0]; got.usage != "" {
		t.Fatalf("statistics sent to another host: %+v", got)
	}
}

func TestTransportSendsMarksAgainAfterAFailure(t *testing.T) {
	status := http.StatusBadGateway
	srv, seen := fakeServer(t, &status)
	client := &http.Client{Transport: &Transport{Host: strings.TrimPrefix(srv.URL, "http://"), Enabled: func() bool { return true }, Version: "1.3.0", File: filepath.Join(t.TempDir(), FileName)}}
	check(t, client, srv.URL+"/releases/latest")
	status = http.StatusOK
	check(t, client, srv.URL+"/releases/latest")
	if (*seen)[0].usage != (*seen)[1].usage || !strings.Contains((*seen)[1].usage, "neu=tag,woche,monat") {
		t.Fatalf("marks after failure: %q then %q", (*seen)[0].usage, (*seen)[1].usage)
	}
}

func TestRecorderReportsFinishedDaysOnly(t *testing.T) {
	var reports []Report
	status := http.StatusNoContent
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var rep Report
		if err := json.Unmarshal(body, &rep); err != nil {
			t.Errorf("report %s: %v", body, err)
		}
		reports = append(reports, rep)
		w.WriteHeader(status)
	}))
	defer srv.Close()
	on := true
	clock := time.Date(2026, 11, 17, 10, 0, 0, 0, time.Local)
	r := &Recorder{File: filepath.Join(t.TempDir(), DetailsFileName), Enabled: func() bool { return on }, URL: srv.URL, Version: "1.3.0", Now: func() time.Time { return clock }}

	r.Record("bereich:invoices")
	r.Record("funktion:rechnung_festschreiben")
	r.Record("funktion:rechnung_festschreiben")
	r.Record("funktion:Kein Name!") // ignored
	if err := r.Send(context.Background()); err != nil || len(reports) != 0 {
		t.Fatalf("today was sent: %v %v", reports, err)
	}
	clock = clock.Add(24 * time.Hour)
	r.Record("funktion:rechnung_pdf")
	status = http.StatusBadGateway
	if err := r.Send(context.Background()); err == nil {
		t.Fatal("failed report reported no error")
	}
	status = http.StatusNoContent
	if err := r.Send(context.Background()); err != nil {
		t.Fatal(err)
	}
	last := reports[len(reports)-1]
	want := map[string]int{"bereich:invoices": 1, "funktion:rechnung_festschreiben": 2}
	if len(last.Days) != 1 || len(last.Days["2026-11-17"]) != 2 || last.Days["2026-11-17"]["funktion:rechnung_festschreiben"] != want["funktion:rechnung_festschreiben"] || last.Version != "1.3.0" || last.V != 1 {
		t.Fatalf("report %+v", last)
	}
	n := len(reports)
	if err := r.Send(context.Background()); err != nil || len(reports) != n {
		t.Fatalf("sent day sent again: %d reports, %v", len(reports), err)
	}
	if d := r.load(); len(d.Days) != 1 || d.Days["2026-11-18"]["funktion:rechnung_pdf"] != 1 {
		t.Fatalf("kept %+v", d.Days)
	}
}

func TestRecorderForgetsEverythingWhenTurnedOff(t *testing.T) {
	on := true
	file := filepath.Join(t.TempDir(), DetailsFileName)
	r := &Recorder{File: file, Enabled: func() bool { return on }, URL: "http://127.0.0.1:1"}
	r.Record("bereich:settings")
	if _, err := os.Stat(file); err != nil {
		t.Fatal(err)
	}
	on = false
	r.Record("bereich:settings")
	if err := r.Send(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("counters kept after turning off: %v", err)
	}
	var nilRecorder *Recorder
	nilRecorder.Record("bereich:settings") // no panic before Run set it up
}

func TestRecorderKeepsAtMostAMonth(t *testing.T) {
	clock := time.Date(2026, 9, 1, 10, 0, 0, 0, time.Local)
	r := &Recorder{File: filepath.Join(t.TempDir(), DetailsFileName), Enabled: func() bool { return true }, Now: func() time.Time { return clock }}
	for i := 0; i < keepDays+10; i++ {
		r.Record("bereich:dashboard")
		clock = clock.Add(24 * time.Hour)
	}
	if d := r.load(); len(d.Days) != keepDays {
		t.Fatalf("%d days kept, want %d", len(d.Days), keepDays)
	}
}

func TestRecorderForgetDeletesAtOnce(t *testing.T) {
	file := filepath.Join(t.TempDir(), DetailsFileName)
	r := &Recorder{File: file, Enabled: func() bool { return true }}
	r.Record("funktion:rechnung_pdf")
	if err := r.Forget(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("counters kept: %v", err)
	}
	if err := r.Forget(); err != nil {
		t.Fatalf("second forget: %v", err)
	}
}
