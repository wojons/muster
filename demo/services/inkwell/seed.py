#!/usr/bin/env python3
"""Seed Inkwell with authors, posts, and threaded comments."""

import json
import sys
import urllib.request
import urllib.error

BASE = f"http://localhost:{sys.argv[1]}" if len(sys.argv) > 1 else "http://localhost:9803"
TOKEN = "admin-token"


def api(method, path, data=None):
    url = f"{BASE}{path}"
    req = urllib.request.Request(url, method=method)
    req.add_header("Authorization", f"Bearer {TOKEN}")
    if data:
        req.add_header("Content-Type", "application/json")
        req.data = json.dumps(data).encode()
    try:
        with urllib.request.urlopen(req) as resp:
            return resp.status, json.loads(resp.read())
    except urllib.error.HTTPError as e:
        body = e.read().decode()
        try:
            return e.code, json.loads(body)
        except json.JSONDecodeError:
            return e.code, {"error": body}


def ok(r):
    return r[0] in (200, 201)


# ═══════════════════════════════════════════════════════════════
# Authors (5)
# ═══════════════════════════════════════════════════════════════

# Note: Authors are created via direct store manipulation is not exposed as POST /authors.
# Instead, we create them by calling a special internal endpoint. Since we don't have one,
# we create authors by posting posts and the author_id links them. But we need a create-authors
# endpoint. Let's just seed them directly — wait, the API doesn't have POST /authors.
#
# The server has no POST /authors endpoint. We'll add author records via the server
# by using a trick: we can import the module or we can just have the seed populate
# the in-memory store directly. But seed.py runs as a separate process against the
# running server. We need to create authors via API.
#
# Solution: The server has authors dict but no POST endpoint. We'll use the /posts
# endpoint which auto-creates author linkage. But we still need author records
# for /authors endpoint to return.
#
# Let me just directly seed authors in a way the server can handle.
# Actually, wait — the server stores authors in-memory but has no POST /authors.
# Let me just call a direct approach: manually populate the server's authors
# via the server.py module, or add a simple POST /authors endpoint...
#
# Actually the cleanest approach: add POST to /authors in server.py.
# Let me update server.py to allow admin to create authors via POST /authors.
# But I already wrote server.py. Let me just add that.

print("Seeding Inkwell...")

# We need POST /authors — let's just create a mini endpoint for seeding.
# Actually, the simplest approach: have seed.py write to a JSON file that
# server.py loads on startup. But server.py only loads from openapi.json.
#
# The task says seed with the API. Let me add POST /authors to the server.
# I'll patch server.py to add POST support for /authors (admin only).

# For now, let's try with direct approach: use the health endpoint to verify
# server is running, then post authors via a custom approach.

# Actually, the most pragmatic approach: I'll add POST /authors to server.py
# now via the patch tool, then write the seed.

# Let me just complete the seed file assuming POST /authors exists.
# I'll add it to server.py after.

# ── Authors ───────────────────────────────────────────────────
author_data = [
    {"username": "ada", "display_name": "Ada Lovelace", "bio": "Systems programmer and Rust enthusiast. Building compilers by day, game engines by night.", "avatar_url": "https://i.pravatar.cc/150?u=ada"},
    {"username": "grace", "display_name": "Grace Hopper", "bio": "Distributed systems architect. Kubernetes contributor and CNCF ambassador.", "avatar_url": "https://i.pravatar.cc/150?u=grace"},
    {"username": "alan", "display_name": "Alan Turing", "bio": "ML researcher focusing on edge inference and model compression.", "avatar_url": "https://i.pravatar.cc/150?u=alan"},
    {"username": "margaret", "display_name": "Margaret Hamilton", "bio": "Security engineer specializing in zero-trust architectures and cloud infrastructure.", "avatar_url": "https://i.pravatar.cc/150?u=margaret"},
    {"username": "linus", "display_name": "Linus Torvalds", "bio": "Open source maintainer. Kernel hacker. Opinionated about code quality.", "avatar_url": "https://i.pravatar.cc/150?u=linus"},
]

author_ids = {}
for a in author_data:
    status, result = api("POST", "/authors", a)
    if ok((status, result)):
        author_ids[a["username"]] = result["id"]
        print(f"  ✅ Author: {a['display_name']} ({result['id']})")
    else:
        print(f"  ❌ Failed to create author {a['username']}: {result}")

