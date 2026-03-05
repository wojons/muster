# Installation Guide

This guide covers all installation methods for Muster CLI.

## System Requirements

- **OS**: macOS 11+, Linux (Ubuntu 20.04+, Debian 10+, RHEL 8+)
- **Architecture**: AMD64, ARM64
- **Dependencies**: Git (for Go install)

## Homebrew (Recommended)

Install using Homebrew on macOS and Linux:

```bash
brew tap your-org/muster
brew install muster
```

Update to the latest version:

```bash
brew upgrade muster
```

## Direct Download

Download pre-built binaries from [GitHub Releases](https://github.com/your-org/muster/releases):

```bash
# macOS (Intel)
curl -L https://github.com/your-org/muster/releases/latest/download/muster_darwin_amd64 -o muster

# macOS (Apple Silicon)
curl -L https://github.com/your-org/muster/releases/latest/download/muster_darwin_arm64 -o muster

# Linux (AMD64)
curl -L https://github.com/your-org/muster/releases/latest/download/muster_linux_amd64 -o muster

# Linux (ARM64)
curl -L https://github.com/your-org/muster/releases/latest/download/muster_linux_arm64 -o muster

# Make executable and move to PATH
chmod +x muster
sudo mv muster /usr/local/bin/
```

## Go Install

Install directly using Go 1.21+:

```bash
go install github.com/your-org/muster/cmd/muster@latest
```

Ensure `$GOPATH/bin` is in your PATH:

```bash
export PATH=$PATH:$(go env GOPATH)/bin
```

## Docker

Run Muster using Docker:

```bash
docker pull ghcr.io/your-org/muster:latest

# Run with config mounted
docker run -v ~/.config/muster:/root/.config/muster \
  ghcr.io/your-org/muster:latest --help
```

## Verification

Verify installation:

```bash
muster --version
```

Expected output:
```
muster version 1.0.0
commit: abc123
date: 2024-01-15
```

## Shell Completion

Enable tab completion for your shell:

```bash
# Bash
muster completion bash > /etc/bash_completion.d/muster

# Zsh
muster completion zsh > "${fpath[1]}/_muster"

# Fish
muster completion fish > ~/.config/fish/completions/muster.fish
```

## Uninstallation

Remove Muster from your system:

```bash
# Homebrew
brew uninstall muster
brew untap your-org/muster

# Direct download
sudo rm /usr/local/bin/muster
rm -rf ~/.config/muster

# Go
go clean -i github.com/your-org/muster/cmd/muster
```
