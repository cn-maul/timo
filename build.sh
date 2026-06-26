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
command -v wails3 >/dev/null 2>&1 || error "wails3 not found (go install github.com/wailsapp/wails/v3/cmd/wails3@latest)"

# ── Install frontend deps if needed ──
if [ ! -d "frontend/node_modules" ]; then
    info "Installing frontend dependencies..."
    (cd frontend && npm install)
fi

# ── Build ──
info "Building frontend..."
(cd frontend && npm run build)

info "Building Go binary..."
CGO_ENABLED=1 go build -tags production -trimpath -buildvcs=false -ldflags="-w -s" -o bin/timo .

SIZE=$(du -h bin/timo | cut -f1)
info "Done: bin/timo ($SIZE)"
