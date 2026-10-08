package soap

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wojons/muster/pkg/errors"
	"github.com/wojons/muster/pkg/protocol"
)

// ── Fixtures ────────────────────────────────────────────────────────────────

const wsdl11 = `<?xml version="1.0" encoding="utf-8"?>
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
            <xsd:element name="when" type="xsd:dateTime" nillable="true"/>
            <xsd:element name="tags" type="xsd:string" maxOccurs="unbounded"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="EchoResponse">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="message" type="xsd:string"/>
            <xsd:element name="tag" type="xsd:string" maxOccurs="unbounded"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="PingRequest">
        <xsd:complexType><xsd:sequence/></xsd:complexType>
      </xsd:element>
      <xsd:element name="Pong">
        <xsd:complexType>
          <xsd:sequence><xsd:element name="at" type="xsd:dateTime"/></xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <message name="EchoInput"><part name="parameters" element="tns:EchoRequest"/></message>
  <message name="EchoOutput"><part name="parameters" element="tns:EchoResponse"/></message>
  <message name="PingInput"><part name="parameters" element="tns:PingRequest"/></message>
  <message name="PongOutput"><part name="parameters" element="tns:Pong"/></message>

  <portType name="EchoPortType">
    <operation name="Echo">
      <input message="tns:EchoInput"/>
      <output message="tns:EchoOutput"/>
    </operation>
    <operation name="Ping">
      <input message="tns:PingInput"/>
      <output message="tns:PongOutput"/>
    </operation>
  </portType>

  <binding name="EchoBinding" type="tns:EchoPortType">
    <soap:binding style="document" transport="http://schemas.xmlsoap.org/soap/http"/>
    <operation name="Echo">
      <soap:operation soapAction="http://example.com/echo/Echo"/>
      <input><soap:body use="literal"/></input>
      <output><soap:body use="literal"/></output>
    </operation>
    <operation name="Ping">
      <soap:operation soapAction=""/>
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

const wsdl20 = `<?xml version="1.0" encoding="utf-8"?>
<description xmlns="http://www.w3.org/ns/wsdl"
             xmlns:wsoap="http://www.w3.org/ns/wsdl/soap"
             xmlns:xsd="http://www.w3.org/2001/XMLSchema"
             targetNamespace="http://example.com/calc">
  <types>
    <xsd:schema targetNamespace="http://example.com/calc">
      <xsd:element name="AddRequest">
        <xsd:complexType>
          <xsd:sequence>
            <xsd:element name="a" type="xsd:double"/>
            <xsd:element name="b" type="xsd:double"/>
          </xsd:sequence>
        </xsd:complexType>
      </xsd:element>
      <xsd:element name="AddResponse">
        <xsd:complexType>
          <xsd:sequence><xsd:element name="sum" type="xsd:double"/></xsd:sequence>
        </xsd:complexType>
      </xsd:element>
    </xsd:schema>
  </types>

  <interface name="CalcInterface">
    <operation name="Add"
               pattern="http://www.w3.org/ns/wsdl/in-out">
      <input element="tns:AddRequest"/>
      <output element="tns:AddResponse"/>
    </operation>
  </interface>

  <binding name="CalcBinding" interface="tns:CalcInterface"
           type="http://www.w3.org/ns/wsdl/soap"
           wsoap:version="1.2"
           wsoap:protocol="http://www.w3.org/2003/05/soap/bindings/HTTP/">
    <operation ref="tns:Add" wsoap:action="http://example.com/calc/Add"/>
  </binding>

  <service name="CalcService" interface="tns:CalcInterface">
    <endpoint name="CalcEndpoint" binding="tns:CalcBinding"
              address="http://localhost/calc"/>
  </service>
