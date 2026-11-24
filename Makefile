# bv Makefile
#
# Build with SQLite FTS5 (full-text search) support enabled

.PHONY: web build install clean test

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
