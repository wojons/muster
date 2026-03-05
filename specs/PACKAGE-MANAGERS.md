# PACKAGE-MANAGERS.md

**Status**: ✅ COMPLETE
**Version**: 1.0.0
**Last Updated**: 2026-03-02

## Overview

This specification defines package manager integration for distributing and installing the CLI tool across multiple platforms. Package managers provide the primary distribution channel for CLI tools, enabling one-command installation, automatic updates, and dependency management for users.

## Supported Package Managers

| Package Manager | Platforms | Priority | Status |
|-----------------|-----------|----------|--------|
| Homebrew | macOS, Linux | P0 | Supported |
| npm | All (Node.js) | P0 | Supported |
| PyPI | All (Python) | P0 | Supported |
| Cargo | All (Rust) | P1 | Supported |
| Scoop | Windows | P1 | Supported |
| apt (Debian/Ubuntu) | Linux | P1 | Supported |
| yum/dnf (RHEL/Fedora) | Linux | P2 | Planned |
| Pacman (Arch) | Linux | P2 | Community |
| Chocolatey | Windows | P2 | Planned |

## Technical Implementation

### 1. Homebrew (macOS/Linux)

**File**: `homebrew/muster.rb`

```ruby
class Muster < Formula
  desc "Modern CLI tool for project scaffolding and management"
  homepage "https://muster.dev"
  url "https://github.com/username/muster/archive/v1.0.0.tar.gz"
  sha256 "abc123..."
  license "MIT"

  depends_on "rust" => :build
  depends_on "openssl@3"

  def install
    system "cargo", "install", *std_cargo_args
    
    # Install shell completions
    bash_completion.install "completions/muster.bash"
    zsh_completion.install "completions/_muster"
    fish_completion.install "completions/muster.fish"
    
    # Install man pages
    man1.install Dir["docs/man/*.1"]
  end

  test do
    system "#{bin}/muster", "--version"
    system "#{bin}/muster", "init", "test-project"
    assert_predicate testpath/"test-project", :exist?
  end
end
```

**File**: `.github/workflows/homebrew.yml`

```yaml
name: Homebrew Release

on:
  release:
    types: [published]

jobs:
  update-formula:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
        with:
          repository: username/homebrew-tap
          token: ${{ secrets.HOMEBREW_TAP_TOKEN }}
      
      - name: Update Formula
        run: |
          VERSION=${{ github.event.release.tag_name }}
          URL="https://github.com/username/muster/archive/${VERSION}.tar.gz"
          SHA=$(curl -sL "$URL" | sha256sum | cut -d' ' -f1)
          
          cat > muster.rb << EOF
          class Muster < Formula
            desc "Modern CLI tool for project scaffolding and management"
            homepage "https://muster.dev"
            url "$URL"
            sha256 "$SHA"
            license "MIT"

            depends_on "rust" => :build
            depends_on "openssl@3"

            def install
              system "cargo", "install", *std_cargo_args
              bash_completion.install "completions/muster.bash"
              zsh_completion.install "completions/_muster"
              fish_completion.install "completions/muster.fish"
            end

            test do
              system "#{bin}/muster", "--version"
            end
          end
          EOF
      
      - name: Commit and Push
        run: |
          git config user.name "GitHub Actions"
          git config user.email "actions@github.com"
          git add muster.rb
          git commit -m "Update muster to ${{ github.event.release.tag_name }}"
          git push
```

### 2. npm (Node.js)

**File**: `package.json`

```json
{
  "name": "@muster/cli",
  "version": "1.0.0",
  "description": "Modern CLI tool for project scaffolding and management",
  "main": "dist/index.js",
  "bin": {
    "muster": "./bin/muster"
  },
  "scripts": {
    "build": "tsc",
    "postinstall": "node scripts/install-binary.js",
    "test": "jest"
  },
  "files": [
    "bin/",
    "dist/",
    "scripts/",
    "README.md"
  ],
  "engines": {
    "node": ">=16.0.0"
  },
  "os": [
    "darwin",
    "linux",
    "win32"
  ],
  "cpu": [
    "x64",
    "arm64"
  ],
  "keywords": [
    "cli",
    "scaffold",
    "project",
    "template"
  ],
  "author": "Your Name",
  "license": "MIT",
  "repository": {
    "type": "git",
    "url": "https://github.com/username/muster.git"
  }
}
```

