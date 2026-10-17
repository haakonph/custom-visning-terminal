GO ?= go
VERSION ?= dev
BIN := bin/custom-visning

.PHONY: build run test lint clean
build:
	$(GO) build -trimpath -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/custom-visning
run:
	$(GO) run ./cmd/custom-visning
test:
	$(GO) test ./...
	$(GO) test -race ./...
lint:
	$(GO) vet ./...
	@unformatted="$$(gofmt -l .)" || exit 1; test -z "$$unformatted" || { printf 'Run gofmt on:\n%s\n' "$$unformatted"; exit 1; }
clean:
	rm -rf bin dist
