# wslpp — top-level Makefile
#
# Build model:
#   make dev / run     # No frontend build; binary serves a "UI not embedded" placeholder.
#                      # Run `make ui-dev` in a separate terminal for the Vite dev server.
#   make ui-build      # Build the SPA into internal/web/static/
#   make build         # Build the Windows binary (runs ui-build, adds -tags embedui).
#   make all           # Equivalent: ui-build + build
#
# wslpp only runs on Windows; every binary target sets GOOS=windows.
# On Windows the recipes need a POSIX shell: Git for Windows' sh is used
# automatically when sh is not already on PATH (e.g. make from PowerShell).
# ProgramW6432 is the 64-bit Program Files even when make.exe is 32-bit.

ifeq ($(OS),Windows_NT)
ifeq ($(shell where sh 2>NUL),)
GIT_USR_BIN := $(subst \,/,$(or $(ProgramW6432),$(ProgramFiles)))/Git/usr/bin
export PATH := $(GIT_USR_BIN);$(PATH)
SHELL := sh.exe
endif
endif

APP_NAME    := wslpp
MODULE      := github.com/HobaiRiku/wsl2-auto-portproxy
PKG_VERSION := $(MODULE)/internal/version

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE    := $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
-X $(PKG_VERSION).Version=$(VERSION) \
-X $(PKG_VERSION).Commit=$(COMMIT) \
-X $(PKG_VERSION).BuildDate=$(DATE)

BUILD_DIR  := build/bin
DIST_DIR   := build/dist
UI_DIR     := ui
EMBED_DIR  := internal/web/static
LOCAL_HOME := $(CURDIR)/.wslpp-dev
GOARCH     ?= $(shell go env GOARCH)
# corepack ships with Node.js; plain `pnpm` is used when it is on PATH.
PNPM       ?= $(shell command -v pnpm >/dev/null 2>&1 && echo pnpm || echo corepack pnpm)
export COREPACK_ENABLE_DOWNLOAD_PROMPT := 0

DIST_TARGETS := \
windows/amd64 \
windows/arm64

.PHONY: all build run dev tidy fmt fmt-check vet test race check \
ui-install ui-dev ui-build ui-lint ui-clean \
dist dist-clean dist-list clean \
release release-snapshot release-check print-version

all: ui-build build

# ---- Go --------------------------------------------------------------------

build: ui-build
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=windows GOARCH=$(GOARCH) go build -trimpath -tags embedui -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME).exe .

# Foreground instance with an isolated data root; allows the Vite dev origin.
run:
	WSLPP_HOME=$(LOCAL_HOME) go run -ldflags "$(LDFLAGS)" . run --ui-dev

dev: run

tidy:
	go mod tidy

fmt:
	go fmt ./...

fmt-check:
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "these files need gofmt:"; echo "$$unformatted"; exit 1; fi

# Windows-only files are type-checked even when vetting from Linux/macOS.
vet:
	go vet ./...
	GOOS=windows go vet ./...

test:
	WSLPP_HOME=$(LOCAL_HOME) go test ./...

# -race needs cgo and a C compiler (gcc) on PATH.
race:
	@command -v gcc >/dev/null 2>&1 || { echo "race needs a C compiler (gcc) on PATH"; exit 1; }
	WSLPP_HOME=$(LOCAL_HOME) CGO_ENABLED=1 go test -race ./...

check: fmt-check vet test ui-lint ui-build

# ---- UI --------------------------------------------------------------------

ui-install:
	cd $(UI_DIR) && $(PNPM) install --frozen-lockfile

ui-dev:
	cd $(UI_DIR) && $(PNPM) run dev

ui-build: ui-install
	cd $(UI_DIR) && $(PNPM) run build

ui-lint: ui-install
	cd $(UI_DIR) && $(PNPM) run type-check && $(PNPM) run lint

ui-clean:
	rm -rf $(UI_DIR)/node_modules $(UI_DIR)/dist $(EMBED_DIR)

# ---- Distribution ----------------------------------------------------------

dist: dist-clean ui-build
	@mkdir -p $(DIST_DIR)
	@for target in $(DIST_TARGETS); do \
os=$${target%/*}; arch=$${target#*/}; \
out="$(DIST_DIR)/$(APP_NAME)_$(VERSION)_$${os}_$${arch}.exe"; \
echo ">> building $$out"; \
CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
go build -trimpath -tags embedui \
-ldflags "$(LDFLAGS)" \
-o "$$out" . || exit 1; \
done
	@echo ">> writing $(DIST_DIR)/SHA256SUMS"
	@cd $(DIST_DIR) && \
( command -v sha256sum >/dev/null && sha256sum $(APP_NAME)_$(VERSION)_* > SHA256SUMS ) || \
shasum -a 256 $(APP_NAME)_$(VERSION)_* > SHA256SUMS
	@ls -lh $(DIST_DIR)

dist-clean:
	rm -rf $(DIST_DIR)

dist-list:
	@echo "DIST_TARGETS:"
	@for t in $(DIST_TARGETS); do echo "  $$t"; done

clean:
	rm -rf build $(EMBED_DIR)

# ---- Release (GoReleaser; CI runs `release` on v* tags) ---------------------

release:
	goreleaser release --clean

release-snapshot:
	goreleaser release --snapshot --clean

release-check:
	goreleaser check

print-version:
	@echo "version=$(VERSION) commit=$(COMMIT) date=$(DATE)"
