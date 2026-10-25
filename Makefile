SHELL := /bin/sh
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: all web embed build build-go test test-integration lint run migrate compose-up compose-down smoke

all: build

web:
	cd web && npm ci --no-audit --no-fund && npm run build

embed: web
	rm -rf internal/webui/dist
	mkdir -p internal/webui/dist
	cp -a web/out/. internal/webui/dist/

build: embed
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath \
		-ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)" \
		-o bin/lip ./cmd/lip

build-go:
	mkdir -p bin
	CGO_ENABLED=0 go build -trimpath \
		-ldflags="-s -w -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildDate=$(BUILD_DATE)" \
		-o bin/lip ./cmd/lip

test:
	go test ./...

# Requires a disposable PostgreSQL database; the suite resets the public schema.
test-integration:
	@test -n "$$LIP_TEST_DATABASE_URL" || (echo "set LIP_TEST_DATABASE_URL to a disposable PostgreSQL database" >&2; exit 1)
	go test ./...

lint:
	gofmt -l cmd internal
	go vet ./...
	cd web && npm run typecheck

run:
	go run ./cmd/lip

migrate:
	go run ./cmd/lip migrate

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down

smoke:
	./scripts/smoke.sh
