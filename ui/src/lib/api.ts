export type JobStatus = 'queued' | 'preparing' | 'running' | 'completed' | 'failed' | 'cancelled' | 'interrupted'
export interface Job {
  id: string; path: string; name: string; status: JobStatus; phase: string
  model: string; effort: string; result: string; recentOutput: string
  outputPath: string; error: string; createdAt: string; startedAt?: string; completedAt?: string; prompt: string
}
export interface Snapshot { jobs: Job[]; codexReady: boolean; whisperReady: boolean; whisperInstalling: boolean; whisperError: string }
export interface Config { listen: string; codexBinary: string; outputDir: string; whisperModel: string; ffmpegBinary: string; prompt: string }
export interface BinaryEnvironment { path: string; ready: boolean; version?: string; error?: string }
export interface Environment {
  codex: BinaryEnvironment
  whisper: { model: string; modelPath: string; modelReady: boolean; binaryPath: string; binaryReady: boolean; installerVersion: string }
  ffmpeg: BinaryEnvironment
}
export interface Entry { name: string; path: string; isDir: boolean; size: number }
export interface BrowseResult { path: string; parent: string; entries: Entry[] }
export interface Model { id: string; model: string; displayName: string; supportedReasoningEfforts: { reasoningEffort: string }[] }

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.method && init.method !== 'GET') headers.set('X-Meet-To-MD', '1')
  if (init.body && !(init.body instanceof FormData)) headers.set('Content-Type', 'application/json')
  const response = await fetch(`/api${path}`, { ...init, headers })
  if (!response.ok) {
    const body = await response.json().catch(() => ({ error: response.statusText }))
    throw new Error(body.error || `HTTP ${response.status}`)
  }
  if (response.status === 204) return undefined as T
  return response.json() as Promise<T>
}
export const api = {
  state: () => request<Snapshot>('/state'),
  config: () => request<Config>('/config'),
  environment: () => request<Environment>('/environment'),
  saveConfig: (value: Config) => request<Config>('/config', { method: 'PUT', body: JSON.stringify(value) }),
  browse: (path = '') => request<BrowseResult>(`/browse?path=${encodeURIComponent(path)}`),
  expand: (paths: string[]) => request<{ paths: string[] }>('/expand', { method: 'POST', body: JSON.stringify({ paths }) }),
  preview: (path: string) => request<{ text: string }>(`/preview?path=${encodeURIComponent(path)}`),
  importFiles: async (files: FileList | File[]) => {
    const data = new FormData()
    for (const file of Array.from(files)) data.append('files', file, file.name)
    return request<{ paths: string[] }>('/import', { method: 'POST', body: data })
  },
  createJobs: (paths: string[], model: string, effort: string) => request<{ jobs: Job[] }>('/jobs', { method: 'POST', body: JSON.stringify({ paths, model, effort }) }),
  deleteJob: (id: string) => request<void>(`/jobs/${id}`, { method: 'DELETE' }),
  cancelJob: (id: string) => request<Snapshot>(`/jobs/${id}/cancel`, { method: 'POST' }),
  models: () => request<{ models: Model[] }>('/models'),
  testCodex: (model: string) => request<{ result: string; models: Model[] }>('/codex/test', { method: 'POST', body: JSON.stringify({ model }) }),
  installWhisper: () => request<{ status: string }>('/whisper/install', { method: 'POST' }),
}
