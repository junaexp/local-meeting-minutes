export interface CartFile { path: string; name: string; disabled?: boolean; sourcePath?: string }
export function addPaths(existing: CartFile[], paths: string[]): CartFile[] {
  const seen = new Set(existing.map(item => item.path))
  const result = [...existing]
  for (const path of paths) {
    if (seen.has(path)) continue
    seen.add(path)
    result.push({ path, name: path.split(/[\\/]/).pop() || path })
  }
  return result
}
export function moveFile(files: CartFile[], from: number, to: number): CartFile[] {
  if (from < 0 || to < 0 || from >= files.length || to >= files.length || from === to) return files
  const next = [...files]
  const [item] = next.splice(from, 1)
  next.splice(to, 0, item)
  return next
}
export function removeFile(files: CartFile[], path: string): CartFile[] { return files.filter(file => file.path !== path) }

export function attachTranscript(files: CartFile[], sourcePath: string, transcriptPath: string): CartFile[] {
  const source = files.find(file => file.path === sourcePath && !file.sourcePath)
  if (!source || !transcriptPath) return files
  const existing = files.find(file => file.sourcePath === sourcePath)
  if (source.disabled && existing?.path === transcriptPath) return files
  const next = files.filter(file => file.sourcePath !== sourcePath && file.path !== transcriptPath)
  const index = next.findIndex(file => file.path === sourcePath)
  next[index] = { ...next[index], disabled: true }
  next.splice(index + 1, 0, { path: transcriptPath, name: transcriptPath.split(/[\\/]/).pop() || transcriptPath, sourcePath })
  return next
}
