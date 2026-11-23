# bv Makefile
#
# Build with SQLite FTS5 (full-text search) support enabled

.PHONY: web build install clean test release-gate release-package release-verify release-audit

.DEFAULT_GOAL := build

# Enable FTS5 for full-text search in SQLite exports
export CGO_CFLAGS := -DSQLITE_ENABLE_FTS5

# GUI frontend: npm ci (only when the lockfile changes), then vite build into
# web/dist/app, which the Go build embeds. Requires Node 24 (web/.nvmrc).
web/node_modules/.package-lock.json: web/package-lock.json
	npm --prefix web ci

web: web/node_modules/.package-lock.json
	npm --prefix web run build

build: web
	go build -o bv ./cmd/bv

install: web
	go install ./cmd/bv

clean:
	rm -f bv
	rm -rf web/dist/app
	go clean

test:
	go test ./...

# These targets let DSR dispatch the complete release workflow as a Make build.
# The release gate owns toolchain settings; do not inherit the legacy FTS5 flag.
release-gate: web
	env -u CGO_CFLAGS bash scripts/release_gate.sh run

release-package: web
	env -u CGO_CFLAGS bash scripts/release_gate.sh package

release-verify:
	env -u CGO_CFLAGS bash scripts/release_gate.sh verify

# Keep the audit tool outside the application's module requirements.
release-audit:
	env -u CGO_CFLAGS bash scripts/release_gate.sh audit
