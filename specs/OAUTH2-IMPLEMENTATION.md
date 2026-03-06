# OAuth2 Implementation Specification

**Status**: ✅ COMPLETE  
**Version**: 1.0.0  
**Last Updated**: 2026-03-01  
**Component**: Authentication

## Overview

Complete specification of **OAuth2 authentication** flow implementation for CLI and daemon modes.

## Architecture

```mermaid
graph TB
    Client["Client<br/>(CLI/Daemon)"]
    AuthServer["Authorization Server"]
    TokenServer["Token Endpoint"]
    Storage["Token Storage<br/>(OS Keychain)"]
    
    Client -->|"Authorization Request"| AuthServer
    AuthServer -->|"Authorization Code"| Client
    Client -->|"Code Exchange"| TokenServer
    TokenServer -->|"Access Token<br/>+ Refresh Token"| Client
    Client -->|"Token Storage"| Storage
    
    TokenServer -->|"Refresh"| Storage
    Storage -->|"Token Retrieval"| Client
```

## Authorization Code Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI as CLI Tool
    participant Auth as Authorization Server
    participant Token as Token Endpoint
    participant Keychain as OS Keychain
    
    User->>CLI: Execute login command
    CLI->>Auth: Request authorization URL
    Auth->>CLI: Return authorization URL
    CLI->>User: Display URL for manual approval
    User->>Auth: Visit URL and approve
    Auth->>CLI: Redirect with auth code
    CLI->>Token: Exchange code for tokens
    Token->>CLI: Return access + refresh tokens
    CLI->>Keychain: Store tokens
    Keychain-->>CLI: Confirmation
    CLI->>User: Login successful
    
    Note over CLI,Keychain: Subsequent requests use stored tokens
    CLI->>Keychain: Retrieve access token
    Keychain-->>CLI: Return token
    CLI->>CLI: Use token for API requests
```

## Device Code Flow

```mermaid
sequenceDiagram
    participant User
    participant CLI as CLI Tool
    participant Device as Device Endpoint
    participant Token as Token Endpoint
    
    User->>CLI: Execute login command
    CLI->>Device: Request device code
    Device->>CLI: Return device code + user code
    CLI->>User: Display user code
    User->>User: Visit activation URL + enter code
    Loop Poll until approved
        CLI->>Token: Poll for token exchange
        alt Authorization pending
            Token->>CLI: authorization_pending
            CLI-->>CLI: Wait interval seconds
        else Access granted
            Token->>CLI: Return access + refresh tokens
        end
    End
```

## 1. OAuth2 Flow Types

### 1.1 Authorization Code Flow

```go
type AuthorizationCodeFlow struct {
    ClientID     string
    ClientSecret string
    AuthURL      string
    TokenURL     string
    RedirectURL  string
    Scopes       []string
    State        string
    PKCE         bool
}

func (f *AuthorizationCodeFlow) Start() (*AuthRequest, error) {
    // Generate state
    state := generateSecureState()
    
    // Build authorization URL
    params := url.Values{
        "client_id":     {f.ClientID},
        "response_type": {"code"},
        "redirect_uri":  {f.RedirectURL},
        "scope":         {strings.Join(f.Scopes, " ")},
        "state":         {state},
    }
    
    // Add PKCE if enabled
    if f.PKCE {
        codeVerifier := generateCodeVerifier()
        params.Set("code_challenge", generateCodeChallenge(codeVerifier))
        params.Set("code_challenge_method", "S256")
        f.codeVerifier = codeVerifier
    }
    
    authURL := f.AuthURL + "?" + params.Encode()
    
    return &AuthRequest{
        URL:   authURL,
        State: state,
    }, nil
}
```

### 1.2 Device Code Flow

```go
type DeviceCodeFlow struct {
    ClientID    string
    DeviceURL   string
    TokenURL    string
    Scopes      []string
}

func (f *DeviceCodeFlow) Start() (*DeviceAuthResponse, error) {
    data := url.Values{
        "client_id": {f.ClientID},
        "scope":     {strings.Join(f.Scopes, " ")},
    }
    
    resp, err := http.PostForm(f.DeviceURL, data)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var result DeviceAuthResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, err
    }
    
    return &result, nil
}

