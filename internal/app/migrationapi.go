package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"papierleicht.de/e-rechnung-studio/internal/umzug"
)

// APIStatus is the state of a move over the API of a program. The page asks
// for it while the run goes on in the background.
type APIStatus struct {
	// State is "idle", "checking", "running", "done", "failed" or "cancelled".
	State   string `json:"state"`
	Source  string `json:"source"`
	Account string `json:"account"`
	// Phase says what the run does right now, shown under the progress.
	Phase      string          `json:"phase"`
	Total      int             `json:"total"`
	Done       int             `json:"done"`
	Imported   int             `json:"imported"`
	Duplicates int             `json:"duplicates"`
	Failed     []umzug.Skipped `json:"failed"`
	Message    string          `json:"message"`
	// Customers counts the customers read; they wait for the customer's review.
	Customers int `json:"customers"`
}

type apiRun struct {
	mu        sync.Mutex
	status    APIStatus
	cancel    context.CancelFunc
	customers *umzug.Table
}

func (r *apiRun) update(f func(*APIStatus)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f(&r.status)
}

// ConnectAPI checks the key and starts reading the program in the
// background. The key stays in memory for this run only.
func (s *MigrationService) ConnectAPI(source, key string) (APIStatus, error) {
	provider, ok := umzug.NewProvider(source, key)
	if !ok {
		return APIStatus{}, errors.New("Für dieses Programm gibt es noch keine direkte Verbindung.")
	}
	if len(key) < 8 {
		return APIStatus{}, errors.New("Gib den vollständigen Schlüssel ein.")
	}
	s.apiMu.Lock()
	if s.api != nil {
		s.api.mu.Lock()
		state := s.api.status.State
		s.api.mu.Unlock()
		if state == "checking" || state == "running" {
			s.apiMu.Unlock()
			return APIStatus{}, errors.New("Ein Abruf läuft schon.")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := &apiRun{cancel: cancel, status: APIStatus{State: "checking", Source: source, Phase: "Schlüssel wird geprüft …", Failed: []umzug.Skipped{}}}
	s.api = run
	s.apiMu.Unlock()

	check, checkCancel := context.WithTimeout(ctx, 30*time.Second)
	account, err := provider.Check(check)
	checkCancel()
	if err != nil {
		cancel()
		run.update(func(st *APIStatus) { st.State, st.Message, st.Phase = "failed", err.Error(), "" })
		return run.snapshot(), err
	}
	run.update(func(st *APIStatus) { st.State, st.Account, st.Phase = "running", account, "Kunden werden gelesen …" })
	go s.runAPI(ctx, run, source, provider)
	return run.snapshot(), nil
}

func (r *apiRun) snapshot() APIStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	st := r.status
	st.Failed = append([]umzug.Skipped{}, r.status.Failed...)
	return st
}

func (s *MigrationService) runAPI(ctx context.Context, run *apiRun, source string, provider umzug.Provider) {
	defer run.cancel()
	finish := func(err error) {
		run.update(func(st *APIStatus) {
			st.Phase = ""
			switch {
			case errors.Is(ctx.Err(), context.Canceled):
				st.State, st.Message = "cancelled", "Abgebrochen. Was schon übernommen ist, bleibt im Altbestand; ein neuer Abruf holt nur den Rest."
			case err != nil:
				st.State, st.Message = "failed", err.Error()
			default:
				st.State = "done"
			}
		})
	}
	customers, err := provider.Customers(ctx)
	if err != nil {
		if ctx.Err() != nil {
			finish(nil)
			return
		}
		// Documents matter more than contacts: go on and say what failed.
		run.update(func(st *APIStatus) {
			st.Failed = append(st.Failed, umzug.Skipped{Origin: "Kunden", Reason: err.Error()})
		})
	} else {
		run.mu.Lock()
		run.customers = &customers
		run.status.Customers = len(customers.Rows)
		run.mu.Unlock()
	}
	run.update(func(st *APIStatus) { st.Phase = "Belegliste wird gelesen …" })
	refs, err := provider.Documents(ctx)
	if err != nil {
		finish(err)
		return
	}
	own, err := s.own()
	if err != nil {
		finish(err)
		return
	}
	run.update(func(st *APIStatus) { st.Total, st.Phase = len(refs), "Belege werden übernommen …" })
	for _, ref := range refs {
		if ctx.Err() != nil {
			break
		}
		sourceID := source + ":" + ref.ID
		known, err := s.invoices.store.HasLegacySource(source, sourceID)
		if err != nil {
			finish(err)
			return
		}
		if known {
			run.update(func(st *APIStatus) { st.Done++; st.Duplicates++ })
			continue
		}
		files, err := provider.Files(ctx, ref)
		if err != nil || len(files) == 0 {
			if ctx.Err() != nil {
				break
			}
			reason := "Zu diesem Beleg gibt es keine Datei."
			if err != nil {
				reason = err.Error()
			}
			label := ref.Meta.Number
			if label == "" {
				label = ref.ID
			}
			run.update(func(st *APIStatus) {
				st.Done++
				st.Failed = append(st.Failed, umzug.Skipped{Origin: label, Reason: reason})
			})
			continue
		}
		d, err := umzug.Read(ref.Group(files), source, own)
		if err == nil {
			d.SourceID = sourceID
			d.Origin = ref.Meta.Number
			if d.Basis == "api" && ref.Kind != "" {
				d.Kind = ref.Kind
			}
			var added bool
			if _, added, err = s.invoices.store.AddLegacyDocument(d); err == nil {
				run.update(func(st *APIStatus) {
					st.Done++
					if added {
						st.Imported++
					} else {
						st.Duplicates++
					}
				})
				continue
			}
		}
		run.update(func(st *APIStatus) {
			st.Done++
			st.Failed = append(st.Failed, umzug.Skipped{Origin: ref.Meta.Number, Reason: err.Error()})
		})
	}
	finish(nil)
}

// APIStatus returns the state of the current or last run.
func (s *MigrationService) APIStatus() APIStatus {
	s.apiMu.Lock()
	run := s.api
	s.apiMu.Unlock()
	if run == nil {
		return APIStatus{State: "idle", Failed: []umzug.Skipped{}}
	}
	return run.snapshot()
}

// CancelAPI stops the current run; what is archived stays.
func (s *MigrationService) CancelAPI() {
	s.apiMu.Lock()
	run := s.api
	s.apiMu.Unlock()
	if run != nil {
		run.cancel()
	}
}

func (s *MigrationService) apiCustomers() (umzug.Table, error) {
	s.apiMu.Lock()
	run := s.api
	s.apiMu.Unlock()
	if run == nil {
		return umzug.Table{}, errors.New("Es gibt keinen Abruf mit Kunden.")
	}
	run.mu.Lock()
	defer run.mu.Unlock()
	if run.customers == nil {
		return umzug.Table{}, errors.New("Es gibt keinen Abruf mit Kunden.")
	}
	return *run.customers, nil
}

// PreviewAPIList shows the customers read over the API before saving.
func (s *MigrationService) PreviewAPIList(mapping map[string]int) (ListPreview, error) {
	return s.readList(umzug.ListCustomers, s.apiCustomers, mapping)
}

// ImportAPIList saves the new customers read over the API.
func (s *MigrationService) ImportAPIList(mapping map[string]int) (ListImportResult, error) {
	return s.importList(umzug.ListCustomers, s.apiCustomers, mapping)
}
