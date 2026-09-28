import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import App from './App.svelte'
import { api, type Snapshot } from '$lib/api'

vi.mock('$lib/api', () => ({ api: {
  state: vi.fn(), config: vi.fn(), models: vi.fn(), browse: vi.fn(), expand: vi.fn(), preview: vi.fn(),
  importFiles: vi.fn(), createJobs: vi.fn(), saveConfig: vi.fn(), testCodex: vi.fn(), installWhisper: vi.fn(),
  deleteJob: vi.fn(), cancelJob: vi.fn(),
} }))

class FakeEventSource {
  static latest: FakeEventSource
  private handlers = new Map<string, (event: MessageEvent) => void>()
  onerror?: () => void
  onopen?: () => void
  constructor(_url: string) { FakeEventSource.latest = this }
  addEventListener(name: string, handler: EventListener) { this.handlers.set(name, handler as (event: MessageEvent) => void) }
  emit(name: string, value: unknown) { this.handlers.get(name)?.({ data: JSON.stringify(value) } as MessageEvent) }
  close() {}
}

const baseState: Snapshot = { jobs: [], codexReady: true, whisperReady: false, whisperInstalling: false, whisperError: '' }
beforeEach(() => {
  vi.stubGlobal('EventSource', FakeEventSource)
  vi.spyOn(window, 'confirm').mockReturnValue(true)
  vi.mocked(api.state).mockResolvedValue(baseState)
  vi.mocked(api.config).mockResolvedValue({ listen: '127.0.0.1:8791', codexBinary: 'codex', outputDir: '', whisperModel: 'base', ffmpegBinary: 'ffmpeg', prompt: '한국어로 회의록 작성' })
  vi.mocked(api.models).mockResolvedValue({ models: [{ id: 'gpt-5.6-sol', model: 'gpt-5.6-sol', displayName: 'Sol', supportedReasoningEfforts: ['low','medium','high','xhigh'].map(reasoningEffort => ({ reasoningEffort })) }] })
  vi.mocked(api.browse).mockResolvedValue({ path: '/tmp/회의', parent: '/tmp', entries: [{ name: 'a.srt', path: '/tmp/회의/a.srt', isDir: false, size: 12 }, { name: 'b.vtt', path: '/tmp/회의/b.vtt', isDir: false, size: 12 }] })
  vi.mocked(api.expand).mockResolvedValue({ paths: ['/tmp/회의/a.srt', '/tmp/회의/b.vtt'] })
  vi.mocked(api.preview).mockResolvedValue({ text: '김: 안녕하세요' })
  vi.mocked(api.createJobs).mockResolvedValue({ jobs: [] })
  vi.mocked(api.testCodex).mockResolvedValue({ result: 'Hello world!', models: [] })
})
afterEach(() => { cleanup(); vi.clearAllMocks(); vi.unstubAllGlobals() })

describe('workspace flow', () => {
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
    await fireEvent.click(screen.getByRole('button', { name: 'GPT-5.6 Luna로 테스트' }))
    await waitFor(() => expect(api.testCodex).toHaveBeenCalledWith('gpt-5.6-luna'))
    expect(await screen.findByText('모델 응답: Hello world!')).toBeTruthy()
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
})