</description>`

// echoSOAPHandler is the in-process SOAP 1.1 server fixture: it validates the
// envelope shape it received and answers per the requested path — success
// (with a repeated element), a SOAP 1.1 fault, or a SOAP 1.2 fault.
func echoSOAPHandler(t *testing.T, fault1_1, fault1_2 bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), ":Envelope") && !strings.Contains(string(body), "Envelope") {
			t.Errorf("server got a non-SOAP body: %s", body)
		}
		action := r.Header.Get("SOAPAction")
		ct := r.Header.Get("Content-Type")
		if action == "" && !strings.Contains(ct, "action=") {
			t.Errorf("server got no SOAPAction header and no action content-type parameter")
		}
		w.Header().Set("Content-Type", `text/xml; charset=utf-8`)
		switch {
		case fault1_1:
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
<soapenv:Body><soapenv:Fault>
  <faultcode>soapenv:Client</faultcode>
  <faultstring>Invalid message format</faultstring>
  <faultactor>http://example.com/echo</faultactor>
  <detail><e:reason xmlns:e="http://example.com/errors">missing field</e:reason></detail>
</soapenv:Fault></soapenv:Body></soapenv:Envelope>`)
		case fault1_2:
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `<?xml version="1.0"?><env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">
<env:Body><env:Fault>
  <env:Code><env:Value>env:Sender</env:Value>
    <env:Subcode><env:Value>http://example.com/errors#BadCount</env:Value></env:Subcode></env:Code>
  <env:Reason><env:Text xml:lang="en">count must be positive</env:Text></env:Reason>
  <env:Detail><e:limit xmlns:e="http://example.com/errors">100</e:limit></env:Detail>
</env:Fault></env:Body></env:Envelope>`)
		default:
			_, _ = io.WriteString(w, `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
<soapenv:Body><EchoResponse>
  <message>echoed</message>
  <tag>alpha</tag>
  <tag>beta</tag>
</EchoResponse></soapenv:Body></soapenv:Envelope>`)
		}
	}
}

// writeWSDL writes a fixture document to a temp file and returns its path.
func writeWSDL(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "service.wsdl")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// ── Ingestion tests ─────────────────────────────────────────────────────────

func TestDiscover_WSDL11_MapsTypedParts(t *testing.T) {
	c := NewClient("")
	ops, err := c.Discover(context.Background(), writeWSDL(t, wsdl11))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("want 2 operations, got %d", len(ops))
	}
	echo := ops[0]
	if echo.ID != "EchoService-Echo" {
		t.Fatalf("operation ID = %q", echo.ID)
	}
	if echo.Source[SourceSOAPAction] != "http://example.com/echo/Echo" {
		t.Errorf("soap_action = %q", echo.Source[SourceSOAPAction])
	}
	if echo.Source[SourceEndpoint] != "http://localhost/echo" {
		t.Errorf("endpoint = %q", echo.Source[SourceEndpoint])
	}
	if echo.Source[SourceInputElem] != "{http://example.com/echo}EchoRequest" {
		t.Errorf("input_element = %q", echo.Source[SourceInputElem])
	}

	// Typed parts: required, optional, nillable, repeated.
	type gotField struct {
		Type     string
		Required bool
		Repeated bool
		ItemType string
		Descr    string
	}
	got := map[string]gotField{}
	for _, p := range echo.RequestBody {
		got[p.Name] = gotField{p.Type, p.Required, p.Repeated, p.ItemType, p.Description}
	}
	if g := got["message"]; !g.Required || g.Type != "string" {
		t.Errorf("message = %+v, want required string", g)
	}
	if g := got["count"]; g.Required || g.Type != "integer" {
		t.Errorf("count = %+v, want optional integer (minOccurs=0)", g)
	}
	if g := got["when"]; g.Type != "string" || g.Descr == "" {
		t.Errorf("when = %+v, want nillable note in description", g)
	}
	if g := got["tags"]; !g.Repeated || g.Type != "array" || g.ItemType != "string" {
		t.Errorf("tags = %+v, want repeated array of string (maxOccurs=unbounded)", g)
	}
	if t2 := got["tags"].Type; t2 != "array" {
		t.Errorf("tags type = %q", t2)
	}
}

