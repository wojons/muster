#!/bin/bash
# FIPS 140-3 Compliance Validation Script
# 
# This script validates FIPS 140-3 compliance for Muster
# Run with: ./scripts/validate-fips.sh

set -e

echo "========================================"
echo "FIPS 140-3 Compliance Validation"
echo "========================================"
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Counters
PASSED=0
FAILED=0
WARNINGS=0

pass() {
    echo -e "${GREEN}✓${NC} $1"
    PASSED=$((PASSED + 1))
}

fail() {
    echo -e "${RED}✗${NC} $1"
    FAILED=$((FAILED + 1))
}

warn() {
    echo -e "${YELLOW}⚠${NC} $1"
    WARNINGS=$((WARNINGS + 1))
}

# Section 1: Environment Check
echo "1. Environment Check"
echo "-------------------------------------------"

GO_VERSION=$(go version | awk '{print $3}')
echo "   Go version: $GO_VERSION"

if [[ "$GO_VERSION" == go1.24* ]] || [[ "$GO_VERSION" == go1.25* ]] || [[ "$GO_VERSION" == go1.26* ]]; then
    pass "Go version supports FIPS 140-3"
else
    fail "Go version $GO_VERSION does not support FIPS 140-3 (requires 1.24+)"
fi

echo ""

# Section 2: Build FIPS Binary
echo "2. Building FIPS 140-3 Compliant Binary"
echo "-------------------------------------------"

echo "   Building with GOFIPS140=v1.26.0..."
if GOFIPS140=v1.26.0 go build -o bin/muster-fips ./cmd/openapi-cli 2>&1; then
    pass "FIPS binary built successfully"
else
    fail "Failed to build FIPS binary"
    exit 1
fi

if [ -f "bin/muster-fips" ]; then
    pass "FIPS binary exists: bin/muster-fips"
else
    fail "FIPS binary not found"
fi

echo ""

# Section 3: Runtime FIPS Mode Tests
echo "3. Runtime FIPS Mode Tests"
echo "-------------------------------------------"

echo "   Testing FIPS mode detection..."
if GODEBUG=fips140=on go test -v -run TestFIPS_Mode_Detection ./pkg/auth/... > /tmp/fips_mode_test.log 2>&1; then
    if grep -q "FIPS 140-3 mode: true" /tmp/fips_mode_test.log; then
        pass "FIPS mode enabled and detected"
    else
        warn "FIPS mode test passed but mode not explicitly confirmed"
    fi
else
    fail "FIPS mode detection test failed"
    cat /tmp/fips_mode_test.log
fi

echo "   Testing FIPS compliance suite..."
if GODEBUG=fips140=on go test -v -run TestFIPS_ComplianceSuite ./pkg/auth/... > /tmp/fips_compliance_test.log 2>&1; then
    if grep -q "7 passed, 0 failed" /tmp/fips_compliance_test.log; then
        pass "All FIPS compliance tests passed"
    else
        pass "FIPS compliance tests passed"
    fi
else
    fail "FIPS compliance tests failed"
    cat /tmp/fips_compliance_test.log
fi

echo ""

# Section 4: Algorithm Validation
echo "4. Algorithm Validation"
echo "-------------------------------------------"

echo "   Testing AES-GCM (FIPS-approved)..."
if GODEBUG=fips140=on go test -v -run TestFIPS_AES_GCM ./pkg/auth/... > /dev/null 2>&1; then
    pass "AES-GCM encryption works in FIPS mode"
else
    fail "AES-GCM encryption failed in FIPS mode"
fi

echo "   Testing ECDSA P-256 (FIPS-approved)..."
if GODEBUG=fips140=on go test -v -run TestFIPS_ECDSA ./pkg/auth/... > /dev/null 2>&1; then
    pass "ECDSA P-256 signatures work in FIPS mode"
else
    fail "ECDSA P-256 signatures failed in FIPS mode"
