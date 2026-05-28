# opencode-axiom

Axiom platform extension plugin for [OpenCode](https://opencode.ai). Adds enterprise observability, progressive onboarding, session analytics, security controls, and backend management to your OpenCode sessions.

## Quick Start

### Install

```bash
npm install opencode-axiom
```

### Register in opencode.json

Add the plugin to your project's `opencode.json` (or `opencode.jsonc`):

```json
{
  "$schema": "https://opencode.ai/config.json",
  "plugin": ["opencode-axiom/plugin"]
}
```

That's it. The plugin loads automatically on the next OpenCode session.

> **Why `/plugin` and not `opencode-axiom` directly?**
> OpenCode's plugin loader iterates all module exports and calls each as a function. The main
> `opencode-axiom` entrypoint exports the full library API (~66 symbols including objects,
> numbers, and strings), which causes a `"Plugin export is not a function"` error. The
> `opencode-axiom/plugin` subpath exports only `CodeOpsPlugin` — safe for the loader.
> Use `opencode-axiom` (bare) only for programmatic TypeScript/JavaScript imports.

### Optional: Create a config file

For advanced features (automation hooks, network awareness, Langfuse), create `.axiom/plugin.yaml` in your repo root. Without it, the plugin runs in zero-config mode with local cost tracking and security controls active.

```bash
# Copy the annotated example
cp node_modules/opencode-axiom/plugin.yaml.example .axiom/plugin.yaml
# Then edit to taste
```

---

## Migrating from bare `opencode-axiom` to `opencode-axiom/plugin`

If you previously configured the plugin as:

```json
{ "plugin": ["opencode-axiom"] }
```

Update it to:

```json
{ "plugin": ["opencode-axiom/plugin"] }
```

**Why**: The bare `opencode-axiom` entrypoint is the library API — it exports ~66 symbols
including objects, numbers, and strings. OpenCode's plugin loader throws
`"Plugin export is not a function"` for any non-function export. The `/plugin` subpath
exports only `CodeOpsPlugin` (a function), which is safe for all loader paths.

This change was introduced in v0.40.0. The bare entrypoint path was never documented as
working for plugin loading — it was always broken for `getLegacyPlugins`. If you see
`"Plugin export is not a function"` in your OpenCode logs, this is the fix.

---

## How it works

The plugin is a standard OpenCode plugin — a TypeScript module that hooks into OpenCode's event system. It exports `CodeOpsPlugin` which OpenCode loads at startup.

The plugin reads `.axiom/plugin.yaml` (if present) to configure advanced features. Without the file, it runs with safe defaults: local cost tracking on, automation off, network off.

---

## Maturity Levels

The plugin detects five maturity levels based on your repository structure:

| Level | Name | Signals | Unlocked Commands |
|-------|------|---------|-------------------|
| 0 | New | No Axiom structure | `/axiom-setup`, `/axiom-help`, `/axiom-status`, `/axiom-next`, `/axiom-glossary` |
| 1 | Scaffolded | `.opencode/` exists | + `/axiom-spec` |
| 2 | Specced | `specs/` has content | + `/axiom-plan` |
| 3 | Active | specs + plans + `.memory-bank/` | + `/axiom-run`, `/axiom-verify`, `/axiom-loop`, `/axiom-share`, `/axiom-pulse`, `/axiom-spinup`, `/axiom-backend` |
| 4 | Mature | Evidence bundles + CI integration | + `/axiom-release` |

Commands not yet available for your maturity level are hidden. If you try to invoke a command above your level, the plugin responds conversationally with guidance on what to do next.

---

## Commands

### Level 0 (Available Immediately)

| Command | Description |
|---------|-------------|
| `/axiom-setup` | Bootstrap Axiom in this repository |
| `/axiom-help` | Show available commands and quick-start guide |
| `/axiom-status` | Show maturity level, active work items, and open gaps |
| `/axiom-next` | Show the recommended next action to advance maturity |
| `/axiom-glossary` | Explain Axiom concepts at your current level |

### Level 1+

| Command | Min Level | Description |
|---------|-----------|-------------|
| `/axiom-spec` | 1 | Create or update a spec |
| `/axiom-plan` | 2 | Generate or update an implementation plan |
| `/axiom-run` | 3 | Execute a work item plan step |
| `/axiom-verify` | 3 | Run verification for a work item |
| `/axiom-loop` | 3 | Start an automated Ralph loop |
| `/axiom-share` | 3 | Share a session replay |
| `/axiom-pulse` | 3 | Show team activity pulse |
| `/axiom-spinup` | 3 | Spin up a backend for this repository |
| `/axiom-backend` | 3 | Show/switch available backends |
| `/axiom-release` | 4 | Prepare and publish a release |

### Built-in Tools

These tools are registered in the agent's palette automatically:

| Tool | Description |
|------|-------------|
| `codeops_detect` | Returns maturity level, detection signals, and repo metadata as JSON |
| `codeops_backend` | Show available backends and switch between Python/Go execution modes |
| `codeops_privacy` | Show what data the plugin collects and how to delete it |

---

## Configuration

The plugin reads `.axiom/plugin.yaml` from your repo root. This file is **Axiom-specific** — it is not part of `opencode.json`. It configures the plugin's own features: automation hooks, metrics sinks, network awareness, and Langfuse.

### Minimal (enables automation)

```yaml
version: 1

layers:
  automation: true
  tools: true
```

### Full reference

```yaml
version: 1

# Layer toggles — what's active
layers:
  metrics: true           # L2 — local cost tracking (default: on)
  session_sharing: false  # L3 — session replay/broadcast
  network: false          # L4 — team pulse + heartbeat
  automation: true        # L5 — hooks, watches, scripts
  tools: true             # L6 — custom tool registration

# Metrics (L2)
metrics:
  session_budget_usd: 5.00
  warning_threshold_pct: 80
  persist_sessions: true
  publish_enabled: true
  stream_interval_seconds: 30
  reconciliation_interval_seconds: 1800
  flush_interval_seconds: 600
  max_buffered_intervals: 12
  user_email: null          # null = auto-resolve via git config
  sinks: []                 # "axiom-backend", "prometheus", "cloudwatch"

  # Prometheus push gateway (add "prometheus" to sinks)
  prometheus:
    push_gateway_url: "http://localhost:9091"
    job_name: "codeops_plugin"
    namespace: "axiom"

  # AWS CloudWatch (add "cloudwatch" to sinks)
  cloudwatch:
    namespace: "Axiom/Plugin"
    region: null
    log_group: "/axiom/metrics"

  # Override embedded pricing (USD per 1M tokens)
  pricing_overrides:
    "claude-sonnet-4-20250514":
      input_per_1m: 3.00
      output_per_1m: 15.00
      cache_read_per_1m: 0.30
      cache_write_per_1m: 3.75
      reasoning_per_1m: 15.00

# Network (L4) — requires AXIOM_API_TOKEN env var
network:
  backend_url: "https://axiom.example.com"
  team_id: "my-team"
  heartbeat_interval_seconds: 60
  user_identifier: "email"   # "email", "username", or "anonymous"
  visibility: "public"       # "public", "team_only", or "hidden"

# Langfuse LLM observability
# Requires experimental.openTelemetry: true in opencode.json
# Requires LANGFUSE_PUBLIC_KEY + LANGFUSE_SECRET_KEY + LANGFUSE_BASEURL
#        OR network.backend_url (proxy mode)
langfuse:
  enabled: true
  proxy_mode: true
  cache_ttl_seconds: 3600

# Hooks — run on OpenCode lifecycle events
# Action types: run (shell command), cmd (alias), prompt (inject into session)
hooks:
  on_session_start:
    - name: "load-context"
      run: "cat .axiom/context.md 2>/dev/null || true"
  on_session_idle:
    - name: "keep-alive"
      prompt: "Check .memory-bank/TODO.md for the next unchecked item and continue"
      condition: "maturity >= 3"
  on_session_end:
    - name: "save-notes"
      run: "echo 'Session ended' >> .axiom/session-log.txt"

# Watches — react to tool events
watches:
  - name: "env-file-guard"
    trigger: "tool.read"
    pattern: ".env*"
    action: "block"
    message: "Reading .env files is blocked by security policy"
  - name: "spec-change-reminder"
    trigger: "tool.write"
    pattern: "specs/*.md"
    run: "echo 'Spec changed — remember to update tests'"

# Scripts — invocable via codeops_run_script tool
scripts:
  test:
    run: "npm test 2>&1 | tail -20"
    description: "Run project tests"
  deploy-staging:
    run: "axiom deploy --env staging"
    description: "Deploy to staging"
    requires_confirmation: true

# Custom tools — added to the agent's palette
tools:
  - name: "project_test"
    description: "Run project tests and return results"
    run: "npm test 2>&1 | tail -50"

# Idle timeout — ms before on_session_idle fires (default: 60000)
idle_timeout_ms: 60000

# Profile-based config scoping (AXIOM_PROFILE env var)
profiles:
  ci:
    hooks:
      on_session_start:
        - name: "ci-mode"
          run: "echo 'CI mode active'"
  local:
    hooks:
      on_session_idle:
        - name: "keep-alive"
          prompt: "Check TODO.md for the next unchecked item and continue"
          condition: "maturity >= 3"
```

---

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `AXIOM_API_TOKEN` | For L4/backend sink | Bearer token for Axiom backend API |
| `LANGFUSE_PUBLIC_KEY` | For Langfuse direct mode | Langfuse project public key |
| `LANGFUSE_SECRET_KEY` | For Langfuse direct mode | Langfuse project secret key |
| `LANGFUSE_BASEURL` | For Langfuse direct mode | Langfuse instance URL (default: `https://cloud.langfuse.com`) |
| `LANGFUSE_ENVIRONMENT` | Optional | Langfuse environment tag (default: `development`) |
| `AXIOM_USER_EMAIL` | Optional | Override email identity resolution |
| `AXIOM_PROFILE` | Optional | Active config profile (default: `default`) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | For Langfuse auto-detect | Set by OpenCode when `experimental.openTelemetry: true` |

---

## Metrics Sinks

The plugin supports four off-system metrics sinks. **Stdout is always active** as a fallback.

### Stdout (always on)

No configuration needed. Every `plugin.metrics.snapshot` and `plugin.session.ended` event is written as structured JSON to stdout.

### Axiom Backend

```yaml
# .axiom/plugin.yaml
metrics:
  sinks: ["axiom-backend"]
network:
  backend_url: "https://axiom.example.com"
```
```bash
export AXIOM_API_TOKEN="your-api-token"
```

### Prometheus Push Gateway

```yaml
metrics:
  sinks: ["prometheus"]
  prometheus:
    push_gateway_url: "http://localhost:9091"
    job_name: "codeops_plugin"
    namespace: "axiom"
```

### AWS CloudWatch

```yaml
metrics:
  sinks: ["cloudwatch"]
  cloudwatch:
    namespace: "Axiom/Plugin"
    region: "us-east-1"
    log_group: "/axiom/metrics"
```

Requires `@aws-sdk/client-cloudwatch` and `@aws-sdk/client-cloudwatch-logs`.

---

## Langfuse LLM Observability

Sends LLM traces to [Langfuse](https://langfuse.com) with Axiom metadata and secret redaction.

### Setup (direct mode)

1. Add `experimental.openTelemetry: true` to `opencode.json`
2. Set env vars:
   ```bash
   export LANGFUSE_PUBLIC_KEY="pk-lf-..."
   export LANGFUSE_SECRET_KEY="sk-lf-..."
   export LANGFUSE_BASEURL="https://cloud.langfuse.com"
   ```
3. Enable in `.axiom/plugin.yaml`:
   ```yaml
   langfuse:
     enabled: true
     proxy_mode: false
   ```

### Local testing with docker-compose

```bash
cd backend && docker compose up -d
# Wait ~30s, then verify:
curl http://localhost:3100/api/public/health
# Login: admin@axiom.local / Admin123!@#
# API keys: pk-lf-axiom-dev / sk-lf-axiom-dev
```

---

## Security

### Blocked File Patterns

The plugin blocks reads of sensitive files via `tool.execute.before`:
- `.env`, `.env.*`
- `credentials*`, `*credential*`
- `*secret*`, `*token*`, `*key*`
- `*.pem`, `*.p12`, `*.pfx`

Add custom patterns via `watches` in `.axiom/plugin.yaml`.

### Content Redaction

Shared content and Langfuse traces are redacted for: AWS keys, JWTs, API keys, Bearer tokens, GitHub tokens, connection strings, private key blocks.

Run `/axiom-privacy` to see what data is collected and how to delete it.

---

## Session Analytics

- Per-model token usage (input, output, cache_read, cache_write, reasoning)
- Cost computation using embedded pricing tables (overridable)
- Live pricing fetched from [models.dev](https://models.dev) in background, cached 24h
- Session state saved to `.axiom/sessions/<session-id>.json` (atomic write, auto-gitignored)

---

## Development

```bash
cd .axiom/plugin
bun install
bun test
bun run typecheck
bun run build
```

---

## Peer Dependencies

- `@opencode-ai/plugin` >= 1.2.0

## Optional Dependencies

- `@langfuse/otel` — Langfuse LLM observability
- `@opentelemetry/sdk-node` — required for Langfuse
- `@aws-sdk/client-cloudwatch` — CloudWatch sink
- `@aws-sdk/client-cloudwatch-logs` — CloudWatch sink

## License

MIT

### Use

Open any repository in OpenCode. The plugin automatically detects your Axiom maturity level and progressively unlocks commands as your project matures.

Start with `/axiom-setup` to bootstrap a new repository, or `/axiom-status` to see where you are.

---

## Maturity Levels

The plugin detects five maturity levels based on your repository structure:

| Level | Name | Signals | Unlocked Commands |
|-------|------|---------|-------------------|
| 0 | New | No Axiom structure | `/axiom-setup`, `/axiom-help`, `/axiom-status`, `/axiom-next`, `/axiom-glossary` |
| 1 | Scaffolded | `.opencode/` exists | + `/axiom-spec` |
| 2 | Specced | `specs/` has content | + `/axiom-plan` |
| 3 | Active | specs + plans + `.memory-bank/` | + `/axiom-run`, `/axiom-verify`, `/axiom-loop`, `/axiom-share`, `/axiom-pulse`, `/axiom-spinup`, `/axiom-backend` |
| 4 | Mature | Evidence bundles + CI integration | + `/axiom-release` |

Commands not yet available for your maturity level are hidden. If you try to invoke a command above your level, the plugin returns conversational guidance on what to do next.

---

## Commands

### Level 0 (Available Immediately)

| Command | Description |
|---------|-------------|
| `/axiom-setup` | Bootstrap Axiom in this repository |
| `/axiom-help` | Show available commands and quick-start guide |
| `/axiom-status` | Show maturity level, active work items, and open gaps |
| `/axiom-next` | Show the recommended next action to advance maturity |
| `/axiom-glossary` | Explain Axiom concepts at your current level |

### Level 1+

| Command | Min Level | Description |
|---------|-----------|-------------|
| `/axiom-spec` | 1 | Create or update a spec |
| `/axiom-plan` | 2 | Generate or update an implementation plan |
| `/axiom-run` | 3 | Execute a work item plan step |
| `/axiom-verify` | 3 | Run verification for a work item |
| `/axiom-loop` | 3 | Start an automated Ralph loop |
| `/axiom-share` | 3 | Share a session replay |
| `/axiom-pulse` | 3 | Show team activity pulse |
| `/axiom-spinup` | 3 | Spin up a backend for this repository |
| `/axiom-backend` | 3 | Show/switch available backends |
| `/axiom-release` | 4 | Prepare and publish a release |

### Built-in Tools

| Tool | Description |
|------|-------------|
| `codeops_detect` | Returns maturity level, detection signals, and repo metadata as JSON |
| `codeops_backend` | Show available backends and switch between Python/Go execution modes |
| `codeops_privacy` | Show what data the plugin collects and how to delete it |

### /axiom-dashboard

Returns a one-time snapshot of all active Axiom sessions. For live refresh, use the standalone CLI:

```bash
axiom-dashboard --watch  # Auto-refresh every 5 seconds
```

**Note**: The plugin tool is one-shot by design (OpenCode SDK v1.2.27 does not support persistent TUI panels). See ADR-TUI-001 for the architectural decision.

---

## Configuration

Advanced plugin behavior is configured in `.axiom/plugin.yaml`. The plugin works without this file (zero-config), but creating it unlocks Layer 5 (automation) and Layer 6 (custom tools).

A fully-annotated example is at `.axiom/plugin.yaml` in any Axiom-bootstrapped repo. The key sections:

### Minimal config (enables automation)

```yaml
version: 1

layers:
  automation: true
  tools: true
```

### Full config reference

```yaml
version: 1

# Layer toggles
layers:
  metrics: true           # L2 — local cost tracking (default: on)
  session_sharing: false  # L3 — session replay/broadcast
  network: false          # L4 — team pulse + heartbeat
  automation: true        # L5 — hooks, watches, scripts
  tools: true             # L6 — custom tool registration

# Metrics (L2) — cost tracking and off-system publishing
metrics:
  session_budget_usd: 5.00        # Per-session budget (advisory)
  warning_threshold_pct: 80       # Warn at 80% of budget
  persist_sessions: true          # Save to .axiom/sessions/
  publish_enabled: true           # Off-system publishing (default: on)
  stream_interval_seconds: 30     # Delta snapshot cadence
  reconciliation_interval_seconds: 1800  # Reconciliation cadence (30 min)
  flush_interval_seconds: 600     # Disk flush cadence (10 min)
  max_buffered_intervals: 12      # Buffer cap before auto-summarize
  user_email: null                # null = auto-resolve via git config
  sinks: []                       # "axiom-backend", "prometheus", "cloudwatch"

  # Prometheus push gateway (add "prometheus" to sinks)
  prometheus:
    push_gateway_url: "http://localhost:9091"
    job_name: "codeops_plugin"
    namespace: "axiom"

  # AWS CloudWatch (add "cloudwatch" to sinks)
  cloudwatch:
    namespace: "Axiom/Plugin"
    region: null          # null = AWS_DEFAULT_REGION
    log_group: "/axiom/metrics"

  # Override embedded pricing table (USD per 1M tokens)
  pricing_overrides:
    "claude-sonnet-4-20250514":
      input_per_1m: 3.00
      output_per_1m: 15.00
      cache_read_per_1m: 0.30
      cache_write_per_1m: 3.75
      reasoning_per_1m: 15.00

# Network (L4) — team pulse and heartbeat
# Requires: AXIOM_API_TOKEN env var
network:
  backend_url: "https://axiom.example.com"
  team_id: "my-team"
  heartbeat_interval_seconds: 60
  user_identifier: "email"   # "email", "username", or "anonymous"
  visibility: "public"       # "public", "team_only", or "hidden"

# Langfuse LLM observability
# Requires: experimental.openTelemetry: true in opencode.jsonc
# Requires: LANGFUSE_PUBLIC_KEY, LANGFUSE_SECRET_KEY, LANGFUSE_BASEURL
#        OR network.backend_url (proxy mode)
langfuse:
  enabled: true
  proxy_mode: true          # true = fetch keys from backend
  cache_ttl_seconds: 3600

# Hooks (L5) — lifecycle event handlers
hooks:
  on_session_start:
    - name: "load-context"
      run: "cat .axiom/context.md 2>/dev/null || true"
  on_session_idle:
    - name: "keep-alive"
      prompt: "Check TODO.md for the next unchecked item and continue"
      condition: "maturity >= 3"
  on_session_end:
    - name: "save-notes"
      run: "echo 'Session ended' >> .axiom/session-log.txt"

# Watches (L5) — react to tool events
watches:
  - name: "env-file-guard"
    trigger: "tool.read"
    pattern: ".env*"
    action: "block"
    message: "Reading .env files is blocked by security policy"
  - name: "spec-change-reminder"
    trigger: "tool.write"
    pattern: "specs/*.md"
    run: "echo 'Spec changed — remember to update tests'"

# Scripts (L5) — named scripts invocable via codeops_run_script tool
scripts:
  test:
    run: "npm test 2>&1 | tail -20"
    description: "Run project tests"
  deploy-staging:
    run: "axiom deploy --env staging"
    description: "Deploy to staging"
    requires_confirmation: true

# Custom tools (L6) — add tools to the agent's palette
tools:
  - name: "project_test"
    description: "Run project tests and return results"
    run: "npm test 2>&1 | tail -50"
  - name: "project_lint"
    description: "Run the project linter"
    run: "npm run lint 2>&1"

# Idle timeout — ms before on_session_idle fires (default: 60000)
idle_timeout_ms: 60000

# Profile-based config scoping (AXIOM_PROFILE env var)
profiles:
  ci:
    hooks:
      on_session_start:
        - name: "ci-mode"
          run: "echo 'CI mode active'"
  local:
    hooks:
      on_session_idle:
        - name: "keep-alive"
          prompt: "Check TODO.md for the next unchecked item and continue"
          condition: "maturity >= 3"
```

---

## Environment Variables

| Variable | Required | Description |
|----------|----------|-------------|
| `AXIOM_API_TOKEN` | For L4/backend sink | Bearer token for Axiom backend API |
| `LANGFUSE_PUBLIC_KEY` | For Langfuse direct mode | Langfuse project public key |
| `LANGFUSE_SECRET_KEY` | For Langfuse direct mode | Langfuse project secret key |
| `LANGFUSE_BASEURL` | For Langfuse direct mode | Langfuse instance URL (default: `https://cloud.langfuse.com`) |
| `LANGFUSE_ENVIRONMENT` | Optional | Langfuse environment tag (default: `development`) |
| `AXIOM_USER_EMAIL` | Optional | Override email identity resolution |
| `AXIOM_PROFILE` | Optional | Active config profile (default: `default`) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | For Langfuse auto-detect | Set by OpenCode when `experimental.openTelemetry: true` |

---

## Metrics Sinks

The plugin supports four off-system metrics sinks. **Stdout is always active** as a fallback — metrics events are logged as structured JSON even with no config.

### Stdout (always on)

No configuration needed. Every `plugin.metrics.snapshot` and `plugin.session.ended` event is written as a single-line JSON to stdout.

### Axiom Backend

```yaml
# .axiom/plugin.yaml
metrics:
  sinks: ["axiom-backend"]

network:
  backend_url: "https://axiom.example.com"
```

```bash
export AXIOM_API_TOKEN="your-api-token"
```

Events are POSTed to `POST <backend_url>/api/v1/events`. 400/401 errors are non-retryable. 429/503 errors are retried with Retry-After.

### Prometheus Push Gateway

```yaml
metrics:
  sinks: ["prometheus"]
  prometheus:
    push_gateway_url: "http://localhost:9091"
    job_name: "codeops_plugin"
    namespace: "axiom"
```

Pushes per-model token counts and session metrics in Prometheus text exposition format.

### AWS CloudWatch

```yaml
metrics:
  sinks: ["cloudwatch"]
  cloudwatch:
    namespace: "Axiom/Plugin"
    region: "us-east-1"
    log_group: "/axiom/metrics"
```

Requires `@aws-sdk/client-cloudwatch` and `@aws-sdk/client-cloudwatch-logs` to be installed. Uses the default AWS credential chain.

---

## Langfuse LLM Observability

The Langfuse layer sends LLM traces to [Langfuse](https://langfuse.com) with:
- **Axiom metadata** on every span: `axiom.work_item_id`, `axiom.session_identity`, `axiom.user_email`, `axiom.repo_root`, `axiom.maturity_level`
- **Secret redaction** before sending: API keys, JWTs, AWS keys, Bearer tokens, connection strings
- **Proxy mode**: plugin fetches Langfuse keys from the Axiom backend (recommended — keeps secret keys off developer machines)
- **Direct mode**: plugin reads `LANGFUSE_PUBLIC_KEY` / `LANGFUSE_SECRET_KEY` from env vars

### Setup (direct mode)

1. Enable `experimental.openTelemetry: true` in `opencode.jsonc`
2. Set env vars:
   ```bash
   export LANGFUSE_PUBLIC_KEY="pk-lf-..."
   export LANGFUSE_SECRET_KEY="sk-lf-..."
   export LANGFUSE_BASEURL="https://cloud.langfuse.com"  # or your self-hosted URL
   ```
3. Enable in `.axiom/plugin.yaml`:
   ```yaml
   langfuse:
     enabled: true
     proxy_mode: false
   ```

### Local testing with docker-compose

```bash
# Start Langfuse v3 locally (from backend/)
docker compose up -d

# Wait ~30s for migrations, then verify:
curl http://localhost:3100/api/public/health
# → {"status":"OK","version":"3.167.0"}

# Login at http://localhost:3100
# Email: admin@axiom.local / Password: Admin123!@#
# API keys: pk-lf-axiom-dev / sk-lf-axiom-dev
```

---

## Security

### Blocked File Patterns

The plugin automatically blocks reads of sensitive files via the `tool.execute.before` hook:

- `.env`, `.env.*` — environment variable files
- `credentials*`, `*credential*` — credential files
- `*secret*`, `*token*`, `*key*` — secret-related names
- `*.pem`, `*.p12`, `*.pfx` — certificate and key files

Add custom patterns via `watches` in `.axiom/plugin.yaml`:

```yaml
watches:
  - name: "custom-secret-guard"
    trigger: "tool.read"
    pattern: "*.vault"
    action: "block"
    message: "Vault files are blocked"
```

### Content Redaction

Shared content and Langfuse traces are automatically redacted for:
- AWS keys (`AKIA...`)
- JWTs (`eyJ...`)
- API keys (`api_key=`, `apikey:`, `sk-...`, `pk-lf-...`)
- Bearer tokens
- GitHub tokens (`ghp_...`, `ghs_...`)
- Connection strings (`postgresql://...`, `mongodb://...`)
- Private key blocks (`-----BEGIN PRIVATE KEY-----`)

### Privacy

Run `/axiom-privacy` to see what data is collected, where it's stored, and how to delete it.

---

## Session Analytics

### Cost Tracking

The plugin tracks per-session cost using the OpenCode message stream:
- Per-model token usage (input, output, cache_read, cache_write, reasoning)
- Cost computation using embedded pricing tables (overridable)
- No network calls required — all pricing is local

Live pricing is fetched from [models.dev](https://models.dev) in the background and cached for 24 hours. User-configured `pricing_overrides` always win.

### Budget Alerts

```yaml
metrics:
  session_budget_usd: 5.00
  warning_threshold_pct: 80
```

Budget states: `ok` → `warning` (at 80%) → `exceeded` (advisory only, never blocks).

### Session Persistence

Session analytics are saved to `.axiom/sessions/<session-id>.json`:
- Atomic write (write-to-temp-then-rename)
- Automatic retention: max 1000 files, max 90 days
- `.gitignore` handling: sessions directory is automatically gitignored

---

## Development

### Prerequisites

- [Bun](https://bun.sh) >= 1.0
- Node.js >= 18

### Setup

```bash
cd .axiom/plugin
bun install
```

### Test

```bash
bun test
```

### Typecheck

```bash
bun run typecheck
```

### Build

```bash
bun run build
```

---

## API Reference

### Core Exports

```typescript
import { detectMaturity, MaturityLevel, MATURITY_LABELS } from "opencode-axiom"
import { bootstrap } from "opencode-axiom"
import { getCommandsForLevel, handleAboveLevelCommand } from "opencode-axiom"
import { detectBackendMode, createBackendRouter } from "opencode-axiom"
import { createSpinupManager, BUILTIN_TARGETS } from "opencode-axiom"
```

### Metrics Exports

```typescript
// Local cost tracking
import { parseSessionMessages, computeSessionCost, loadPricingTable } from "opencode-axiom"
import { loadSessionBudgetConfig, evaluateSessionBudget } from "opencode-axiom"
import { persistSessionAnalytics, enforceRetentionPolicy } from "opencode-axiom"

// Off-system publishing (sinks)
import {
  createStdoutSink, createBackendSink, createPrometheusSink, createCloudWatchSink,
  createPublisher, createBufferManager,
  usdToMicrodollars, microdollarsToUsd, computeCostMicrodollars,
} from "opencode-axiom"
```

### Langfuse Exports

```typescript
import { initLangfuse } from "opencode-axiom"
import { fetchLangfuseConfig } from "opencode-axiom"
```

### Security Exports

```typescript
import { redactContent, createRedactionEngine } from "opencode-axiom"
import { isBlockedFile, DEFAULT_BLOCKED_PATTERNS } from "opencode-axiom"
import { buildPrivacyReport, formatPrivacyReport } from "opencode-axiom"
```

### Configuration Exports

```typescript
import { loadPluginConfig, validatePluginConfig } from "opencode-axiom"
import { readConfigYaml, writeConfigYaml, validateJiraProjectKey } from "opencode-axiom"
```

---

## Peer Dependencies

- `@opencode-ai/plugin` >= 1.2.0

## Optional Dependencies

- `@langfuse/otel` — required for Langfuse LLM observability
- `@opentelemetry/sdk-node` — required for Langfuse
- `@aws-sdk/client-cloudwatch` — required for CloudWatch sink
- `@aws-sdk/client-cloudwatch-logs` — required for CloudWatch sink

## License

MIT
