SHELL := /bin/sh

GO ?= go
PNPM ?= pnpm
GOLEO_ADDR ?= :7871
SMOKE_URL ?= http://127.0.0.1:7871

.PHONY: help
help:
	@printf '%s\n' 'Goleo development commands:'
	@printf '%s\n' '  make fmt          Format Go files'
	@printf '%s\n' '  make test         Run Go tests'
	@printf '%s\n' '  make vet          Run go vet'
	@printf '%s\n' '  make frontend-install  Install frontend dependencies'
	@printf '%s\n' '  make frontend-dev      Run the frontend dev server'
	@printf '%s\n' '  make frontend-test     Run frontend tests'
	@printf '%s\n' '  make frontend-build    Build embedded frontend assets'
	@printf '%s\n' '  make readme-assets     Generate README screenshots'
	@printf '%s\n' '  make check        Run fmt, vet, tests, frontend tests, and frontend build'
	@printf '%s\n' '  make run-simple   Run examples/simple'
	@printf '%s\n' '  make run-audio    Run examples/audio'
	@printf '%s\n' '  make run-chat     Run examples/chat'
	@printf '%s\n' '  make run-voice    Run examples/voice'
	@printf '%s\n' '  make run-http     Run examples/http-wrapper'
	@printf '%s\n' '  make run-showcase-form  Run examples/showcase-form'
	@printf '%s\n' '  make run-showcase-chat  Run examples/showcase-chat'
	@printf '%s\n' '  make run-showcase-adapters  Run examples/showcase-adapters'
	@printf '%s\n' '  make run-ollama   Run examples/ollama'
	@printf '%s\n' '  make run-openai-stream  Run examples/openai-stream'
	@printf '%s\n' '  make smoke        Run a local HTTP smoke test'

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: test
test:
	$(GO) test ./...

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: check
check: fmt vet test frontend-test frontend-build

.PHONY: frontend-install
frontend-install:
	$(PNPM) --dir frontend install

.PHONY: frontend-dev
frontend-dev:
	$(PNPM) --dir frontend dev

.PHONY: frontend-test
frontend-test:
	$(PNPM) --dir frontend test

.PHONY: frontend-build
frontend-build:
	$(PNPM) --dir frontend build

.PHONY: readme-assets
readme-assets: frontend-build
	./scripts/capture-readme-assets.sh

.PHONY: run-simple
run-simple:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/simple

.PHONY: run-audio
run-audio:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/audio

.PHONY: run-chat
run-chat:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/chat

.PHONY: run-voice
run-voice:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/voice

.PHONY: run-http
run-http:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/http-wrapper

.PHONY: run-showcase-form
run-showcase-form:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/showcase-form

.PHONY: run-showcase-chat
run-showcase-chat:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/showcase-chat

.PHONY: run-showcase-adapters
run-showcase-adapters:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/showcase-adapters

.PHONY: run-ollama
run-ollama:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/ollama

.PHONY: run-openai-stream openai-stream
run-openai-stream:
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/openai-stream

openai-stream: run-openai-stream

.PHONY: smoke
smoke:
	@set -eu; \
	log_file=$$(mktemp); \
	GOLEO_ADDR=$(GOLEO_ADDR) $(GO) run ./examples/simple > "$$log_file" 2>&1 & \
	pid=$$!; \
	trap 'kill $$pid >/dev/null 2>&1 || true; rm -f "$$log_file"' EXIT INT TERM; \
	for attempt in 1 2 3 4 5 6 7 8 9 10; do \
		if curl -fsS "$(SMOKE_URL)/api/schema" >/dev/null 2>&1; then \
			break; \
		fi; \
		if ! kill -0 $$pid >/dev/null 2>&1; then \
			cat "$$log_file"; \
			exit 1; \
		fi; \
		sleep 0.5; \
	done; \
	curl -fsS "$(SMOKE_URL)/" | grep -q '<title>Goleo</title>'; \
	curl -fsS "$(SMOKE_URL)/api/schema" | grep -q '"interface-1"'; \
	curl -fsS -X POST "$(SMOKE_URL)/api/predict" \
		-H 'Content-Type: application/json' \
		-d '{"interface_id":"interface-1","data":["Smoke"]}' | grep -q 'Hello Smoke'; \
	printf '%s\n' 'smoke ok'
