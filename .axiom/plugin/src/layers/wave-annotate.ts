/**
 * Wave /axiom-annotate command handler.
 *
 * REQ-WEAVE-041: /axiom-annotate creates an artifact_review Space (or uses
 *   an existing one for the work item) and posts the file content as an
 *   artifact_review item.
 *
 * Spec: specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-041
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-041 plan=phase-2.8/task-2.8.1/step-2.8.1
 */

import { readFileSync, statSync, realpathSync } from "fs"
import { resolve, sep } from "path"

const MAX_FILE_SIZE = 1024 * 1024 // 1MB

export interface AnnotateCommandOptions {
  backendUrl: string
  authToken?: string
  workItemId?: string
  /** Repository root path for boundary validation */
  repoPath?: string
  /** Override fetch for testing */
  _mockFetch?: (url: string, opts?: RequestInit) => Promise<Response>
  /** Override file read for testing */
  _mockReadFile?: (filePath: string) => string
  /** Override file stat for testing */
  _mockStat?: (filePath: string) => { size: number }
  /** Override realpath for testing symlink scenarios */
  _mockRealpath?: (path: string) => string
}

export interface AnnotateResult {
  spaceId: string
  itemId: string
  url: string
  filePath: string
}

/**
 * Handle /axiom-annotate <file_path>
 *
 * 1. Reads the file at file_path from the repo.
 * 2. Creates (or reuses) an artifact_review Space for the work item.
 * 3. Creates an item with type="artifact_review" and the file content.
 * 4. Returns the Space URL for the user to open in browser.
 *
 * axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-041 plan=phase-2.8/task-2.8.1/step-2.8.1
 */
export async function handleAnnotateCommand(
  filePath: string,
  opts: AnnotateCommandOptions,
): Promise<AnnotateResult> {
  if (!filePath || filePath.trim() === "") {
    throw new Error("file_path is required for /axiom-annotate")
  }

  const fetch = opts._mockFetch ?? globalThis.fetch

  // Validate path is within repo boundary (prevent path traversal + symlink attacks)
  // Uses realpathSync to resolve symlinks - prevents symlink-based boundary bypass
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-041 plan=phase-2.10/task-2.10.symlink-fix/step-1
  const realpath = opts._mockRealpath ?? realpathSync
  const repoRoot = realpath(resolve(opts.repoPath ?? process.cwd()))
  let resolvedPath: string
  try {
    resolvedPath = realpath(resolve(repoRoot, filePath))
  } catch (err: unknown) {
    if (err instanceof Error && "code" in err && (err as NodeJS.ErrnoException).code === "ENOENT") {
      throw new Error(`Error: File not found: ${filePath}`)
    }
    throw new Error(`Error: Cannot resolve path: ${filePath}`)
  }
  if (!resolvedPath.startsWith(repoRoot + sep) && resolvedPath !== repoRoot) {
    throw new Error(`Error: File path '${filePath}' is outside the repository boundary.`)
  }

  // Read file content
  // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-041 plan=phase-2.8/task-2.8.1/step-2.8.1
  let fileContent: string
  try {
    // Check file size before reading
    // axiom:trace work_item=wave-01 spec=specs/79-Axiom-Weave-Interactive-Review-Portal.md#REQ-WEAVE-041 plan=phase-2.9/task-2.9.annotate-limits/step-1
    let stats: { size: number }
    if (opts._mockStat) {
      stats = opts._mockStat(resolvedPath)
    } else {
      stats = statSync(resolvedPath)
    }
    if (stats.size > MAX_FILE_SIZE) {
      throw new Error(`Error: File too large (${Math.round(stats.size / 1024)}KB). Maximum size is 1MB.`)
    }

    if (opts._mockReadFile) {
      fileContent = opts._mockReadFile(resolvedPath)
    } else {
      fileContent = readFileSync(resolvedPath, "utf-8")
    }
  } catch (err: unknown) {
    if (err instanceof Error) {
      if ("code" in err && (err as NodeJS.ErrnoException).code === "ENOENT") {
        throw new Error(`Error: File not found: ${filePath}`)
      }
      // Re-throw our custom error messages
      if (err.message.startsWith("Error:")) {
        throw err
      }
      throw new Error(`Error reading file: ${err.message}`)
    }
    throw new Error(`Error reading file: ${String(err)}`)
  }

  const authHeaders: Record<string, string> = opts.authToken
    ? { Authorization: `Bearer ${opts.authToken}` }
    : {}

  // Step 1: Create an artifact_review Space for the work item
  const spaceResponse = await fetch(`${opts.backendUrl}/api/v1/spaces`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      ...authHeaders,
    },
    body: JSON.stringify({
      title: `Artifact Review: ${filePath}`,
      type: "artifact_review",
      work_item_id: opts.workItemId ?? null,
      access_scope: "link",
    }),
  })

  if (!spaceResponse.ok) {
    throw new Error(
      `Failed to create artifact_review Space: ${spaceResponse.status} ${spaceResponse.statusText}`,
    )
  }

  const space = (await spaceResponse.json()) as { space_id: string; title: string }

  // Step 2: Create an artifact_review item in the Space
  const itemResponse = await fetch(
    `${opts.backendUrl}/api/v1/spaces/${space.space_id}/items`,
    {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        ...authHeaders,
      },
      body: JSON.stringify({
        type: "artifact_review",
        content: {
          artifact_type: "markdown",
          file_path: filePath,
          text: fileContent,
        },
      }),
    },
  )

  if (!itemResponse.ok) {
    throw new Error(
      `Failed to create artifact_review item: ${itemResponse.status} ${itemResponse.statusText}`,
    )
  }

  const item = (await itemResponse.json()) as { item_id: string }
  const spaceUrl = `${opts.backendUrl}/ui/spaces/${space.space_id}`

  return {
    spaceId: space.space_id,
    itemId: item.item_id,
    url: spaceUrl,
    filePath,
  }
}