func TestDiscover_WSDL20_Soap12Version(t *testing.T) {
	c := NewClient("")
	ops, err := c.Discover(context.Background(), writeWSDL(t, wsdl20))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(ops) != 1 {
		t.Fatalf("want 1 operation, got %d", len(ops))
	}
	op := ops[0]
	if op.Source[SourceSOAPVersion] != "1.2" {
		t.Errorf("soap_version = %q, want 1.2", op.Source[SourceSOAPVersion])
	}
	if op.Source[SourceSOAPAction] != "http://example.com/calc/Add" {
		t.Errorf("soap_action = %q", op.Source[SourceSOAPAction])
	}
	if len(op.RequestBody) != 2 {
		t.Fatalf("want 2 request fields, got %d", len(op.RequestBody))
	}
	if op.RequestBody[0].Name != "a" || op.RequestBody[0].Type != "number" {
		t.Errorf("field a = %+v", op.RequestBody[0])
	}
	if c.envelopeVersion() != SOAP12 {
		t.Errorf("document version must select the 1.2 envelope")
	}
}

func TestDiscover_RejectsNonWSDL(t *testing.T) {
	c := NewClient("")
	if _, err := c.Discover(context.Background(), writeWSDL(t, "<html><body/>")); err == nil {
		t.Fatal("non-WSDL root must fail")
	}
}

// ── Envelope golden tests ───────────────────────────────────────────────────

// envelopeGolden discovers the named operation and builds its envelope.
func envelopeGolden(t *testing.T, wsdl, opID string, args map[string]interface{}, opts ...Option) *Envelope {
	t.Helper()
	c := NewClient("", opts...)
	ops, err := c.Discover(context.Background(), writeWSDL(t, wsdl))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	var target *protocol.Operation
	for _, o := range ops {
		if o.ID == opID {
			target = o
		}
	}
	if target == nil {
		t.Fatalf("operation %q not discovered", opID)
	}
	env, err := c.BuildEnvelope(target, args)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	return env
}

func TestBuildEnvelope_SOAP11_Golden(t *testing.T) {
	env := envelopeGolden(t, wsdl11, "EchoService-Echo", map[string]interface{}{
		"message": "hello & <world>",
		"tags":    []interface{}{"a", "b"},
	})

	got := string(env.XML)
	want := `<?xml version="1.0" encoding="utf-8"?>` +
		`<soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">` +
		`<soapenv:Body><EchoRequest>` +
		`<message>hello &amp; &lt;world&gt;</message>` +
		`<tags>a</tags><tags>b</tags>` +
		`</EchoRequest></soapenv:Body></soapenv:Envelope>`
	if got != want {
		t.Fatalf("envelope bytes mismatch:\n got: %s\nwant: %s", got, want)
	}
	if env.ContentType != "text/xml; charset=utf-8" {
		t.Errorf("content type = %q", env.ContentType)
	}
	if env.SOAPAction != "http://example.com/echo/Echo" {
		t.Errorf("SOAPAction = %q", env.SOAPAction)
	}
}

func TestBuildEnvelope_SOAP12_ContentTypeAction(t *testing.T) {
	env := envelopeGolden(t, wsdl20, "CalcService-Add", map[string]interface{}{"a": 1.5, "b": 2})
	if env.SOAPAction != "" {
		t.Errorf("SOAP 1.2 must not set the SOAPAction HTTP header; got %q", env.SOAPAction)
	}
	if env.ContentType != `application/soap+xml; charset=utf-8; action="http://example.com/calc/Add"` {
		t.Errorf("content type = %q", env.ContentType)
	}
	if !strings.Contains(string(env.XML), `<env:Envelope xmlns:env="http://www.w3.org/2003/05/soap-envelope">`) {
		t.Errorf("wrong envelope namespace: %s", env.XML)
	}
	if !strings.Contains(string(env.XML), "<a>1.5</a>") {
		t.Errorf("number rendering broken: %s", env.XML)
	}
}