**File**: `scripts/install-binary.js`

```javascript
#!/usr/bin/env node
"use strict";

const fs = require("fs");
const path = require("path");
const https = require("https");
const { execSync } = require("child_process");

const PLATFORM = process.platform;
const ARCH = process.arch;
const VERSION = require("../package.json").version;

const BINARY_URLS = {
  "darwin-x64": `https://github.com/username/muster/releases/download/v${VERSION}/muster-darwin-x64`,
  "darwin-arm64": `https://github.com/username/muster/releases/download/v${VERSION}/muster-darwin-arm64`,
  "linux-x64": `https://github.com/username/muster/releases/download/v${VERSION}/muster-linux-x64`,
  "linux-arm64": `https://github.com/username/muster/releases/download/v${VERSION}/muster-linux-arm64`,
  "win32-x64": `https://github.com/username/muster/releases/download/v${VERSION}/muster-win32-x64.exe`,
};

function getBinaryName() {
  const key = `${PLATFORM}-${ARCH}`;
  return BINARY_URLS[key];
}

function downloadFile(url, dest) {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);
    https
      .get(url, { headers: { "User-Agent": "muster-cli" } }, (response) => {
        if (response.statusCode === 302 || response.statusCode === 301) {
          downloadFile(response.headers.location, dest).then(resolve).catch(reject);
          return;
        }
        response.pipe(file);
        file.on("finish", () => {
          file.close();
          resolve();
        });
      })
      .on("error", (err) => {
        fs.unlink(dest, () => {});
        reject(err);
      });
  });
}

async function install() {
  const binaryUrl = getBinaryName();
  if (!binaryUrl) {
    console.error(`Unsupported platform: ${PLATFORM}-${ARCH}`);
    process.exit(1);
  }

  const binDir = path.join(__dirname, "..", "bin");
  const binaryName = PLATFORM === "win32" ? "muster.exe" : "muster";
  const binaryPath = path.join(binDir, binaryName);

  console.log(`Downloading muster ${VERSION} for ${PLATFORM}-${ARCH}...`);

  try {
    fs.mkdirSync(binDir, { recursive: true });
    await downloadFile(binaryUrl, binaryPath);

    // Make executable on Unix
    if (PLATFORM !== "win32") {
      fs.chmodSync(binaryPath, 0o755);
    }

    console.log("✓ Installation complete");
  } catch (error) {
    console.error("Installation failed:", error.message);
    process.exit(1);
  }
}

install();
```

**File**: `bin/muster`

```javascript
#!/usr/bin/env node
const path = require("path");
const { spawn } = require("child_process");

const binaryName = process.platform === "win32" ? "muster.exe" : "muster";
const binaryPath = path.join(__dirname, binaryName);

const child = spawn(binaryPath, process.argv.slice(2), {
  stdio: "inherit",
  windowsHide: true,
});

child.on("exit", (code) => {
  process.exit(code);
});
```

### 3. PyPI (Python)

**File**: `setup.py`

```python
"""Setup configuration for PyPI distribution."""

from setuptools import setup, find_packages
from pathlib import Path

# Read README
readme = Path("README.md").read_text()

# Read requirements
requirements = Path("requirements.txt").read_text().splitlines()