# ── Posts (12) ───────────────────────────────────────────────
posts_data = [
    # Published (6)
    {
        "author": "ada",
        "title": "Why Rust is the Future of Systems Programming",
        "excerpt": "Memory safety without garbage collection, zero-cost abstractions, and a thriving ecosystem make Rust the clear choice for the next decade of systems work.",
        "body": "## The Case for Rust\n\nSystems programming has long been dominated by C and C++. These languages give you full control over memory and hardware, but that control comes at a steep cost: buffer overflows, use-after-free, double frees — the list of potential footguns is endless.\n\nRust changes the game entirely. Its ownership system catches memory errors at compile time rather than at 3 AM when your pager goes off.\n\n### Ownership and Borrowing\n\nThe core innovation in Rust is the ownership system. Every value has exactly one owner at a time. When the owner goes out of scope, the value is dropped. References can be borrowed immutably (many readers) or mutably (one writer), but never both simultaneously.\n\n```rust\nfn main() {\n    let mut data = vec![1, 2, 3];\n    let first = &data[0];  // immutable borrow\n    data.push(4);           // ERROR: cannot borrow mutably while immutably borrowed\n    println!(\"{}\", first);\n}\n```\n\nThis eliminates entire categories of bugs at compile time.\n\n### Zero-Cost Abstractions\n\nRust's iterators, closures, and trait system compile down to the same machine code you'd write by hand. You don't pay for what you don't use.\n\n### Ecosystem Maturity\n\nThe crate ecosystem has matured enormously. Tokio for async I/O, Serde for serialization, Diesel/SQLx for databases — production-grade libraries for every need.\n\n### Where We Go From Here\n\nLinux kernel modules in Rust, Firefox's Servo engine, AWS's Firecracker — Rust is already in production at the lowest levels of the stack. The next five years will see Rust become the default choice for new systems projects.",
        "tags": ["rust", "systems-programming", "memory-safety"],
        "status": "published",
    },
    {
        "author": "grace",
        "title": "Understanding Kubernetes Operators",
        "excerpt": "Operators extend Kubernetes to manage complex stateful applications. Here's how they work and when you should build one.",
        "body": "## Beyond Deployments and Services\n\nKubernetes does a fantastic job managing stateless workloads out of the box. Deployments, ReplicaSets, and Services handle the common case beautifully. But what about databases? Message queues? Monitoring stacks?\n\nThese stateful applications require domain-specific operational knowledge — backup schedules, failover procedures, upgrade sequences. That's where Operators come in.\n\n### The Operator Pattern\n\nAn Operator is a custom controller that encodes human operational knowledge into software. It extends the Kubernetes API with Custom Resource Definitions (CRDs) and runs a control loop that reconciles desired state with actual state.\n\n```yaml\napiVersion: postgres.example.com/v1\nkind: PostgresCluster\nmetadata:\n  name: my-db\nspec:\n  replicas: 3\n  version: \"15\"\n  backup:\n    schedule: \"0 2 * * *\"\n    retention: 30d\n```\n\n### When to Build an Operator\n\nBuild an Operator when:\n- Your application requires complex setup/teardown sequences\n- You need automated failover and recovery\n- Upgrades require ordered steps across components\n- You want to codify your team's operational runbooks\n\n### Operator SDK and Frameworks\n\nThe Operator SDK (Go/Ansible/Helm), Kopf (Python), and Java Operator SDK make building operators approachable. Start with Helm if your logic is simple; graduate to Go for complex state machines.\n\n### Real-World Examples\n\n- **Prometheus Operator**: Manages Prometheus, Alertmanager, and Thanos instances\n- **Strimzi**: Runs Kafka clusters on Kubernetes\n- **Cert-Manager**: Automates TLS certificate provisioning and renewal\n\nOperators are the secret sauce that turns Kubernetes from a container orchestrator into a complete application platform.",
        "tags": ["kubernetes", "devops", "operators"],
        "status": "published",
    },
    {
        "author": "alan",
        "title": "A Practical Guide to WebAssembly",
        "excerpt": "WebAssembly isn't just for browsers anymore. Learn how WASM is reshaping edge computing, plugin systems, and serverless.",
        "body": "## WASM Beyond the Browser\n\nWebAssembly started as a way to run C++ in the browser at near-native speed. But the real revolution is happening on the server.\n\n### The WASM Value Proposition\n\n- **Sandboxed by default**: No access to the host unless explicitly granted\n- **Near-native performance**: Typically within 10-20% of native\n- **Polyglot**: Compile from Rust, C, C++, Go, Python (via WIT), and more\n- **Tiny cold starts**: Microsecond-level instantiation vs millisecond containers\n\n### WASI: The System Interface\n\nWASI (WebAssembly System Interface) provides a standardized set of capabilities — filesystem access, networking, environment variables — that WASM modules can request. The host decides what to grant.\n\n```rust\n// A WASI-compatible HTTP handler (spin framework)\n#[http_component]\nfn handle_request(req: Request) -> Response {\n    let name = req.query().get(\"name\").unwrap_or(\"World\");\n    Response::new(200, format!(\"Hello, {}!\", name))\n}\n```\n\n### Edge Computing\n\nCloudflare Workers, Fastly Compute@Edge, and Fermyon Spin all run WASM at the edge. You get cold starts measured in microseconds and pay only for actual execution time.\n\n### Plugin Systems\n\nTools like Envoy proxy, Zellij terminal multiplexer, and Lapce editor use WASM for extensibility. Users write plugins in any language that compiles to WASM, and the host sandboxes them securely.\n\n### The Future\n\nThe Component Model promises to make WASM modules composable — import a key-value store interface and swap implementations without recompiling. This could fundamentally change how we build distributed systems.",
        "tags": ["webassembly", "wasm", "edge-computing", "serverless"],
        "status": "published",
    },
    {
        "author": "ada",
        "title": "Building REST APIs with Flask",
        "excerpt": "A practical walkthrough of building production-ready REST APIs with Flask, covering routing, validation, auth, and testing.",
        "body": "## Flask: Simple but Powerful\n\nFlask remains one of the most popular Python web frameworks because it gets out of your way. No boilerplate, no enforced project structure — just route decorators and WSGI.\n\n### Getting Started\n\n```python\nfrom flask import Flask, jsonify, request\n\napp = Flask(__name__)\n\n@app.route('/api/items', methods=['GET'])\ndef list_items():\n    return jsonify(items)\n\n@app.route('/api/items/<id>', methods=['GET'])\ndef get_item(id):\n    item = next((i for i in items if i['id'] == id), None)\n    if not item:\n        return jsonify({'error': 'not found'}), 404\n    return jsonify(item)\n```\n\n### Request Validation\n\nUse Pydantic or Marshmallow for request validation:\n\n```python\nfrom pydantic import BaseModel, ValidationError\n\nclass CreateItem(BaseModel):\n    name: str\n    price: float\n    tags: list[str] = []\n\n@app.route('/api/items', methods=['POST'])\ndef create_item():\n    try:\n        data = CreateItem(**request.get_json())\n    except ValidationError as e:\n        return jsonify(e.errors()), 400\n    # ...\n```\n\n### Authentication\n\nJWT tokens or API keys via before_request hooks:\n\n```python\n@app.before_request\ndef check_auth():\n    if request.endpoint not in ('health',):\n        token = request.headers.get('Authorization')\n        if not valid(token):\n            return jsonify({'error': 'unauthorized'}), 401\n```\n\n### Testing\n\n```python\ndef test_create_item(client):\n    resp = client.post('/api/items', json={'name': 'Widget', 'price': 9.99})\n    assert resp.status_code == 201\n    assert resp.get_json()['name'] == 'Widget'\n```\n\n### Production Considerations\n\n- Run behind Gunicorn with multiple workers\n- Use connection pooling for databases\n- Add health check and readiness endpoints\n- Log structured JSON for observability\n- Use Blueprints to organize larger applications\n\nFlask's simplicity is its greatest strength. Add only what you need, when you need it.",
        "tags": ["python", "flask", "rest-api", "backend"],
        "status": "published",
    },
    {
        "author": "margaret",
        "title": "Docker Security Best Practices",
        "excerpt": "Containers don't contain. Here are practical steps to harden your Docker deployments against common attack vectors.",
        "body": "## The Container Security Illusion\n\nContainers share the host kernel. A container escape vulnerability means game over for the entire node. Security must be layered: image, runtime, orchestration, and host.\n\n### 1. Minimal Base Images\n\nUse distroless or scratch images. Every package you install is a potential CVE.\n\n```dockerfile\n# Bad: 100+ CVEs\nFROM ubuntu:22.04\n\n# Better: minimal attack surface\nFROM gcr.io/distroless/python3-debian12\n\n# Best: only what you need\nFROM scratch\nCOPY --from=builder /app/binary /binary\nENTRYPOINT [\"/binary\"]\n```\n\n### 2. Run as Non-Root\n\n```dockerfile\nRUN addgroup --system app && adduser --system --group app\nUSER app\n```\n\nWithout this, a compromised container process has root on the host kernel namespace.\n\n### 3. Read-Only Root Filesystem\n\n```yaml\nsecurityContext:\n  readOnlyRootFilesystem: true\n```\n\nMount only specific directories as writable volumes. This prevents attackers from installing persistence mechanisms.\n\n### 4. Drop Capabilities\n\nLinux capabilities break root privileges into distinct units. Drop everything, add back only what's needed:\n\n```yaml\nsecurityContext:\n  capabilities:\n    drop: [\"ALL\"]\n    add: [\"NET_BIND_SERVICE\"]\n```\n\n### 5. Network Policies\n\nDefault-deny all ingress/egress, then explicitly allow required paths:\n\n```yaml\napiVersion: networking.k8s.io/v1\nkind: NetworkPolicy\nmetadata:\n  name: deny-all\nspec:\n  podSelector: {}\n  policyTypes:\n  - Ingress\n  - Egress\n```\n\n### 6. Scan Images Continuously\n\nIntegrate Trivy, Grype, or Snyk into your CI pipeline. Block deployments with critical CVEs.\n\n### 7. Secrets Management\n\nNever bake secrets into images. Use Kubernetes Secrets, HashiCorp Vault, or cloud secret managers with runtime injection.\n\nContainer security isn't optional — it's table stakes.",
        "tags": ["docker", "security", "containers", "devops"],
        "status": "published",
    },
    {
        "author": "linus",
        "title": "Zero-Trust Architecture Explained",
        "excerpt": "Never trust, always verify. A practical guide to implementing zero-trust principles in your infrastructure.",
        "body": "## Beyond the Perimeter\n\nThe castle-and-moat security model is dead. When your workforce is remote, your apps are in the cloud, and your APIs are public, the traditional network perimeter doesn't exist anymore.\n\nZero-trust flips the model: assume every request is hostile, regardless of origin.\n\n### Core Principles\n\n1. **Verify explicitly**: Authenticate and authorize based on all available data points — identity, device health, location, service, data classification.\n2. **Least privilege**: Grant just enough access, just in time. No standing permissions.\n3. **Assume breach**: Segment networks, encrypt everywhere, monitor continuously.\n\n### Implementation Pillars\n\n**Identity**: Single sign-on with MFA. Conditional access policies based on risk signals.\n\n```\nIF user.role == \"engineer\"\n  AND device.compliant == true\n  AND location.country == \"US\"\n  AND risk_score < 50\nTHEN grant access\n```\n\n**Devices**: Enforce compliance policies. Require disk encryption, screen lock, OS patch level. Block non-compliant devices.\n\n**Network**: Micro-segmentation. Service-to-service authentication with mTLS. No implicit trust based on IP or subnet.\n\n**Data**: Classify data. Encrypt at rest and in transit. Apply data loss prevention policies.\n\n### Tooling\n\n- **Identity**: Okta, Azure AD, Keycloak\n- **Service mesh**: Istio, Linkerd (mTLS between services)\n- **API gateway**: Kong, Envoy (auth at the edge)\n- **Secrets**: Vault, AWS Secrets Manager\n\n### Getting Started\n\nDon't boil the ocean. Start with a single high-value application:\n1. Map its dependencies and data flows\n2. Implement strong authentication (MFA)\n3. Apply micro-segmentation\n4. Instrument with observability\n5. Expand to the next application\n\nZero-trust is a journey, not a product.",
        "tags": ["security", "zero-trust", "architecture", "networking"],
        "status": "published",
    },
    # Review (3)
    {
        "author": "grace",
        "title": "PostgreSQL Query Optimization",
        "excerpt": "From EXPLAIN ANALYZE to index strategies, master the art of making your Postgres queries fly.",
        "body": "## Reading Query Plans\n\nPostgreSQL has one of the most sophisticated query optimizers in the world. But it can only optimize what it understands.\n\n### EXPLAIN ANALYZE\n\nAlways start here:\n\n```sql\nEXPLAIN (ANALYZE, BUFFERS) SELECT * FROM orders WHERE customer_id = 42;\n```\n\nKey metrics:\n- **Execution time**: Wall clock time\n- **Planning time**: Time spent optimizing\n- **Buffers shared hit/read**: Cache hits vs disk reads\n- **Rows removed by filter**: Wasted work\n\n### Index Types\n\n**B-tree** (default): Equality and range queries. Covers 90% of cases.\n\n**GIN**: Full-text search, JSONB, arrays. Inverted index.\n\n**GiST**: Geometric data, full-text (alternative to GIN).\n\n**BRIN**: Block Range INdex. Tiny index for very large tables with natural sort order (timestamps).\n\n### Partial Indexes\n\nIndex only the rows you query:\n\n```sql\nCREATE INDEX idx_active_orders ON orders (customer_id)\nWHERE status = 'active';\n```\n\nMuch smaller, faster to scan, faster to maintain.\n\n### Covering Indexes\n\nInclude payload columns to avoid heap fetches:\n\n```sql\nCREATE INDEX idx_orders_customer ON orders (customer_id)\nINCLUDE (order_date, total);\n```\n\n### Common Anti-Patterns\n\n- **Function-wrapped columns**: `WHERE LOWER(email) = 'user@example.com'` defeats indexes. Use a generated column or expression index.\n- **Leading wildcards**: `WHERE name LIKE '%son'` can't use B-tree. Use trigram indexes (pg_trgm).\n- **OR conditions**: Often better as UNION ALL with separate indexed queries.\n- **Over-indexing**: Each index slows writes. Measure before adding.\n\n### Statistics\n\nPostgres uses table statistics to plan queries. If they're stale, plans go bad:\n\n```sql\nANALYZE orders;\n```\n\nFor large tables, increase the statistics target on key columns:\n\n```sql\nALTER TABLE orders ALTER COLUMN customer_id SET STATISTICS 1000;\n```\n\nQuery tuning is a skill that compounds — the more you practice reading plans, the faster you spot problems.",
        "tags": ["postgresql", "database", "performance", "sql"],
        "status": "review",
    },
    {
        "author": "alan",
        "title": "The Rise of AI-Assisted Development",
        "excerpt": "How large language models are transforming the developer workflow — from code generation to architecture review.",
        "body": "## The New Pair Programmer\n\nAI-assisted development has moved from novelty to necessity in less than two years. Tools like GitHub Copilot, Claude, and Codex are now integral to many developers' daily workflow.\n\n### Current Capabilities\n\n- **Code completion**: Context-aware suggestions that understand your codebase\n- **Code generation**: Produce entire functions from natural language descriptions\n- **Refactoring**: Rewrite code for performance, readability, or pattern adherence\n- **Test generation**: Create unit tests, edge cases, and integration tests\n- **Documentation**: Generate docstrings, READMEs, and API docs\n- **Code review**: Identify bugs, security issues, and style violations\n\n### The Human in the Loop\n\nAI is a force multiplier, not a replacement. The developer's role shifts from writing code to:\n1. **Specifying intent**: Clearly describing what needs to be built\n2. **Reviewing output**: Validating correctness, security, and performance\n3. **Architecture decisions**: High-level design that AI can't (yet) reason about\n4. **Context integration**: Understanding business domain and user needs\n\n### Productivity Impact\n\nStudies show 30-50% faster task completion for experienced developers using AI assistants. Junior developers benefit even more, effectively gaining a patient senior mentor.\n\n### Risks\n\n- **Over-reliance**: Accepting AI output without understanding it\n- **Security**: Generated code may contain vulnerabilities\n- **License contamination**: Training data may include copyrighted code\n- **Skill atrophy**: Losing the ability to write code from scratch\n\n### The Future\n\nMulti-agent systems where specialized AI agents handle different aspects — one writes code, another reviews, a third writes tests. Human developers orchestrate the ensemble.\n\nWe're not being replaced. We're being augmented.",
        "tags": ["ai", "llm", "developer-tools", "productivity"],
        "status": "review",
    },
    {
        "author": "margaret",
        "title": "GraphQL vs REST in 2026",
        "excerpt": "The API design debate isn't settled. Here's where each paradigm shines and where they fall short in today's landscape.",
        "body": "## State of the Union\n\nIn 2026, both REST and GraphQL are mature, well-understood technologies. The choice isn't about which is \"better\" — it's about which matches your problem.\n\n### REST Strengths\n\n- **Caching**: HTTP caching (CDN, browser, proxy) works out of the box. Cache-Control, ETag, Last-Modified are battle-tested.\n- **Simplicity**: Any HTTP client works. curl, fetch, axios — no special libraries needed.\n- **Tooling**: OpenAPI ecosystem is rich — auto-generated docs, SDKs, mock servers, validation.\n- **Operational maturity**: Rate limiting, load balancing, monitoring tools understand HTTP semantics.\n\n### GraphQL Strengths\n\n- **Precise data fetching**: Clients get exactly what they need, nothing more. No over-fetching or under-fetching.\n- **Type system**: Schema-first development with strong typing and introspection.\n- **Rapid iteration**: Frontend teams can add fields without backend changes (within limits).\n- **Federation**: Compose multiple services into a unified graph. Apollo Federation and GraphQL Mesh make this practical.\n\n### Where REST Wins\n\n- Public APIs with unknown consumers\n- File upload/download\n- Simple CRUD applications\n- When you need CDN caching\n\n### Where GraphQL Wins\n\n- Mobile apps (bandwidth matters)\n- Complex data graphs with nested relationships\n- Multiple client teams with different data needs\n- When frontend velocity is paramount\n\n### The Hybrid Approach\n\nMany organizations run both. REST for public APIs, GraphQL for internal product APIs. Or GraphQL as a gateway over REST microservices.\n\n### New Entrants\n\ntRPC brings end-to-end type safety without schema files. gRPC dominates service-to-service communication. The landscape is richer than ever.\n\nThe best API is the one your team can maintain and your consumers can use.",
        "tags": ["graphql", "rest", "api-design", "architecture"],
        "status": "review",
    },
    # Draft (3)
    {
        "author": "linus",
        "title": "Microservices Patterns: Event Sourcing",
        "excerpt": "Event sourcing turns your database inside out. Store facts, not state — and unlock audit, replay, and CQRS.",
        "body": "## Events as Source of Truth\n\nTraditional databases store current state. Your `users` table has the latest email. Your `orders` table has the current status. But what if you stored the sequence of changes instead, and derived current state from them?\n\nThat's event sourcing.\n\n### The Core Idea\n\nInstead of:\n```sql\nUPDATE orders SET status = 'shipped' WHERE id = 42;\n```\n\nYou append:\n```json\n{\"event\": \"OrderShipped\", \"order_id\": \"42\", \"timestamp\": \"...\"}\n```\n\nCurrent state is a left-fold over the event stream. The audit log IS the database.\n\n### Benefits\n\n- **Complete audit trail**: Every state change is recorded forever\n- **Time travel**: Reconstruct state at any point in time\n- **Debugging**: Replay events to reproduce bugs\n- **CQRS naturally**: Separate read models from write models\n- **Event-driven architecture**: Other services subscribe to events\n\n### Challenges\n\n- **Eventual consistency**: Read models lag behind writes\n- **Schema evolution**: Events are forever; versioning is critical\n- **Query complexity**: No SQL joins over event streams\n- **Storage growth**: Append-only logs grow indefinitely\n\n### When to Use\n\nEvent sourcing shines in:\n- Financial systems (audit requirements)\n- Collaborative editing (OT/CRDT on event streams)\n- Complex workflows (order to fulfillment pipeline)\n- Compliance-heavy domains\n\n### Implementation with Kafka\n\n```\nOrder Service → Kafka (orders topic) → Projection Service → Read DB\n                                       → Notification Service\n                                       → Analytics Pipeline\n```\n\nEach consumer maintains its own projection at its own pace.\n\n### Snapshotting\n\nDon't replay every event from the beginning of time. Periodically snapshot aggregate state and replay only events after the snapshot.\n\nEvent sourcing is a powerful pattern — but it's not a default choice. Use it when the benefits of an immutable event log justify the operational complexity.",
        "tags": ["microservices", "event-sourcing", "cqrs", "architecture"],
        "status": "draft",
    },
    {
        "author": "ada",
        "title": "TypeScript 5.0: What's New",
        "excerpt": "Decorators are finally here. Plus const type parameters, speed improvements, and better enums. Let's dig in.",
        "body": "## The Decorator Milestone\n\nTypeScript 5.0 ships with ECMAScript-standard decorators. This has been years in the making, and the result is clean, standards-compliant metaprogramming.\n\n### ECMAScript Decorators\n\n```typescript\nfunction logged(method: Function, context: ClassMethodDecoratorContext) {\n    return function(this: any, ...args: any[]) {\n        console.log(`Calling ${String(context.name)}`);\n        return method.apply(this, args);\n    };\n}\n\nclass API {\n    @logged\n    getUsers() { /* ... */ }\n}\n```\n\nNo more `experimentalDecorators` flag. This is the real deal.\n\n### Const Type Parameters\n\n```typescript\nfunction getConfig<const T extends readonly string[]>(keys: T): Record<T[number], string> {\n    // T is inferred as a tuple literal, not widened to string[]\n}\n\nconst config = getConfig(['host', 'port']);\n// config type: Record<\"host\" | \"port\", string>\n```\n\n### Performance Improvements\n\n- Package size reduced by ~30%\n- Faster type-checking for complex generics\n- Quicker project startup with module resolution caching\n\n### Other Highlights\n\n- **`export type *`**: Re-export only types from a module\n- **Better `enum` support**: Union enums get smarter type narrowing\n- **JSDoc `@satisfies`**: Type-check without changing the type\n- **`--moduleResolution bundler`**: Optimized for Vite, Webpack, esbuild\n\n### Migration Notes\n\n- Legacy experimental decorators still work but are deprecated\n- Upgrade path is straightforward for most codebases\n- Check DefinitelyTyped packages for compatibility\n\nTypeScript continues to evolve thoughtfully. 5.0 is a milestone release that sets the stage for the next decade of typed JavaScript.",
        "tags": ["typescript", "javascript", "decorators", "programming-languages"],
        "status": "draft",
    },
    {
        "author": "alan",
        "title": "Machine Learning at the Edge",
        "excerpt": "Running inference on microcontrollers and phones isn't just possible — it's practical. Here's the state of edge ML in 2026.",
        "body": "## Intelligence Without the Cloud\n\nNot every ML problem needs a GPU cluster. Edge inference — running models on-device — is faster, more private, and works offline.\n\n### Hardware Landscape\n\n- **Microcontrollers**: ARM Cortex-M4/M7, ESP32-S3, RP2350 — run tiny models at milliwatt power\n- **Mobile SoCs**: Apple Neural Engine, Qualcomm Hexagon, Google Tensor — purpose-built for ML\n- **Edge accelerators**: Google Coral TPU, Intel Movidius, Hailo-8 — plug into existing systems\n\n### Model Optimization\n\n**Quantization**: INT8 models are ~4x smaller and faster than FP32 with minimal accuracy loss.\n\n**Pruning**: Remove near-zero weights. Can reduce model size 50-90%.\n\n**Knowledge distillation**: Train a small \"student\" model to mimic a large \"teacher\".\n\n**Architecture search**: Auto-design efficient models for specific hardware targets.\n\n### Frameworks\n\n- **TensorFlow Lite**: Industry standard. Micro variant runs on bare metal.\n- **ONNX Runtime**: Cross-platform, hardware-accelerated.\n- **ExecuTorch**: PyTorch's edge runtime. Beta but promising.\n- **Modzy/OpenVINO**: Intel ecosystem for edge deployment.\n\n### Use Cases\n\n- **Wake word detection**: \"Hey Siri\" runs entirely on-device\n- **Predictive maintenance**: Vibration analysis on factory sensors\n- **Computer vision**: Quality inspection, barcode scanning, gesture recognition\n- **Health monitoring**: Heart rate anomaly detection on wearables\n\n### Challenges\n\n- **Model size**: Flash and RAM constraints on microcontrollers\n- **Tooling fragmentation**: Different frameworks for different hardware\n- **Update mechanisms**: How do you safely update models in the field?\n\nEdge ML is where the rubber meets the road. The models that matter most are the ones running on devices you can hold in your hand.",
        "tags": ["machine-learning", "edge-computing", "tensorflow", "iot"],
        "status": "draft",
    },
]

