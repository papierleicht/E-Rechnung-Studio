package app

import (
	"context"
	"log"
	"slices"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"papierleicht.de/e-rechnung-studio/internal/usagestats"
)

// usage counts areas and functions for "Bei der Verbesserung helfen". Run
// sets it up; until then, and in tests, recordUse does nothing.
var usage *usagestats.Recorder

// usageMarks adds version and system to the update check and, with "Bei der
// Verbesserung helfen" on, the first-of-period marks. Run sets it up when an
// update server is configured.
var usageMarks *usagestats.Transport

// recordUse counts one use of a function, see usagestats.Recorder. Names are
// short lower-case words; the report shows them as "funktion:<name>".
func recordUse(name string) { usage.Record("funktion:" + name) }

// usageAreas are the menu areas the frontend may report.
var usageAreas = []string{"dashboard", "invoices", "received", "shops", "finance", "taxes", "customers", "products", "community", "settings"}

// usageReportEvery is how often the finished days are sent.
const usageReportEvery = 6 * time.Hour

// UsageService lets the frontend report the opened area and sends the
// counters of finished days in the background.
type UsageService struct {
	recorder *usagestats.Recorder
	stop     context.CancelFunc
	done     sync.WaitGroup
}

// RecordArea counts one visit of a menu area.
func (s *UsageService) RecordArea(area string) {
	if slices.Contains(usageAreas, area) {
		s.recorder.Record("bereich:" + area)
	}
}

func (s *UsageService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if s.recorder == nil {
		return nil
	}
	ctx, s.stop = context.WithCancel(context.Background())
	s.done.Add(1)
	go func() {
		defer s.done.Done()
		// Wait a little, so the report does not slow the start.
		wait := time.Minute
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(wait):
			}
			if err := s.recorder.Send(ctx); err != nil && ctx.Err() == nil {
				log.Printf("usage report: %v", err)
			}
			wait = usageReportEvery
		}
	}()
	return nil
}

func (s *UsageService) ServiceShutdown() error {
	if s.stop != nil {
		s.stop()
		s.done.Wait()
	}
	return nil
}