func TestBuildEnvelope_AddresssingHeaderBlock(t *testing.T) {
	env := envelopeGolden(t, wsdl11, "EchoService-Echo", map[string]interface{}{"message": "x"},
		WithAddressing(true), WithVersion(SOAP12))
	xml := string(env.XML)
	if !strings.Contains(xml, `<wsa:Action>http://example.com/echo/Echo</wsa:Action>`) {
		t.Errorf("wsa:Action missing: %s", xml)
	}
	if !strings.Contains(xml, AddressingNS) {
		t.Errorf("wsa namespace missing: %s", xml)
	}
	if !strings.Contains(xml, "<wsa:MessageID>urn:uuid:") {
		t.Errorf("wsa:MessageID missing: %s", xml)
	}
}

func TestBuildEnvelope_OptionalOmitted(t *testing.T) {
	env := envelopeGolden(t, wsdl11, "EchoService-Echo", map[string]interface{}{"message": "x"})
	if strings.Contains(string(env.XML), "<count>") || strings.Contains(string(env.XML), "<tags>") {
		t.Errorf("omitted optional/repeated fields must not appear: %s", env.XML)
	}
}

// ── Round-trip tests (httptest loopback) ────────────────────────────────────

func soapServer(t *testing.T, fault1_1, fault1_2 bool) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(echoSOAPHandler(t, fault1_1, fault1_2))
	t.Cleanup(srv.Close)
	return srv
}

func clientOver(t *testing.T, wsdl, endpoint string, opts ...Option) *Client {
	t.Helper()
	c := NewClient(endpoint, opts...)
	_, err := c.Discover(context.Background(), writeWSDL(t, wsdl))
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	return c
}

