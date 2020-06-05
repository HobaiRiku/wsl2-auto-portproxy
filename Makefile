.PHONY:build clean test dev
# Go parameters
GOCMD=go
GORUN=$(GOCMD) run
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get

# name
BINARY_NAME=wslpp

# git version
VERSION := $(shell git describe --always |sed -e "s/^v//")

# LDFLAGS
LDFLAGS = -ldflags "-s -w -X main.version=$(VERSION)"


build: mod-tidy
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GOBUILD) \
	 $(LDFLAGS)  -o ./dist/$(BINARY_NAME).exe

test:
	$(GOTEST) -v ./...

clean:
	$(GOCLEAN)
	@rm -f ./dist/$(BINARY_NAME)_*

mod-tidy:
	$(GOCMD) mod tidy

dev:
	$(GORUN) ./main.go

