import { Plugin } from "@opencode/plugin/tui"
import { randomUUID } from "node:crypto"
import { join } from "node:path"
import { filename, pickerOpen, removeOwned, stateDirectory, visibleClient, writeSnapshot } from "./bridge"
import { initialState, reconcile, reduce, status, type ConversationState } from "./state"
import { observeDisplayed } from "./observe"

async function tmux(socket: string, args: string[]): Promise<string> {
  const child = Bun.spawn(["tmux", "-S", socket, ...args], { stdout: "pipe", stderr: "pipe" })
  const [text, code] = await Promise.all([new Response(child.stdout).text(), child.exited])
  if (code !== 0) throw new Error("tmux observation failed")
  return text.trimEnd()
}

/** CLI-only bridge: the shared server cannot identify the terminal's tmux pane. */
export default Plugin.define({
  id: "sesh.tmux-status",
  async setup(context) {
    const environment = process.env.TMUX
    const pane = process.env.TMUX_PANE
    if (!environment || !pane || !/^%\d+$/.test(pane)) return
    const paneID = pane
    const lastComma = environment.lastIndexOf(",")
    const pidComma = environment.lastIndexOf(",", lastComma - 1)
    if (pidComma < 0) return
    const socket = environment.slice(0, pidComma)
    const server = await tmux(socket, ["display-message", "-p", "-t", pane, "#{socket_path}:#{pid}"])
    const directory = stateDirectory()
    const owner = randomUUID()
    const path = join(directory, filename(server, pane) + "-" + owner)
    const states = new Map<string, ConversationState>()
    const [saved, save] = context.storage.store<{ acknowledged: Record<string, string> }>("acknowledged", { initial: { acknowledged: {} } })
    let closed = false
    let terminalFocused: boolean | undefined
    let busy = false
    let pending = false
    let task: Promise<void> = Promise.resolve()
    const lifetime = new AbortController()

    const focus = () => { terminalFocused = true; schedule() }
    const blur = () => { terminalFocused = false; schedule() }
    context.renderer.on("focus", focus)
    context.renderer.on("blur", blur)

    async function refresh(): Promise<void> {
      const route = context.ui.router.current()
      if (route.type !== "session") { await removeOwned(path, owner); return }
      const conversation = route.sessionID
      const observation = await observeDisplayed(conversation, () => {
        const current = context.ui.router.current()
        return !closed && current.type === "session" ? current.sessionID : undefined
      }, async signal => {
        const [info, active] = await Promise.all([
          context.client.session.get({ sessionID: conversation }, { signal }),
          context.client.session.active({ signal }),
        ])
        signal.throwIfAborted()
        const ids = [...new Set([conversation, ...context.data.session.family(conversation)])]
        await Promise.all(ids.map(async sessionID => {
          await Promise.all([context.data.session.permission.sync(sessionID), context.data.session.form.sync(sessionID, context.location)])
        }))
        signal.throwIfAborted()
        const permission = ids.some(id => (context.data.session.permission.list(id)?.length ?? 0) > 0)
        const question = ids.some(id => (context.data.session.form.list(id, context.location)?.length ?? 0) > 0)
        const [clients, popup] = await Promise.all([
          tmux(socket, ["list-clients", "-F", "#{pane_id}\t#{client_flags}"]),
          pickerOpen(directory, server, Date.now()),
        ])
        return { title: info.title ?? conversation, agent: info.agent ?? "", directory: info.location.directory, running: active[conversation] !== undefined, outcome: info.outcome, idle: info.time.idle, visible: terminalFocused !== false && visibleClient(clients, paneID, popup), attention: permission ? "permission" : question ? "question" : undefined }
      }, { signal: lifetime.signal })
      if (!observation) return
      let state = states.get(conversation) ?? initialState()
      state = reduce(state, { type: "visible", visible: observation.visible })
      state = reduce(state, { type: "attention", reason: observation.attention })
      state = reconcile(state, observation, saved.acknowledged[conversation])
      states.set(conversation, state)
      const published = status(state)
      if (published.kind === "finished" && published.read && saved.acknowledged[conversation] !== published.execution) {
        await save(draft => { draft.acknowledged[conversation] = published.execution })
      }
      const final = context.ui.router.current()
      if (!closed && final.type === "session" && final.sessionID === conversation) await writeSnapshot(path, { version: 1, server, pane: paneID, owner, conversation, title: observation.title, agent: observation.agent, directory: observation.directory, heartbeat: Date.now(), status: published })
    }

    function schedule(): void {
      if (closed) return
      if (busy) { pending = true; return }
      busy = true
      task = refresh().catch(() => {
        // Leave the heartbeat to expire instead of claiming idle on disconnect.
      }).finally(() => {
        busy = false
        if (pending) { pending = false; schedule() }
      })
    }

    const stop = context.data.listen(({ details }) => {
      if (details.type.startsWith("session.execution.") || details.type.startsWith("permission.") || details.type.startsWith("form.")) schedule()
    })
    const timer = setInterval(schedule, 1000)
    schedule()
    return async () => {
      closed = true
      lifetime.abort()
      clearInterval(timer)
      stop()
      context.renderer.off("focus", focus)
      context.renderer.off("blur", blur)
      await task
      await removeOwned(path, owner)
    }
  },
})
