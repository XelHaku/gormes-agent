.PHONY: build run test fmt clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o bin/www-gormes ./cmd/www-gormes

run:
	go run ./cmd/www-gormes -listen :8080

test:
	go test ./...

fmt:
	gofmt -w .

clean:
	rm -rf bin
