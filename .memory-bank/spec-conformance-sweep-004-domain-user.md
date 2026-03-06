## Spec Conformance Sweep #004: DOMAIN-USER.md (2026-03-05)

**Status**: ⚠️ OUT OF SCOPE (multi-tenant user management not required)

**Spec Location**: `specs/DOMAIN-USER.md`

**Analysis**: Not applicable to this project (0% conformance expected)

**Context Analysis**:

**Project Scope**: OpenAPI CLI + MCP Server (client-side tool for API interaction)
**Spec Scope**: Multi-tenant SaaS user management system with sessions, user preferences, UI settings

**Domain Mismatch**: This spec describes features for a multi-tenant web application:
- User session management with expiration
- Permission-based access control
- User preferences (themes, notifications, editors)
- Display settings, UI customization
- Authentication state tracking

**What the Spec Describes**:

| # | Entity | Purpose | Applicability |
|---|---------|---------|---------------|
| 1 | UserSession | User session management | ❌ CLI client doesn't need user sessions |
| 2 | SessionType | Interactive/API/Service sessions | ❌ Single-user CLI with no user database |
| 3 | SessionState | Session state tracking | ❌ No session state in CLI tool |
| 4 | SessionMetadata | IP, UserAgent, Device, Geo | ❌ CLI runs on user's machine |
| 5 | Permission | RBAC for multi-tenant | ❌ No multi-tenant access control needed |
| 6 | Condition | Condition-based access | ❌ No user-based access control |
| 7 | AuthenticationState | Auth state tracking | ❌ Uses OS keychain, not session auth |
| 8 | UserPreferences | Display, notifications, editor | ❌ CLI tool has no UI preferences |
| 9 | DisplayPreferences | Theme, language, timezone | ❌ No configurable UI |
| 10 | NotificationPreferences | Email, in-app, push | ❌ CLI doesn't send notifications |
| 11 | EditorPreferences | Auto-save, font, tab size | ❌ No built-in editor |
| 12 | PrivacyPreferences | Profile settings | ❌ No user profiles |
| 13 | Validation methods | Session/User validation | ❌ Not applicable |

**Project Architecture**:

**What This Project Actually Has**:
- OpenAPI spec parsing and command generation
- CLI interface for API interaction
- MCP server for AI assistant integration
- WASM plugin system for extensibility
- Authentication via OAuth2/mTLS (for external APIs)
- OS keychain for credential storage
- Config file for settings
- Profile switching (not user profiles)

**What This Spec Describes**:
- Multi-tenant user database
- User session management
- Permission-based access control
- UI preferences and settings
- Notification system
- User profiles
- Authentication state tracking

**Conclusion**: This spec appears to be from a different project or a future multi-tenant version. It does not apply to the current OpenAPI CLI client tool.

**Recommendation**:
- ✅ **IGNORE this spec** - it's out of scope for current project
- This spec describes a multi-tenant SaaS application
- Current project is a CLI client tool, not a SaaS platform
- No action required for this spec

**Evidence**:
- `grep` shows all types exist only in spec file
- No pkg/session/, pkg/user/, pkg/preference/ packages
- No Go files implement UserSession, AuthenticationState, UserPreferences
- Project description: "Dynamic API client & MCP server" (not multi-tenant SaaS)

**Priority**: N/A (Out of scope)
