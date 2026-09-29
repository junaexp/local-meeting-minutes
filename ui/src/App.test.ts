import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import App from './App.svelte'
import { api, type Job, type Snapshot } from '$lib/api'

vi.mock('$lib/api', () => ({ api: {
  state: vi.fn(), config: vi.fn(), environment: vi.fn(), models: vi.fn(), browse: vi.fn(), expand: vi.fn(), preview: vi.fn(),
  importFiles: vi.fn(), createJobs: vi.fn(), createTranscription: vi.fn(), saveConfig: vi.fn(), testCodex: vi.fn(), installWhisper: vi.fn(),
  deleteJob: vi.fn(), deleteFinishedJobs: vi.fn(), cancelJob: vi.fn(), job: vi.fn(), jobTranscript: vi.fn(),
} }))

class FakeEventSource {
  static latest: FakeEventSource
  private handlers = new Map<string, (event: MessageEvent) => void>()
  onerror?: () => void
  onopen?: () => void
  constructor(_url: string) { FakeEventSource.latest = this }
  addEventListener(name: string, handler: EventListener) { this.handlers.set(name, handler as (event: MessageEvent) => void) }
  emit(name: string, value: unknown) { this.handlers.get(name)?.({ data: JSON.stringify(value) } as MessageEvent) }
  fail() { this.onerror?.() }
  close() {}
}

const baseState: Snapshot = { jobs: [], codexReady: true, whisperReady: false, whisperInstalling: false, whisperError: '' }
beforeEach(() => {
  window.localStorage.clear()
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  vi.mocked(api.state).mockResolvedValue(baseState)
  vi.mocked(api.config).mockResolvedValue({ listen: '127.0.0.1:8791', codexBinary: 'codex', outputDir: '', whisperModel: 'large-v3-turbo', ffmpegBinary: 'ffmpeg', prompt: '한국어로 회의록 작성' })
  vi.mocked(api.environment).mockResolvedValue({
    codex: { path: '/usr/local/bin/codex', ready: true },
    whisper: { model: 'large-v3-turbo', modelPath: '/tmp/whisper/models/ggml-large-v3-turbo.bin', modelReady: false, binaryPath: '', binaryReady: false, installerVersion: 'v1.8.3' },
    ffmpeg: { path: '/usr/local/bin/ffmpeg', ready: true, version: '8.0' },
  })
  vi.mocked(api.models).mockResolvedValue({ models: [{ id: 'gpt-5.6-sol', model: 'gpt-5.6-sol', displayName: 'Sol', supportedReasoningEfforts: ['low','medium','high','xhigh'].map(reasoningEffort => ({ reasoningEffort })) }] })
  vi.mocked(api.browse).mockResolvedValue({ path: '/tmp/회의', parent: '/tmp', entries: [{ name: 'a.srt', path: '/tmp/회의/a.srt', isDir: false, size: 12 }, { name: 'b.vtt', path: '/tmp/회의/b.vtt', isDir: false, size: 12 }] })
  vi.mocked(api.expand).mockResolvedValue({ paths: ['/tmp/회의/a.srt', '/tmp/회의/b.vtt'] })
  vi.mocked(api.preview).mockResolvedValue({ text: '김: 안녕하세요', transcribed: false })
  vi.mocked(api.jobTranscript).mockResolvedValue({ text: '1\n00:00:01,000 --> 00:00:02,000\n안녕하세요' })
  vi.mocked(api.createJobs).mockResolvedValue({ jobs: [] })
  vi.mocked(api.testCodex).mockResolvedValue({ result: 'Hello world!', models: [] })
  vi.mocked(api.installWhisper).mockResolvedValue({ status: 'installing' })
  vi.mocked(api.deleteFinishedJobs).mockResolvedValue({ deleted: 0, state: baseState })
})
afterEach(() => { cleanup(); vi.restoreAllMocks(); vi.clearAllMocks(); vi.unstubAllGlobals() })

