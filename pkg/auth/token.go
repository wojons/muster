package auth

import "time"

// Token represents an OAuth2 token.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	TokenType    string    `json:"token_type"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// IsExpired returns true if the token is expired or will expire within 60 seconds.
func (t *Token) IsExpired() bool {
	return time.Now().Add(60 * time.Second).After(t.ExpiresAt)
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
