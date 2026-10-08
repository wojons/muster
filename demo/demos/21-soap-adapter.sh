#!/bin/bash
# Demo 21: SOAP/WSDL adapter (PROTO-008) — ingest, envelope, round-trip, fault.
#
# Gate this script serves: specs/PROTOCOLS.md §6 order 7 (board row PROTO-008).
#
# Fixture ownership: this script starts its own Go SOAP fixture on loopback
# (pkg/protocol/soap test harness compiled via `go run` of a tiny main) — no
# external service. The server validates the SOAPAction header and answers
# with a repeated element (success) or a SOAP 1.1 fault.
set -euo pipefail
cd "$(dirname "$0")/../.."
GREEN='\033[0;32m'; RED='\033[0;31m'; NC='\033[0m'
pass() { echo -e "${GREEN}✓ PASS${NC}: $1"; }
fail() { echo -e "${RED}✗ FAIL${NC}: $1"; exit 1; }

echo "═══════════════════════════════════════════"
echo " Demo 21: SOAP/WSDL adapter (order 7)"
echo "═══════════════════════════════════════════"

BIN=./bin/openapi-cli
[ -x "$BIN" ] || fail "$BIN not found — run 'go build -o bin/openapi-cli ./cmd/openapi-cli' first"

WORK=$(mktemp -d)
FIXTURE_PID=""
cleanup() {
  if [ -n "$FIXTURE_PID" ]; then kill "$FIXTURE_PID" 2>/dev/null || true; fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# ── WSDL fixture (same document the unit tests use) ─────────────────────────
cat > "$WORK/echo.wsdl" <<'EOF'
<?xml version="1.0" encoding="utf-8"?>
<definitions xmlns="http://schemas.xmlsoap.org/wsdl/"
             xmlns:soap="http://schemas.xmlsoap.org/wsdl/soap/"
             xmlns:xsd="http://www.w3.org/2001/XMLSchema"
             xmlns:tns="http://example.com/echo"
             targetNamespace="http://example.com/echo">
  <types>
    <xsd:schema targetNamespace="http://example.com/echo">
      <xsd:element name="EchoRequest">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="message" type="xsd:string"/>
            <xsd:element name="count" type="xsd:int" minOccurs="0"/>
            <xsd:element name="tags" type="xsd:string" maxOccurs="unbounded"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="EchoResponse">
        <xsd:complexType>
          <xsd:sequence><xsd:element name="message" type="xsd:string"/></xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>
  <message name="EchoInput"><part name="parameters" element="tns:EchoRequest"/></message>
  <message name="EchoOutput"><part name="parameters" element="tns:EchoResponse"/></message>
  <portType name="EchoPortType">
    <operation name="Echo">
      <input message="tns:EchoInput"/>
      <output message="tns:EchoOutput"/>
    </operation>
  </portType>
  <binding name="EchoBinding" type="tns:EchoPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Echo">
      <soap:operation soapAction="http://example.com/echo/Echo"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
  </binding>
  <service name="EchoService">
    <port name="EchoPort" binding="tns:EchoBinding">
      <soap:address location="ENDPOINT_PLACEHOLDER"/>
    </port>
  </service>
</definitions>
EOF

# ── SOAP fixture server (loopback, in-process Go) ────────────────────────────
cat > "$WORK/soapfixture.go" <<'EOF'
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	http.HandleFunc("/echo", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("probe") == "1" {
			// Readiness probe: answer 200 without asserting headers.
			_, _ = io.WriteString(w, "ok")
			return
		}
		if _, err := io.ReadAll(r.Body); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		if r.Header.Get("SOAPAction") != "http://example.com/echo/Echo" {
			fmt.Println("fixture: missing SOAPAction header")
			os.Exit(9) // loud fixture failure — the demo must not measure a stranger
		}
		if mode := r.URL.Query().Get("mode"); mode == "fault" {
			w.Header().Set("Content-Type", "text/xml; charset=utf-8")
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><soapenv:Fault><faultcode>soapenv:Server</faultcode><faultstring>boom</faultstring></soapenv:Fault></soapenv:Body></soapenv:Envelope>`)
			return
		}
		w.Header().Set("Content-Type", "text/xml; charset=utf-8")
		_, _ = io.WriteString(w, `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/"><soapenv:Body><EchoResponse><message>ok</message><tag>a</tag><tag>b</tag></EchoResponse></soapenv:Body></soapenv:Envelope>`)
	})
	fmt.Println("ready")
	if err := http.ListenAndServe("127.0.0.1:0", nil); err != nil {
		os.Exit(1)
	}
}
EOF
# ListenAndServe with :0 is not discoverable; bind a fixed high port instead.
sed -i 's/127.0.0.1:0/127.0.0.1:18891/' "$WORK/soapfixture.go"

