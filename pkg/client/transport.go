// Copyright 2025 The Muster Authors
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"

	"github.com/wojons/muster/pkg/errors"
)

// DefaultTransport returns the base transport a client installs when no
// transport option is given: a clone of http.DefaultTransport with HTTP/2
// explicitly enabled.
//
// Two properties matter. ForceAttemptHTTP2 stops being an accident — a
// hand-built &http.Transport{} silently drops to HTTP/1.1 against a server that
// offers h2 (measured in PROTO-010 §2.1 / PROTO-012 M2), so the first such
// transport anyone adds would silently remove HTTP/2 from every request. And
// the value is a real transport rather than the nil-transport fallback, so the
// option pipeline composes against a known base (PROTOCOLS.md §4.7 N2).
//
// Consequences worth knowing: mutating the process-global http.DefaultTransport
// no longer changes a client built afterwards (the clone is taken at
// construction), and callers that need different TLS or dialler settings use
// WithTransport with their own transport.
func DefaultTransport() *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok || base == nil {
		// A non-*http.Transport global (a test or embedding application can
		// install any RoundTripper): build a plain HTTP/2-capable transport
		// instead of guessing what was there.
		return &http.Transport{ForceAttemptHTTP2: true}
	}
	t := base.Clone()
	t.ForceAttemptHTTP2 = true
	return t
}

// HTTP11Transport returns a base transport that speaks HTTP/1.1 only — the
// transport that realises the "1.1" policy, because version selection IS
// transport selection (PROTOCOLS.md §4.7 N6).
//
// HTTP/2 is disabled on both routes the stdlib can take: the TLS handshake
// stops offering "h2" in ALPN, and a non-nil (empty) TLSNextProto map disables
// the upgrade path. Measured behaviour of the equivalent hand-built transport
// (PROTO-010 C2 / PROTO-012 M2): HTTP/1.1 on both sides against a server that
// offers h2.
func HTTP11Transport() *http.Transport {
	t := DefaultTransport()
	t.ForceAttemptHTTP2 = false
	t.TLSNextProto = make(map[string]func(string, *tls.Conn) http.RoundTripper)
	if t.TLSClientConfig == nil {
		t.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	} else {
		t.TLSClientConfig = t.TLSClientConfig.Clone()
	}
	t.TLSClientConfig.NextProtos = []string{"http/1.1"}
	return t
}

// WithHTTPVersion installs the transport that realises an HTTP transport
// version policy and records the policy on the client's ProtocolRecorder
// (PROTOCOLS.md §4.7 N2/N4):
//
//	HTTPVersionAuto (default) — negotiate opportunistically (HTTP/2 over TLS)
//	HTTPVersion11            — require HTTP/1.1; HTTP/2 is turned off
//	HTTPVersion2             — require HTTP/2; a peer that offers only HTTP/1.1
//	                           is reported as CodeProtocolMismatch after the
//	                           exchange (auto never fails)
//	HTTPVersion3             — requires an HTTP/3 transport (PROTO-015); without
//	                           one the request fails loudly rather than using h2
//
// Apply it BEFORE WithRateLimiter/WithCache: those options wrap whatever
// transport exists when they run, so a later WithHTTPVersion discards them.
// Do not change the version through Client.SetHTTPClient, which replaces the
// whole *http.Client and discards the option-installed chain (defect DF-042).
//
// An empty version means the default, "auto". An unrecognised version is
// reported as CodeValidation when the request runs — an option cannot return
// an error, and degrading to "auto" would be the silent downgrade N5 forbids.
func WithHTTPVersion(version string) ClientOption {
	return func(c *Client) {
		c.httpVersion = version
		if c.protocol != nil {
			c.protocol.SetPolicy(version)
		}
		switch version {
		case HTTPVersion11:
			c.installTransport(HTTP11Transport())
		case HTTPVersion3:
			// PROTO-015 installs an HTTP/3 transport (WithTransport) and takes
			// this branch over; until then protocolPreflight reports the
			// missing transport instead of quietly using HTTP/2.
		default:
			c.installTransport(DefaultTransport())
		}
	}
}

// WithTransport installs rt as the base transport — the value that performs the
// exchange (PROTOCOLS.md §4.7 N2). The client's ProtocolRecorder wraps it, so
// every exchange that reaches the network is recorded, below every wrapper
// option.
//
// Apply it BEFORE WithRateLimiter/WithCache for the same reason as
// WithHTTPVersion. A nil rt is a no-op and keeps the default transport.
func WithTransport(rt http.RoundTripper) ClientOption {
	return func(c *Client) {
		if rt == nil {
			return
		}
		c.installTransport(rt)
	}
}

// installTransport makes rt the exchange-performing transport and re-wraps it
// with the protocol recorder, so the recorder is always directly above whatever
// performs the exchange.
func (c *Client) installTransport(rt http.RoundTripper) {
	c.baseTransport = rt
	c.transportExplicit = true
	if c.httpClient != nil {
		c.httpClient.Transport = newProtocolTransport(rt, c.protocol)
	}
}

// NormalizeHTTPVersion validates a version policy value and returns its
// canonical form (PROTOCOLS.md §4.7 N4). An empty value means the default,
// "auto". Anything else is a CodeValidation AppError naming the accepted set.
func NormalizeHTTPVersion(version string) (string, error) {
	norm := strings.ToLower(strings.TrimSpace(version))
	switch norm {
	case "":
		return HTTPVersionAuto, nil
	case HTTPVersionAuto, HTTPVersion11, HTTPVersion2, HTTPVersion3:
		return norm, nil
	default:
		return "", errors.New(errors.CodeValidation,
			fmt.Sprintf("invalid http version %q: want auto, 1.1, 2 or 3", version)).
			WithDetail("value", version)
	}
}
