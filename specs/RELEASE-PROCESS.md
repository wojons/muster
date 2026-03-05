# Release Process

**Version:** 1.0.0  
**Last Updated:** 2026-02-28

---

## Semantic Versioning

Format: `MAJOR.MINOR.PATCH[-prerelease][+build]`

- **MAJOR**: Breaking changes
- **MINOR**: New features (backward compatible)
- **PATCH**: Bug fixes
- **prerelease**: `alpha`, `beta`, `rc1`

Examples: `v1.0.0`, `v1.2.0-beta.1`, `v2.0.0-rc.1`

---

## Pre-Release Checklist

```markdown
## Release v1.2.0 Checklist

- [ ] All tests passing (`make test`)
- [ ] Linting clean (`make lint`)
- [ ] Version bumped in `version.go`
- [ ] CHANGELOG.md updated
- [ ] Documentation updated
- [ ] Migration guide written (if breaking changes)
- [ ] Security scan passed (`make security-scan`)
- [ ] Performance benchmarks acceptable
- [ ] Manual QA completed
```

---

## Changelog Generation

```bash
# Install git-chglog
go install github.com/git-chglog/git-chglog@latest

# Generate changelog
git-chglog -o CHANGELOG.md

# Or use conventional commits
git log --pretty=format:"%s" $(git describe --tags --abbrev=0)..HEAD | \
  grep -E "^(feat|fix|docs|style|refactor|test|chore)" > changes.txt
```

---

## Git Tagging Process

```bash
# 1. Update version
$EDITOR version.go

# 2. Commit version bump
git add version.go
git commit -m "chore: bump version to v1.2.0"

# 3. Create annotated tag
git tag -a v1.2.0 -m "Release v1.2.0 - Feature release"

# 4. Push to remote
git push origin main
git push origin v1.2.0

# 5. Verify tag
git show v1.2.0
```

---

## GitHub Release Creation

```bash
# Using GitHub CLI
gh release create v1.2.0 \
  --title "Release v1.2.0" \
  --notes-file CHANGELOG.md \
  dist/muster-*.{tar.gz,zip}

# Or via API
curl -X POST \
  -H "Authorization: token $GITHUB_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "tag_name": "v1.2.0",
    "name": "Release v1.2.0",
    "body": "See CHANGELOG.md for details"
  }' \
  https://api.github.com/repos/muster/muster/releases
```

---

## Homebrew Formula Update

```ruby
# Formula template: muster.rb
class Muster < Formula
  desc "OpenAPI CLI tool for API testing and exploration"
  homepage "https://github.com/muster/muster"
  version "1.2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "https://github.com/muster/muster/releases/download/v1.2.0/muster_1.2.0_darwin_arm64.tar.gz"
      sha256 "ABC123..."
    else
      url "https://github.com/muster/muster/releases/download/v1.2.0/muster_1.2.0_darwin_amd64.tar.gz"
      sha256 "DEF456..."
    end
  end

  on_linux do
    url "https://github.com/muster/muster/releases/download/v1.2.0/muster_1.2.0_linux_amd64.tar.gz"
    sha256 "GHI789..."
  end

  def install
    bin.install "muster"
  end
end
```

---

## Docker Image Tagging

```bash
# Build and tag
VERSION=1.2.0
docker build -t muster/muster:$VERSION .
docker tag muster/muster:$VERSION muster/muster:latest

# Push to registry
docker push muster/muster:$VERSION
docker push muster/muster:latest

# Multi-arch build
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t muster/muster:$VERSION \
  -t muster/muster:latest \
  --push .
```

---

## Post-Release Verification

```bash
# Verify GitHub release
curl -s https://api.github.com/repos/muster/muster/releases/latest | jq .tag_name

# Verify Homebrew
brew update && brew info muster

# Verify Docker Hub
docker pull muster/muster:latest && docker run muster/muster:latest version
```
