.PHONY: build run test test-live lint fmt clean update-readme

BUILD_FLAGS := -trimpath -ldflags="-s -w"
BINARY_PATH := bin/gormes

build: $(BINARY_PATH)
	@$(call record-benchmark)
	@$(call update-readme)

$(BINARY_PATH):
	CGO_ENABLED=0 go build $(BUILD_FLAGS) -o $(BINARY_PATH) ./cmd/gormes

define record-benchmark
	@echo "Recording benchmark..."
	@bash scripts/record-benchmark.sh
endef

define update-readme
	@echo "Updating README.md..."
	@bash scripts/update-readme.sh
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
