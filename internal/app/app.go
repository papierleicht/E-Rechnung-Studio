// Package app is the desktop application: it wires the Wails services the
// frontend calls and opens the main window. The root package main only embeds
// the built frontend and the product version and calls Run.
package app

import (
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
	"papierleicht.de/e-rechnung-studio/internal/account"
	"papierleicht.de/e-rechnung-studio/internal/model"
	"papierleicht.de/e-rechnung-studio/internal/updatesign"
	"papierleicht.de/e-rechnung-studio/internal/usagestats"
)

// Run starts the application with the embedded frontend build (assets, as
// main embeds frontend/dist) and the product version from version.txt.
// It returns when the last window closed; startup errors end the process.
func Run(assets fs.FS, version string, releaseNotes fs.FS) {
	currentVersion = strings.TrimSpace(version)
	configureWindowInteractions()
	dataDir, err := defaultDataDir()
	if err != nil {
		log.Fatal(err)
	}
	// A reset that the app could not finish on the way out ends here,
	// before anything opens the data.
	finishResetAtStart(dataDir)
	invoices, err := NewInvoiceService(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	defer invoices.Close()
	bankingService, err := NewBankingService(dataDir, invoices.store.DB())
	if err != nil {
		log.Fatal(err)
	}
	defer bankingService.Close()
	invoices.bank = bankingService.service
	bankingService.invoices = invoices
	mailService, err := NewMailService(dataDir, invoices)
	if err != nil {
		log.Fatal(err)
	}
	invoices.mail = mailService.mail
	if err := restoreKeptSetup(dataDir, invoices); err != nil {
		log.Printf("reset: restore setup: %v", err)
	}
	defer mailService.Close()
	accounts, err := NewAccountService(dataDir)
	if err != nil {
		log.Fatal(err)
	}
	bankingService.service.SetSubscriptionSource(accounts.manager.ProductID)
	backups := NewBackupService(dataDir, invoices, accounts.manager, account.SystemVault{})
	audits := NewAuditService(invoices, accounts.manager)
	chats := NewChatService(accounts)
	shopService := NewShopService(invoices)
	migrations := NewMigrationService(invoices)
	defer shopService.Close()
	updates := &UpdateService{store: invoices.store, notes: releaseNotes}
	setting := func(on func(model.AppSettings) bool) func() bool {
		return func() bool {
			v, err := invoices.store.AppSettings()
			return err == nil && on(v)
		}
	}
	usage = &usagestats.Recorder{
		File:    filepath.Join(dataDir, usagestats.DetailsFileName),
		Enabled: setting(func(v model.AppSettings) bool { return v.ImprovementStatistics }),
		Version: currentVersion,
	}
	// The update check and the usage report go to the account server.
	serverURL, _, _ := accountConfig()
	serverURL = strings.TrimRight(serverURL, "/")
	if serverURL != "" {
		usage.URL = serverURL + "/api/updates/usage"
	}
	usageService := &UsageService{recorder: usage}
	resets := NewResetService(dataDir, invoices, accounts.manager, account.SystemVault{})

	app := application.New(application.Options{
		Name:        "papierleicht",
		Description: "Offline electronic invoicing for German businesses",
		Services: []application.Service{
			application.NewService(invoices),
			application.NewService(updates),
			application.NewService(bankingService),
			application.NewService(mailService),
			application.NewService(accounts),
			application.NewService(backups),
			application.NewService(audits),
			application.NewService(shopService),
			application.NewService(chats),
			application.NewService(resets),
			application.NewService(migrations),
			application.NewService(usageService),
		},
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
		Mac:    application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		// On macOS the process ends inside app.Run, so deferred calls in Run
		// never run. After a reset in the app, close every file here and
		// remove the data folder once the services have shut down.
		PostShutdown: func() {
			if !resetPending(dataDir) {
				return
			}
			shopService.Close()
			mailService.Close()
			bankingService.Close()
			invoices.Close()
			if err := finishPendingReset(dataDir); err != nil {
				log.Printf("reset: %v", err)
			}
		},
	})
	// Updates come from GitHub Releases and install only with a signature
	// that matches the public key built into the app (internal/updatesign).
	// Until 1.2 the repository was papierleicht/E-Rechnung-Studio; GitHub
	// redirects that name, so older versions still find these releases.
	// The check asks papierleicht's server first, which answers with GitHub's
	// release and counts the check by app version and system; with "Bei der
	// Verbesserung helfen" on, it also carries marks for the first check of a
	// day, week and month (internal/usagestats). GitHub itself is asked only
	// when that server cannot be reached. The packages come from GitHub, on
	// this path through a redirect of the server that counts the update.
	const releaseRepository = "papierleicht/papierleicht-app"
	updateKey, err := updatesign.ParsePublicKey(updatesign.PublicKey)
	if err != nil {
		log.Fatal(err)
	}
	var providers []updater.Provider
	if server, err := url.Parse(serverURL); err == nil && server.Host != "" {
		usageMarks = &usagestats.Transport{
			Host:    server.Host,
			Enabled: setting(func(v model.AppSettings) bool { return v.ImprovementStatistics }),
			Version: currentVersion,
			File:    filepath.Join(dataDir, usagestats.FileName),
		}
		viaServer, err := github.New(github.Config{
			Repository: releaseRepository,
			BaseURL:    server.String() + "/api/updates/github",
			HTTPClient: &http.Client{Timeout: 30 * time.Second, Transport: usageMarks},
		})
		if err != nil {
			log.Fatal(err)
		}
		providers = append(providers, &updatesign.Provider{Inner: viaServer, Repository: releaseRepository, BaseURL: server.String() + "/api/updates/files", Label: "papierleicht"})
	}
	releases, err := github.New(github.Config{Repository: releaseRepository, BaseURL: updateAPIURL})
	if err != nil {
		log.Fatal(err)
	}
	providers = append(providers, &updatesign.Provider{Inner: releases, Repository: releaseRepository, BaseURL: updateFilesURL})
	if err = app.Updater.Init(updater.Config{
		CurrentVersion: currentVersion,
		Providers:      providers,
		PublicKey:      updateKey,
	}); err != nil {
		log.Fatal(err)
	}
	updates.app = app
	resets.quit = app.Quit
	updates.updater = app.Updater
	for _, name := range []string{updater.EventDownloadProgress, updater.EventVerifying, updater.EventInstalling} {
		app.Event.On(name, func(e *application.CustomEvent) { updates.onUpdaterEvent(e.Name, e.Data) })
	}

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:          "papierleicht",
		Width:          1440,
		Height:         900,
		MinWidth:       900,
		MinHeight:      600,
		EnableFileDrop: true,
		Mac: application.MacWindow{
			// Keep native traffic lights; the page head fills the titlebar row.
			TitleBar: application.MacTitleBarHiddenInset,
		},
		BackgroundColour: application.NewRGB(244, 241, 234),
		URL:              "/",
	})
	// Native file drops land on the tiles that also open the file picker; the
	// frontend marks them with data-file-drop-target and these IDs.
	window.OnWindowEvent(events.Common.WindowFilesDropped, func(event *application.WindowEvent) {
		target := event.Context().DropTargetDetails()
		if target == nil {
			return
		}
		switch target.ElementID {
		case "received-file-drop":
			app.Event.Emit("received:file-drop", readReceivedDrop(event.Context().DroppedFiles()))
		case "bank-csv-drop", "bank-csv-dialog-drop":
			app.Event.Emit("payments:csv-drop", readCSVDrop(event.Context().DroppedFiles()))
		}
	})
	// macOS hides the traffic lights in full screen: the frontend then moves the
	// sidebar toggle into their place.
	window.OnWindowEvent(events.Mac.WindowWillEnterFullScreen, func(*application.WindowEvent) {
		app.Event.Emit("window:fullscreen", true)
	})
	window.OnWindowEvent(events.Mac.WindowWillExitFullScreen, func(*application.WindowEvent) {
		app.Event.Emit("window:fullscreen", false)
	})
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if !invoices.close.needsConfirmation() {
			return
		}
		event.Cancel()
		if !invoices.close.beginPrompt() {
			return
		}

		dialog := app.Dialog.Question().
			AttachToWindow(window).
			SetTitle("Ungespeicherte Änderungen").
			SetMessage("Wenn du jetzt schließt, gehen deine ungespeicherten Änderungen verloren.")
		cancel := dialog.AddButton("Abbrechen").OnClick(func() {
			invoices.close.finishPrompt(false)
		})
		dialog.AddButton("Änderungen verwerfen").OnClick(func() {
			invoices.close.finishPrompt(true)
			go window.Close()
		})
		dialog.SetDefaultButton(cancel)
		dialog.SetCancelButton(cancel)
		dialog.Show()
	})

	shopService.Start()
	if err = app.Run(); err != nil {
		log.Fatal(err)
	}
}
