export interface CartFile { path: string; name: string }
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
