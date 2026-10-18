GO ?= go
SQLC ?= sqlc
VERSION ?= dev
BIN := bin/custom-visning

.PHONY: build run test lint clean generate check-generated
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

# Generated code is committed; building the app does not require sqlc.
generate:
	$(SQLC) generate
check-generated:
	$(SQLC) compile
	$(SQLC) generate
	git diff --exit-code -- internal/storage/dbgen
	@test -z "$$(git ls-files --others --exclude-standard -- internal/storage/dbgen)" || { git ls-files --others --exclude-standard -- internal/storage/dbgen; exit 1; }