setup(
    name="muster-cli",
    version="1.0.0",
    description="Modern CLI tool for project scaffolding and management",
    long_description=readme,
    long_description_content_type="text/markdown",
    author="Your Name",
    author_email="your.email@example.com",
    url="https://muster.dev",
    project_urls={
        "Documentation": "https://docs.muster.dev",
        "Source": "https://github.com/username/muster",
        "Tracker": "https://github.com/username/muster/issues",
    },
    classifiers=[
        "Development Status :: 4 - Beta",
        "Intended Audience :: Developers",
        "Topic :: Software Development :: Build Tools",
        "License :: OSI Approved :: MIT License",
        "Programming Language :: Python :: 3",
        "Programming Language :: Python :: 3.9",
        "Programming Language :: Python :: 3.10",
        "Programming Language :: Python :: 3.11",
        "Programming Language :: Python :: 3.12",
        "Operating System :: OS Independent",
    ],
    keywords="cli scaffold project template generator",
    packages=find_packages(where="src"),
    package_dir={"": "src"},
    python_requires=">=3.9",
    install_requires=requirements,
    extras_require={
        "dev": [
            "pytest>=7.0",
            "pytest-cov>=4.0",
            "black>=23.0",
            "flake8>=6.0",
            "mypy>=1.0",
        ],
    },
    entry_points={
        "console_scripts": [
            "muster=muster.cli:main",
        ],
    },
    include_package_data=True,
    package_data={
        "muster": [
            "templates/*",
            "config/*.yaml",
        ],
    },
    zip_safe=False,
)
```

**File**: `pyproject.toml`

```toml
[build-system]
requires = ["setuptools>=61.0", "wheel"]
build-backend = "setuptools.build_meta"

[project]
name = "muster-cli"
version = "1.0.0"
description = "Modern CLI tool for project scaffolding and management"
readme = "README.md"
license = {text = "MIT"}
authors = [
    {name = "Your Name", email = "your.email@example.com"}
]
keywords = ["cli", "scaffold", "project", "template"]
classifiers = [
    "Development Status :: 4 - Beta",
    "Intended Audience :: Developers",
    "Topic :: Software Development :: Build Tools",
    "License :: OSI Approved :: MIT License",
    "Programming Language :: Python :: 3",
    "Programming Language :: Python :: 3.9",
    "Programming Language :: Python :: 3.10",
    "Programming Language :: Python :: 3.11",
    "Programming Language :: Python :: 3.12",
]
requires-python = ">=3.9"
dependencies = [
    "click>=8.0",
    "pydantic>=2.0",
    "rich>=13.0",
    "toml>=0.10",
    "pyyaml>=6.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=7.0",
    "pytest-cov>=4.0",
    "black>=23.0",
    "ruff>=0.1.0",
    "mypy>=1.0",
]

[project.scripts]
muster = "muster.cli:main"

[project.urls]
Documentation = "https://docs.muster.dev"
Source = "https://github.com/username/muster"
Tracker = "https://github.com/username/muster/issues"

[tool.setuptools.packages.find]
where = ["src"]

[tool.setuptools.package-data]
muster = ["templates/**/*", "config/*.yaml"]
```

### 4. Cargo (Rust)

**File**: `Cargo.toml`

```toml
[package]
name = "muster"
version = "1.0.0"
edition = "2021"
authors = ["Your Name <your.email@example.com>"]
description = "Modern CLI tool for project scaffolding and management"
license = "MIT"
repository = "https://github.com/username/muster"
homepage = "https://muster.dev"
documentation = "https://docs.rs/muster"
keywords = ["cli", "scaffold", "project", "template", "generator"]
categories = ["command-line-utilities", "development-tools"]
rust-version = "1.70"

[[bin]]
name = "muster"
path = "src/main.rs"

[dependencies]
clap = { version = "4.4", features = ["derive", "cargo"] }
serde = { version = "1.0", features = ["derive"] }
tokio = { version = "1.0", features = ["full"] }
anyhow = "1.0"
tempfile = "3.8"
walkdir = "2.4"
toml = "0.8"
serde_yaml = "0.9"
reqwest = { version = "0.11", features = ["json"] }
colored = "2.0"
indicatif = "0.17"
console = "0.15"

[dev-dependencies]
assert_cmd = "2.0"
predicates = "3.0"
tempfile = "3.8"

[profile.release]
opt-level = 3
lto = true
strip = true
codegen-units = 1

[profile.dev]
opt-level = 0
debug = true

