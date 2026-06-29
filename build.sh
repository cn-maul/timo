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

# ── Detect nvm and use a compatible Node version ──
# Locate nvm.sh robustly: honor $NVM_DIR if set, otherwise probe the two
# common install locations (~/.config/nvm and ~/.nvm). We can't rely on the
# $NVM_DIR env var alone because it's only exported from .bashrc, which
# non-interactive shells (e.g. double-clicking build.sh) never source.
NVM_SH=""
for _cand in "$NVM_DIR/nvm.sh" "$HOME/.config/nvm/nvm.sh" "$HOME/.nvm/nvm.sh"; do
    [ -s "$_cand" ] && { NVM_SH="$_cand"; break; }
done
if [ -n "$NVM_SH" ]; then
    # shellcheck source=/dev/null
    . "$NVM_SH"
    # Check if current node meets Vite's requirement (>=20.19 or >=22.12)
    NODE_VER=$(node --version 2>/dev/null | sed 's/v//' | cut -d. -f1)
    if [ -z "$NODE_VER" ] || [ "$NODE_VER" -lt 22 ] 2>/dev/null; then
        info "Node ${NODE_VER:-none} active, switching to v22+ via nvm..."
        # Prefer an installed 22.x; fall back to the default alias.
        if nvm ls 22 >/dev/null 2>&1; then
            nvm use 22 >/dev/null 2>&1 && info "Switched to $(node --version) via nvm" || true
        elif nvm use default >/dev/null 2>&1; then
            info "Switched to $(node --version) (nvm default)" || true
        fi
    fi
fi

# ── Check deps ──
command -v go     >/dev/null 2>&1 || error "go not found"
command -v node   >/dev/null 2>&1 || error "node not found"
command -v npm    >/dev/null 2>&1 || error "npm not found"
# wails3 may be in Go's bin directory (not always in $PATH)
if ! command -v wails3 >/dev/null 2>&1; then
    # Search common Go binary locations
    GOBIN=$(go env GOBIN 2>/dev/null)
    GOPATH_BIN=$(go env GOPATH 2>/dev/null)/bin
    GOROOT_BIN=$(go env GOROOT 2>/dev/null)/bin
    HOME_GO_BIN="$HOME/go/bin"
    HOME_GO_WORKSPACE_BIN="$HOME/go-workspace/bin"
    LOCAL_BIN="$HOME/.local/bin"

    FOUND=""
    for dir in "$GOBIN" "$GOPATH_BIN" "$GOROOT_BIN" "$HOME_GO_BIN" "$HOME_GO_WORKSPACE_BIN" "$LOCAL_BIN"; do
        if [ -n "$dir" ] && [ -f "$dir/wails3" ]; then
            FOUND="$dir"
            break
        fi
    done

    if [ -n "$FOUND" ]; then
        export PATH="$FOUND:$PATH"
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
