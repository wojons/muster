# Daemon Socket Authentication Token Design

## Overview

This document outlines the token generation scheme for authenticating Unix socket connections to the Muster daemon.

## Threat Model

- **Threat**: Unauthorized local processes connecting to the Unix socket
- **Risk**: Data exposure, unauthorized command execution
- **Mitigation**: Token-based authentication with restricted file permissions

## Design Decisions

### 1. Token Format
- **Type**: Cryptographically random byte sequence
- **Length**: 32 bytes (256 bits)
- **Encoding**: Base64 URL encoding (URL-safe, no padding issues)
- **Result**: 43-character string (e.g., `xJ9sL2kP...`)

### 2. Token Generation
- **Source**: `crypto/rand` (CSPRNG)
- **Timing**: Generated on daemon startup
- **Uniqueness**: Guaranteed by randomness (2^256 combinations)
- **Collision resistance**: Negligible probability

### 3. Token Storage
- **Location**: `~/.muster/daemon/token` (user-specific)
- **Permissions**: 0600 (owner read/write only)
- **Format**: Single line base64-encoded token
- **Cleanup**: Deleted on daemon shutdown

### 4. Token Lifecycle
1. **Generation**: Daemon generates token on startup
2. **Distribution**: Written to token file (0600)
3. **Validation**: Client reads token from file, sends in Authorization header
4. **Expiration**: Token valid for daemon lifetime (regenerated on restart)
5. **Cleanup**: Token file removed on graceful shutdown

### 5. Authentication Flow

```
┌──────────┐                    ┌──────────┐
│  Client  │                    │  Daemon  │
└────┬─────┘                    └────┬─────┘
     │                               │
     │  1. Read token from file      │
     │     (permission check: 0600)  │
     │◄──────────────────────────────┤
     │                               │
     │  2. Connect to Unix socket    │
     ├──────────────────────────────►│
     │                               │
     │  3. Send HTTP request with    │
     │     Authorization: Bearer <token>
     ├──────────────────────────────►│
     │                               │
     │  4. Validate token            │
     │     (constant-time compare)   │
     │──────────────────────────────►│
     │                               │
     │  5. Return response           │
     │◄──────────────────────────────┤
     │                               │
```

### 6. Security Properties
- **Confidentiality**: Token file 0600 permissions ensure only owner can read
- **Integrity**: File permissions prevent modification by other users
- **Authentication**: Token proves client has access to the user's token file
- **Replay resistance**: Token is unique per daemon instance

### 7. Implementation

#### TokenManager Responsibilities
- Generate secure random tokens
- Write tokens to file with 0600 permissions
- Read and validate tokens from HTTP headers
- Clean up token files on shutdown

#### Token Validation
- Extract `Authorization: Bearer <token>` header
- Constant-time comparison to prevent timing attacks
- Reject requests with missing/invalid tokens (401 Unauthorized)
- Log failed authentication attempts (security audit)

### 8. Error Handling
- **Token file missing**: Client cannot connect (expected if daemon not running)
- **Permission denied**: Client lacks access to token file (security boundary)
- **Invalid token**: 401 Unauthorized response with logging
- **Token regeneration**: Old tokens invalidated on daemon restart

### 9. Future Enhancements (Out of Scope)
- Token expiration/rotation during daemon lifetime
- Token revocation list
- Multiple token support (scoped access)
- mTLS alternative for high-security environments

## References
- spec: specs/UNIX-SOCKET-PROXY.md
- spec: specs/DAEMON-LIFECYCLE.md
- spec: specs/EVENTS-AUDIT.md
