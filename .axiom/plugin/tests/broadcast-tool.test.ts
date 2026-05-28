// axiom:trace work_item=opencode-plugin-01 spec=specs/70-OpenCode-Plugin.md#REQ-PLG-208 plan=phase-70-6/task-70-6-1/step-70-6-1-1
import { describe, it, expect } from "bun:test";
import { getCommandsForLevel } from "../src/commands";
import { MaturityLevel } from "../src/detect";

// /axiom-broadcast was renamed to /axiom-share (same functionality,
// shares session replays and broadcasts to team members).
const BROADCAST_COMMAND = "/axiom-share";

describe("REQ-PLG-208: /axiom-broadcast command registration", () => {
	it("appears in command palette at MaturityLevel.Active (Level 3+)", () => {
		const commands = getCommandsForLevel(MaturityLevel.Active);
		const names = commands.map((c) => c.name);
		expect(names).toContain(BROADCAST_COMMAND);
	});

	it("has correct description", () => {
		const commands = getCommandsForLevel(MaturityLevel.Active);
		const broadcast = commands.find((c) => c.name === BROADCAST_COMMAND);
		expect(broadcast).toBeDefined();
		expect(broadcast!.description.toLowerCase()).toMatch(/share|broadcast/);
	});

	it("is not visible at MaturityLevel.New (Level 0)", () => {
		const commands = getCommandsForLevel(MaturityLevel.New);
		const names = commands.map((c) => c.name);
		// share/broadcast requires Active level (3) — should not appear at New (0)
		expect(names).not.toContain(BROADCAST_COMMAND);
	});
});
