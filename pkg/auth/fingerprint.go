package auth

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Fingerprint struct {
	IP        string
	UserAgent string
	Headers   map[string]string
	TLSInfo   *TLSFingerprint
	Timestamp time.Time
	Hash      string
}

type TLSFingerprint struct {
	JA3Hash      string
	JA4Hash      string
	CipherSuites []uint16
	Extensions   []uint16
}

func CalculateJA3(tlsState *tls.ConnectionState) string {
	if tlsState == nil {
		return ""
	}

	version := ""
	switch tlsState.Version {
	case tls.VersionTLS10:
		version = "769"
	case tls.VersionTLS11:
		version = "770"
	case tls.VersionTLS12:
		version = "771"
	case tls.VersionTLS13:
		version = "772"
	default:
		version = strconv.Itoa(int(tlsState.Version))
	}

	ja3 := version
	ja3 += ","

	ja3 += ""
	ja3 += ","

	ja3 += ""
	ja3 += ","

	ja3 += ""
	ja3 += ","

	ja3 += ""

	h := sha256.Sum256([]byte(ja3))
	return hex.EncodeToString(h[:])
}

func FingerprintRequest(r *http.Request) Fingerprint {
	if r == nil {
		return Fingerprint{}
	}

	fp := Fingerprint{
		IP:        getClientIP(r),
		UserAgent: r.UserAgent(),
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
	}

	relevantHeaders := []string{
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
	}

	for _, h := range relevantHeaders {
		if val := r.Header.Get(h); val != "" {
			fp.Headers[h] = val
		}
	}

	if r.TLS != nil {
		fp.TLSInfo = &TLSFingerprint{
			JA3Hash: CalculateJA3(r.TLS),
		}
	}

	fp.Hash = fp.calculateHash()
	return fp
}

func getClientIP(r *http.Request) string {
	if r == nil {
		return ""
	}

	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return strings.Split(ip, ",")[0]
	}

	if ip := r.Header.Get("X-Real-Ip"); ip != "" {
		return ip
	}

	return r.RemoteAddr
}

func (fp *Fingerprint) calculateHash() string {
	h := sha256.New()
	h.Write([]byte(fp.IP))
	h.Write([]byte(fp.UserAgent))

	for k, v := range fp.Headers {
		h.Write([]byte(k))
		h.Write([]byte(v))
	}

	if fp.TLSInfo != nil {
		h.Write([]byte(fp.TLSInfo.JA3Hash))
	}

	return hex.EncodeToString(h.Sum(nil))
}

func (fp1 *Fingerprint) Similarity(fp2 *Fingerprint) float64 {
	if fp1.Hash == fp2.Hash {
		return 1.0
	}

	score := 0.0

	if fp1.IP == fp2.IP {
		score += 0.3
	}

	if fp1.UserAgent == fp2.UserAgent {
		score += 0.3
	}

	headerMatches := 0
	for k, v1 := range fp1.Headers {
		if v2, ok := fp2.Headers[k]; ok && v1 == v2 {
			headerMatches++
		}
	}
	if len(fp1.Headers) > 0 {
		score += 0.2 * float64(headerMatches) / float64(len(fp1.Headers))
	}

	if fp1.TLSInfo != nil && fp2.TLSInfo != nil {
		if fp1.TLSInfo.JA3Hash == fp2.TLSInfo.JA3Hash {
			score += 0.2
		}
	}

	return score
}

func CalculateFingerprintHash(ip, userAgent string, headers map[string]string) string {
	h := sha256.New()
	h.Write([]byte(ip))
	h.Write([]byte(userAgent))

	for k, v := range headers {
		h.Write([]byte(k))
		h.Write([]byte(v))
	}

	return hex.EncodeToString(h.Sum(nil))
}

func NewFingerprint(ip, userAgent string, headers map[string]string) *Fingerprint {
	fp := &Fingerprint{
		IP:        ip,
		UserAgent: userAgent,
		Headers:   make(map[string]string),
		Timestamp: time.Now(),
	}

	for k, v := range headers {
		fp.Headers[k] = v
	}

	fp.Hash = fp.calculateHash()
	return fp
}

func (fp *Fingerprint) String() string {
	return fmt.Sprintf("Fingerprint{IP: %s, Hash: %s}", fp.IP, fp.Hash)
}

func (tlsInfo *TLSFingerprint) String() string {
	return fmt.Sprintf("TLSFingerprint{JA3Hash: %s}", tlsInfo.JA3Hash)
}
