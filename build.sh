#!/bin/bash
set -e

GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'
info()  { echo -e "${GREEN}[✓]${NC} $1"; }
error() { echo -e "${RED}[✗]${NC} $1"; exit 1; }

# ── Clean ──
if [[ "$1" == "--clean" ]]; then
    rm -rf bin/ frontend/dist/
    info "Cleaned build artifacts"
fi

# ── Check deps ──
command -v go     >/dev/null 2>&1 || error "go not found"
command -v node   >/dev/null 2>&1 || error "node not found"
command -v npm    >/dev/null 2>&1 || error "npm not found"
# wails3 may be in Go's bin directory (not always in $PATH)
if ! command -v wails3 >/dev/null 2>&1; then
    GOBIN=$(go env GOBIN 2>/dev/null)
    GOPATH_BIN=$(go env GOPATH 2>/dev/null)/bin
    if [ -n "$GOBIN" ] && [ -f "$GOBIN/wails3" ]; then
        export PATH="$GOBIN:$PATH"
    elif [ -f "$GOPATH_BIN/wails3" ]; then
        export PATH="$GOPATH_BIN:$PATH"
    else
        error "wails3 not found (go install github.com/wailsapp/wails/v3/cmd/wails3@latest)"
    fi
fi

# ── Install frontend deps if needed ──
if [ ! -d "frontend/node_modules" ]; then
    info "Installing frontend dependencies..."
    (cd frontend && npm install)
fi

# ── Build ──
info "Building frontend..."
(cd frontend && npm run build)

info "Building Go binary..."
# Check if Go module cache is writable; fall back to /tmp if not
GOCACHE_DEF=$(go env GOCACHE)
GOMODCACHE_DEF=$(go env GOMODCACHE)
CACHE_WRITABLE=true
touch "$GOCACHE_DEF/.write-test" 2>/dev/null && rm "$GOCACHE_DEF/.write-test" || CACHE_WRITABLE=false
touch "$GOMODCACHE_DEF/.write-test" 2>/dev/null && rm "$GOMODCACHE_DEF/.write-test" || CACHE_WRITABLE=false

if [ "$CACHE_WRITABLE" = "false" ]; then
    export GOCACHE=/tmp/gobuild
    export GOMODCACHE=/tmp/gomod
    info "Go cache not writable at default location, using /tmp"
fi

CGO_ENABLED=1 go build -tags production -trimpath -buildvcs=false -ldflags="-w -s" -o bin/timo .

SIZE=$(du -h bin/timo | cut -f1)
info "Done: bin/timo ($SIZE)"
