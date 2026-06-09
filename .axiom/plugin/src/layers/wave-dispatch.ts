/**
 * Wave command dispatch — wires /axiom-wave, /axiom-review,
 * /axiom-annotate, /axiom-render, /axiom-status-page, and /axiom-privacy to handlers.
 *
 * REQ-WEAVE-100: /axiom-wave creates or opens a Space for the current work item.
 * REQ-WEAVE-101: /axiom-review opens the current session in a Space for live review.
 * REQ-WEAVE-102: Wave commands open the Wave in the user's default browser.
 * REQ-WEAVE-305: /axiom-privacy returns what Wave data is stored, where, and how to delete it.
 *
 * axiom:trace work_item=wave-01 spec=specs/70-OpenCode-Plugin.md#wave-integration,specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-1.3/task-1.3.2/step-1.3.2.1
 */

import { handleReviewCommand, handleWaveCommand, WaveCommandOptions } from "./wave"
import { handleAnnotateCommand } from "./wave-annotate"
// axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-5.0/task-5.0.privacy-command/step-5.0.1
import { handlePrivacyCommand } from "./wave-privacy"
import { handleRenderCommand } from "./wave-render"
// axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
import { formatStatusPageMessage, handleStatusPageCommand } from "./wave-status-page"

export interface WaveDispatchConfig {
  backendUrl: string
  authToken?: string
  workItemId?: string
  sessionId?: string
  spaceId?: string
}

/**
 * Dispatch a Wave command by name.
 * Returns a user-facing message string.
 */
export async function dispatchWaveCommand(
  commandName: string,
  args: string,
  config: WaveDispatchConfig,
): Promise<string> {
  const opts: WaveCommandOptions = {
    backendUrl: config.backendUrl,
    authToken: config.authToken,
    workItemId: config.workItemId,
    sessionId: config.sessionId,
  }

  switch (commandName) {
    case "/axiom-wave": {
      const title = args.trim() || undefined
      const result = await handleWaveCommand(title, opts)
      return `✓ Wave Space created: ${result.title}\n  Opening in browser: ${result.url}`
    }
    case "/axiom-review": {
      const result = await handleReviewCommand(opts)
      return `✓ Session review Space created: ${result.title}\n  Opening in browser: ${result.url}`
    }
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.1/step-2.8.1
    case "/axiom-annotate": {
      const filePath = args.trim()
      if (!filePath) {
        return "Usage: /axiom-annotate <file_path>"
      }
      const result = await handleAnnotateCommand(filePath, {
        backendUrl: config.backendUrl,
        authToken: config.authToken,
        workItemId: config.workItemId,
      })
      return `✓ Artifact review Space created for "${result.filePath}"\n  Open in browser: ${result.url}`
    }
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-2.8/task-2.8.2/step-2.8.2
    case "/axiom-render": {
      const parts = args.trim().split(/\s+(.+)/)
      const templateName = parts[0] ?? ""
      const dataJson = parts[1]
      if (!templateName) {
        return "Usage: /axiom-render <template> [data_json]\nTemplates: report, dashboard, timeline, kanban"
      }
      const result = await handleRenderCommand(templateName, dataJson, {
        backendUrl: config.backendUrl,
        authToken: config.authToken,
        spaceId: config.spaceId,
      })
      return `✓ Rendered "${result.template}" template in Space ${result.spaceId} (item: ${result.itemId})`
    }
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    case "/axiom-status-page": {
      // Parse optional --title and --prompt flags from args string
      const titleMatch = args.match(/--title\s+"([^"]+)"/) ?? args.match(/--title\s+(\S+)/)
      const promptMatch = args.match(/--prompt\s+"([^"]+)"/) ?? args.match(/--prompt\s+(\S+)/)
      const result = await handleStatusPageCommand(
        {
          title: titleMatch?.[1],
          prompt: promptMatch?.[1],
        },
        { backendUrl: config.backendUrl },
      )
      return formatStatusPageMessage(result)
    }
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-5.0/task-5.0.privacy-command/step-5.0.1
    case "/axiom-privacy": {
      return handlePrivacyCommand()
    }
    default:
      return `Unknown Wave command: ${commandName}`
  }
}

/**
 * Returns true if the given command name is a Wave command.
 */
export function isWaveCommand(commandName: string): boolean {
  return (
    commandName === "/axiom-wave" ||
    commandName === "/axiom-review" ||
    commandName === "/axiom-annotate" ||
    commandName === "/axiom-render" ||
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-100 plan=phase-4.9/task-4.9.command/step-4.9.1
    commandName === "/axiom-status-page" ||
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-305 plan=phase-5.0/task-5.0.privacy-command/step-5.0.1
    commandName === "/axiom-privacy"
  )
}