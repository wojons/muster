# GraphQL Adapter (PROTO-005)

Muster can ingest a GraphQL schema and call it as typed commands. The adapter
implements the protocol seam described in [specs/PROTOCOLS.md](../../specs/PROTOCOLS.md):
schema ingestion produces protocol-neutral operations, and each operation is one
GraphQL-over-HTTP request.

## Commands

```bash
# Ingest a schema and list what it exposes (no server contact for a document):
openapi-cli protocol graphql --schema schema.graphql

# Ingest a recorded introspection response instead of SDL:
openapi-cli protocol graphql --schema introspection.json

# Run the standard introspection query against a live endpoint:
openapi-cli protocol graphql --schema graphql://api.example.com/graphql

# Call one operation (one POST to one endpoint, document + variables):
openapi-cli protocol graphql --schema schema.graphql \
  --endpoint https://api.example.com/graphql \
  --operation user --args '{"id":"u1"}'

# Print the document that would be sent, without sending it:
openapi-cli protocol graphql --schema schema.graphql --operation user \
  --args '{"id":"u1"}' --document
```

A live introspection target becomes the default `--endpoint`, so a call against
the same server needs no repeated flag.

## What ingestion produces

| GraphQL | Command surface |
|---|---|
| Query field | command (`query` tag) |
| Mutation field | command (`mutation` tag) |
| Field argument (non-null, no default) | required flag |
| Field argument with a default | optional flag carrying the default |
| Enum argument | flag plus the enum's values (`enum` in the MCP input schema) |
| Input-object argument | object flag, nested properties described recursively |
| Custom scalar | string flag (GraphQL type named in the flag description) |
| Subscription field | **not** ingested (follow-up row) |

Discovery output records where each command came from (`source.schema_source`,
`source.operation_type`, `source.field`, `source.field_type`,
`source.arg_types`, plus `source.endpoint` for an introspection target).

## The request shape

GraphQL is one endpoint, one POST:

```json
{"query":"query UserQuery($id: ID!) {\n  user(id: $id) {\n    id\n  }\n}","variables":{"id":"u1"},"operationName":"UserQuery"}
```

Argument values always travel in `variables` — they are never interpolated into
the query document, so no value can change the document's shape. The REST-style
path/method builder is not used: there are no paths in GraphQL.

## Selection sets are derived, never hand-written

* `--fields a,b` restricts the operation's selection (in the caller's order).
* `--max-depth N` (default 2) bounds how deep object fields expand.
* `--max-fields N` (default 25) caps the fields selected per level.
* Union-typed fields are selected as `__typename` (inline fragments are out of
  scope); interface-typed fields select the interface's own fields.
* A field whose arguments include a required argument is skipped when selecting,
  because the derived document carries no values for it.

## Known limits (bounded first row)

* **Subscriptions** are recognized and skipped; a live subscription needs the
  bounded-collect contract from the streaming work (matrix row 6).
* **Inline fragments** are not generated, so union selections are `__typename`
  only.
* **Directives** are parsed and ignored (no semantics, no propagation).
* **Custom scalars** have no JSON mapping of their own: they map to the neutral
  `string` type and are sent through unchanged.
* **Introspection disabled** by a hardened server produces an actionable error
  naming the SDL fallback — never an empty command set.

## Errors

Transport failures and GraphQL `errors[]` entries map onto Muster's error codes
per the cross-cutting contract: `UNAUTHENTICATED`/`FORBIDDEN` →
`UNAUTHORIZED`, `BAD_USER_INPUT` → `VALIDATION_ERROR`,
`GRAPHQL_PARSE_FAILED`/`GRAPHQL_VALIDATION_FAILED` → `BAD_REQUEST`,
`NOT_FOUND`/`PERSISTED_QUERY_NOT_FOUND` → `NOT_FOUND`,
`INTERNAL_SERVER_ERROR` → `INTERNAL_ERROR`, `TOO_MANY_REQUESTS` →
`RATE_LIMITED`, HTTP 408 → `TIMEOUT`.

A response carrying **both** data and errors is partial success: the CLI prints
both, and the MCP tool result carries both, rather than discarding half the
answer.