function transcriptionJob(status: Job['status'], log = ''): Job {
  return {
    id: 'recover-1', kind: 'transcription', path: '/tmp/회의/recover.mp4', name: 'recover.mp4', status,
    stage: status === 'completed' ? 'completed' : 'transcribing', phase: status === 'completed' ? '전사 완료' : 'Whisper 전사 중',
    model: '', effort: '', transcriptionModel: 'large-v3-turbo', transcriptionLog: log,
    result: '', recentOutput: '', outputPath: '', error: '', prompt: '', createdAt: '2026-09-29T00:00:00Z',
  }
}

describe('workspace flow', () => {
  it('recovers the completed job when the live connection fails, then clears the warning on reconnect', async () => {
    vi.mocked(api.state)
      .mockResolvedValueOnce({ ...baseState, jobs: [transcriptionJob('running')] })
      .mockResolvedValueOnce({ ...baseState, jobs: [transcriptionJob('completed', '전사 완료 로그')], whisperReady: true })
    render(App)
    await screen.findByText('Whisper 전사 중')
    FakeEventSource.latest.fail()
    await waitFor(() => expect(api.state).toHaveBeenCalledTimes(2))
    expect(await screen.findByText('전사 완료')).toBeTruthy()
    expect(screen.getByRole('status').textContent).toContain('실시간 연결을 다시 확인')
    await fireEvent.click(screen.getByRole('button', { name: '설정 열기' }))
    expect(await screen.findByRole('dialog')).toBeTruthy()
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [transcriptionJob('completed', '전사 완료 로그')] })
    await waitFor(() => expect(screen.queryByRole('status')).toBeNull())
  })
  it('shows malformed stream data and restores state through the HTTP endpoint', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {})
    vi.mocked(api.state)
      .mockResolvedValueOnce({ ...baseState, jobs: [transcriptionJob('running')] })
      .mockResolvedValueOnce({ ...baseState, jobs: [transcriptionJob('completed', '전사 완료 로그')] })
    render(App)
    await screen.findByText('Whisper 전사 중')
    FakeEventSource.latest.emit('state', { ...baseState, jobs: { broken: true } })
    expect((await screen.findByRole('status')).textContent).toContain('실시간 상태를 읽지 못했습니다')
    expect(await screen.findByText('전사 완료')).toBeTruthy()
    expect(console.error).toHaveBeenCalled()
  })
  it('checks current job state after stream silence', async () => {
    let watchdog: (() => void) | undefined
    const setInterval = window.setInterval.bind(window)
    vi.spyOn(window, 'setInterval').mockImplementation((handler, timeout, ...args) => {
      if (timeout === 5_000) { watchdog = handler as () => void; return 987654 as number }
      return setInterval(handler, timeout, ...args)
    })
    vi.mocked(api.state)
      .mockResolvedValueOnce({ ...baseState, jobs: [transcriptionJob('running')] })
      .mockResolvedValueOnce({ ...baseState, jobs: [transcriptionJob('completed')] })
    render(App)
    await screen.findByText('Whisper 전사 중')
    const now = Date.now()
    vi.spyOn(Date, 'now').mockReturnValue(now + 46_000)
    watchdog?.()
    expect(await screen.findByText('전사 완료')).toBeTruthy()
    expect(screen.getByRole('status').textContent).toContain('실시간 연결을 다시 확인')
  })
  it('keeps a newer stream state when the initial HTTP response arrives late', async () => {
    let resolveInitial!: (state: Snapshot) => void
    vi.mocked(api.state).mockReturnValueOnce(new Promise(resolve => { resolveInitial = resolve }))
    render(App)
    FakeEventSource.latest.emit('state', { ...baseState, jobs: [transcriptionJob('completed')] })
    resolveInitial({ ...baseState, jobs: [transcriptionJob('running')] })
    await screen.findByDisplayValue('한국어로 회의록 작성')
    expect(screen.getByText('전사 완료')).toBeTruthy()
    expect(screen.queryByText('Whisper 전사 중')).toBeNull()
  })
  it('refreshes the transcription log after the start request even without a stream event', async () => {
    const media = '/tmp/회의/recover.mp4'
    vi.mocked(api.state)
      .mockResolvedValueOnce({ ...baseState, whisperReady: true })
      .mockResolvedValueOnce({ ...baseState, whisperReady: true, jobs: [transcriptionJob('running', '12:00:00  FFmpeg 음성 추출 시작\n')] })
    vi.mocked(api.expand).mockResolvedValue({ paths: [media] })
    vi.mocked(api.preview).mockResolvedValue({ text: '미디어 파일입니다.', transcribed: false })
    vi.mocked(api.createTranscription).mockResolvedValue({ job: transcriptionJob('running') })
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '폴더 선택' }))
    await fireEvent.click(await screen.findByRole('button', { name: '현재 폴더 추가' }))
    await fireEvent.click(await screen.findByRole('button', { name: '전사 시작' }))
    await waitFor(() => expect(api.state).toHaveBeenCalledTimes(2))
    await waitFor(() => expect((screen.getByRole('textbox', { name: 'Whisper 전사 로그' }) as HTMLTextAreaElement).value).toContain('FFmpeg 음성 추출 시작'))
  })
  it('selects a folder, previews content, reorders files and confirms enqueue', async () => {
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '폴더 선택' }))
    await screen.findByRole('dialog')
    await fireEvent.click(screen.getByRole('button', { name: '현재 폴더 추가' }))
    await screen.findByText('김: 안녕하세요')
    expect(screen.getByText('2개 파일')).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: 'a.srt 아래로' }))
    await fireEvent.click(screen.getByRole('button', { name: '회의록 정리 시작' }))
    await waitFor(() => expect(api.createJobs).toHaveBeenCalledWith(['/tmp/회의/b.vtt', '/tmp/회의/a.srt'], 'gpt-5.6-sol', 'medium'))
    expect(window.confirm).toHaveBeenCalled()
  })
  it('opens settings and runs the Luna response test', async () => {
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '설정 열기' }))
    await screen.findByRole('dialog')
    expect(await screen.findByText('/tmp/whisper/models/ggml-large-v3-turbo.bin')).toBeTruthy()
    expect(screen.getByText('설치된 버전: 8.0')).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: 'GPT-5.6 Luna로 테스트' }))
    await waitFor(() => expect(api.testCodex).toHaveBeenCalledWith('gpt-5.6-luna'))
    expect(await screen.findByText('모델 응답: Hello world!')).toBeTruthy()
  })
  it('shows Whisper installation bytes, model and live log', async () => {
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '설치' }))
    await waitFor(() => expect(api.installWhisper).toHaveBeenCalled())
    await screen.findByRole('dialog')
    FakeEventSource.latest.emit('state', {
      ...baseState, whisperInstalling: true,
      whisperInstall: { model: 'large-v3-turbo', stage: 'model_download', message: 'Whisper large-v3-turbo 모델 다운로드 중', downloaded: 50, total: 100, log: '다운로드 시작\n' },
    })
    expect(await screen.findByText('모델: large-v3-turbo')).toBeTruthy()
    expect(screen.getByRole('progressbar', { name: 'Whisper 설치 다운로드 진행률' }).getAttribute('aria-valuenow')).toBe('50')
    expect((screen.getByRole('textbox', { name: 'Whisper 설치 로그' }) as HTMLTextAreaElement).value).toContain('다운로드 시작')
    FakeEventSource.latest.emit('state', {
      ...baseState, whisperReady: true,
      whisperInstall: { model: 'large-v3-turbo', stage: 'completed', message: '설치 완료', downloaded: 0, total: 0, log: '설치 완료\n' },
    })
    expect(await screen.findByText('설치 완료')).toBeTruthy()
  })
  it('shows live Codex result and recent output in the job detail', async () => {
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    FakeEventSource.latest.emit('state', { ...baseState, jobs: [{
      id: 'job1', path: '/tmp/회의/a.srt', name: 'a.srt', status: 'running', phase: '작성 중', model: 'gpt-5.6-sol', effort: 'low',
      result: '# 회의록', recentOutput: '10:00 작업 시작', outputPath: '', error: '', prompt: '한국어로 회의록 작성', createdAt: '2026-09-29T00:00:00Z',
    }] })
    await fireEvent.click(await screen.findByRole('button', { name: /a.srt/ }))
    await waitFor(() => expect((screen.getByRole('textbox', { name: 'Codex 결과' }) as HTMLTextAreaElement).value).toContain('# 회의록'))
    expect((screen.getByRole('textbox', { name: '최근 출력' }) as HTMLTextAreaElement).value).toContain('10:00 작업 시작')
  })
  it('shows a saved SRT path when video minutes fail', async () => {
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    FakeEventSource.latest.emit('state', { ...baseState, jobs: [{
      id: 'video1', kind: 'minutes', path: '/tmp/회의/meeting.mp4', name: 'meeting.mp4', status: 'failed', phase: '회의록 실패 · SRT 저장됨',
      model: 'gpt-5.6-sol', effort: 'low', transcriptPath: '/tmp/회의/meeting_전사.srt', result: '', recentOutput: '', outputPath: '',
      error: 'Codex 실행 파일을 찾을 수 없습니다', prompt: '한국어로 회의록 작성', createdAt: '2026-09-29T00:00:00Z',
    }] })
    await fireEvent.click(await screen.findByRole('button', { name: /meeting.mp4/ }))
    expect(await screen.findByText(/SRT 저장 위치: \/tmp\/회의\/meeting_전사.srt/)).toBeTruthy()
    expect(screen.getByText('Codex 실행 파일을 찾을 수 없습니다')).toBeTruthy()
  })
  it('loads long output only when a completed job detail is opened', async () => {
    const slim = { ...transcriptionJob('completed'), transcriptionLog: '', transcriptPath: '/tmp/회의/recover_전사.srt' }
    vi.mocked(api.job).mockResolvedValue({ ...slim, transcriptionLog: 'Whisper 상세 로그' })
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    FakeEventSource.latest.emit('state', { ...baseState, jobs: [slim] })
    await fireEvent.click(await screen.findByRole('button', { name: /recover.mp4/ }))
    await waitFor(() => expect(api.job).toHaveBeenCalledWith(slim.id))
    await waitFor(() => expect((screen.getByRole('textbox', { name: 'Whisper 전사 로그' }) as HTMLTextAreaElement).value).toBe('Whisper 상세 로그'))
  })
  it('starts media transcription and shows file-specific live output', async () => {
    const media = '/tmp/회의/제품회의.mp4'
    const job: Job = {
      id: 'transcription-1', kind: 'transcription', path: media, name: '제품회의.mp4', status: 'running', stage: 'transcribing', phase: 'Whisper 전사 중',
      model: '', effort: '', transcriptionModel: 'large-v3-turbo', transcriptPreview: '[00:00:01.000 --> 00:00:02.000] 안녕하세요', transcriptionLog: '10:00:00  Whisper 전사 시작\n',
      result: '', recentOutput: '', outputPath: '', error: '', prompt: '', createdAt: '2026-09-29T00:00:00Z',
    }
    vi.mocked(api.state).mockResolvedValue({ ...baseState, whisperReady: true })
    vi.mocked(api.expand).mockResolvedValue({ paths: [media] })
    vi.mocked(api.preview).mockResolvedValue({ text: '미디어 파일입니다.', transcribed: false })
    vi.mocked(api.createTranscription).mockResolvedValue({ job })
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '폴더 선택' }))
    await fireEvent.click(await screen.findByRole('button', { name: '현재 폴더 추가' }))
    await fireEvent.click(await screen.findByRole('button', { name: '전사 시작' }))
    await waitFor(() => expect(api.createTranscription).toHaveBeenCalledWith(media))
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [job] })
    await waitFor(() => expect((screen.getByRole('textbox', { name: '전사문 미리보기' }) as HTMLTextAreaElement).value).toContain('안녕하세요'))
    expect((screen.getByRole('textbox', { name: 'Whisper 전사 로그' }) as HTMLTextAreaElement).value).toContain('Whisper 전사 시작')
    vi.mocked(api.preview).mockResolvedValue({ text: '1\n00:00:01,000 --> 00:00:02,000\n안녕하세요', transcribed: true })
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [{ ...job, status: 'completed', stage: 'completed', phase: '전사 완료', transcriptPreview: '' }] })
    await waitFor(() => expect((screen.getByRole('textbox', { name: '전사문 미리보기' }) as HTMLTextAreaElement).value).toContain('00:00:01,000'))
    await fireEvent.click(document.querySelector('.job-card') as HTMLButtonElement)
    await waitFor(() => expect(api.jobTranscript).toHaveBeenCalledWith(job.id))
    expect((screen.getByRole('textbox', { name: '전사문' }) as HTMLTextAreaElement).value).toContain('00:00:01,000')
  })
  it('adds the exported SRT after its disabled source and submits only active files', async () => {
    const media = '/tmp/회의/meeting.mp4'
    const srt = '/tmp/회의/meeting_전사.srt'
    const job = { ...transcriptionJob('running'), path: media, name: 'meeting.mp4', id: 'export-1' }
    vi.mocked(api.state).mockResolvedValue({ ...baseState, whisperReady: true })
    vi.mocked(api.expand).mockResolvedValue({ paths: [media] })
    vi.mocked(api.preview).mockResolvedValue({ text: '전사문', transcribed: true })
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '폴더 선택' }))
    await fireEvent.click(await screen.findByRole('button', { name: '현재 폴더 추가' }))
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [job] })
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [{ ...job, status: 'completed', stage: 'completed', phase: '전사 완료', transcriptPath: srt }] })
    expect(await screen.findByTitle(srt)).toBeTruthy()
    expect(screen.getByTitle(media).closest('.cart-item')?.classList.contains('disabled')).toBe(true)
    expect(screen.getByText('전사 완료 · 회의록 처리 제외')).toBeTruthy()
    expect(screen.getByText('1개 파일')).toBeTruthy()
    await fireEvent.click(screen.getByRole('button', { name: '회의록 정리 시작' }))
    await waitFor(() => expect(api.createJobs).toHaveBeenCalledWith([srt], 'gpt-5.6-sol', 'medium'))
  })
  it('restores the processing list after a reload and attaches a finished transcript', async () => {
    const media = '/tmp/회의/reloaded.mp4'
    const srt = '/tmp/회의/reloaded_전사.srt'
    window.localStorage.setItem('meet-to-md-cart-v1', JSON.stringify([{ path: media, name: 'reloaded.mp4' }]))
    vi.mocked(api.state).mockResolvedValue({ ...baseState, whisperReady: true, jobs: [{ ...transcriptionJob('completed'), path: media, name: 'reloaded.mp4', transcriptPath: srt }] })
    render(App)
    expect(await screen.findByTitle(srt)).toBeTruthy()
    expect(screen.getByTitle(media).closest('.cart-item')?.classList.contains('disabled')).toBe(true)
    expect(JSON.parse(window.localStorage.getItem('meet-to-md-cart-v1') || '[]')).toHaveLength(2)
  })
  it('pins transcript and Whisper log independently and lets the user pause scrolling', async () => {
    const media = '/tmp/회의/scroll.mp4'
    const job = { ...transcriptionJob('running', '첫 로그'), path: media, name: 'scroll.mp4', transcriptPreview: '첫 문장' }
    vi.mocked(api.state).mockResolvedValue({ ...baseState, whisperReady: true })
    vi.mocked(api.expand).mockResolvedValue({ paths: [media] })
    vi.mocked(api.preview).mockResolvedValue({ text: '전사 중', transcribed: false })
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '폴더 선택' }))
    await fireEvent.click(await screen.findByRole('button', { name: '현재 폴더 추가' }))
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [job] })
    await waitFor(() => expect((screen.getByRole('textbox', { name: '전사문 미리보기' }) as HTMLTextAreaElement).value).toBe('첫 문장'))
    const preview = screen.getByRole('textbox', { name: '전사문 미리보기' }) as HTMLTextAreaElement
    const log = screen.getByRole('textbox', { name: 'Whisper 전사 로그' }) as HTMLTextAreaElement
    for (const area of [preview, log]) {
      Object.defineProperty(area, 'scrollHeight', { configurable: true, value: 900 })
      Object.defineProperty(area, 'clientHeight', { configurable: true, value: 100 })
      area.scrollTop = 0
    }
    const previewPin = screen.getByRole('button', { name: '전사문 미리보기 하단 고정' })
    const logPin = screen.getByRole('button', { name: 'Whisper 전사 로그 하단 고정' })
    expect(previewPin.getAttribute('aria-pressed')).toBe('true')
    expect(logPin.getAttribute('aria-pressed')).toBe('true')
    await fireEvent.click(previewPin)
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [{ ...job, transcriptPreview: '둘째 문장', transcriptionLog: '둘째 로그' }] })
    await waitFor(() => expect(log.scrollTop).toBe(900))
    expect(preview.scrollTop).toBe(0)
    expect(previewPin.getAttribute('aria-pressed')).toBe('false')
    await fireEvent.click(previewPin)
    expect(preview.scrollTop).toBe(900)
    await fireEvent.click(logPin)
    log.scrollTop = 100
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [{ ...job, transcriptPreview: '셋째 문장', transcriptionLog: '셋째 로그' }] })
    await waitFor(() => expect(log.value).toBe('셋째 로그'))
    expect(log.scrollTop).toBe(100)
    expect(logPin.getAttribute('aria-pressed')).toBe('false')
  })
  it('stops an active transcription from the processing list', async () => {
    const media = '/tmp/회의/stop.mp4'
    const running = { ...transcriptionJob('running'), path: media, name: 'stop.mp4', id: 'stop-1' }
    const stopped = { ...baseState, whisperReady: true, jobs: [{ ...running, status: 'cancelled' as const, stage: 'cancelled', phase: '취소됨' }] }
    vi.mocked(api.state).mockResolvedValueOnce({ ...baseState, whisperReady: true }).mockResolvedValue(stopped)
    vi.mocked(api.expand).mockResolvedValue({ paths: [media] })
    vi.mocked(api.cancelJob).mockResolvedValue(stopped)
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '폴더 선택' }))
    await fireEvent.click(await screen.findByRole('button', { name: '현재 폴더 추가' }))
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [running] })
    await fireEvent.click(await screen.findByRole('button', { name: '전사 중지' }))
    await waitFor(() => expect(api.cancelJob).toHaveBeenCalledWith('stop-1'))
    expect(await screen.findByText('취소됨')).toBeTruthy()
  })
  it('shows only the selected media file log', async () => {
    const first = '/tmp/회의/first.mp4'
    const second = '/tmp/회의/second.mp4'
    const makeJob = (id: string, path: string, log: string): Job => ({
      id, kind: 'transcription', path, name: path.split('/').pop() || path, status: 'running', stage: 'transcribing', phase: 'Whisper 전사 중',
      model: '', effort: '', transcriptionModel: 'large-v3-turbo', transcriptionLog: log, result: '', recentOutput: '', outputPath: '', error: '', prompt: '', createdAt: '2026-09-29T00:00:00Z',
    })
    vi.mocked(api.state).mockResolvedValue({ ...baseState, whisperReady: true })
    vi.mocked(api.expand).mockResolvedValue({ paths: [first, second] })
    vi.mocked(api.preview).mockResolvedValue({ text: '전사 중', transcribed: false })
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '폴더 선택' }))
    await fireEvent.click(await screen.findByRole('button', { name: '현재 폴더 추가' }))
    FakeEventSource.latest.emit('state', { ...baseState, whisperReady: true, jobs: [makeJob('first', first, '첫 파일 로그'), makeJob('second', second, '둘째 파일 로그')] })
    await waitFor(() => expect((screen.getByRole('textbox', { name: 'Whisper 전사 로그' }) as HTMLTextAreaElement).value).toBe('첫 파일 로그'))
    await fireEvent.click(screen.getByTitle(second))
    await waitFor(() => expect((screen.getByRole('textbox', { name: 'Whisper 전사 로그' }) as HTMLTextAreaElement).value).toBe('둘째 파일 로그'))
  })
  it('refreshes a selected transcript when the Whisper model changes', async () => {
    const media = '/tmp/회의/model-change.mp4'
    const nextConfig = { listen: '127.0.0.1:8791', codexBinary: 'codex', outputDir: '', whisperModel: 'base', ffmpegBinary: 'ffmpeg', prompt: '한국어로 회의록 작성' }
    vi.mocked(api.state).mockResolvedValue({ ...baseState, whisperReady: true })
    vi.mocked(api.expand).mockResolvedValue({ paths: [media] })
    vi.mocked(api.preview).mockResolvedValueOnce({ text: '기존 모델 SRT', transcribed: true }).mockResolvedValue({ text: '새 모델 전사가 필요합니다.', transcribed: false })
    vi.mocked(api.saveConfig).mockResolvedValue(nextConfig)
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    await fireEvent.click(screen.getByRole('button', { name: '폴더 선택' }))
    await fireEvent.click(await screen.findByRole('button', { name: '현재 폴더 추가' }))
    await waitFor(() => expect((screen.getByRole('textbox', { name: '전사문 미리보기' }) as HTMLTextAreaElement).value).toContain('기존 모델 SRT'))
    await fireEvent.click(screen.getByRole('button', { name: '설정 열기' }))
    await fireEvent.change(screen.getByLabelText('Whisper 모델'), { target: { value: 'base' } })
    await fireEvent.click(screen.getByRole('button', { name: '설정 저장' }))
    await waitFor(() => expect((screen.getByRole('textbox', { name: '전사문 미리보기' }) as HTMLTextAreaElement).value).toContain('새 모델 전사가 필요합니다.'))
    expect(screen.getByRole('button', { name: '전사 시작' }).hasAttribute('disabled')).toBe(false)
  })
  it('turns completion notifications off and on while preserving the app preference', async () => {
    class TestNotification {
      static permission: NotificationPermission = 'granted'
      static requestPermission = vi.fn(async () => TestNotification.permission)
      constructor(_title: string, _options?: NotificationOptions) {}
    }
    vi.stubGlobal('Notification', TestNotification)
    window.localStorage.setItem('meet-to-md-notifications-enabled', '1')
    render(App)
    await screen.findByDisplayValue('한국어로 회의록 작성')
    const off = screen.getByRole('button', { name: '알림 끄기' })
    expect(off.getAttribute('aria-pressed')).toBe('true')
    await fireEvent.click(off)
    expect(window.localStorage.getItem('meet-to-md-notifications-enabled')).toBeNull()
    const on = screen.getByRole('button', { name: '알림 켜기' })
    expect(on.getAttribute('aria-pressed')).toBe('false')
    await fireEvent.click(on)
    expect(window.localStorage.getItem('meet-to-md-notifications-enabled')).toBe('1')
    expect(screen.getByRole('button', { name: '알림 끄기' }).getAttribute('aria-pressed')).toBe('true')
  })
  it('clears finished history while keeping active jobs', async () => {
    const finished = { ...transcriptionJob('completed'), id: 'old-1', name: 'old.mp4', path: '/tmp/회의/old.mp4' }
    const active = { ...transcriptionJob('running'), id: 'live-1', name: 'live.mp4', path: '/tmp/회의/live.mp4' }
    const initial = { ...baseState, whisperReady: true, jobs: [finished, active] }
    const remaining = { ...baseState, whisperReady: true, jobs: [active] }
    vi.mocked(api.state).mockResolvedValue(initial)
    vi.mocked(api.deleteFinishedJobs).mockResolvedValue({ deleted: 1, state: remaining })
    render(App)
    await screen.findByText('old.mp4')
    const clear = screen.getByRole('button', { name: '완료 기록 지우기' }) as HTMLButtonElement
    expect(clear.disabled).toBe(false)
    await fireEvent.click(clear)
    await waitFor(() => expect(api.deleteFinishedJobs).toHaveBeenCalledTimes(1))
    expect(window.confirm).toHaveBeenCalledWith('완료·실패·중단 기록을 모두 지울까요?\n생성된 SRT와 Markdown 파일은 삭제되지 않습니다.')
    await waitFor(() => expect(screen.queryByText('old.mp4')).toBeNull())
    expect(screen.getByText('live.mp4')).toBeTruthy()
    expect(clear.disabled).toBe(true)
  })
})
