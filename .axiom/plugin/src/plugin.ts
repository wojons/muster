/**
 * Plugin-only entrypoint for OpenCode package-style loading.
 *
 * This file exports ONLY the CodeOpsPlugin function — nothing else.
 * It is the safe entrypoint for users who install opencode-axiom as an
 * npm package and configure it via `"plugin": ["opencode-axiom/plugin"]`
 * in opencode.jsonc.
 *
 * WHY THIS EXISTS:
 * OpenCode's getLegacyPlugins loader iterates Object.values(module) and calls
 * each export as a plugin function. If any export is not a function, it throws
 * "Plugin export is not a function". The main dist/index.js exports ~66 symbols
 * including objects, numbers, and strings (library API surface), making it
 * unsafe for direct plugin loading.
 *
 * This file re-exports only CodeOpsPlugin, making it safe for the loader.
 *
 * USAGE (opencode.jsonc):
 *   "plugin": ["opencode-axiom/plugin"]
 *
 * The main entrypoint (dist/index.js) remains the library API for programmatic
 * use by other TypeScript/JavaScript consumers.
 *
 * Spec: specs/70-OpenCode-Plugin.md#REQ-PLG-001
 * axiom:trace work_item=opencode-plugin-entrypoints-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-001 plan=phase-1/task-1-1/step-1-1-1
 */

export { CodeOpsPlugin } from "./index.js"
export { CodeOpsPlugin as default } from "./index.js"
