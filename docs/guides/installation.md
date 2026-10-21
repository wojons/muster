# Installation Guide

Muster is a dynamic OpenAPI CLI, MCP server, and daemon. The project source
lives in the private repository `github.com/wojons/muster` — there are no
published Homebrew taps, Docker Hub images, package repositories, or
pre-built release binaries at this time. Installation is **build from
source**.

> **Access caveat**: the repository is private. You need access to
> `github.com/wojons/muster` (an org member or collaborator with read
> access) before any of the commands below will work. See the README for
> the current status of public distribution.

## Prerequisites

- **Go 1.26.9+** (see `go.mod`; older toolchains get a hard toolchain error)
- **A C compiler** — `gcc` or `clang`. SQLite support is a CGO package
  (`github.com/mattn/go-sqlite3`) and `make` builds with `CGO_ENABLED=1`, so
  without one `make build-cli` stops with
  `cgo: C compiler "gcc" not found` (a bare `go build` does not fail — Go
  silently disables cgo and the driver's cgo files are excluded). Install with
  `sudo apt-get install -y gcc` (Debian/Ubuntu) or
  `xcode-select --install` (macOS)
- **git** (for cloning the repository)
- **make** (optional — you can also invoke `go build` directly)
- **Disk Space**: ~100MB free space
- **Network**: Internet connection for module downloads on first build

## Build from source (all platforms)

### 1. Clone the repository

```bash
git clone https://github.com/wojons/muster.git
cd muster
```

### 2. Build the CLI

Using make (recommended):

```bash
make build-cli
```

Or directly with Go:

```bash
go build -o bin/openapi-cli ./cmd/openapi-cli
```

The binary lands at `bin/openapi-cli` (the `bin/` directory is
gitignored, so it is created locally and never committed).

### 3. Add the binary to your PATH

```bash
# macOS / Linux
export PATH="$PWD/bin:$PATH"        # add to ~/.bashrc, ~/.zshrc, or ~/.profile to persist
```

Windows (PowerShell):

```powershell
$env:Path += ";$PWD\bin"
# Persist permanently (run as Administrator):
setx PATH "%PATH%;$PWD\bin"
```

### 4. Verify

```bash
openapi-cli --version
# Expected output: openapi-cli 1.0.0 (or the current version)

openapi-cli --help
```

## Building the other components

The repo also ships an MCP server and a background daemon. Build them the
same way:

```bash
make build-mcp      # -> bin/mcp-server
make build-daemon   # -> bin/openapi-daemon
```

Or directly:

```bash
go build -o bin/mcp-server ./cmd/mcp-server
go build -o bin/openapi-daemon ./cmd/daemon
```

## Troubleshooting

### Command Not Found

**Error**:

```
command not found: openapi-cli
```

**Solutions**:

1. **Check PATH**:
   ```bash
   which openapi-cli  # Should show the bin/ path
   echo $PATH         # Verify bin/ is in PATH
   ```

2. **Use the full path**:
   ```bash
   $PWD/bin/openapi-cli --version
   ```

### Go toolchain error

**Error**:

```
go.mod requires go >= 1.26.9
```

**Solution**: your installed Go is older than the version in `go.mod`.
Upgrade to Go 1.26.9+ (see https://go.dev/dl).

### First build is slow / network errors

The first `make build-cli` downloads all Go module dependencies. If module
downloads fail, check:

```bash
go env GOPROXY        # should be a reachable proxy, e.g. https://proxy.golang.org,direct
```

## Upgrading an existing install

Build from source installs do not self-update: re-run the build and replace
the binary (see the guide
[upgrade-verification.md](upgrade-verification.md) for the tagged
release → HEAD verification the repo ships, including the v0.1.0 module-path
stamping caveat):

```bash
git pull && make build-cli        # rebuild the new version over the old install
scripts/verify-upgrade-path.sh    # prove a tagged upgrade works end to end
```

## Uninstallation

The build-from-source install leaves nothing outside the repo clone:

```bash
# Remove the built binary (or the whole clone)
rm -f bin/openapi-cli
rm -rf ~/.config/openapi-cli/     # CLI config (if you ran init)
rm -rf ~/.cache/openapi-cli/      # cached inventory data (if any)
```

## Next Steps

- [Quickstart Guide](quickstart.md) - Start using OpenAPI CLI
- [Configuration Guide](configuration.md) - Customize your setup
- [Command Reference](commands.md) - Explore all commands

## Additional Resources

- [GitHub Repository](https://github.com/wojons/muster) - Source code and issues
- [Documentation Index](../README.md) - Full documentation
