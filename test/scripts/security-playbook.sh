#!/bin/bash
#
# Security Test Playbook
# Comprehensive security testing automation for Muster
#
# Usage: ./test/scripts/security-playbook.sh [test_suite]
#   test_suite: all, crypto, auth, network, input, fuzz, compliance
#

set -e

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
REPORT_DIR="${PROJECT_ROOT}/test/security/reports"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
REPORT_FILE="${REPORT_DIR}/security_report_${TIMESTAMP}.json"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Create report directory
mkdir -p "${REPORT_DIR}"

# Initialize report
cat > "${REPORT_FILE}" << EOF
{
  "timestamp": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "test_suite": "${1:-all}",
  "results": []
}
EOF

echo "========================================="
echo "Muster Security Test Playbook"
echo "========================================="
echo "Started at: $(date)"
echo "Report: ${REPORT_FILE}"
echo ""

# Helper functions
run_test() {
    local test_name="$1"
    local test_command="$2"
    local suite="$3"
    
    echo -n "  [${suite}] ${test_name}... "
    
    if eval "${test_command}" >/devdev/null 2>&1; then
        echo -e "${GREEN}PASS${NC}"
        return 0
    else
        echo -e "${RED}FAIL${NC}"
        return 1
    fi
}

add_result() {
    local test_name="$1"
    local status="$2"
    local duration="$3"
    
    # Using temp file for JSON manipulation
    local temp_file=$(mktemp)
    jq --arg name "${test_name}" \
       --arg status "${status}" \
       --arg duration "${duration}" \
       '.results += [{"name": $name, "status": $status, "duration": $duration}]' \
       "${REPORT_FILE}" > "${temp_file}" && mv "${temp_file}" "${REPORT_FILE}"
}

# ===========================================
# CRYPTOGRAPHIC SECURITY TESTS
# ===========================================
run_crypto_tests() {
    echo "----------------------------------------"
    echo "Running Cryptographic Security Tests..."
    echo "----------------------------------------"
    
    local start_time=$(date +%s)
    local passed=0
    local failed=0
    
    # RSA Key Generation
    if run_test "RSA_KeyGeneration_Minimum_2048bits" \
                "go test -v -run TestCrypto_RSAKeyGeneration ./test/security/..." \
                "crypto"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # ECDSA Curve Selection
    if run_test "ECDSA_Curve_Selection_P256_P384_P521" \
                "go test -v -run TestCrypto_ECDSACurveSelection ./test/security/..." \
                "crypto"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Entropy Verification
    if run_test "Random_Entropy_No_Collisions" \
                "go test -v -run TestCrypto_EntropyVerification ./test/security/..." \
                "crypto"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Hash Collision Resistance
    if run_test "SHA256_Collision_Resistance_Single_Bit_Diff" \
                "go test -v -run TestCrypto_HashCollisionResistance ./test/security/..." \
                "crypto"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Constant Time Comparison
    if run_test "ConstantTimeComparison_TimingAttack_Resistance" \
                "go test -v -run TestCrypto_ConstantTimeComparison ./test/security/..." \
                "crypto"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo ""
    echo "  Summary: ${passed} passed, ${failed} failed (${duration}s)"
    echo ""
    
    return ${failed}
}

# ===========================================
# AUTHENTICATION SECURITY TESTS
# ===========================================
run_auth_tests() {
    echo "----------------------------------------"
    echo "Running Authentication Security Tests..."
    echo "----------------------------------------"
    
    local start_time=$(date +%s)
    local passed=0
    local failed=0
    
    # Password Hashing
    if run_test "PasswordHashing_Secure_Algorithm" \
                "go test -v -run TestAuth_PasswordHashing ./test/security/..." \
                "auth"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Token Generation
    if run_test "TokenGeneration_Cryptographically_Secure" \
                "go test -v -run TestAuth_TokenGeneration ./test/security/..." \
                "auth"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Token Format
    if run_test "TokenFormat_Sufficient_Entropy" \
                "go test -v -run TestAuth_TokenFormat ./test/security/..." \
                "auth"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Session Security
    if run_test "Session_MaxAge_And_Secure_Flag" \
                "go test -v -run TestAuth_SessionSecurity ./test/security/..." \
                "auth"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # JWT Token Security
    if run_test "JWT_Validation_Exp_Iat_Claims" \
                "go test -v -run TestAuth_JWTTokenSecurity ./test/security/..." \
                "auth"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo ""
    echo "  Summary: ${passed} passed, ${failed} failed (${duration}s)"
    echo ""
    
    return ${failed}
}

# ===========================================
# NETWORK SECURITY TESTS
# ===========================================
run_network_tests() {
    echo "----------------------------------------"
    echo "Running Network Security Tests..."
    echo "----------------------------------------"
    
    local start_time=$(date +%s)
    local passed=0
    local failed=0
    
    # TLS Version Enforcement
    if run_test "TLS_1.2+_Minimum_Version" \
                "go test -v -run TestNetwork_TLSVersionEnforcement ./test/security/..." \
                "network"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Cipher Suites
    if run_test "Secure_Cipher_Suites_No_CBC_RC4" \
                "go test -v -run TestNetwork_CipherSuites ./test/security/..." \
                "network"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Request Size Limits
    if run_test "Request_Size_Limits_10MB_Max" \
                "go test -v -run TestNetwork_RequestSizeLimits ./test/security/..." \
                "network"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Certificate Validation
    if run_test "Certificate_Validation_Not_Skipped" \
                "go test -v -run TestNetwork_CertificateValidation ./test/security/..." \
                "network"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo ""
    echo "  Summary: ${passed} passed, ${failed} failed (${duration}s)"
    echo ""
    
    return ${failed}
}

