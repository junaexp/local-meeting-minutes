import { describe, expect, it } from 'vitest'
import { addPaths, attachTranscript, moveFile, removeFile } from './cart'

describe('cart ordering', () => {
  it('keeps a transcribed media source disabled and inserts one active SRT after it', () => {
    const media = '/tmp/meeting.mp4'
    const first = attachTranscript(addPaths([], [media, '/tmp/notes.txt']), media, '/tmp/meeting_전사.srt')
    expect(first.map(file => [file.path, !!file.disabled])).toEqual([
      [media, true], ['/tmp/meeting_전사.srt', false], ['/tmp/notes.txt', false],
    ])
    expect(attachTranscript(first, media, '/tmp/meeting_전사.srt')).toBe(first)
    const updated = attachTranscript(first, media, '/tmp/meeting_전사_2.srt')
    expect(updated.map(file => file.path)).toEqual([media, '/tmp/meeting_전사_2.srt', '/tmp/notes.txt'])
  })
  it('preserves folder expansion order and ignores duplicate paths', () => {
    const files = addPaths([], ['C:\\회의\\a.srt', 'C:\\회의\\b.vtt'])
    expect(files.map(file => file.name)).toEqual(['a.srt', 'b.vtt'])
    expect(addPaths(files, ['C:\\회의\\a.srt'])).toEqual(files)
    expect(moveFile(files, 0, 1).map(file => file.name)).toEqual(['b.vtt', 'a.srt'])
    expect(removeFile(files, files[0].path).map(file => file.name)).toEqual(['b.vtt'])
  })
  it('ignores out-of-range moves', () => {
    const files = addPaths([], ['/tmp/one.srt'])
    expect(moveFile(files, 0, 5)).toBe(files)
  })
})
