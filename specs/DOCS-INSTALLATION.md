# Installation Guide

This guide covers all installation methods for OpenAPI CLI.

## System Requirements

- **OS**: macOS 10.15 (Catalina)+, Linux (glibc 2.17+), Windows 10+
- **Architecture**: AMD64, ARM64
- **Dependencies**: Git (for Go install)
- **Disk Space**: 100MB free space
- **Network**: Internet connection for initial download

## macOS

### Homebrew (Recommended)

The easiest way to install on macOS is via Homebrew:

```bash
brew tap openapi-cli/openapi-cli
brew install openapi-cli
```

Update to the latest version:

```bash
brew upgrade openapi-cli
```

**Verify Installation**:

```bash
openapi-cli --version
# Expected output: openapi-cli 1.0.0
```

### Manual Binary Install

Download the latest macOS binary:

```bash
# Download the binary
curl -L https://github.com/openapi-cli/openapi-cli/releases/latest/download/openapi-cli-darwin-amd64 -o /usr/local/bin/openapi-cli

# Make it executable
chmod +x /usr/local/bin/openapi-cli

# Verify
openapi-cli --version
```

For Apple Silicon (M1/M2/M3):

```bash
curl -L https://github.com/openapi-cli/openapi-cli/releases/latest/download/openapi-cli-darwin-arm64 -o /usr/local/bin/openapi-cli
chmod +x /usr/local/bin/openapi-cli
```

## Linux

### Debian/Ubuntu

Using the pre-built binary:

```bash
# Download binary
sudo curl -L https://github.com/openapi-cli/openapi-cli/releases/latest/download/openapi-cli-linux-amd64 -o /usr/local/bin/openapi-cli

# Set permissions
sudo chmod +x /usr/local/bin/openapi-cli

# Verify
openapi-cli --version
```

### Arch Linux (AUR)

```bash
yay -S openapi-cli
```

### Package Repository (Optional)

Add the OpenAPI CLI APT repository:

```bash
# Add GPG key
curl -fsSL https://openapi-cli.dev/gpg.key | sudo gpg --dearmor -o /usr/share/keyrings/openapi-cli.gpg

# Add repository
echo "deb [signed-by=/usr/share/keyrings/openapi-cli.gpg] https://openapi-cli.dev/deb stable main" | sudo tee /etc/apt/sources.list.d/openapi-cli.list

# Update and install
sudo apt update
sudo apt install openapi-cli
```

## Windows

### PowerShell (One-Line Install)

```powershell
irm https://openapi-cli.dev/install.ps1 | iex
```

### Scoop (Recommended)

```powershell
scoop bucket add openapi-cli https://github.com/openapi-cli/scoop-bucket
scoop install openapi-cli
```

### Manual Install

1. Download the latest Windows binary from [GitHub Releases](https://github.com/openapi-cli/openapi-cli/releases)
2. Extract `openapi-cli.exe` to a directory in your PATH (e.g., `C:\Program Files`)
3. Add to PATH if needed:
   ```powershell
   # Add to PATH (current session)
   $env:Path += ";C:\Program Files"

   # Add permanently (run as Administrator)
   setx PATH "%PATH%;C:\Program Files"
   ```

### Chocolatey

```powershell
choco install openapi-cli
```

## Go Install

Install directly using Go 1.21+:

```bash
go install github.com/openapi-cli/openapi-cli/cmd/openapi-cli@latest
```

Ensure `$GOPATH/bin` is in your PATH:

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

## Docker

```bash
# Pull the image
docker pull openapi-cli/openapi-cli:latest

# Run
docker run -it --rm openapi-cli/openapi-cli --version

# With volume mount for OpenAPI files
docker run -it --rm -v $(pwd):/workdir openapi-cli/openapi-cli docs generate /workdir/openapi.yaml
```

## Verification

After installation, verify everything works:

```bash
# Version check
openapi-cli --version

# System check
openapi-cli doctor

# Test command
openapi-cli docs --help
```

Expected output:
```
openapi-cli version 1.0.0
commit: abc123
date: 2024-01-15
```

## Shell Completion

Enable tab completion for your shell:

```bash
# Bash
openapi-cli completion bash > /etc/bash_completion.d/openapi-cli

# Zsh
openapi-cli completion zsh > "${fpath[1]}/_openapi-cli"

# Fish
openapi-cli completion fish > ~/.config/fish/completions/openapi-cli.fish
```

## Troubleshooting

### Permission Denied (Linux/macOS)

**Error**:
```
bash: /usr/local/bin/openapi-cli: Permission denied
```

**Solution**:
```bash
# Fix permissions
chmod +x /usr/local/bin/openapi-cli

# Or install locally
mkdir -p ~/.local/bin
curl -L https://github.com/openapi-cli/openapi-cli/releases/latest/download/openapi-cli-linux-amd64 -o ~/.local/bin/openapi-cli
chmod +x ~/.local/bin/openapi-cli
```

### Command Not Found

**Error**:
```
command not found: openapi-cli
```

**Solutions**:

1. **Check PATH**:
   ```bash
   which openapi-cli  # Should show full path
   echo $PATH        # Verify binary location is in PATH
   ```

2. **Add to PATH** (add to `~/.bashrc`, `~/.zshrc`, or `~/.profile`):
   ```bash
   export PATH="$HOME/.local/bin:$PATH"
   # Reload
   source ~/.bashrc
   ```

3. **Use full path**:
   ```bash
   /usr/local/bin/openapi-cli --version
   ```

### Windows Not in Path

**Solution**:
```powershell
# Use System Properties to add to PATH, or:
$env:Path += ";C:\Program Files"
```

### Binary Permissions

Linux/macOS systems may require explicit permission for downloaded binaries:

```bash
# Remove quarantine attribute (macOS)
xattr -d com.apple.quarantine /usr/local/bin/openapi-cli

# Verify binary integrity
file /usr/local/bin/openapi-cli
# Should show: Mach-O 64-bit executable or ELF 64-bit LSB executable
```

## Uninstallation

Remove OpenAPI CLI from your system:

```bash
# Homebrew
brew uninstall openapi-cli
brew untap openapi-cli/openapi-cli

# Direct download
sudo rm /usr/local/bin/openapi-cli
rm -rf ~/.config/openapi-cli/
rm -rf ~/.cache/openapi-cli/

# Go
go clean -i github.com/openapi-cli/openapi-cli/cmd/openapi-cli

# Scoop
scoop uninstall openapi-cli

# Docker
docker rmi openapi-cli/openapi-cli
```

## Next Steps

- [Quickstart Guide](../../docs/guides/quickstart.md) - Get started in 5 minutes
- [Configuration Guide](../../docs/guides/configuration.md) - Customize your setup
- [Command Reference](../../docs/guides/commands.md) - Explore all commands
