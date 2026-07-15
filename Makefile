# Grok Build Remote — agent cross-compile
MODULE  := github.com/LinespottingOrg/GrokBuildRemote-Agents
BIN     := gbr-agent
CMD     := ./cmd/gbr-agent
DIST    := dist
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all build clean test tidy cross windows darwin linux

all: build

tidy:
	go mod tidy

build:
	go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN) $(CMD)

test:
	go test ./...

clean:
	rm -rf $(DIST)

# Cross-compile matrix (run on any host with Go installed)
cross: windows darwin linux

windows:
	mkdir -p $(DIST)
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-windows-amd64.exe $(CMD)
	GOOS=windows GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-windows-arm64.exe $(CMD)

darwin:
	mkdir -p $(DIST)
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-darwin-amd64 $(CMD)
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-darwin-arm64 $(CMD)

linux:
	mkdir -p $(DIST)
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-linux-amd64 $(CMD)
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o $(DIST)/$(BIN)-linux-arm64 $(CMD)
