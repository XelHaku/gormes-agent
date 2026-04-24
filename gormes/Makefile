.PHONY: build run test test-live lint fmt clean update-readme validate-progress generate-progress orchestrator-test orchestrator-test-all orchestrator-lint

BUILD_FLAGS := -trimpath -ldflags="-s -w"
BINARY_PATH := bin/gormes

build: validate-progress $(BINARY_PATH)
	@$(call record-benchmark)
	@$(call record-progress)
	$(MAKE) -s generate-progress
	@$(call update-readme)

$(BINARY_PATH):
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY_PATH) ./cmd/gormes

validate-progress:
	@echo "Validating progress.json..."
	@go run ./cmd/progress-gen -validate

generate-progress:
	@echo "Regenerating progress-driven markdown..."
	@go run ./cmd/progress-gen -write

define record-benchmark
	@echo "Recording benchmark..."
	@bash scripts/record-benchmark.sh
endef

define update-readme
	@echo "Updating README.md..."
	@bash scripts/update-readme.sh
endef

define record-progress
	@echo "Updating progress..."
	@bash scripts/record-progress.sh
endef

update-readme:
	@$(call update-readme)

run: build
	./bin/gormes

test:
	go test ./...

test-live:
	go test -tags=live ./...

lint:
	golangci-lint run

fmt:
	gofmt -w .
	goimports -w .

clean:
	rm -rf bin/ coverage.out

orchestrator-test:
	@bash scripts/orchestrator/tests/run.sh unit

orchestrator-test-all:
	@bash scripts/orchestrator/tests/run.sh unit integration

orchestrator-lint:
	@if command -v shellcheck >/dev/null 2>&1; then \
	  shellcheck scripts/gormes-auto-codexu-orchestrator.sh scripts/orchestrator/lib/*.sh; \
	else \
	  echo "shellcheck not installed; skipping"; \
	fi
