package builtin

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// soapWSDL is the discovery fixture for the CLI-level tests (same document
// the pkg/protocol/soap round-trip tests use).
const soapWSDL = `<?xml version="1.0" encoding="utf-8"?>
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
      <soap:address location="http://localhost/echo"/>
    </port>
  </service>
</definitions>`

// soapEchoServer answers one Echo call with the repeated-element response
// shape and records the last envelope it received (fixture: in-process,
// loopback only).
func soapEchoServer(t *testing.T) (*httptest.Server, *string) {
	t.Helper()
	var lastBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		lastBody = string(raw)
		if r.Header.Get("SOAPAction") != `http://example.com/echo/Echo` {
			t.Errorf("server expected SOAPAction header; got %q (Content-Type %q)", r.Header.Get("SOAPAction"), r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", `text/xml; charset=utf-8`)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
<soapenv:Body><EchoResponse><message>ok</message><tag>a</tag><tag>b</tag></EchoResponse></soapenv:Body>
</soapenv:Envelope>`)
	}))
	t.Cleanup(srv.Close)
	return srv, &lastBody
}

func runProtocolSOAP(t *testing.T, args ...string) string {
	t.Helper()
	out := &bytes.Buffer{}
	cmd := newProtocolSOAPCommand()
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	require.NoError(t, cmd.Execute())
	return out.String()
}

func writeSoapWSDL(t *testing.T, serverURL string) string {
	t.Helper()
	// Point the wsdl:service address at the live loopback server so a call
	// needs no --endpoint override (and one test proves the override works).
	doc := bytes.ReplaceAll([]byte(soapWSDL),
		[]byte("http://localhost/echo"), []byte(serverURL))
	path := filepath.Join(t.TempDir(), "echo.wsdl")
	require.NoError(t, os.WriteFile(path, doc, 0o600))
	return path
}

func TestProtocolSOAP_Discover_PrintsTypedParts(t *testing.T) {
	srv, _ := soapEchoServer(t)
	out := runProtocolSOAP(t, "--wsdl", writeSoapWSDL(t, srv.URL))

	assert.Contains(t, out, `"id": "EchoService-Echo"`)
	assert.Contains(t, out, `"soap_action": "http://example.com/echo/Echo"`)
	assert.Contains(t, out, `"soap_version": "1.1"`)
	assert.Contains(t, out, `"repeated": true`, "the repeated element fact must reach the listing")
}

func TestProtocolSOAP_Call_RoundTripThroughHTTP(t *testing.T) {
	srv, lastBody := soapEchoServer(t)
	out := runProtocolSOAP(t,
		"--wsdl", writeSoapWSDL(t, srv.URL),
		"--operation", "EchoService-Echo",
		"--args", `{"message":"hi","tags":["x","y"]}`,
	)

	assert.Contains(t, out, `"message": "ok"`, "the echoed response must render as JSON")
	assert.Contains(t, out, `"a"`, "the repeated element renders as an array")
	assert.Contains(t, *lastBody, "<tags>x</tags><tags>y</tags>",
		"the repeated argument must render as repeated XML elements")
	assert.Contains(t, *lastBody, "EchoRequest", "document-style wrapper element")
}

func TestProtocolSOAP_Call_RawXMLResponse(t *testing.T) {
	srv, _ := soapEchoServer(t)
	out := runProtocolSOAP(t,
		"--wsdl", writeSoapWSDL(t, srv.URL),
		"--operation", "EchoService-Echo",
		"--args", `{"message":"hi","_output":"xml"}`,
	)
	assert.Contains(t, out, "EchoResponse", "raw XML rendering must return the response envelope")
}

func TestProtocolSOAP_Document_PrintsEnvelopeNotCall(t *testing.T) {
	srv, lastBody := soapEchoServer(t)
	out := runProtocolSOAP(t,
		"--wsdl", writeSoapWSDL(t, srv.URL),
		"--operation", "EchoService-Echo",
		"--args", `{"message":"hi"}`,
		"--document",
	)
	assert.Contains(t, out, "Content-Type: text/xml; charset=utf-8")
	assert.Contains(t, out, "SOAPAction: http://example.com/echo/Echo")
	assert.Empty(t, *lastBody, "--document must not contact the endpoint")
}

func TestProtocolSOAP_AddresssingFlagAddsHeaderBlock(t *testing.T) {
	srv, lastBody := soapEchoServer(t)
	out := runProtocolSOAP(t,
		"--wsdl", writeSoapWSDL(t, srv.URL),
		"--operation", "EchoService-Echo",
		"--args", `{"message":"hi"}`,
		"--document", "--addressing",
	)
	assert.Contains(t, out, "wsa:Action", "the addressing flag must add the WS-Addressing block")
	assert.Empty(t, *lastBody)
}

func TestProtocolSOAP_Call_EndpointOverrideBeatsWSDLAddress(t *testing.T) {
	// Endpoint resolution lives in the adapter client (pkg/protocol/soap
	// Client.Execute prefers the endpoint it was constructed with and only
	// then falls back to the wsdl:service address). Pin that contract at the
	// CLI seam: the document names a dead address, --endpoint points at the
	// live loopback server, and the call must land on the server.
	srv, lastBody := soapEchoServer(t)
	path := filepath.Join(t.TempDir(), "echo-dead-address.wsdl")
	require.NoError(t, os.WriteFile(path, []byte(soapWSDL), 0o600))

	out := runProtocolSOAP(t,
		"--wsdl", path,
		"--endpoint", srv.URL,
		"--operation", "EchoService-Echo",
		"--args", `{"message":"hi"}`,
	)

	assert.Contains(t, out, `"message": "ok"`,
		"the --endpoint override must route the call to the live server")
	assert.Contains(t, *lastBody, "EchoRequest",
		"the envelope must reach the server named by --endpoint")
}

func TestProtocolSOAP_RootWiring(t *testing.T) {
	// The soap group must be mounted under the protocol group (frozen root).
	root := NewRootCommand(nil)
	found := false
	for _, c := range root.Commands() {
		if c.Name() == "protocol" {
			for _, sub := range c.Commands() {
				if sub.Name() == "soap" {
					found = true
					assert.Contains(t, sub.Short, "SOAP")
				}
			}
		}
	}
	assert.True(t, found, "protocol soap command must be registered under the protocol group")
}