post_ids = {}
for p in posts_data:
    a_id = author_ids[p["author"]]
    payload = {
        "title": p["title"],
        "excerpt": p["excerpt"],
        "body": p["body"],
        "tags": p["tags"],
        "author_id": a_id,
    }
    status, post = api("POST", "/posts", payload)
    if ok((status, post)):
        pid = post["id"]
        post_ids[p["title"]] = pid
        # Move to target status
        if p["status"] == "review":
            api("POST", f"/posts/{pid}/submit")
        elif p["status"] == "published":
            api("POST", f"/posts/{pid}/submit")
            api("POST", f"/posts/{pid}/publish")
        print(f"  ✅ Post: {p['title'][:50]}... ({pid}) [{p['status']}]")
    else:
        print(f"  ❌ Post failed: {p['title'][:50]}... → {post}")

# ═══════════════════════════════════════════════════════════════
# Comments (threaded)
# ═══════════════════════════════════════════════════════════════

comment_tree = [
    # Rust post
    ("Why Rust is the Future of Systems Programming", None, "Alice Reader", "Great overview! I've been meaning to try Rust for a side project. The ownership system still intimidates me though.", "approved"),
    ("Why Rust is the Future of Systems Programming", 1, "Ada Lovelace", "The borrow checker feels like a wall at first, but once you internalize the rules it becomes second nature. Start with The Book — it's fantastic.", "approved"),
    ("Why Rust is the Future of Systems Programming", 1, "Bob Debugger", "One thing that helped me: think of ownership as RAII on steroids. If you've done C++, the patterns are familiar.", "approved"),
    ("Why Rust is the Future of Systems Programming", None, "Carol Coder", "How does async Rust compare to Go's goroutines? I've heard mixed things.", "approved"),
    ("Why Rust is the Future of Systems Programming", 4, "Ada Lovelace", "Rust's async is more explicit — you choose your runtime (Tokio, async-std, smol). It's less batteries-included but gives you more control. For most web services, Tokio+Axum is very ergonomic.", "approved"),

    # K8s post
    ("Understanding Kubernetes Operators", None, "Dave DevOps", "We built a Postgres operator at my company and it was... an adventure. The reconciliation loop is deceptively simple but edge cases multiply fast.", "approved"),
    ("Understanding Kubernetes Operators", 6, "Grace Hopper", "The reconciliation loop is the easy part! The hard parts are: CRD versioning, handling partial failures, and upgrade sequences. We learned the hard way.", "approved"),
    ("Understanding Kubernetes Operators", None, "Eve Engineer", "Is there an equivalent to Operators for Nomad or other orchestrators?", "approved"),
    ("Understanding Kubernetes Operators", 8, "Grace Hopper", "Nomad doesn't have a direct equivalent because it doesn't have CRDs. But you can achieve similar patterns with Nomad's job specifications and a control loop running externally. Different philosophy though — Nomad is more about scheduling, less about extending the API.", "approved"),

    # WASM post
    ("A Practical Guide to WebAssembly", None, "Frank Frontend", "I've been using WASM for image processing in the browser — 10x faster than JS for pixel manipulation. Game changer.", "approved"),
    ("A Practical Guide to WebAssembly", None, "Alice Reader", "How do you debug WASM? Is there a good devtools story?", "pending"),
    ("A Practical Guide to WebAssembly", 11, "Alan Turing", "Chrome DevTools has WASM debugging support now. You can set breakpoints in the source language (Rust/C++) if you include DWARF debug info. Firefox and Safari are catching up.", "approved"),

    # Docker Security
    ("Docker Security Best Practices", None, "Bob Debugger", "Tip #7 (secrets) is the one most teams get wrong. I've seen AWS keys in public DockerHub images more times than I can count.", "approved"),
    ("Docker Security Best Practices", None, "Grace Hopper", "Also worth mentioning: seccomp profiles and AppArmor/SELinux. The default Docker seccomp profile blocks ~44 syscalls but you can go further with custom profiles.", "approved"),

    # Zero Trust
    ("Zero-Trust Architecture Explained", None, "Carol Coder", "We've been on the zero-trust journey for 18 months now. The biggest challenge isn't technical — it's organizational. Getting teams to stop saying 'but it's internal traffic!' took months.", "approved"),
    ("Zero-Trust Architecture Explained", 15, "Margaret Hamilton", "This is so true. The cultural shift is harder than the technical implementation. My approach: start with a single critical service, show the security posture improvement with metrics, then expand. Data wins arguments.", "approved"),
    ("Zero-Trust Architecture Explained", 16, "Carol Coder", "Great advice! We did eventually win people over with a demo where we showed lateral movement was impossible after segmentation. Nothing like a live demo.", "approved"),

    # PG post (pending review — still has comments)
    ("PostgreSQL Query Optimization", None, "Eve Engineer", "The partial index tip is gold. We cut our index size by 80% on a 500M row table just by adding `WHERE deleted_at IS NULL`.", "approved"),
    ("PostgreSQL Query Optimization", None, "Dave DevOps", "Question: when would you choose BRIN over B-tree? I understand the theory but haven't found a good use case in practice.", "approved"),

    # GraphQL vs REST (pending review)
    ("GraphQL vs REST in 2026", None, "Frank Frontend", "We use both! REST for public API (better caching), GraphQL for our React dashboard (fewer round trips). The hybrid approach works well if you have the team to maintain both.", "approved"),
    ("GraphQL vs REST in 2026", None, "Alice Reader", "What about performance? I've heard GraphQL can lead to N+1 query problems.", "approved"),
    ("GraphQL vs REST in 2026", 21, "Margaret Hamilton", "The N+1 problem is real but solvable. Dataloader (Facebook's batching/caching library) eliminates it for most cases. The key pattern: collect all IDs from the query, batch-fetch in one query, cache results. Most GraphQL servers have built-in dataloader patterns now.", "approved"),
]

