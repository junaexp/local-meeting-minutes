import { afterEach, describe, expect, it, vi } from 'vitest'
import { api } from './api'

afterEach(() => vi.restoreAllMocks())
describe('backend API contract', () => {
  it('encodes Windows paths and sets a local write header', async () => {
    const fetcher = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(new Response(JSON.stringify({ text: '원문' }), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify({ jobs: [] }), { status: 201 }))
    expect((await api.preview('C:\\회의\\a.srt')).text).toBe('원문')
    await api.createJobs(['C:\\회의\\a.srt'], 'gpt-6-sol', 'xhigh')
    expect(fetcher.mock.calls[0][0]).toContain(encodeURIComponent('C:\\회의\\a.srt'))
    const init = fetcher.mock.calls[1][1]!
    expect((init.headers as Headers).get('X-Meet-To-MD')).toBe('1')
    expect(JSON.parse(init.body as string)).toEqual({
      paths: ['C:\\회의\\a.srt'],
      model: 'gpt-6-sol',
      effort: 'xhigh',
    })
  })
  it('surfaces backend errors', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(
      new Response(JSON.stringify({ error: '잘못된 경로' }), { status: 400 }),
    )
    await expect(api.browse('/missing')).rejects.toThrow('잘못된 경로')
  })
})
