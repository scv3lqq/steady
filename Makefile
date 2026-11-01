BIN := bin/steady
BIN_LINUX := bin/steady-linux-amd64
SQLC_VERSION := v1.31.1
GENERATED := web/dist internal/repository/sqlite/sqlcdb

.PHONY: check check-generated check-deploy check-text lint test web generate run build build-linux

check: check-generated lint test check-deploy check-text

check-generated: web generate
	go mod tidy -diff
	git diff --exit-code -- $(GENERATED)
	@untracked="$$(git ls-files --others --exclude-standard -- $(GENERATED))"; \
	if [ -n "$$untracked" ]; then echo "untracked generated files:"; echo "$$untracked"; exit 1; fi

web:
	cd web && npm ci --no-audit --no-fund && npm run build

generate:
	CGO_ENABLED=0 go run github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION) generate

lint:
	golangci-lint run ./...

# cgo stays enabled here on purpose: -race needs it on Linux. Only the binaries are built with CGO_ENABLED=0.
test:
	go test -race ./...
	cd web && npm test

check-deploy:
	set -e; for script in deploy/*.sh; do bash -n "$$script"; done
	@if command -v shellcheck >/dev/null 2>&1; then shellcheck deploy/*.sh; else echo "shellcheck not installed, skipped"; fi

check-text:
	python3 scripts/check_text.py

run: web
	go run ./cmd/steady

build:
	CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o $(BIN) ./cmd/steady

build-linux:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags='-s -w' -o $(BIN_LINUX) ./cmd/steady
