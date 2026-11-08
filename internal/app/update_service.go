package app

import (
	"context"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
)

// currentVersion is the product version from version.txt (managed by Release
// Please). Run sets it; tests see "dev".
var currentVersion = "dev"

type UpdateStatus struct {
	CurrentVersion   string `json:"currentVersion"`
	AvailableVersion string `json:"availableVersion,omitempty"`
	UpdateAvailable  bool   `json:"updateAvailable"`
}

type UpdateService struct {
	app     *application.App
	updater *updater.Updater
}

func (s *UpdateService) Status() UpdateStatus {
	return UpdateStatus{CurrentVersion: currentVersion}
}

// Check looks for a newer release without opening the updater window.
func (s *UpdateService) Check() (UpdateStatus, error) {
	status := s.Status()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	release, err := s.updater.Check(ctx)
	if err != nil {
		return status, err
	}
	if release != nil {
		status.AvailableVersion = release.Version
		status.UpdateAvailable = true
	}
	return status, nil
}

// Open shows Wails' built-in update flow. The call returns immediately so the
// frontend remains responsive while Wails checks, downloads, and verifies.
func (s *UpdateService) Open() {
	go func() {
		if err := s.updater.CheckAndInstall(context.Background()); err != nil {
			s.app.Logger.Error("update flow", "error", err)
		}
	}()
}