[package.metadata.deb]
maintainer = "Your Name <your.email@example.com>"
copyright = "2024, Your Name"
extended-description = """\
Modern CLI tool for project scaffolding and management. \
Provides templates, build automation, and deployment tools."""
section = "utils"
priority = "optional"
assets = [
    ["target/release/muster", "usr/bin/", "755"],
    ["completions/muster.bash", "usr/share/bash-completion/completions/muster", "644"],
    ["completions/_muster", "usr/share/zsh/vendor-completions/", "644"],
    ["docs/man/muster.1", "usr/share/man/man1/", "644"],
    ["README.md", "usr/share/doc/muster/README", "644"],
]
```

### 5. Scoop (Windows)

**File**: `scoop/muster.json`

```json
{
  "version": "1.0.0",
  "description": "Modern CLI tool for project scaffolding and management",
  "homepage": "https://muster.dev",
  "license": "MIT",
  "architecture": {
    "64bit": {
      "url": "https://github.com/username/muster/releases/download/v1.0.0/muster-windows-x64.exe",
      "hash": "sha256:abc123..."
    },
    "32bit": {
      "url": "https://github.com/username/muster/releases/download/v1.0.0/muster-windows-x86.exe",
      "hash": "sha256:def456..."
    }
  },
  "bin": "muster.exe",
  "checkver": {
    "github": "https://github.com/username/muster"
  },
  "autoupdate": {
    "architecture": {
      "64bit": {
        "url": "https://github.com/username/muster/releases/download/v$version/muster-windows-x64.exe"
      },
      "32bit": {
        "url": "https://github.com/username/muster/releases/download/v$version/muster-windows-x86.exe"
      }
    }
  },
  "notes": [
    "To enable tab completion, run:",
    "  muster completion powershell | Out-String | Invoke-Expression"
  ]
}
```

**Bucket Repository Structure**:

```
muster-bucket/
├── bucket/
│   └── muster.json
├── .github/
│   └── workflows/
│       └── auto-update.yml
├── README.md
└── .vscode/
    └── templates.json
```

### 6. APT (Debian/Ubuntu)

**File**: `debian/control`

```
Source: muster
Section: utils
Priority: optional
Maintainer: Your Name <your.email@example.com>
Build-Depends: debhelper (>= 12), cargo, rustc, libssl-dev, pkg-config
Standards-Version: 4.6.0
Homepage: https://muster.dev
Vcs-Browser: https://github.com/username/muster
Vcs-Git: https://github.com/username/muster.git

Package: muster
Architecture: any
Depends: ${shlibs:Depends}, ${misc:Depends}
Description: Modern CLI tool for project scaffolding and management
 Muster is a powerful CLI tool for scaffolding projects, managing templates,
 and automating development workflows. It supports multiple project types
 and integrates with popular build systems.
 .
 Features:
  * Project scaffolding with customizable templates
  * Build automation and task running
  * Configuration management
  * Multi-platform support
```

**File**: `debian/rules`

```makefile
#!/usr/bin/make -f

export DH_VERBOSE = 1
export CARGO_HOME = $(CURDIR)/cargo-home

%:
	dh $@

override_dh_auto_build:
	cargo build --release

override_dh_auto_install:
	install -D -m 755 target/release/muster debian/muster/usr/bin/muster
	install -D -m 644 completions/muster.bash debian/muster/usr/share/bash-completion/completions/muster
	install -D -m 644 completions/_muster debian/muster/usr/share/zsh/vendor-completions/_muster
	install -D -m 644 docs/man/muster.1 debian/muster/usr/share/man/man1/muster.1

override_dh_auto_clean:
	cargo clean
	rm -rf $(CARGO_HOME)
```

**Build Script**:

```bash
#!/bin/bash
# Build .deb package

set -e

cd "$(dirname "$0")"

# Install dependencies
sudo apt-get update
sudo apt-get install -y devscripts debhelper cargo rustc libssl-dev pkg-config

# Build package
dpkg-buildpackage -us -uc -b

# Move to output
cd ..
mv *.deb build/