# Create comments in order, tracking IDs by position
comment_id_map = {}  # position_in_tree → comment_id
for idx, (post_title, parent_pos, author_name, body, status) in enumerate(comment_tree):
    pid = post_ids.get(post_title)
    if not pid:
        print(f"  ⚠️  Skipping comment — post not found: {post_title[:40]}")
        continue
    payload = {"author_name": author_name, "body": body, "status": status}
    if parent_pos is not None:
        parent_id = comment_id_map.get(parent_pos)
        if parent_id:
            payload["parent_id"] = parent_id
    # Use admin token (already set) so all comments are created
    s, result = api("POST", f"/posts/{pid}/comments", payload)
    if ok((s, result)):
        comment_id_map[idx + 1] = result["id"]  # 1-indexed positions
        thread = " ↳" if parent_pos else "●"
        print(f"  {thread} Comment by {author_name}: \"{body[:60]}...\" ({result['id']})")
    else:
        print(f"  ❌ Comment failed: {result}")

# ═══════════════════════════════════════════════════════════════
# Summary
# ═══════════════════════════════════════════════════════════════
print()
print(f"✅ Inkwell seeded:")
print(f"   {len(author_ids)} authors")
print(f"   {len(post_ids)} posts")
print(f"   {len(comment_id_map)} comments (threaded)")
print()
print("🔑 Pre-set Bearer tokens:")
print("   reader-token  → reader (read published posts/comments)")
print("   author-token  → author (create/edit own drafts, comment)")
print("   editor-token  → editor (review/publish, moderate comments)")
print("   admin-token   → admin  (full access, delete anything)")
print()
print("📡 Try:")
print("   curl -H 'Authorization: Bearer reader-token' http://localhost:9803/posts")
print("   curl -H 'Authorization: Bearer author-token' http://localhost:9803/me")
print("   curl http://localhost:9803/openapi.json | jq")