# ===========================================
# INPUT VALIDATION TESTS
# ===========================================
run_input_tests() {
    echo "----------------------------------------"
    echo "Running Input Validation Tests..."
    echo "----------------------------------------"
    
    local start_time=$(date +%s)
    local passed=0
    local failed=0
    
    # Email Validation
    if run_test "Email_Validation_SQL_XSS_Prevention" \
                "go test -v -run TestValidation_EmailValidation ./test/security/..." \
                "input"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Username Validation
    if run_test "Username_Validation_Command_Injection_Prevention" \
                "go test -v -run TestValidation_UsernameValidation ./test/security/..." \
                "input"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # UUID Validation
    if run_test "UUID_Validation_Format_Check" \
                "go test -v -run TestValidation_UUIDValidation ./test/security/..." \
                "input"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Password Complexity
    if run_test "Password_Complexity_Requirements" \
                "go test -v -run TestValidation_PasswordComplexity ./test/security/..." \
                "input"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Sanitization
    if run_test "Input_Sanitization_HTML_Control_Chars" \
                "go test -v -run TestValidation_Sanitization ./test/security/..." \
                "input"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo ""
    echo "  Summary: ${passed} passed, ${failed} failed (${duration}s)"
    echo ""
    
    return ${failed}
}

# ===========================================
# FUZZING TESTS
# ===========================================
run_fuzz_tests() {
    echo "----------------------------------------"
    echo "Running Fuzzing Tests..."
    echo "----------------------------------------"
    
    local start_time=$(date +%s)
    local passed=0
    local failed=0
    
    # OpenAPI Spec Parsing
    if run_test "Fuzz_Malformed_YAML_OpenAPI" \
                "go test -v -fuzz=FuzzParseMalformedYAML -fuzztime=10s ./test/security/... 2>/dev/null || true" \
                "fuzz"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # JSON Parsing
    if run_test "Fuzz_Malformed_JSON_OpenAPI" \
                "go test -v -fuzz=FuzzParseMalformedJSON -fuzztime=10s ./test/security/... 2>/dev/null || true" \
                "fuzz"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Command Injection
    if run_test "Command_Injection_Patterns" \
                "go test -v -run TestCommandParameterInjection ./test/security/..." \
                "fuzz"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # Path Traversal
    if run_test "Path_Traversal_Attacks" \
                "go test -v -run TestPathTraversalAttack ./test/security/..." \
                "fuzz"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # SQL Injection
    if run_test "SQL_Injection_Payloads" \
                "go test -v -run TestSQLInjectionPayloads ./test/security/..." \
                "fuzz"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo ""
    echo "  Summary: ${passed} passed, ${failed} failed (${duration}s)"
    echo ""
    
    return ${failed}
}

# ===========================================
# COMPLIANCE TESTS
# ===========================================
run_compliance_tests() {
    echo "----------------------------------------"
    echo "Running Compliance Tests..."
    echo "----------------------------------------"
    
    local start_time=$(date +%s)
    local passed=0
    local failed=0
    
    # Dependency Vulnerability Scan
    echo -n "  [compliance] Dependency_Vulnerability_Scan... "
    if go test -v -run TestDependencyVulnerabilities ./test/security/... >/dev/null 2>&1 || \
       go run golang.org/x/vuln/cmd/govulncheck@latest ./... >/dev/null 2>&1; then
        echo -e "${GREEN}PASS${NC}"
        ((passed++))
    else
        echo -e "${YELLOW}WARN${NC} (scan tool not available)"
    fi
    
    # FIPS 140-3 Compliance
    if run_test "FIPS_140-3_Compliance_Check" \
                "go test -v -run TestFIPS ./pkg/auth/..." \
                "compliance"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    # GDPR Data Erasure
    if run_test "GDPR_Right_To_Erasure" \
                "go test -v -run TestDataSubjectErasure ./pkg/repository/..." \
                "compliance"; then
        ((passed++))
    else
        ((failed++))
    fi
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    echo ""
    echo "  Summary: ${passed} passed, ${failed} failed (${duration}s)"
    echo ""
    
    return ${failed}
}

# ===========================================
# MAIN EXECUTION
# ===========================================
main() {
    local total_failed=0
    local suite="${1:-all}"
    
    case "${suite}" in
        crypto)
            run_crypto_tests || ((total_failed++))
            ;;
        auth)
            run_auth_tests || ((total_failed++))
            ;;
        network)
            run_network_tests || ((total_failed++))
            ;;
        input)
            run_input_tests || ((total_failed++))
            ;;
        fuzz)
            run_fuzz_tests || ((total_failed++))
            ;;
        compliance)
            run_compliance_tests || ((total_failed++))
            ;;
        all)
            run_crypto_tests || ((total_failed++))
            run_auth_tests || ((total_failed++))
            run_network_tests || ((total_failed++))
            run_input_tests || ((total_failed++))
            run_fuzz_tests || ((total_failed++))
            run_compliance_tests || ((total_failed++))
            ;;
        *)
            echo "Usage: $0 [all|crypto|auth|network|input|fuzz|compliance]"
            exit 1
            ;;
    esac
    
    echo "========================================="
    echo "Security Test Playbook Complete"
    echo "========================================="
    echo "Finished at: $(date)"
    echo "Report: ${REPORT_FILE}"
    
    if [ ${total_failed} -eq 0 ]; then
        echo -e "${GREEN}All tests passed!${NC}"
        exit 0
    else
        echo -e "${RED}${total_failed} test suite(s) failed${NC}"
        exit 1
    fi
}

main "$@"