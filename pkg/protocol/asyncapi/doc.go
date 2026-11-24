// Package asyncapi implements the AsyncAPI 3.x protocol adapter
// (PROTO-021, implementing the PROTO-007 spec gate).
//
// The adapter ingests an AsyncAPI 3.x document (file path or http(s) URL,
// JSON or YAML), maps its channels/messages/operations onto the
// protocol-neutral operation model (specs/implementation/PROTO-007 §2), and
// executes the operations over ONE transport end-to-end: Kafka via
// github.com/segmentio/kafka-go.
//
// Transport choice note: the PROTO-007 spec recommends NATS first, but the
// PROTO-021 board row records the owner decision (2026-11-23) that Kafka
// lands first. Every other constraint of the spec binds unchanged.
//
// Kinds:
//
//	operation action "send"    → protocol.KindUnary         (one bounded publish)
//	operation action "receive" → protocol.KindEventSource   (bounded collect, never long-lived)
//
// Executable Kafka binding subset (declared per PROTO-007 §3.1):
//
//   - channel address = topic name, with {parameter} expressions resolved
//     from the operation's channel parameters;
//   - publish carries an optional message key ("_key" invocation argument)
//     and the message headers the document declares;
//   - consume runs a bounded collect with default bounds of 1000 messages /
//     30 seconds (PROTO-007 §4): subscribe, collect, close, return. There is
//     no unbounded mode.
//
// NOT executed (deliberately): consumer groups, offset commits, durable
// subscriptions, transactions, exactly-once semantics. A receive operation
// whose Kafka binding declares a group obligation (groupId) is rejected at
// execution time with CodeValidation rather than silently ignored
// (PROTO-007 §9.2). Broker topology remains an execution concern; the
// binding facts are preserved verbatim in Operation.Source.
//
// Authentication maps configured SASL credentials and TLS settings into the
// kafka-go dialer/transport (PROTO-007 §5). Credentials are never serialized
// into operations, logs, transcripts, or tool schemas.
//
// Errors are mapped through pkg/errors with the PROTO-007 §6 table
// (typed kafka error codes — never message substring matching).
package asyncapi
