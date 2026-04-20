.PHONY: build run test fmt clean

BENCHMARKS_SRC := ../benchmarks.json
DATA_DIR := internal/site/data

build: $(DATA_DIR)/benchmarks.json
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/www-gormes ./cmd/www-gormes

$(DATA_DIR)/benchmarks.json: $(BENCHMARKS_SRC)
	@mkdir -p $(DATA_DIR)
	@cp $(BENCHMARKS_SRC) $(DATA_DIR)/benchmarks.json

run:
	go run ./cmd/www-gormes -listen :8080

test:
	go test ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin
