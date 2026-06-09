/**
 * Wave /axiom-privacy command handler.
 * Returns what Wave data is stored, where, and how to delete it.
 * REQ-WEAVE-305
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-5.0/task-5.0.privacy-command/step-5.0.2
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-6.0/task-6.0.privacy-fix/step-6.0.4
 */

export function handlePrivacyCommand(): string {
  return formatPrivacyMessage();
}

export function formatPrivacyMessage(): string {
  return [
    "## Wave Privacy Information",
    "",
    "### What Wave stores",
    "- **Spaces**: title, type, work_item_id, created_at (Postgres: wave_spaces)",
    "- **Panels**: panel layout and type within a Space (Postgres: wave_panels)",
    "- **Items**: session messages, chat, rich renders, WYSIWYG content (Postgres: wave_items)",
    "- **Comments**: inline annotations with author and target (Postgres: wave_comments)",
    "- **Actions**: agent-proposed action items with approval state (Postgres: wave_actions)",
    "- **Participants**: participant_id, display_name, presence status (Postgres: wave_participants)",
    "- **Shares**: encrypted share links with expiry (Postgres: wave_shares; S3 if configured)",
    "- **Automations**: trigger rules and execution history (Postgres: wave_automations)",
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-6.0/task-6.0.privacy-fix/step-6.0.4
    "- **Audit log**: injection attempts, security events, and admin actions (Postgres: wave_audit_log)",
    "",
    "### Where data is stored",
    "- **Local**: Postgres database configured in `.axiom/axiom.config.yaml`",
    "- **Cloud (optional)**: S3 bucket for encrypted share artifacts",
    "- **In-memory only**: WebSocket connections, typing indicators, presence heartbeats",
    "",
    "### How to delete your data",
    "- **Delete a Space**: `DELETE /api/v1/spaces/{space_id}` — removes Space + all items, comments, participants",
    "- **Delete a share**: `DELETE /api/v1/spaces/{space_id}/shares/{share_id}`",
    "- **Right to erasure**: `DELETE /api/v1/spaces/{space_id}/items/{item_id}/purge` — removes item from all history",
    "- **Full wipe**: Drop and recreate the `wave_*` tables in Postgres",
    "",
    "### Encryption",
    "- Share URLs use AES-256-GCM client-side encryption. The decryption key is in the URL fragment and never sent to the server.",
    "- WebSocket connections use short-lived JWTs (1 hour expiry).",
    "",
    "For more information, see `specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-300`.",
  ].join("\n");
}
