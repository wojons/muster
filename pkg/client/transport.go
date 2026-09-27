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
//	HTTPVersion3             — prefer HTTP/3, fall back to HTTP/2 with the
//	                           fallback always reported
//
// Policy "3" installs an HTTP/3 transport (HTTP3FallbackTransport): the
// request is attempted over QUIC and, when the peer cannot be reached that
// way, retried over HTTP/2 — the record then carries
// fallback{from,to,reason} and Satisfied is false, so the caller can always
// see that the preference was not met. The result is never a silent HTTP/2
// request. A caller that wants HTTP/3 with no fallback installs
// HTTP3Transport() with WithTransport; a caller that installs a transport
// which cannot attempt HTTP/3 at all gets a loud failure rather than a quiet
// downgrade.
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
			// The opt-in HTTP/3 transport, plus the HTTP/2 fallback its
			// policy promises (N4/N5). HTTP3Options let a caller tune the
			// QUIC parameters; WithHTTP3Options re-installs it when they
			// arrive after this option.
			c.installTransport(HTTP3FallbackTransport(c.protocol, c.http3Options...))
		default:
			c.installTransport(DefaultTransport())
		}
	}
}

// WithHTTP3Options tunes the HTTP/3 transport the "3" policy installs
// (PROTOCOLS.md §4.7 N4): a TLS configuration for a private CA or a client
// certificate, or QUIC parameters such as a shorter handshake timeout on a
// path known to drop UDP. The options are the same ones HTTP3Transport and
// HTTP3FallbackTransport take, and they are recorded on the client, so they
// can be given before or after WithHTTPVersion("3") — the policy's transport
// is (re-)installed with them either way.
//
// It is a no-op for every other policy, which installs a transport that cannot
// attempt HTTP/3.
func WithHTTP3Options(opts ...HTTP3Option) ClientOption {
	return func(c *Client) {
		if len(opts) == 0 {
			return
		}
		c.http3Options = append(c.http3Options, opts...)
		if c.httpVersion == HTTPVersion3 {
			c.installTransport(HTTP3FallbackTransport(c.protocol, c.http3Options...))
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
//
// Under the "3" policy the installed transport has to be able to attempt
// HTTP/3 (an HTTP3FallbackTransport, an http3.Transport, or anything
// implementing HTTP3Capable); replacing policy "3"'s transport with one that
// cannot is refused before the exchange, because carrying an HTTP/3 request
// over another version with nothing to report is exactly the silent downgrade
// the policy forbids.
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
// performs the exchange. The transport's HTTP/3 capability is recorded here
// because it is a property of the value that was installed (PROTOCOLS.md §4.7
// N5). Any wrapper options that ran earlier are dropped by design — transport
// options must precede wrapper options (§4.7 N2) — so the chain tops at the
// recorder link afterwards.
func (c *Client) installTransport(rt http.RoundTripper) {
	c.baseTransport = rt
	c.h3Capable = SupportsHTTP3(rt)
	if c.httpClient != nil {
		c.httpClient.Transport = newProtocolTransport(rt, c.protocol)
		c.chain = c.httpClient.Transport
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
