> **⚠️ ASPIRATIONAL / NOT YET AVAILABLE** — This guide documents a planned package repository that is not yet operational. The package repository domain (packages.example.com) is a placeholder. For now, install Muster from source using `make build-cli`. See the main [README](../README.md#quick-start) for current installation instructions.

# Package Repository Setup Guide

**Status**: Complete Setup Guide (2026-03-12)
**Purpose**: Configure and manage package repositories for distributing OpenAPI CLI across Linux distributions
**Scope**: Debian/APT, RHEL/YUM, Alpine, Arch packages

## Overview

This guide provides complete instructions for setting up package repositories to distribute the OpenAPI CLI and MCP Server across multiple Linux distributions.

---

## Repository Architecture

### Repository Structure

```
packages.example.com/
├── apt/                    # Debian/Ubuntu APT repository
│   ├── dists/
│   │   ├── focal/
│   │   ├── jammy/
│   │   └── bookworm/
│   └── pool/
│       └── main/
│           └── all/
│               └── openapi-cli/
├── yum/                    # RHEL/CentOS/DNF repository
│   ├── el8/
│   ├── el9/
│   └── fedora/
├── alpine/                 # Alpine Linux repository
│   ├── v3.18/
│   └── v3.19/
└── arch/                   # Arch Linux AUR (for reference)
    └── PKGBUILD (submitted to AUR)
```

### Key Requirements

- **Domain**: packages.example.com (or custom domain)
- **GPG Signing**: Required for all repositories
- **Web Server**: nginx+static or CDN
- **CI/CD**: GitHub Actions for automated repository updates
- **Monitoring**: Repository availability and package downloads

---

## GPG Key Management

### Create Repository GPG Key

#### Generate Repository Signing Key

```bash
# Generate GPG key for repository signing
gpg --batch --gen-key <<EOF
Key-Type: RSA
Key-Length: 4096
Name-Real: OpenAPI CI Repository
Name-Comment: Repository Signing Key
Name-Email: repository@openapi-cli.io
Expire-Date: 0
%no-protection
EOF

# Export public key for distribution
gpg --armor --export repository@openapi-cli.io > repository-public-key.asc

# Export secret key for backup
gpg --export-secret-keys repository@openapi-cli.io > /root/repository-secret-key.asc

# Set GPG key ID for repository signing
REPO_KEY_ID="repository@openapi-cli.io"
```

#### Upload Public Key to Domain

```bash
# Download signing key URL
https://packages.example.com/gpg

# Add to APT keyring
curl -fsSL https://packages.example.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/openapi-cli-archive-keyring.gpg

# Add to RPM keyring (RHEL/CentOS)
sudo rpm --import https://packages.example.com/gpg
```

#### Backup and Security

```bash
# Backup secret key to secure location
cp /root/repository-secret-key.asc /secure/backup/
chmod 600 /secure/backup/repository-secret-key.asc

# Set GPG key passphrase (optional)
gpg --edit-key repository@openapi-cli.io
> passwd
```

---

## Debian/Ubuntu APT Repository

### Repository Structure

```
/apt
├── dists/
│   ├── bookworm/          # Debian 12
│   ├── focal/             # Ubuntu 20.04
│   └── jammy/             # Ubuntu 22.04
│       ├── main/
│       │   ├── binary-amd64/
│       │   └── binary-arm64/
│       └── Release
└── pool/
    └── main/
        └── all/
            └── openapi-cli/
                ├── openapi-cli_1.0.0_amd64.deb
                └── openapi-cli-mcp_1.0.0_amd64.deb
```

### APT Repository Setup

#### Using reprepro (Recommended)

```bash
# Install reprepro
sudo apt install reprepro

# Create repository configuration directory
sudo mkdir -p /var/www/packages/conf

# Configure distributions (conf/distributions)
cat > /var/www/packages/conf/distributions <<EOF
Origin: openapi-cli
Label: OpenAPI CLI Repository
Suite: stable
Codename: bookworm
Architectures: amd64 arm64
Components: main
Description: OpenAPI CLI packages for Debian
SignWith: $REPO_KEY_ID
DebIndices: Packages Release . .gz .bz2

Origin: openapi-cli
Label: OpenAPI CLI Repository
Suite: stable
Codename: focal
Architectures: amd64 arm64
Components: main
Description: OpenAPI CLI packages for Ubuntu
SignWith: $REPO_KEY_ID
DebIndices: Packages Release . .gz .bz2

Origin: openapi-cli
Label: OpenAPI CLI Repository
Suite: stable
Codename: jammy
Architectures: amd64 arm64
Components: main
Description: OpenAPI CLI packages for Ubuntu
SignWith: $REPO_KEY_ID
DebIndices: Packages Release . .gz .bz2
EOF

# Configure options (conf/options)
cat > /var/www/packages/conf/options <<EOF
ask-passphrase
EOF

# Upload repository directory (using rsync)
rsync -avz /var/www/packages/ user@packages.example.com:/var/www/html/packages/
```

#### Manual APT Repository Creation

```bash
# Create directory structure
mkdir -p apt/dists/{bookworm,focal,jammy}/main/binary-amd64
mkdir -p apt/pool/main/all/openapi-cli

# Copy .deb packages
cp packaging/deb/*.deb apt/pool/main/all/openapi-cli/

# Generate Packages files (for each codename and architecture)
for codename in bookworm focal jammy; do
  cd apt/dists/$codename/main/binary-amd64
  apt-ftparchive packages ../../../pool/main/openapi-cli > Packages
  gzip -9c Packages > Packages.gz
  bzip2 -9c Packages > Packages.bz2
  cd -
done

# Generate Release files
for codename in bookworm focal jammy; do
  cd apt/dists/$codename
  apt-ftparchive release . > Release
  gpg --default-key "$REPO_KEY_ID" --armor --detach-sign --output Release.gpg Release
  gpg --default-key "$REPO_KEY_ID" --clearsign --output Release.gpg Release
  cd -
done
```

#### GitHub Actions Workflow (APT)

```yaml
name: Publish APT Repository

on:
  push:
    tags:
      - 'v*'

jobs:
  publish-apt:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Download artifacts
        run: wget -q https://github.com/wojons/muster/releases/download/${{ github.ref_name }}/openapi-cli*.deb

      - name: Setup reprepro
        run: |
          sudo apt-get update
          sudo apt-get install -y reprepro gnupg2

      - name: Configure repository
        env:
          REPO_KEY: ${{ secrets.REPO_GPG_KEY_PASSPHRASE }}
        run: |
          # Import GPG key
          echo "$REPO_KEY" > repo-key.b64
          base64 -d repo-key.b64 | gpg --batch --import
          rm repo-key.b64

          # Configure distributions
          mkdir conf
          cat > conf/distributions <<EOF
          Origin: openapi-cli
          Label: OpenAPI CLI Repository
          Suite: stable
          Codename: jammy
          Architectures: amd64 arm64
          Components: main
          Description: OpenAPI CLI packages for Ubuntu
          SignWith: ${{ secrets.REPO_GPG_KEY_ID }}
          EOF

      - name: Add packages
        run: |
          reprepro --nosignatures --ask-passphrase includedeb jammy openapi-cli_*.deb
          reprepro --nosignatures --ask-passphrase includedeb jammy openapi-cli-mcp_*.deb

      - name: Publish to repository
        env:
          DEPLOY_KEY: ${{ secrets.REPO_DEPLOY_KEY }}
        run: |
          # Setup SSH
          mkdir -p ~/.ssh
          echo "$DEPLOY_KEY" > ~/.ssh/id_rsa
          chmod 600 ~/.ssh/id_rsa

          # Deploy
          rsync -avz --delete dists/ user@packages.example.com:/var/www/apt/dists/
          rsync -avz --delete pool/ user@packages.example.com:/var/www/apt/pool/
```

### Client-Side APT Configuration

```bash
# Add repository (Ubuntu/Debian)
curl -fsSL https://packages.example.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/openapi-cli-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/openapi-cli-archive-keyring.gpg] https://packages.example.com/apt jammy main" | sudo tee /etc/apt/sources.list.d/openapi-cli.list

# Update and install
sudo apt update
sudo apt install openapi-cli openapi-cli-mcp
```

---

## RHEL/CentOS/YUM Repository

### Repository Structure

```
/yum
├── el8/                    # RHEL/CentOS 8
│   ├── Packages/
│   │   ├── x86_64/
│   │   └── aarch64/
│   └── repodata/
│       ├── primary.xml.gz
│       ├── filelists.xml.gz
│       └── other.xml.gz
├── el9/                    # RHEL/CentOS 9
│   └── Packages/
└── fedora/                 # Fedora
    └── Packages/
```

### YUM Repository Setup

#### Using createrepo and signrpm

```bash
# Install tools
sudo apt install createrepo rsync

# Create repository directory structure
mkdir -p yum/{el8,el9}/{Packages,repodata}

# Build RPM packages (from packaging/rpm directory)
make build-rpm-el8
make build-rpm-el9

# Copy .rpm packages to repository
cp packaging/rpm/*.rpm yum/el8/Packages/
cp packaging/rpm/*.rpm yum/el9/Packages/

# Generate repodata
for release in el8 el9; do
  cd yum/$release
  createrepo Packages/
  cd -
done

# Sign all packages
find yum/ -name "*.rpm" -exec rpm --resign --addsign {} \;

# Generate repository metadata with signatures
for release in el8 el9; do
  cd yum/$release
  createrepo --update --sign Packages/
  cd -
done
```

#### Manual Repository Metadata

```bash
# Generate repodata with compression
createrepo --database --compress-type=gz yum/el8/Packages/
createrepo --database --compress-type=gz yum/el9/Packages/

# Sign repomd.xml (metadata signature)
for release in el8 el9; do
  gpg --default-key "$REPO_KEY_ID" --detach-sign --armor --output yum/$release/repodata/repomd.xml.asc yum/$release/repodata/repomd.xml
done
```

#### GitHub Actions Workflow (YUM)

```yaml
name: Publish YUM Repository

on:
  push:
    tags:
      - 'v*'

jobs:
  publish-yum:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Download artifacts
        run: wget -q https://github.com/wojons/muster/releases/download/${{ github.ref_name }}/openapi-cli*.rpm

      - name: Setup RPM tools
        run: |
          sudo apt-get update
          sudo apt-get install -y createrepo rpmsign gnupg2

      - name: Sign packages
        env:
          REPO_KEY: ${{ secrets.REPO_GPG_KEY_PASSPHRASE }}
        run: |
          echo "$REPO_KEY" > repo-key.b64
          base64 -d repo-key.b64 | gpg --batch --import
          rm repo-key.b64
          rpmsign --resign --addsign openapi-cli*.rpm

      - name: Create repository
        run: |
          mkdir -p yum/{el8,el9}/Packages
          cp *.rpm yum/el8/Packages/
          cp *.rpm yum/el9/Packages/
          cd yum/el8 && createrepo --database Packages/ --sign
          cd ../el9 && createrepo --database Packages/ --sign
          cd -

      - name: Publish
        run: |
          rsync -avz --delete yum/ user@packages.example.com:/var/www/yum/
```

### Client-Side YUM Configuration

```bash
# Add repository (RHEL/CentOS)
sudo rpm --import https://packages.example.com/gpg

# Create DNF/YUM repository file
sudo tee /etc/yum.repos.d/openapi-cli.repo <<EOF
[openapi-cli]
name=OpenAPI CLI Repository
baseurl=https://packages.example.com/yum/\$releasever
enabled=1
gpgcheck=1
gpgkey=https://packages.example.com/gpg
EOF

# Install
sudo dnf install openapi-cli openapi-cli-mcp
```

---

## Alpine Linux Repository

### Repository Structure

```
/alpine
├── v3.18/
│   └── community/
│       └── x86_64/
│           └── APKBUILD
└── v3.19/
    └── community/
        └── x86_64/
            └── APKBUILD
```

### Alpine Repository Setup

#### Using abuild and apkbuild-pkg

```bash
# Install alpine-sdk on Alpine system
apk add alpine-sdk

# Create APKBUILD file
cat > openapi-cli/APKBUILD <<EOF
# Maintainer: OpenAPI CI Team <maintainers@openapi-cli.io>
pkgname=openapi-cli
pkgver=1.0.0
pkgrel=1
pkgdesc="Dynamic OpenAPI CLI with MCP server"
url="https://github.com/wojons/muster"
arch="x86_64 aarch64"
license="MIT"
source="$pkgname-$pkgver.tar.gz::https://github.com/wojons/muster/archive/v$pkgver.tar.gz"

build() {
  cd "$srcdir/$pkgname-$pkgver"
  make build-cli
  make build-mcp
}

package() {
  install -Dm755 openapi-cli "$pkgdir"/usr/bin/openapi-cli
  install -Dm755 openapi-mcp "$pkgdir"/usr/bin/openapi-mcp
}
EOF

# Build APK package
cd openapi-cli
abuild -r
```

#### Repository Index Creation

```bash
# Create alpine repository structure
mkdir -p alpine/v3.18/community/x86_64

# Copy APK files
cp openapi-cli*.apk alpine/v3.18/community/x86_64/

# Generate repository index
apk-tools/apk index --output alpine/v3.18/community/APKINDEX.tar.gz alpine/v3.18/community/x86_64/*.apk

# Sign repository index
abuild-sign alpine/v3.18/community/APKINDEX.tar.gz
```

#### GitHub Actions Workflow (Alpine)

```yaml
name: Publish Alpine Repository

on:
  push:
    tags:
      - 'v*'

jobs:
  publish-alpine:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Setup Alpine SDK
        run: |
          sudo apt-get update
          sudo apt-get install -y build-base git
          wget -q https://gitlab.alpinelinux.org/api/v4/projects/aports/packages/generic/apk-tools/2.10.6/x86_64/apk-tools-static-2.10.6.apk
          sudo tar xzf apk-tools-static-*.apk -C /usr/local/

      - name: Build APK
        run: |
          apk add -U --allow-untrusted alpine-sdk
          cd packaging/alpine
          abuild -r

      - name: Publish
        run: |
          rsync -avz --delete alpine/ user@packages.example.com:/var/www/alpine/
```

### Client-Side Alpine Configuration

```bash
# Add Alpine repository
sudo wget -O /etc/apk/keys/openapi-cli.rsa.pub https://packages.example.com/alpine/openapi-cli.rsa.pub
echo "https://packages.example.com/alpine/community" >> /etc/apk/repositories

# Install
apk update
apk add openapi-cli openapi-cli-mcp
```

---

## Arch/AUR Repository

### Arch Linux Package (AUR Submission)

Arch packages are typically distributed via the Arch User Repository (AUR). Submit to AUR instead of hosting your own repository.

```bash
# Create PKGBUILD file
cat > PKGBUILD <<EOF
pkgname=openapi-cli
pkgver=1.0.0
pkgrel=1
pkgdesc="Dynamic OpenAPI CLI with MCP server"
arch=('x86_64')
url="https://github.com/wojons/muster"
license=('MIT')
depends=('glibc')
source=("$pkgname-$pkgver.tar.gz::https://github.com/wojons/muster/archive/v$pkgver.tar.gz")
sha256sums=('SKIP')

build() {
  cd "$pkgname-$pkgver"
  make build-cli
  make build-mcp
}

package() {
  cd "$pkgname-$pkgver"
  install -Dm755 bin/openapi-cli "$pkgdir"/usr/bin/openapi-cli
  install -Dm755 bin/openapi-mcp "$pkgdir"/usr/bin/openapi-mcp
}
EOF

# Generate .SRCINFO (for AUR metadata)
makepkg --printsrcinfo > .SRCINFO

# Submit to AUR
git clone aur@aur.archlinux.org:openapi-cli.git
cd openapi-cli
cp ../PKGBUILD .
cp ../.SRCINFO .
git add . && git commit -m "Update to $pkgver"
git push
```

### Client-Side Arch Installation

```bash
# Install from AUR (using yay/paru)
yay -S openapi-cli
# OR
paru -S openapi-cli
```

---

## Repository Publishing Automation

### GitHub Actions (Full Repository Publish)

```yaml
name: Publish All Repositories

on:
  push:
    tags:
      - 'v*'

jobs:
  publish:
    runs-on: ubuntu-latest
    strategy:
      matrix:
        repo: [apt, yum, alpine]
    steps:
      - uses: actions/checkout@v3

      - name: Download release artifacts
        run: |
          wget -q https://github.com/wojons/muster/releases/download/${{ github.ref_name }}/*

      - name: Publish to ${{ matrix.repo }}
        env:
          DEPLOY_KEY: ${{ secrets.REPO_DEPLOY_KEY }}
        run: |
          # Setup SSH key
          mkdir -p ~/.ssh
          echo "$DEPLOY_KEY" > ~/.ssh/id_rsa
          chmod 600 ~/.ssh/id_rsa

          # Deploy based on repository type
          case "${{ matrix.repo }}" in
            apt)
              rsync -avz --delete dists/ user@packages.example.com:/var/www/apt/
              rsync -avz --delete pool/ user@packages.example.com:/var/www/apt/
              ;;
            yum)
              rsync -avz --delete yum/ user@packages.example.com:/var/www/yum/
              ;;
            alpine)
              rsync -avz --delete alpine/ user@packages.example.com:/var/www/alpine/
              ;;
          esac

          # Verify deployment
          curl -I https://packages.example.com/${{ matrix.repo }}/
```

---

## Repository Monitoring and Validation

### Repository Health Checks

```bash
#!/bin/bash
# Check repository health

# APT repository health
curl -f https://packages.example.com/apt/dists/jammy/Release || echo "APT: FAIL"

# YUM repository health
curl -f https://packages.example.com/yum/el9/repodata/repomd.xml || echo "YUM: FAIL"

# Alpine repository health
curl -f https://packages.example.com/alpine/v3.18/community/APKINDEX.tar.gz || echo "Alpine: FAIL"

# GPG key validation
gpg --list-packets https://packages.example.com/gpg || echo "GPG: FAIL"
```

### Repository Version History

```python
#!/usr/bin/env python3
"""Track published package versions across repositories"""

import requests
import json

REPO_BASE = "https://packages.example.com"

def check_repo_versions():
    """Check all repository package versions"""
    versions = {}

    # APT versions
    apt_response = requests.get(f"{REPO_BASE}/apt/dists/jammy/Packages")
    versions['apt'] = apt_response.text if apt_response.status_code == 200 else None

    # YUM versions
    yum_response = requests.get(f"{REPO_BASE}/yum/el9/repodata/primary.xml")
    versions['yum'] = yum_response.text if yum_response.status_code == 200 else None

    # Alpine versions
    alpine_response = requests.get(f"{REPO_BASE}/alpine/v3.18/community/APKINDEX")
    versions['alpine'] = alpine_response.text if alpine_response.status_code == 200 else None

    return versions

if __name__ == "__main__":
    versions = check_repo_versions()
    print(json.dumps(versions, indent=2))
```

---

## Security and Maintenance

### GPG Key Rotation

```bash
# When repository GPG key needs rotation (compromise or periodic)
# 1. Generate new key
gpg --batch --gen-key <<EOF
Key-Type: RSA
Key-Length: 4096
Name-Real: OpenAPI CI Repository (New)
Name-Email: repository@openapi-cli.io
Expire-Date: 0
%no-protection
EOF

# 2. Export new public key
gpg --armor --export repository@openapi-cli.io > new-repository-public-key.asc

# 3. Update repository URLs and client installations

# 4. Archive old key (after transition period)
gpg --export-secret-keys OLD_REPOSITORY@openapi-cli.io > /secure/backup/old-repo-key.asc
```

### Repository Backup

```bash
# Full repository backup script
#!/bin/bash
BACKUP_DIR="/backup/packages.$(date +%Y%m%d)"
rsync -avz /var/www/packages/ "$BACKUP_DIR/"
tar czf "/secure/backups/packages.$(date +%Y%m%d).tar.gz" "$BACKUP_DIR"
rm -rf "$BACKUP_DIR"
```

---

## Summary

This guide provides complete setup for:
- ✅ APT repository (Debian/Ubuntu) with GPG signing and automatic publishing
- ✅ YUM/DNF repository (RHEL/CentOS/YUM) with signature verification
- ✅ Alpine Linux repository with APKBUILD and repository index signing
- ✅ Arch/AUR submission process (not hosting own repository)
- ✅ GitHub Actions automation for all repositories
- ✅ GPG key management and rotation
- ✅ Monitoring and validation checks
- ✅ Security best practices

**Required Components**:
- Domain name for hosting (packages.example.com)
- GPG signing key (4096-bit RSA)
- Web server (nginx) or CDN
- GitHub Actions for CI/CD automation

**Next Steps**:
1. Set up web server with repository directories
2. Generate GPG signing key
3. Configure GitHub Actions workflows
4. Test repository publish workflow
5. Document repository URLs and setup instructions

**Last Updated**: 2026-03-12