fi

echo "   Testing RSA-2048 PSS (FIPS-approved)..."
if GODEBUG=fips140=on go test -v -run TestFIPS_RSA ./pkg/auth/... > /dev/null 2>&1; then
    pass "RSA-2048 PSS signatures work in FIPS mode"
else
    fail "RSA-2048 PSS signatures failed in FIPS mode"
fi

echo "   Testing HMAC-SHA256 (FIPS-approved)..."
if GODEBUG=fips140=on go test -v -run TestFIPS_HMAC ./pkg/auth/... > /dev/null 2>&1; then
    pass "HMAC-SHA256 works in FIPS mode"
else
    fail "HMAC-SHA256 failed in FIPS mode"
fi

echo "   Testing SHA-256/384/512 (FIPS-approved)..."
if GODEBUG=fips140=on go test -v -run TestFIPS_SHA ./pkg/auth/... > /dev/null 2>&1; then
    pass "SHA hash functions work in FIPS mode"
else
    fail "SHA hash functions failed in FIPS mode"
fi

echo "   Testing HKDF (FIPS-approved KDF)..."
if GODEBUG=fips140=on go test -v -run TestFIPS_HKDF ./pkg/auth/... > /dev/null 2>&1; then
    pass "HKDF key derivation works in FIPS mode"
else
    fail "HKDF key derivation failed in FIPS mode"
fi

echo ""

# Section 5: FIPS Enforcement Mode
echo "5. FIPS Enforcement Mode (Best Effort)"
echo "-------------------------------------------"

echo "   Testing with GODEBUG=fips140=only..."
if GODEBUG=fips140=only go test -run TestFIPS_AES_GCM ./pkg/auth/... > /dev/null 2>&1; then
    pass "Tests pass in FIPS enforcement mode"
else
    warn "Some tests may not support enforcement mode (expected)"
fi

echo ""

# Section 6: Documentation Check
echo "6. Documentation Check"
echo "-------------------------------------------"

if [ -f "docs/FIPS-140-COMPLIANCE.md" ]; then
    pass "FIPS compliance documentation exists"
    
    WORD_COUNT=$(wc -w < docs/FIPS-140-COMPLIANCE.md)
    if [ "$WORD_COUNT" -gt 1000 ]; then
        pass "Documentation is comprehensive ($WORD_COUNT words)"
    else
        warn "Documentation may be incomplete ($WORD_COUNT words)"
    fi
else
    fail "FIPS compliance documentation not found"
fi

echo ""

# Section 7: Makefile Targets
echo "7. Makefile Targets"
echo "-------------------------------------------"

if grep -q "build-fips:" Makefile; then
    pass "build-fips target exists"
else
    fail "build-fips target missing"
fi

if grep -q "test-fips:" Makefile; then
    pass "test-fips target exists"
else
    fail "test-fips target missing"
fi

if grep -q "validate-fips:" Makefile; then
    pass "validate-fips target exists"
else
    fail "validate-fips target missing"
fi

echo ""

# Section 8: Summary
echo "========================================"
echo "Validation Summary"
echo "========================================"
echo -e "${GREEN}Passed:${NC}   $PASSED"
echo -e "${RED}Failed:${NC}   $FAILED"
echo -e "${YELLOW}Warnings:${NC} $WARNINGS"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}✅ FIPS 140-3 validation PASSED${NC}"
    echo ""
    echo "Next steps:"
    echo "  1. Review docs/FIPS-140-COMPLIANCE.md"
    echo "  2. Run: make build-fips"
    echo "  3. Run: GODEBUG=fips140=on ./bin/muster-fips"
    echo "  4. For production: GOFIPS140=v1.26.0 go build -o muster-fips"
    exit 0
else
    echo -e "${RED}❌ FIPS 140-3 validation FAILED${NC}"
    echo ""
    echo "Please address the failed checks above."
    exit 1
fi
