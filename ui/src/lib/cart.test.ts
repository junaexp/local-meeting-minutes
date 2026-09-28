import { describe, expect, it } from 'vitest'
import { addPaths, moveFile, removeFile } from './cart'

describe('cart ordering', () => {
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
