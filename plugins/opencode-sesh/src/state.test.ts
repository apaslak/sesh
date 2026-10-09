import { expect, test } from "bun:test"
import { initialState, reduce, status, reconcile } from "./state"

test("hidden successful work remains unread until that execution is viewed", () => {
  let state = reduce(initialState(), { type: "start", execution: "1" })
  state = reduce(state, { type: "success", execution: "1" })
  expect(status(state)).toEqual({ kind: "finished", execution: "1", read: false })
  expect(status(reduce(state, { type: "visible", visible: true }))).toEqual({ kind: "finished", execution: "1", read: true })
})

test("already-visible completion is read", () => {
  let state = reduce(initialState(), { type: "visible", visible: true })
  state = reduce(state, { type: "start", execution: "1" })
  expect(status(reduce(state, { type: "success", execution: "1" }))).toEqual({ kind: "finished", execution: "1", read: true })
})

test("attention wins over execution and resolves back to working", () => {
  let state = reduce(initialState(), { type: "start", execution: "1" })
  state = reduce(state, { type: "attention", reason: "permission" })
  expect(status(state)).toEqual({ kind: "attention", reason: "permission" })
  expect(status(reduce(state, { type: "attention", reason: undefined }))).toEqual({ kind: "working", execution: "1" })
})

test("stale completion cannot replace new work", () => {
  let state = reduce(initialState(), { type: "start", execution: "1" })
  state = reduce(state, { type: "start", execution: "2" })
  expect(status(reduce(state, { type: "success", execution: "1" }))).toEqual({ kind: "working", execution: "2" })
})

test("failure remains attention on focus; next execution clears it", () => {
  let state = reduce(initialState(), { type: "start", execution: "1" })
  state = reduce(state, { type: "failure", execution: "1" })
  state = reduce(state, { type: "visible", visible: true })
  expect(status(state)).toEqual({ kind: "attention", reason: "error" })
  expect(status(reduce(state, { type: "start", execution: "2" }))).toEqual({ kind: "working", execution: "2" })
})

test("interruption is idle, never successful completion", () => {
  const state = reduce(initialState(), { type: "start", execution: "1" })
  expect(status(reduce(state, { type: "interrupt", execution: "1" }))).toEqual({ kind: "idle" })
})

test("reconciliation recovers completed work without confusing idle with success", () => {
  const state = reconcile(initialState(), { running: false, outcome: "succeeded", idle: 123 }, undefined)
  expect(status(state)).toEqual({ kind: "finished", execution: "idle:123", read: false })
  expect(status(reconcile(state, { running: false, outcome: "succeeded", idle: 123 }, "idle:123"))).toEqual({ kind: "finished", execution: "idle:123", read: true })
  expect(status(reconcile(state, { running: false, outcome: "interrupted", idle: 124 }, undefined))).toEqual({ kind: "idle" })
})

test("acknowledging an old completion cannot mark a new completion read", () => {
  const old = reconcile(initialState(), { running: false, outcome: "succeeded", idle: 123 }, "idle:123")
  expect(status(reconcile(old, { running: false, outcome: "succeeded", idle: 124 }, "idle:123"))).toEqual({ kind: "finished", execution: "idle:124", read: false })
})
