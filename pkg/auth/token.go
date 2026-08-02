package auth

import "time"

// Token represents an OAuth2 token.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired reports whether the token should be considered expired,
// applying a 60-second grace period: a token is treated as expired when
// its remaining lifetime is at or below the grace window. The comparison
// is `!Before` (>=) rather than `After` (>) so the exact boundary is
// deterministic regardless of OS clock granularity — with strict `>`,
// two time.Now() calls landing in the same coarse clock tick (Windows
// ~15.6ms) would make now+60s == expiresAt and report a live token.
func (t *Token) IsExpired() bool {
	return !time.Now().Add(60 * time.Second).Before(t.ExpiresAt)
}

// TokenStorage is the interface for token persistence.
type TokenStorage interface {
	Save(service string, token *Token) error
	Load(service string) (*Token, error)
}

// TokenResponse is returned by an OAuth2 token endpoint.
type TokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

// AuthRequest contains the authorization request data.
type AuthRequest struct {
	URL       string
	State     string
	RequestID string
}