go run "$WORK/soapfixture.go" >"$WORK/fixture.out" 2>&1 &
FIXTURE_PID=$!
for _ in $(seq 1 50); do
  if curl -s -o /dev/null "http://127.0.0.1:18891/echo?probe=1"; then break; fi
  sleep 0.2
done
kill -0 "$FIXTURE_PID" 2>/dev/null || fail "SOAP fixture did not start"

sed -i "s|ENDPOINT_PLACEHOLDER|http://127.0.0.1:18891/echo|" "$WORK/echo.wsdl"

# ── 1. Discovery: typed parts with repeated/optional facts ──────────────────
DISCOVER_OUT=$("$BIN" protocol soap --wsdl "$WORK/echo.wsdl") || fail "discovery errored: $DISCOVER_OUT"
echo "$DISCOVER_OUT" | grep -q '"id": "EchoService-Echo"' || fail "operation not discovered: $DISCOVER_OUT"
echo "$DISCOVER_OUT" | grep -q '"repeated": true' || fail "repeated element fact missing from listing"
echo "$DISCOVER_OUT" | grep -q '"soap_action": "http://example.com/echo/Echo"' || fail "SOAPAction missing from listing"
pass "protocol soap --wsdl → EchoService-Echo with typed parts (repeated, optional, SOAPAction)"

# ── 2. Document mode: exact 1.1 header shape, no network ────────────────────
DOC_OUT=$("$BIN" protocol soap --wsdl "$WORK/echo.wsdl" --operation EchoService-Echo \
  --args '{"message":"demo","tags":["x","y"]}' --document) || fail "document mode errored: $DOC_OUT"
echo "$DOC_OUT" | grep -q 'Content-Type: text/xml; charset=utf-8' || fail "1.1 content type missing"
echo "$DOC_OUT" | grep -q 'SOAPAction: http://example.com/echo/Echo' || fail "SOAPAction header missing"
echo "$DOC_OUT" | grep -q '<tags>x</tags><tags>y</tags>' || fail "repeated field not rendered as repeated elements"
pass "--document → SOAP 1.1 envelope with SOAPAction header and repeated elements"

# ── 3. Round-trip: live call through the loopback fixture ───────────────────
CALL_OUT=$("$BIN" protocol soap --wsdl "$WORK/echo.wsdl" --operation EchoService-Echo \
  --args '{"message":"demo","tags":["x","y"]}') || fail "call errored: $CALL_OUT"
echo "$CALL_OUT" | grep -q '"message": "ok"' || fail "success round-trip failed: $CALL_OUT"
echo "$CALL_OUT" | grep -q '"a"' || fail "repeated response element not rendered as array"
pass "round-trip → JSON response with repeated element as array"

# ── 4. Fault: SOAP 1.1 fault maps to a coded AppError ───────────────────────
set +e
FAULT_OUT=$("$BIN" protocol soap --wsdl "$WORK/echo.wsdl" --operation EchoService-Echo \
  --args '{"message":"demo"}' --endpoint "http://127.0.0.1:18891/echo?mode=fault" 2>&1)
FAULT_RC=$?
set -e
[ "$FAULT_RC" -ne 0 ] || fail "a SOAP fault must fail the call"
echo "$FAULT_OUT" | grep -q "boom" || fail "fault reason lost: $FAULT_OUT"
echo "$FAULT_OUT" | grep -q "INTERNAL_ERROR" || fail "Server fault must map to INTERNAL_ERROR: $FAULT_OUT"
pass "SOAP fault → exit $FAULT_RC, reason preserved, INTERNAL_ERROR mapping"

echo ""
pass "Demo 21 complete: SOAP/WSDL adapter verified end to end"
