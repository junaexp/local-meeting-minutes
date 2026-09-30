import { afterEach, expect, it, vi } from 'vitest'
import { api, type Job } from './api'
import type { CartFile } from './cart'
import { createCartRecovery } from './cart-recovery'

afterEach(() => vi.restoreAllMocks())
const row = (): CartFile => ({
  path: '/meeting.txt',
  name: 'meeting.txt',
  minutesJob: { id: 'old', status: 'running' },
})
const settle = async () => {
  for (let i = 0; i < 8; i++) await Promise.resolve()
}

it('recovers finished jobs outside the recent snapshot without polling known active jobs', async () => {
  const fetch = vi.spyOn(api, 'job').mockResolvedValue({ id: 'old', status: 'completed' } as Job)
  let files = [row()]
  const recovery = createCartRecovery(
    () => files,
    (next) => (files = next),
  )
  recovery.accept([{ id: 'old', status: 'running' } as Job])
  expect(fetch).not.toHaveBeenCalled()
  recovery.accept([])
  recovery.accept([])
  await settle()
  expect(fetch).toHaveBeenCalledTimes(1)
  expect(files[0].minutesJob?.status).toBe('completed')
  recovery.dispose()
})

it('does not restore cleared rows after a late lookup', async () => {
  let resolve!: (job: Job) => void
  vi.spyOn(api, 'job').mockImplementation(() => new Promise((done) => (resolve = done)))
  let files = [row()]
  const recovery = createCartRecovery(
    () => files,
    (next) => (files = next),
  )
  recovery.accept([])
  files = []
  resolve({ id: 'old', status: 'completed' } as Job)
  await settle()
  expect(files).toEqual([])
  recovery.dispose()
})

it('unblocks deleted history and limits concurrent recovery requests to four', async () => {
  let reject!: (error: unknown) => void
  const fetch = vi
    .spyOn(api, 'job')
    .mockImplementation(() => new Promise((_, fail) => (reject = fail)))
  let files = Array.from({ length: 10 }, (_, i) => ({
    ...row(),
    path: `/meeting-${i}.txt`,
    minutesJob: { id: `${i}`, status: 'running' as const },
  })) as CartFile[]
  const recovery = createCartRecovery(
    () => files,
    (next) => (files = next),
  )
  recovery.accept([])
  expect(fetch).toHaveBeenCalledTimes(4)
  reject(Object.assign(new Error('missing'), { status: 404 }))
  await settle()
  expect(files[3].minutesJob).toBeUndefined()
  expect(fetch).toHaveBeenCalledTimes(5)
  recovery.dispose()
})
