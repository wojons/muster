# FIPS 140-3 Compliance Guide

## Overview

Starting with Go 1.26.1, Muster can operate in FIPS 140-3 compliant mode using the Go Cryptographic Module. This guide explains how to build, configure, and validate FIPS 140-3 compliance for Muster.

## What is FIPS 140-3?

FIPS 140-3 (Federal Information Processing Standard Publication 140-3) is a U.S. Government standard that specifies security requirements for cryptographic modules. It's required for:

- Federal government systems (FedRAMP)
- Government contractors
- Regulated industries (finance, healthcare, defense)
- Organizations requiring validated cryptographic implementations

## Go Cryptographic Module

Muster uses Go's built-in FIPS 140-3 compliant cryptographic module (`crypto/internal/fips140/...`) which implements approved algorithms including:

- **Symmetric Encryption**: AES-GCM (128, 256-bit)
- **Hash Functions**: SHA-1, SHA-256, SHA-384, SHA-512
- **Message Authentication**: HMAC-SHA256, HMAC-SHA384, HMAC-SHA512
- **Digital Signatures**: ECDSA (P-256, P-384, P-521), RSA (PKCS#1 v1.5, PSS)
- **Key Derivation**: HKDF, PBKDF2
- **Random Number Generation**: SP 800-90A DRBG with platform entropy

### CMVP Validation Status

The Go Cryptographic Module is currently in the CMVP validation process:
- **v1.26.0**: Implementation Under Test (IUT) - available in Go 1.26+
- **v1.0.0**: In Review - available in Go 1.24+

While full CMVP validation is pending, the module implements all required FIPS 140-3 approved algorithms and undergoes rigorous self-testing.

## Building Muster in FIPS Mode

### Option 1: Runtime FIPS Mode (Recommended for Development)

Set the `GODEBUG` environment variable at runtime:

```bash
# Build normally
go build -o muster ./cmd/openapi-cli

# Run with FIPS 140-3 mode enabled
GODEBUG=fips140=on ./muster

# Run with FIPS 140-3 enforcement (non-FIPS algorithms will fail)
GODEBUG=fips140=only ./muster
```

### Option 2: Build-Time FIPS Module (Production)

Build with a specific FIPS-validated module version:

```bash
# Build with FIPS module v1.26.0 (Go 1.26+)
GOFIPS140=v1.26.0 go build -o muster-fips ./cmd/openapi-cli

# Build with latest FIPS module
GOFIPS140=latest go build -o muster-fips ./cmd/openapi-cli
```

### Option 3: Makefile Targets

```bash
# Build FIPS-compliant binary
make build-fips

# Run tests in FIPS mode
make test-fips

# Validate FIPS compliance
make validate-fips
```

## FIPS 140-3 Mode Behavior

When `GODEBUG=fips140=on` or `GOFIPS140=v1.26.0` is set:

### Automatic Self-Tests

1. **Integrity Self-Check**: At startup, verifies the cryptographic module hasn't been tampered with
2. **Known-Answer Tests (KAT)**: Validates algorithms produce correct outputs for known inputs
3. **Pairwise Consistency Tests**: Ensures generated keys are mathematically valid

### Algorithm Restrictions

In FIPS mode, the following restrictions apply:

- **TLS**: Only FIPS-approved cipher suites negotiated
- **Random Numbers**: Uses SP 800-90A DRBG with platform entropy mixing
- **RSA Keys**: Minimum 2048-bit, pairwise consistency checked
- **ECDSA Keys**: Only P-256, P-384, P-521 curves allowed
- **Hash Functions**: SHA-1 only allowed for legacy compatibility (not for signatures)

### Performance Impact

Expect the following performance characteristics:

- **RSA Key Generation**: ~2x slower due to pairwise consistency tests
- **ECDSA Key Generation**: Minimal impact (~10-15% slower)
- **Symmetric Operations**: Negligible impact (< 5%)
- **TLS Handshakes**: ~10-20% slower due to KAT on first use

## Verifying FIPS Mode

### Programmatically

Check if FIPS mode is active in your code:

```go
import "crypto/fips140"

if crypto/fips140.Enabled() {
    fmt.Println("Running in FIPS 140-3 mode")
} else {
    fmt.Println("Running in standard mode")
}
```

### At Runtime

```bash
# Check FIPS status
GODEBUG=fips140=on ./muster version

# Should output: "FIPS 140-3 mode: enabled"
```

### Via Logs

When FIPS mode is enabled, look for these log entries at startup:

```
{"level":"info","msg":"FIPS 140-3 mode enabled","module":"crypto"}
{"level":"info","msg":"Cryptographic module self-check passed","module":"crypto"}
{"level":"info","msg":"Known-answer tests completed successfully","module":"crypto"}
```

## Supported Platforms

FIPS 140-3 mode is supported on:

- ✅ Linux (x86_64, arm64)
- ✅ macOS (x86_64, arm64)
- ✅ Windows (x86_64)
- ✅ FreeBSD (x86_64)

Not supported:

- ❌ OpenBSD
- ❌ WebAssembly (WASM)
- ❌ AIX
- ❌ 32-bit Windows

## Compliance Checklist

For FedRAMP/FIPS 140-3 compliance, ensure:

### Build Configuration
- [ ] Built with Go 1.26.1 or later
- [ ] Built with `GOFIPS140=v1.26.0` or `GOFIPS140=latest`
- [ ] Binary includes FIPS module (check with `go version -m`)

### Runtime Configuration
- [ ] `GODEBUG=fips140=on` or `GODEBUG=fips140=only` set
- [ ] Self-tests pass at startup (check logs)
- [ ] No non-FIPS algorithms used (verify with logs)

### Operational Security
- [ ] TLS configured with FIPS-approved cipher suites only
- [ ] RSA keys are 2048-bit or larger
- [ ] ECDSA keys use P-256/P-384/P-521 curves
- [ ] Random number generation uses approved DRBG
- [ ] Key management follows FIPS guidelines

### Documentation
- [ ] FIPS compliance documented in security policy
- [ ] Self-test results logged and audited
- [ ] Platform listed in CMVP validation
- [ ] Change management process documented

## Testing FIPS Compliance

### Unit Tests

```bash
# Run tests with FIPS mode enabled
GODEBUG=fips140=on go test -v ./pkg/auth/...

# Run tests with FIPS enforcement
GODEBUG=fips140=only go test -v ./pkg/auth/...
```

### Validation Script

```bash
#!/bin/bash
# scripts/validate-fips.sh

set -e

echo "Building with FIPS module..."
GOFIPS140=v1.26.0 go build -o bin/muster-fips ./cmd/openapi-cli

echo "Running FIPS self-tests..."
GODEBUG=fips140=on ./bin/muster-fips test fips

echo "Checking FIPS mode..."
GODEBUG=fips140=on ./bin/muster-fips version | grep -q "FIPS enabled"

echo "Running test suite in FIPS mode..."
GODEBUG=fips140=on go test -race ./...

echo "✅ FIPS validation complete"
```

## Migration from BoringCrypto

If previously using Go+BoringCrypto:

1. Remove BoringSSL dependencies
2. Update to Go 1.26.1+
3. Replace `GODEBUG=boringcrypto=1` with `GOFIPS140=v1.26.0`
4. Test all cryptographic operations
5. Verify no compatibility issues

## Troubleshooting

### "FIPS mode not supported on this platform"

FIPS mode is not available on OpenBSD, WASM, AIX, or 32-bit Windows. Use a supported platform.

### "Self-test failed"

This indicates potential tampering or corruption. Rebuild the binary and ensure:
- Binary hasn't been modified post-build
- Correct Go version used
- FIPS module properly linked

### "Algorithm not FIPS-approved"

Some algorithms are not FIPS-approved. Common issues:
- Using MD5 or SHA-1 for signatures
- Using RC4 or DES encryption
- Using RSA keys < 2048 bits
- Using non-NIST elliptic curves

Solution: Update code to use FIPS-approved alternatives.

## References

- [NIST FIPS 140-3 Standard](https://csrc.nist.gov/publications/detail/fips/140/3/final)
- [Go FIPS 140-3 Documentation](https://go.dev/doc/security/fips140)
- [Go Cryptographic Module Security Policy](https://csrc.nist.gov/Projects/cryptographic-module-validation-program/modules-in-process/iut-list)
- [FedRAMP Requirements](https://www.fedramp.gov/resources/documents/)

## Support

For FIPS compliance questions or issues:
- Review the [Go security mailing list](https://groups.google.com/g/golang-dev)
- Check the [CMVP module list](https://csrc.nist.gov/projects/cryptographic-module-validation-program/validated-modules)
- Contact the Muster security team

---

**Last Updated**: 2026-03-31  
**Go Version**: 1.26.1  
**FIPS Module Version**: v1.26.0  
**Compliance Level**: Implementation Under Test (IUT)
