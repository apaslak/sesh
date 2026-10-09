/** The externally published state is a sum, so completion always has a revision. */
export type Status =
  | { kind: "idle" }
  | { kind: "working"; execution: string }
  | { kind: "attention"; reason: string }
  | { kind: "finished"; execution: string; read: boolean }

type Execution = Exclude<Status, { kind: "attention" }> | { kind: "failed"; execution: string }
export type ConversationState = { execution: Execution; visible: boolean; attention: string | undefined }
export type StatusInput =
  | { type: "start" | "success" | "failure" | "interrupt"; execution: string }
  | { type: "visible"; visible: boolean }
  | { type: "attention"; reason: string | undefined }

export function initialState(): ConversationState {
  return { execution: { kind: "idle" }, visible: false, attention: undefined }
}

/** Reduces only events belonging to this conversation, ignoring old executions. */
export function reduce(state: ConversationState, input: StatusInput): ConversationState {
  if (input.type === "visible") {
    const execution = state.execution.kind === "finished" && input.visible
      ? { ...state.execution, read: true } : state.execution
    return { ...state, execution, visible: input.visible }
  }
  if (input.type === "attention") return { ...state, attention: input.reason }
  if (input.type === "start") return { ...state, execution: { kind: "working", execution: input.execution } }
  if (state.execution.kind !== "working" || state.execution.execution !== input.execution) return state
  switch (input.type) {
    case "success": return { ...state, execution: { kind: "finished", execution: input.execution, read: state.visible } }
    case "failure": return { ...state, execution: { kind: "failed", execution: input.execution } }
    case "interrupt": return { ...state, execution: { kind: "idle" } }
  }
}

export function status(state: ConversationState): Status {
  if (state.attention) return { kind: "attention", reason: state.attention }
  if (state.execution.kind === "failed") return { kind: "attention", reason: "error" }
  return state.execution
}

/** Restores authoritative V2 outcome after reconnect; idle alone isn't success. */
export function reconcile(state: ConversationState, info: { running: boolean; outcome?: "succeeded" | "failed" | "interrupted"; idle?: number }, acknowledged: string | undefined): ConversationState {
  if (info.running) {
    return state.execution.kind === "working" ? state : reduce(state, { type: "start", execution: "running" })
  }
  if (info.outcome === "failed") return { ...state, execution: { kind: "failed", execution: "idle:" + info.idle } }
  if (info.outcome !== "succeeded" || info.idle === undefined) return { ...state, execution: { kind: "idle" } }
  const execution = "idle:" + info.idle
  const read = state.visible || acknowledged === execution || (state.execution.kind === "finished" && state.execution.execution === execution && state.execution.read)
  return { ...state, execution: { kind: "finished", execution, read } }
}
