import { expect, test } from "bun:test"
import { observeDisplayed, type Observation } from "./observe"

test("conversation switched during async observation is not published or acknowledged", async () => {
  let displayed: string | undefined = "ses_first"
  let release: (() => void) | undefined
  const waiting = new Promise<void>(resolve => { release = resolve })
  const observation: Observation = { title: "First", agent: "build", running: false, outcome: "succeeded", idle: 123, visible: true, attention: undefined }
  const result = observeDisplayed("ses_first", () => displayed, async () => { await waiting; return observation })
  displayed = "ses_second"
  if (release) release()
  expect(await result).toBeUndefined()
})

test("stable displayed conversation accepts an authoritative observation", async () => {
  const observation: Observation = { title: "First", agent: "build", running: true, visible: false, attention: "permission" }
  expect(await observeDisplayed("ses_first", () => "ses_first", async () => observation)).toEqual(observation)
})

test("stalled observation expires and a later refresh can recover", async () => {
  let signal: AbortSignal | undefined
  const stalled = observeDisplayed("ses_first", () => "ses_first", async input => {
    signal = input
    return new Promise<Observation>(() => {})
  }, { timeout: 10 })
  expect(await stalled).toBeUndefined()
  expect(signal?.aborted).toBe(true)
  const observation: Observation = { title: "Recovered", agent: "build", running: false, visible: false, attention: undefined }
  expect(await observeDisplayed("ses_first", () => "ses_first", async () => observation)).toEqual(observation)
})

test("cleanup cancels a non-cancellable sync without accepting its late result", async () => {
  const controller = new AbortController()
  let release: ((value: Observation) => void) | undefined
  const stalled = observeDisplayed("ses_first", () => "ses_first", async () => new Promise<Observation>(resolve => { release = resolve }), { signal: controller.signal, timeout: 10000 })
  controller.abort()
  expect(await stalled).toBeUndefined()
  if (release) release({ title: "Too late", agent: "build", running: false, outcome: "succeeded", idle: 123, visible: true, attention: undefined })
  await Promise.resolve()
})
