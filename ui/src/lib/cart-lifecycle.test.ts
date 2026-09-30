import { describe, expect, it } from 'vitest'
import { addPaths, attachTranscript, canSubmit, indexJobs, syncCartJobs } from './cart'
import type { Job } from './api'
const job = (status: Job['status'], id = 'minutes-1'): Job => ({
  id,
  kind: 'minutes',
  path: '/synthetic/meeting.mp4',
  name: 'meeting.mp4',
  status,
  phase: '',
  model: 'gpt-5.6-sol',
  effort: 'low',
  result: '',
  recentOutput: '',
  outputPath: '',
  error: '',
  prompt: '',
  createdAt: '2026-09-30T00:00:00Z',
})
describe('retained cart lifecycle', () => {
  it('keeps input rows, excludes active/completed jobs and permits failed retries', () => {
    const files = addPaths([], [job('queued').path])
    for (const status of ['queued', 'running', 'completed'] as const) {
      const updated = syncCartJobs(files, [job(status)])
      expect(updated).toHaveLength(1)
      expect(canSubmit(updated[0])).toBe(false)
    }
    expect(canSubmit(syncCartJobs(files, [job('failed')])[0])).toBe(true)
  })
  it('does not change the cart for log-only updates or restore a cleared row', () => {
    const files = syncCartJobs(addPaths([], [job('running').path]), [job('running')])
    expect(syncCartJobs(files, [{ ...job('running'), recentOutput: 'more output' }])).toBe(files)
    expect(syncCartJobs([], [job('completed')])).toEqual([])
    expect(attachTranscript([], job('completed').path, '/synthetic/generated.srt')).toEqual([])
  })
  it('inherits the source minutes job when automatic transcription exports an SRT', () => {
    const files = syncCartJobs(addPaths([], [job('running').path]), [job('running')])
    const attached = attachTranscript(files, job('running').path, '/synthetic/generated.srt')
    expect(attached.every((file) => !canSubmit(file))).toBe(true)
    const completed = syncCartJobs(attached, [job('completed')])
    expect(completed[1].minutesJob?.status).toBe('completed')
    expect(syncCartJobs(completed, [])).toBe(completed)
  })
  it('builds latest per-path indexes and handles Windows filenames', () => {
    expect(addPaths([], ['C:\\synthetic\\meeting.srt'])[0].name).toBe('meeting.srt')
    expect(
      indexJobs([job('completed'), job('queued', 'minutes-2')]).minutes.get(job('queued').path)?.id,
    ).toBe('minutes-2')
  })
})
