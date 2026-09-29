export type JobStatus = 'queued' | 'preparing' | 'running' | 'completed' | 'failed' | 'cancelled' | 'interrupted'
export interface Job {
  id: string; kind?: 'minutes' | 'transcription'; path: string; name: string; status: JobStatus; stage?: string; phase: string
  model: string; effort: string; result: string; recentOutput: string
  transcriptionModel?: string; transcriptKey?: string; transcriptPath?: string; transcriptPreview?: string; transcriptionLog?: string
  outputPath: string; error: string; createdAt: string; startedAt?: string; completedAt?: string; prompt: string
}
export interface InstallProgress { model: string; stage: string; message: string; downloaded: number; total: number; log: string }
export interface Snapshot { jobs: Job[]; codexReady: boolean; whisperReady: boolean; whisperInstalling: boolean; whisperError: string; whisperInstall?: InstallProgress }
export interface Config { listen: string; codexBinary: string; outputDir: string; whisperModel: string; ffmpegBinary: string; prompt: string }
export interface BinaryEnvironment { path: string; ready: boolean; version?: string; error?: string }
export interface Environment {
  codex: BinaryEnvironment
  whisper: { model: string; modelPath: string; modelReady: boolean; binaryPath: string; binaryReady: boolean; installerVersion: string }
  ffmpeg: BinaryEnvironment
}
export interface Entry { name: string; path: string; isDir: boolean; size: number }
export interface BrowseResult { path: string; parent: string; entries: Entry[] }
export interface PreviewResult { text: string; transcribed: boolean }
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
  preview: (path: string) => request<PreviewResult>(`/preview?path=${encodeURIComponent(path)}`),
  importFiles: async (files: FileList | File[]) => {
    const data = new FormData()
    for (const file of Array.from(files)) data.append('files', file, file.name)
    return request<{ paths: string[] }>('/import', { method: 'POST', body: data })
  },
  createJobs: (paths: string[], model: string, effort: string) => request<{ jobs: Job[] }>('/jobs', { method: 'POST', body: JSON.stringify({ paths, model, effort }) }),
  createTranscription: (path: string) => request<{ job: Job }>('/transcriptions', { method: 'POST', body: JSON.stringify({ path }) }),
  job: (id: string) => request<Job>(`/jobs/${id}`),
  deleteJob: (id: string) => request<void>(`/jobs/${id}`, { method: 'DELETE' }),
  deleteFinishedJobs: () => request<{ deleted: number; state: Snapshot }>('/jobs', { method: 'DELETE' }),
  cancelJob: (id: string) => request<Snapshot>(`/jobs/${id}/cancel`, { method: 'POST' }),
  jobTranscript: (id: string) => request<{ text: string }>(`/jobs/${id}/transcript`),
  models: () => request<{ models: Model[] }>('/models'),
  testCodex: (model: string) => request<{ result: string; models: Model[] }>('/codex/test', { method: 'POST', body: JSON.stringify({ model }) }),
  installWhisper: () => request<{ status: string }>('/whisper/install', { method: 'POST' }),
}
