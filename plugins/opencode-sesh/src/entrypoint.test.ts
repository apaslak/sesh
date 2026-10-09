import { expect, test } from "bun:test"
import { resolve } from "node:path"

test("V2 host can discover the CLI entrypoint from a configured local directory", async () => {
  const directory = resolve(import.meta.dir, "..")
  // V2 resolves local directories through <directory>/tui, not package exports.
  const entrypoint = Bun.resolveSync("./tui", directory)
  const module: unknown = await import(entrypoint)
  expect(typeof module).toBe("object")
  if (typeof module !== "object" || module === null || !("default" in module)) throw new Error("Plugin default export missing")
  const plugin: unknown = module.default
  if (typeof plugin !== "object" || plugin === null || !("id" in plugin)) throw new Error("Plugin identity missing")
  expect(plugin.id).toBe("sesh.tmux-status")
})
