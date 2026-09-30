import { get, writable } from 'svelte/store'
import { api, type Job } from './api'
const empty = () => ({ id: '', full: null as Job | null, transcript: '' })

/** Full prompts/results are fetched on demand, including active-job prompts omitted by SSE. */
export function createJobDetail(onError: (message: string) => void) {
  const state = writable(empty())
  let version = 0
  async function load(job: Job) {
    const request = ++version
    const current = () => request === version && get(state).id === job.id
    const full = api
      .job(job.id)
      .then((value) => {
        if (current()) state.update((previous) => ({ ...previous, full: value }))
      })
      .catch((error) => {
        if (current()) onError('작업 상세 정보를 읽지 못했습니다: ' + (error as Error).message)
      })
    const transcribing =
      job.kind === 'transcription' ||
      ['extracting', 'transcribing', 'transcribed'].includes(job.stage || '')
    if (transcribing) {
      try {
        const value = await api.jobTranscript(job.id)
        if (current()) state.update((previous) => ({ ...previous, transcript: value.text }))
      } catch {
        if (current()) state.update((previous) => ({ ...previous, transcript: '' }))
      }
    }
    await full
  }
  return {
    subscribe: state.subscribe,
    open(job: Job) {
      state.set({ ...empty(), id: job.id })
      void load(job)
    },
    refresh(job: Job) {
      state.update((value) => ({ ...value, full: null }))
      void load(job)
    },
    close() {
      version++
      state.set(empty())
    },
    dispose() {
      version++
    },
  }
}
