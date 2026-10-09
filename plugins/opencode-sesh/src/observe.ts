/** An authoritative outcome plus client-local visibility, with no transcript. */
export type Observation = {
  title: string
  agent: string
  running: boolean
  outcome?: "succeeded" | "failed" | "interrupted"
  idle?: number
  visible: boolean
  attention: string | undefined
}

/** Bounds even non-cancellable sync calls; expired work can never be projected. */
export async function observeDisplayed(expected: string, current: () => string | undefined, load: (signal: AbortSignal) => Promise<Observation>, options: { signal?: AbortSignal; timeout?: number } = {}): Promise<Observation | undefined> {
  const controller = new AbortController()
  const abort = () => controller.abort()
  options.signal?.addEventListener("abort", abort, { once: true })
  const timer = setTimeout(abort, options.timeout ?? 3000)
  let cancel: (() => void) | undefined
  try {
    if (options.signal?.aborted) controller.abort()
    if (controller.signal.aborted) return undefined
    const cancelled = new Promise<undefined>(resolve => {
      cancel = () => resolve(undefined)
      controller.signal.addEventListener("abort", cancel, { once: true })
    })
    const observation = await Promise.race([load(controller.signal), cancelled])
    return !controller.signal.aborted && current() === expected ? observation : undefined
  } finally {
    clearTimeout(timer)
    options.signal?.removeEventListener("abort", abort)
    if (cancel) controller.signal.removeEventListener("abort", cancel)
  }
}