func TestExecute_RoundTrip_Success(t *testing.T) {
	srv := soapServer(t, false, false)
	c := clientOver(t, wsdl11, srv.URL)

	// Patch the discovered endpoint to the live loopback server.
	for _, op := range c.Operations() {
		op.Source[SourceEndpoint] = srv.URL
	}
	c.endpoint = srv.URL

	result, err := c.Execute(context.Background(), "EchoService-Echo", map[string]interface{}{
		"message": "round",
		"tags":    []interface{}{"alpha", "beta"},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	obj, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type %T", result)
	}
	if obj["message"] != "echoed" {
		t.Errorf("message = %v", obj["message"])
	}
	// The repeated element renders as a JSON array.
	tags, ok := obj["tag"].([]interface{})
	if !ok || len(tags) != 2 || tags[0] != "alpha" {
		t.Errorf("repeated element must render as array; got %#v", obj["tag"])
	}
}

func TestExecute_RoundTrip_RepeatedElementDefaultJSON(t *testing.T) {
	srv := soapServer(t, false, false)
	c := clientOver(t, wsdl11, srv.URL)
	c.endpoint = srv.URL
	for _, op := range c.Operations() {
		op.Source[SourceEndpoint] = srv.URL
	}
	result, err := c.Execute(context.Background(), "EchoService-Echo", map[string]interface{}{"message": "m"})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if _, err := json.Marshal(result); err != nil {
		t.Errorf("JSON default result must be JSON-serializable: %v", err)
	}

	// Raw XML rendering on demand.
	raw, err := c.Execute(context.Background(), "EchoService-Echo",
		map[string]interface{}{"message": "m", "_output": "xml"})
	if err != nil {
		t.Fatalf("execute xml: %v", err)
	}
	if s, ok := raw.(string); !ok || !strings.Contains(s, "EchoResponse") {
		t.Errorf("raw XML output broken: %#v", raw)
	}
}

func TestExecute_Fault11_MapsToBadRequestWithDetail(t *testing.T) {
	srv := soapServer(t, true, false)
	c := clientOver(t, wsdl11, srv.URL)
	c.endpoint = srv.URL
	for _, op := range c.Operations() {
		op.Source[SourceEndpoint] = srv.URL
	}

	_, err := c.Execute(context.Background(), "EchoService-Echo", map[string]interface{}{"message": "bad"})
	if err == nil {
		t.Fatal("fault must fail the call")
	}
	msg := err.Error()
	if !strings.Contains(msg, "Invalid message format") {
		t.Errorf("fault reason lost: %v", err)
	}
	appErr := asAppError(t, err)
	if appErr == nil {
		t.Fatalf("error must be an AppError: %T", err)
	}
	if appErr.Code() != errors.CodeBadRequest {
		t.Errorf("fault code soapenv:Client must map to BAD_REQUEST; got %s", appErr.Code())
	}
	if appErr.Details()["fault_code"] != "soapenv:Client" {
		t.Errorf("fault_code detail = %v", appErr.Details()["fault_code"])
	}
	if !strings.Contains(fmt.Sprint(appErr.Details()["fault_detail"]), "missing field") {
		t.Errorf("fault detail lost: %v", appErr.Details())
	}
}

func TestExecute_Fault12_MapsToBadRequestWithSubcode(t *testing.T) {
	srv := soapServer(t, false, true)
	c := clientOver(t, wsdl11, srv.URL, WithVersion(SOAP12))
	c.endpoint = srv.URL
	for _, op := range c.Operations() {
		op.Source[SourceEndpoint] = srv.URL
	}

	_, err := c.Execute(context.Background(), "EchoService-Echo", map[string]interface{}{"message": "x"})
	if err == nil {
		t.Fatal("fault must fail the call")
	}
	appErr := asAppError(t, err)
	if appErr == nil {
		t.Fatalf("error must be an AppError: %T", err)
	}
	if appErr.Code() != errors.CodeBadRequest {
		t.Errorf("env:Sender must map to BAD_REQUEST; got %s", appErr.Code())
	}
	if appErr.Details()["fault_subcode"] != "http://example.com/errors#BadCount" {
		t.Errorf("subcode lost: %v", appErr.Details())
	}
}

func TestFault_ServerCode_MapsToInternal(t *testing.T) {
	f := &Fault{Version: SOAP11, Code: "soapenv:Server", Local: "Server", Reason: "boom"}
	appErr := f.toAppError()
	if appErr.Code() != errors.CodeInternal {
		t.Errorf("Server fault code must map to INTERNAL_ERROR; got %s", appErr.Code())
	}
}

// fault-on-200: some services answer a valid envelope carrying a Fault with
// HTTP 200 — the mapping must still fire.
func TestExecute_FaultInside200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", `text/xml; charset=utf-8`)
		_, _ = io.WriteString(w, `<?xml version="1.0"?><soapenv:Envelope xmlns:soapenv="http://schemas.xmlsoap.org/soap/envelope/">
<soapenv:Body><soapenv:Fault><faultcode>soapenv:Server</faultcode><faultstring>internal</faultstring></soapenv:Fault></soapenv:Body>
</soapenv:Envelope>`)
	}))
	defer srv.Close()

	c := clientOver(t, wsdl11, srv.URL)
	c.endpoint = srv.URL
	for _, op := range c.Operations() {
		op.Source[SourceEndpoint] = srv.URL
	}
	_, err := c.Execute(context.Background(), "EchoService-Echo", map[string]interface{}{"message": "m"})
	if err == nil {
		t.Fatal("a fault inside a 200 response must still fail the call")
	}
	if appErr := asAppError(t, err); appErr == nil || appErr.Code() != errors.CodeInternal {
		t.Errorf("fault inside 200 must map to INTERNAL_ERROR; got %#v", appErr)
	}
}

// ── Out-of-scope / negative tests ───────────────────────────────────────────

func TestExecute_UnknownOperationFails(t *testing.T) {
	srv := soapServer(t, false, false)
	c := clientOver(t, wsdl11, srv.URL)
	if _, err := c.Execute(context.Background(), "Nope-Nope", nil); err == nil {
		t.Fatal("unknown operation must fail")
	}
}

func TestExecute_NoDiscoveryFails(t *testing.T) {
	c := NewClient("http://localhost/x")
	if _, err := c.Execute(context.Background(), "EchoService-Echo", nil); err == nil {
		t.Fatal("Execute before Discover must fail with a validation error")
	}
}

// asAppError extracts the AppError behind an error. AppError is the concrete
// error type Execute returns (fault mapping builds one directly), so a plain
// type assertion is the contract; nil means the error is not an AppError.
func asAppError(t *testing.T, err error) *errors.AppError {
	t.Helper()
	if appErr, ok := err.(*errors.AppError); ok {
		return appErr
	}
	return nil
}
