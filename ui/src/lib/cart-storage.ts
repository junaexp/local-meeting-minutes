import type { CartFile } from './cart'
const key = 'meet-to-md-cart-v1'
const statuses = [
  'queued',
  'preparing',
  'running',
  'completed',
  'failed',
  'cancelled',
  'interrupted',
]

/** Storage is optional; quota/privacy failures must not break the working list. */
export function createCartStorage() {
  let saved = ''
  return {
    read(): CartFile[] {
      try {
        const stored: unknown = JSON.parse(localStorage.getItem(key) || '[]')
        if (!Array.isArray(stored)) return []
        const seen = new Set<string>()
        const files: CartFile[] = []
        for (const item of stored) {
          if (
            !item ||
            typeof item.path !== 'string' ||
            !item.path ||
            typeof item.name !== 'string' ||
            seen.has(item.path)
          )
            continue
          seen.add(item.path)
          files.push({
            path: item.path,
            name: item.name,
            disabled: item.disabled === true,
            sourcePath: typeof item.sourcePath === 'string' ? item.sourcePath : undefined,
            minutesJob:
              item.minutesJob &&
              typeof item.minutesJob.id === 'string' &&
              statuses.includes(item.minutesJob.status)
                ? item.minutesJob
                : undefined,
          })
          if (files.length === 200) break
        }
        saved = JSON.stringify(files)
        return files
      } catch {
        return []
      }
    },
    write(files: CartFile[]) {
      const value = JSON.stringify(files)
      if (value === saved) return
      try {
        localStorage.setItem(key, value)
        saved = value
      } catch {
        /* Retain the in-memory list. */
      }
    },
  }
}
