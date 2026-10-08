.PHONY: build release ui-install ui-build test race vet check dev clean
GOCMD ?= go
NPMCMD ?= npm
VERSION ?= $(shell git describe --always --tags --dirty)
LDFLAGS = -s -w -X main.version=$(VERSION)

ui-install:
	cd ui && $(NPMCMD) ci

ui-build: ui-install
	cd ui && $(NPMCMD) run build

build: ui-build
	mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOCMD) build -trimpath -tags embedui -ldflags '$(LDFLAGS)' -o dist/wslpp.exe .

release: ui-build
	mkdir -p dist
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOCMD) build -trimpath -tags embedui -ldflags '$(LDFLAGS)' -o dist/wslpp-windows-amd64.exe .
	CGO_ENABLED=0 GOOS=windows GOARCH=arm64 $(GOCMD) build -trimpath -tags embedui -ldflags '$(LDFLAGS)' -o dist/wslpp-windows-arm64.exe .
	cd dist && sha256sum wslpp-windows-amd64.exe wslpp-windows-arm64.exe > SHA256SUMS

test:
	$(GOCMD) test ./...

race:
	$(GOCMD) test -race ./...

vet:
	$(GOCMD) vet ./...

check: test race vet ui-build

# Start Vite separately with `cd ui && npm run dev`.
dev:
	$(GOCMD) run . run --home .wslpp-dev --ui-dev

clean:
	rm -rf dist internal/web/static
