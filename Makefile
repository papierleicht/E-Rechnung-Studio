WAILS_VERSION := v3.1.0
WINDOWS_ARCH ?= amd64
TOOLS_DIR := $(CURDIR)/.tools
export PATH := $(TOOLS_DIR):$(PATH)

.PHONY: setup dev build build-windows check testdata dev-testdata
setup: $(TOOLS_DIR)/wails3

$(TOOLS_DIR)/wails3: export GOBIN := $(TOOLS_DIR)
$(TOOLS_DIR)/wails3: Makefile
	go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

dev: setup
	wails3 dev

# A fictitious design studio for trying the app by hand, kept apart from the
# real data (internal/app/testdata_seed_test.go). Delete .testdaten to start over.
TESTDATA_DIR := $(CURDIR)/.testdaten

testdata:
	E_RECHNUNG_STUDIO_SEED_DIR="$(TESTDATA_DIR)" go test -run TestSeedTestData -count=1 -v ./internal/app | grep -E "Kontostand|Testdaten in|ok|FAIL"

dev-testdata: setup
	@test -f "$(TESTDATA_DIR)/e-rechnung-studio.db" || $(MAKE) testdata
	E_RECHNUNG_STUDIO_DATA_DIR="$(TESTDATA_DIR)" wails3 dev

build: setup
	wails3 build

# Keep Windows artifacts separate from the host's macOS/Linux executable.
build-windows: setup
	wails3 task windows:build ARCH=$(WINDOWS_ARCH) BIN_DIR=bin/windows-$(WINDOWS_ARCH)

check: build
	cd frontend && npm run check
	go test ./...
