import { api, type Job } from './api'
import { isActive, type CartFile } from './cart'

/** Snapshots include all active jobs, but only recent finished jobs. Recover older retained rows
 * on demand, with bounded concurrency and no full-history polling on each log event. */
export function createCartRecovery(read: () => CartFile[], update: (files: CartFile[]) => void) {
  let disposed = false
  let visible = new Set<string>()
  let running = 0
  const attempts = new Map<string, number>()
  const pending = new Set<string>()
  function pump() {
    if (disposed) return
    for (const file of read()) {
      const id = file.minutesJob?.id
      if (running >= 4) break
      if (!id || !isActive(file.minutesJob) || visible.has(id) || pending.has(id)) continue
      if (Date.now() - (attempts.get(id) ?? 0) < 10_000) continue
      attempts.set(id, Date.now())
      pending.add(id)
      running++
      void api
        .job(id)
        .then(
          (job) => {
            if (!isActive(job)) apply(id, job)
          },
          (error) => {
            // A deleted history entry must not leave its input permanently disabled.
            if (error?.status === 404) apply(id)
          },
        )
        .finally(() => {
          pending.delete(id)
          running--
          pump()
        })
    }
  }
  function apply(id: string, job?: Job) {
    if (disposed || visible.has(id)) return
    const files = read()
    let changed = false
    const next = files.map((file) => {
      // A late response cannot resurrect a removed row or overwrite a newer run.
      if (file.minutesJob?.id !== id || !isActive(file.minutesJob)) return file
      changed = true
      return { ...file, minutesJob: job ? { id, status: job.status } : undefined }
    })
    if (changed) update(next)
  }
  return {
    accept(jobs: Job[]) {
      visible = new Set(jobs.map((job) => job.id))
      const retained = new Set(read().map((file) => file.minutesJob?.id))
      for (const id of attempts.keys()) if (!retained.has(id)) attempts.delete(id)
      pump()
    },
    dispose() {
      disposed = true
    },
  }
}