func (f *DeviceCodeFlow) Poll(deviceCode string, interval int) (*TokenResponse, error) {
    for {
        time.Sleep(time.Duration(interval) * time.Second)
        
        data := url.Values{
            "client_id":   {f.ClientID},
            "device_code": {deviceCode},
            "grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
        }
        
        resp, err := http.PostForm(f.TokenURL, data)
        if err != nil {
            return nil, err
        }
        
        var result TokenResponse
        json.NewDecoder(resp.Body).Decode(&result)
        resp.Body.Close()
        
        if result.Error == "" {
            return &result, nil
        }
        
        if result.Error != "authorization_pending" {
            return nil, fmt.Errorf("oauth error: %s", result.Error)
        }
    }
}
```

## 2. Token Management

```go
type TokenManager struct {
    storage TokenStorage
}

type Token struct {
    AccessToken  string    `json:"access_token"`
    RefreshToken string    `json:"refresh_token"`
    TokenType    string    `json:"token_type"`
    ExpiresAt    time.Time `json:"expires_at"`
    Scope        string    `json:"scope"`
}

func (tm *TokenManager) Store(service string, token *Token) error {
    return tm.storage.Save(service, token)
}

func (tm *TokenManager) Get(service string) (*Token, error) {
    token, err := tm.storage.Load(service)
    if err != nil {
        return nil, err
    }
    
    // Refresh if expired
    if time.Now().After(token.ExpiresAt) {
        newToken, err := tm.refresh(token)
        if err != nil {
            return nil, err
        }
        tm.Store(service, newToken)
        return newToken, nil
    }
    
    return token, nil
}

func (tm *TokenManager) refresh(token *Token) (*Token, error) {
    data := url.Values{
        "grant_type":    {"refresh_token"},
        "refresh_token": {token.RefreshToken},
    }
    
    resp, err := http.PostForm(tokenURL, data)
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()
    
    var newToken TokenResponse
    if err := json.NewDecoder(resp.Body).Decode(&newToken); err != nil {
        return nil, err
    }
    
    return &Token{
        AccessToken:  newToken.AccessToken,
        RefreshToken: newToken.RefreshToken,
        ExpiresAt:    time.Now().Add(time.Duration(newToken.ExpiresIn) * time.Second),
    }, nil
}
```

## Token Management Flow

```mermaid
flowchart TD
    Start([API Request]) --> GetToken{Token Stored?}
    GetToken -->|No| AuthRequired[Authentication Required]
    GetToken -->|Yes| CheckExpiry
    
    CheckExpiry{Expired?}
    CheckExpiry -->|No| UseToken[Use Stored Token]
    CheckExpiry -->|Yes| Refresh
    
    Refresh{Refresh Available?}
    Refresh -->|Yes| RefreshToken[Use Refresh Token]
    Refresh -->|No| AuthRequired
    
    RefreshToken --> NewToken{Success?}
    NewToken -->|Yes| StoreNew[Store New Token]
    NewToken -->|No| AuthRequired
    
    StoreNew --> UseToken
    AuthRequired --> Error[Return Auth Error]
    UseToken --> Exec[Execute API Request]
    Exec --> Success([Return Response])
    
    Error --> End([End])
    Success --> End
```

## 3. Local Callback Server

```go
type CallbackServer struct {
    port     int
    state    string
    codeChan chan string
    errChan  chan error
}

func (s *CallbackServer) Start() string {
    mux := http.NewServeMux()
    mux.HandleFunc("/callback", s.handleCallback)
    
    addr := fmt.Sprintf("localhost:%d", s.port)
    go http.ListenAndServe(addr, mux)
    
    return fmt.Sprintf("http://%s/callback", addr)
}

func (s *CallbackServer) handleCallback(w http.ResponseWriter, r *http.Request) {
    code := r.URL.Query().Get("code")
    state := r.URL.Query().Get("state")
    
    if state != s.state {
        s.errChan <- errors.New("invalid state")
        http.Error(w, "Invalid state", 400)
        return
    }
    
    s.codeChan <- code
    fmt.Fprintln(w, "Authorization successful. You can close this window.")
}
```

---

**Bible Standard Compliance**: ✅ DETERMINISTIC
