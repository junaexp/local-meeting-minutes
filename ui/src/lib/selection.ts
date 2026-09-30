import { get, writable } from 'svelte/store'
import { api, type Job } from './api'
import { isActive, isMedia } from './cart'
const empty = () => ({
  path: '',
  preview: '파일을 선택하면 여기에 원문을 표시합니다.',
  transcribed: false,
  loading: false,
  fullJob: null as Job | null,
})

/** Version every selection so late responses cannot resurrect removed files. */
export function createSelection() {
  const state = writable(empty())
  let previewRequest = 0
  let mediaRequest = 0
  async function load(path: string) {
    const request = ++previewRequest
    state.update((value) => ({ ...value, loading: true }))
    try {
      const result = await api.preview(path)
      if (request === previewRequest && get(state).path === path)
        state.update((value) => ({
          ...value,
          preview: result.text,
          transcribed: result.transcribed,
        }))
    } catch (error) {
      if (request === previewRequest && get(state).path === path)
        state.update((value) => ({ ...value, preview: (error as Error).message }))
    } finally {
      if (request === previewRequest && get(state).path === path)
        state.update((value) => ({ ...value, loading: false }))
    }
  }
  async function loadJob(job: Job) {
    const request = ++mediaRequest
    try {
      const fullJob = await api.job(job.id)
      if (request === mediaRequest && get(state).path === job.path)
        state.update((value) => ({ ...value, fullJob }))
    } catch {
      /* The saved preview remains usable if old job history is deleted. */
    }
  }
  function clear() {
    previewRequest++
    mediaRequest++
    state.set(empty())
  }
  return {
    subscribe: state.subscribe,
    load,
    loadJob,
    clear,
    select(path: string, job?: Job) {
      previewRequest++
      mediaRequest++
      state.set({ ...empty(), path, preview: '' })
      void load(path)
      if (isMedia(path) && job && !isActive(job)) void loadJob(job)
    },
    dispose() {
      previewRequest++
      mediaRequest++
    },
  }
}
