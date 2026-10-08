package soap

import (
	"fmt"
	"strings"

	"github.com/wojons/muster/pkg/errors"
)

// Fault is one SOAP Fault, parsed from either version's wire shape:
//
//	SOAP 1.1  <Fault><faultcode/><faultstring/><faultactor/><detail>…</detail></Fault>
//	          faultcode is a QName: soapenv:Client | soapenv:Server | …
//	SOAP 1.2  <Fault><Code><Value>s:Sender|s:Receiver|…</Value><Subcode>…</Subcode></Code>
//	          <Reason><Text xml:lang="…">…</Text></Reason><Detail>…</Detail></Fault>
type Fault struct {
	Version SOAPVersion
	Code    string // raw fault code (e.g. "soapenv:Client" / "s:Sender")
	Local   string // local part of the fault code ("Client", "Sender", "Receiver", "DataEncodingUnknown")
	Reason  string // faultstring (1.1) / first Reason Text (1.2)
	Actor   string // faultactor (1.1); "" for 1.2
	Detail  string // the Detail element's serialized child text (compact)
	Subcode string // 1.2 Subcode Value when present
}

// parseFault detects and parses a SOAP Fault inside an envelope (or a
// fragment that is itself the fault). It returns (nil, nil) when the body
// carries no Fault — distinct from a parse failure, which the callers treat
// as "not a fault" and continue with the normal decode.
func parseFault(raw []byte) (*Fault, error) {
	root, err := parseXMLTree(raw)
	if err != nil {
		return nil, err
	}
	var faultNode *xmlNode
	var find func(n *xmlNode)
	find = func(n *xmlNode) {
		if faultNode != nil {
			return
		}
		if n.Name == "Fault" {
			faultNode = n
			return
		}
		for _, c := range n.Children {
			find(c)
		}
	}
	find(root)
	if faultNode == nil {
		return nil, nil
	}
	return decodeFaultNode(faultNode), nil
}

// decodeFaultNode extracts the version-agnostic Fault facts from a Fault
// element in either wire shape.
func decodeFaultNode(f *xmlNode) *Fault {
	fault := &Fault{}
	if code := f.child("Code"); code != nil {
		// SOAP 1.2 shape.
		fault.Version = SOAP12
		if v := code.child("Value"); v != nil {
			fault.Code = strings.TrimSpace(v.Text)
			fault.Local = faultCodeLocal(fault.Code)
		}
		if sc := code.child("Subcode"); sc != nil {
			if v := sc.child("Value"); v != nil {
				fault.Subcode = strings.TrimSpace(v.Text)
			}
		}
		if reason := f.child("Reason"); reason != nil {
			for _, t := range reason.children("Text") {
				if txt := strings.TrimSpace(t.Text); txt != "" {
					fault.Reason = txt
					break
				}
			}
		}
	} else {
		// SOAP 1.1 shape.
		fault.Version = SOAP11
		if v := f.child("faultcode"); v != nil {
			fault.Code = strings.TrimSpace(v.Text)
			fault.Local = faultCodeLocal(fault.Code)
		}
		if v := f.child("faultstring"); v != nil {
			fault.Reason = strings.TrimSpace(v.Text)
		}
		if v := f.child("faultactor"); v != nil {
			fault.Actor = strings.TrimSpace(v.Text)
		}
	}
	if detail := f.child("Detail"); detail != nil {
		fault.Detail = detailText(detail)
	} else if detail := f.child("detail"); detail != nil {
		fault.Detail = detailText(detail)
	}
	return fault
}

// faultCodeLocal strips a fault code QName to its local part
// ("soapenv:Client" → "Client", "s:Sender" → "Sender").
func faultCodeLocal(code string) string {
	if i := strings.LastIndex(code, ":"); i >= 0 {
		return code[i+1:]
	}
	return code
}

// detailText renders a Detail element's children as a compact deterministic
// string ("<Child>text</Child> ..."). The fault detail must be PRESERVED
// (acceptance criterion 3), so it is never dropped — only rendered compactly.
func detailText(detail *xmlNode) string {
	if len(detail.Children) == 0 {
		return strings.TrimSpace(detail.Text)
	}
	var b strings.Builder
	for _, c := range detail.Children {
		b.WriteString("<" + c.Name + ">" + strings.TrimSpace(c.Text) + "</" + c.Name + ">")
	}
	return b.String()
}

// toAppError maps the fault onto pkg/errors AppError codes (the spec's
// fault-mapping table) with the fault code and detail preserved as details.
//
//	Fault code (local part)        AppError code
//	─────────────────────────────────────────────────────────────
//	Client / Sender                CodeBadRequest   (the caller sent it wrong)
//	Server / Receiver              CodeInternal     (the endpoint failed)
//	VersionMismatch                CodeBadRequest   (envelope version refused)
//	DataEncodingUnknown            CodeBadRequest
//	MustUnderstand                 CodeBadRequest   (a required header was not understood)
//	anything else                  CodeGeneric
func (f *Fault) toAppError() *errors.AppError {
	code := errors.CodeGeneric
	switch f.Local {
	case "Client", "Sender", "VersionMismatch", "DataEncodingUnknown", "MustUnderstand":
		code = errors.CodeBadRequest
	case "Server", "Receiver":
		code = errors.CodeInternal
	}
	reason := f.Reason
	if reason == "" {
		reason = "SOAP fault"
	}
	appErr := errors.New(code, "soap: "+reason).
		WithDetail("fault_code", f.Code).
		WithDetail("fault_version", string(f.Version))
	if f.Subcode != "" {
		appErr = appErr.WithDetail("fault_subcode", f.Subcode)
	}
	if f.Actor != "" {
		appErr = appErr.WithDetail("fault_actor", f.Actor)
	}
	if f.Detail != "" {
		appErr = appErr.WithDetail("fault_detail", f.Detail)
	}
	return appErr
}

// String renders the fault for logs and tests.
func (f *Fault) String() string {
	return fmt.Sprintf("SOAP %s fault %s: %s (detail: %s)", f.Version, f.Code, f.Reason, f.Detail)
}
