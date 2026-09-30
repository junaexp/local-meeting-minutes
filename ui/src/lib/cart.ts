import type { Job, JobStatus } from './api'

/** The cart is a retained input list. Clearing it never cancels server jobs or deletes files. */
export interface CartFile {
  path: string
  name: string
  disabled?: boolean
  sourcePath?: string
  minutesJob?: { id: string; status: JobStatus }
}
export const isActive = (job?: { status: JobStatus }) =>
  !!job && ['queued', 'preparing', 'running'].includes(job.status)
export const isMedia = (path: string) => /\.(wav|mp3|m4a|mp4|mov|ogg)$/i.test(path)
export const canSubmit = (file: CartFile) =>
  !file.disabled && !isActive(file.minutesJob) && file.minutesJob?.status !== 'completed'
export function minuteStatus(status: JobStatus) {
  return {
    queued: '대기 중',
    preparing: '처리 중',
    running: '처리 중',
    completed: '완료',
    failed: '실패',
    cancelled: '중단',
    interrupted: '중단',
  }[status]
}
export function addPaths(existing: CartFile[], paths: string[]): CartFile[] {
  const seen = new Set(existing.map((item) => item.path))
  const result = [...existing]
  for (const path of paths) {
    if (seen.has(path)) continue
    seen.add(path)
    result.push({ path, name: path.split(/[\\/]/).pop() || path })
  }
  return result
}
export function moveFile(files: CartFile[], from: number, to: number): CartFile[] {
  if (from < 0 || to < 0 || from >= files.length || to >= files.length || from === to) return files
  const next = [...files]
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}
export function removeFile(files: CartFile[], path: string) {
  return files.filter((file) => file.path !== path)
}

/** Use one pass per snapshot, instead of allocating and reversing jobs for every row. */
export function indexJobs(jobs: Job[], model?: string) {
  const minutes = new Map<string, Job>()
  const transcriptions = new Map<string, Job>()
  const media = new Map<string, Job>()
  for (const job of jobs) {
    if (isMedia(job.path) && (!model || (job.transcriptionModel || model) === model))
      media.set(job.path, job)
    if (job.kind !== 'transcription') minutes.set(job.path, job)
    else if (!model || (job.transcriptionModel || model) === model)
      transcriptions.set(job.path, job)
  }
  return { minutes, transcriptions, media }
}

/** Keep the last known status when history is cleaned; removed cart rows stay removed. */
export function syncCartJobs(files: CartFile[], jobs: Job[]): CartFile[] {
  const byID = new Map(jobs.map((job) => [job.id, job]))
  const { minutes } = indexJobs(jobs)
  let changed = false
  const next = files.map((file) => {
    const latest = minutes.get(file.path)
    const job = latest || (file.minutesJob ? byID.get(file.minutesJob.id) : undefined)
    if (
      !job ||
      job.kind === 'transcription' ||
      (job.id === file.minutesJob?.id && job.status === file.minutesJob.status)
    )
      return file
    changed = true
    return { ...file, minutesJob: { id: job.id, status: job.status } }
  })
  return changed ? next : files
}

/** A generated SRT inherits its source's current minutes job to prevent duplicate work. */
export function attachTranscript(
  files: CartFile[],
  sourcePath: string,
  transcriptPath: string,
): CartFile[] {
  const source = files.find((file) => file.path === sourcePath && !file.sourcePath)
  if (!source || !transcriptPath) return files
  const existing = files.find((file) => file.sourcePath === sourcePath)
  if (source.disabled && existing?.path === transcriptPath) return files
  const next = files.filter(
    (file) => file.sourcePath !== sourcePath && file.path !== transcriptPath,
  )
  const index = next.findIndex((file) => file.path === sourcePath)
  next[index] = { ...next[index], disabled: true }
  next.splice(index + 1, 0, {
    path: transcriptPath,
    name: transcriptPath.split(/[\\/]/).pop() || transcriptPath,
    sourcePath,
    ...(source.minutesJob ? { minutesJob: source.minutesJob } : {}),
  })
  return next
}
