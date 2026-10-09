import { createHash, randomUUID } from "node:crypto"
import { mkdir, writeFile, rename, readFile, unlink, readdir } from "node:fs/promises"
import { dirname, join } from "node:path"
import { homedir } from "node:os"
import type { Status } from "./state"

/** Wire protocol consumed by sesh's agentstatus package. */
export type Snapshot = { version: 1; server: string; pane: string; owner: string; conversation: string; title: string; agent: string; directory?: string; heartbeat: number; status: Status }
export function filename(server: string, pane: string): string {
  return createHash("sha256").update(server + "\x00" + pane).digest("hex") + ".json"
}
export function stateDirectory(): string {
  return join(process.env.XDG_STATE_HOME || join(homedir(), ".local", "state"), "sesh", "opencode")
}

/** Callers serialize writes so obsolete snapshots cannot win a rename race. */
export async function writeSnapshot(path: string, snapshot: Snapshot): Promise<void> {
  await mkdir(dirname(path), { recursive: true, mode: 0o700 })
  const temporary = path + "." + randomUUID() + ".tmp"
  try {
    await writeFile(temporary, JSON.stringify(snapshot), { mode: 0o600, flag: "wx" })
    await rename(temporary, path)
  } finally { await unlink(temporary).catch(() => {}) }
}

export async function removeOwned(path: string, owner: string): Promise<void> {
  try {
    const parsed: unknown = JSON.parse(await readFile(path, "utf8"))
    if (typeof parsed === "object" && parsed !== null && "owner" in parsed && parsed.owner === owner) await unlink(path)
  } catch { /* Missing/invalid snapshots are already unavailable. */ }
}

export function visibleClient(lines: string, pane: string, popup: boolean): boolean {
  if (popup) return false
  return lines.split("\n").some(line => {
    const [id, rawFlags] = line.split("\t")
    const flags = rawFlags?.split(",") ?? []
    return id === pane && flags.includes("attached") && flags.includes("focused") && !flags.includes("control-mode")
  })
}

/** A short-lived picker lease prevents popup-covered panes being acknowledged. */
export async function pickerOpen(directory: string, server: string, now: number): Promise<boolean> {
  const prefix = "picker-" + filename(server, "") + "-"
  const names = await readdir(directory).catch(() => [])
  for (const name of names) {
    if (!name.startsWith(prefix)) continue
    try {
      const value: unknown = JSON.parse(await readFile(join(directory, name), "utf8"))
      if (typeof value === "object" && value !== null && "heartbeat" in value && typeof value.heartbeat === "number" && Math.abs(now - value.heartbeat) <= 5000) return true
    } catch { /* Ignore expired or incomplete leases. */ }
  }
  return false
}