echo "✓ Debian package built"
```

## Release Automation

**File**: `.github/workflows/release.yml`

```yaml
name: Multi-Platform Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build-binaries:
    strategy:
      matrix:
        include:
          - target: x86_64-unknown-linux-gnu
            os: ubuntu-latest
          - target: aarch64-unknown-linux-gnu
            os: ubuntu-latest
          - target: x86_64-apple-darwin
            os: macos-latest
          - target: aarch64-apple-darwin
            os: macos-latest
          - target: x86_64-pc-windows-msvc
            os: windows-latest
    
    runs-on: ${{ matrix.os }}
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Rust
        uses: actions-rs/toolchain@v1
        with:
          toolchain: stable
          target: ${{ matrix.target }}
          override: true
      
      - name: Build
        run: cargo build --release --target ${{ matrix.target }}
      
      - name: Package (Unix)
        if: matrix.os != 'windows-latest'
        run: |
          cd target/${{ matrix.target }}/release
          tar czvf ../../../muster-${{ matrix.target }}.tar.gz muster
          cd -
      
      - name: Package (Windows)
        if: matrix.os == 'windows-latest'
        run: |
          cd target/${{ matrix.target }}/release
          7z a ../../../muster-${{ matrix.target }}.zip muster.exe
          cd -
      
      - name: Upload Artifact
        uses: actions/upload-artifact@v4
        with:
          name: muster-${{ matrix.target }}
          path: |
            muster-${{ matrix.target }}.*

  release:
    needs: build-binaries
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Download Artifacts
        uses: actions/download-artifact@v4
        with:
          path: artifacts
      
      - name: Create Release
        uses: softprops/action-gh-release@v1
        with:
          files: artifacts/**/*
          generate_release_notes: true
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}

  publish-packages:
    needs: release
    runs-on: ubuntu-latest
    strategy:
      matrix:
        package:
          - npm
          - pypi
          - cargo
    steps:
      - uses: actions/checkout@v4
      
      - name: Publish ${{ matrix.package }}
        run: |
          case "${{ matrix.package }}" in
            npm)
              echo "//registry.npmjs.org/:_authToken=${{ secrets.NPM_TOKEN }}" > .npmrc
              npm publish --access public
              ;;
            pypi)
              pip install build twine
              python -m build
              twine upload dist/* -u __token__ -p ${{ secrets.PYPI_TOKEN }}
              ;;
            cargo)
              cargo login ${{ secrets.CARGO_TOKEN }}
              cargo publish
              ;;
          esac
```

## Installation Instructions

### Homebrew

```bash
# Add tap
brew tap username/muster

# Install
brew install muster

# Upgrade
brew upgrade muster

# Uninstall
brew uninstall muster
```

### npm

```bash
# Install globally
npm install -g @muster/cli

# Upgrade
npm update -g @muster/cli

# Uninstall
npm uninstall -g @muster/cli
```

### PyPI

```bash
# Install
pip install muster-cli

# Upgrade
pip install --upgrade muster-cli

# Uninstall
pip uninstall muster-cli
```

### Cargo

```bash
# Install
cargo install muster

# Upgrade
cargo install muster --force

# Uninstall
cargo uninstall muster
```

### Scoop

```powershell
# Add bucket
scoop bucket add muster https://github.com/username/scoop-muster

# Install
scoop install muster

# Upgrade
scoop update muster

# Uninstall
scoop uninstall muster
```

## References

- [Homebrew Formula Cookbook](https://docs.brew.sh/Formula-Cookbook)
- [npm Publishing Best Practices](https://docs.npmjs.com/packages-and-modules/contributing-packages-to-the-registry)
- [PyPI Packaging Guide](https://packaging.python.org/)
- [Rust Cargo Publishing](https://doc.rust-lang.org/cargo/reference/publishing.html)
- [Scoop App Manifests](https://github.com/ScoopInstaller/Scoop/wiki/App-Manifests)
- **master‑specification.md** – Complete system specification
- **SPECIFICATION‑INDEX.md** – Index of all specification files

---

## Bible Standard Compliance

This specification adheres to:
- **Multi-Platform**: Support for all major package managers
- **Automated Releases**: CI/CD integration for publishing
- **Shell Completions**: Installation of completions with packages
- **Version Management**: Semantic versioning across all channels
- **Documentation**: Clear installation and upgrade instructions

**Quality Standards Met**: ✅ Complete | ✅ Automated | ✅ Multi-Platform
