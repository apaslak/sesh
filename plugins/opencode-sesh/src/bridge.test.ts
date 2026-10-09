import { expect, test } from "bun:test"
import { mkdtemp, readFile, rm, stat } from "node:fs/promises"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { filename, writeSnapshot, removeOwned, visibleClient, pickerOpen } from "./bridge"

test("snapshot is atomic, private and compatible with Go namespace", async () => {
  const dir = await mkdtemp(join(tmpdir(), "sesh-test-"))
  try {
    const path = join(dir, filename("/socket:42", "%1"))
    await writeSnapshot(path, { version: 1, server: "/socket:42", pane: "%1", owner: "one", conversation: "ses_1", title: "Review", agent: "build", heartbeat: 10000, status: { kind: "finished", execution: "1", read: false } })
    const parsed: unknown = JSON.parse(await readFile(path, "utf8"))
    expect(parsed).toEqual({ version: 1, server: "/socket:42", pane: "%1", owner: "one", conversation: "ses_1", title: "Review", agent: "build", heartbeat: 10000, status: { kind: "finished", execution: "1", read: false } })
    expect((await stat(path)).mode & 0o777).toBe(0o600)
    await removeOwned(path, "old")
    expect(await readFile(path, "utf8")).toContain('"owner":"one"')
    await removeOwned(path, "one")
    expect(await Bun.file(path).exists()).toBe(false)
  } finally { await rm(dir, { recursive: true }) }
})

test("picker lease applies only to its server and expires after a crash", async () => {
  const dir = await mkdtemp(join(tmpdir(), "sesh-test-"))
  try {
    await Bun.write(join(dir, "picker-" + filename("server", "") + "-one"), JSON.stringify({ heartbeat: 10000 }))
    expect(await pickerOpen(dir, "server", 10000)).toBe(true)
    expect(await pickerOpen(dir, "other", 10000)).toBe(false)
    expect(await pickerOpen(dir, "server", 15001)).toBe(false)
  } finally { await rm(dir, { recursive: true }) }
})

test("only a focused attached client displaying the pane counts as viewing", () => {
  expect(visibleClient("%1\tattached,focused,UTF-8", "%1", false)).toBe(true)
  expect(visibleClient("%2\tattached,focused,UTF-8", "%1", false)).toBe(false)
  expect(visibleClient("%1\tattached,UTF-8", "%1", false)).toBe(false)
  expect(visibleClient("%1\tattached,focused,UTF-8", "%1", true)).toBe(false)
})
