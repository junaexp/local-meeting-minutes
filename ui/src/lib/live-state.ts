import { api, type Snapshot } from './api'
export type ConnectionIssue = 'none' | 'stream' | 'invalid' | 'server'

/** Owns the stream, recovery timer and stale-response guards. One instance per mounted app. */
export function connectState(
  accept: (state: Snapshot) => void,
  status: (issue: ConnectionIssue) => void,
  checked: (time: number) => void,
) {
  let disposed = false
  let issue: ConnectionIssue = 'none'
  let streamVersion = 0
  let requestVersion = 0
  let lastStreamAt = Date.now()
  let lastAttempt = 0
  let pending = false
  let retry = false
  const setIssue = (value: ConnectionIssue) => {
    issue = value
    status(value)
  }
  function apply(state: Snapshot) {
    if (disposed) return
    requestVersion++
    accept(state)
    checked(Date.now())
  }
  async function resync(force = false) {
    if (disposed) return
    if (pending) {
      if (force) retry = true
      return
    }
    if (!force && Date.now() - lastAttempt < 10_000) return
    lastAttempt = Date.now()
    pending = true
    const version = ++requestVersion
    const observed = streamVersion
    try {
      const next = await api.state()
      if (disposed || observed !== streamVersion || version !== requestVersion) return
      accept(next)
      checked(Date.now())
      if (issue === 'server') setIssue('stream')
    } catch (error) {
      if (!disposed && observed === streamVersion && version === requestVersion) {
        setIssue('server')
        console.error('작업 상태를 다시 읽지 못했습니다.', error)
      }
    } finally {
      pending = false
      if (retry && !disposed) {
        retry = false
        void resync(true)
      }
    }
  }
  const initial = ++requestVersion
  api
    .state()
    .then((next) => {
      if (!disposed && streamVersion === 0 && initial === requestVersion) {
        accept(next)
        checked(Date.now())
      }
    })
    .catch((error) => {
      if (!disposed && streamVersion === 0 && initial === requestVersion) {
        setIssue('server')
        console.error('초기 작업 상태를 읽지 못했습니다.', error)
      }
    })
  const stream = new EventSource('/api/events')
  stream.addEventListener('state', (event) => {
    try {
      accept(JSON.parse((event as MessageEvent).data) as Snapshot)
      streamVersion++
      lastStreamAt = Date.now()
      checked(lastStreamAt)
      setIssue('none')
    } catch (error) {
      if (issue !== 'invalid') console.error('실시간 작업 상태를 처리하지 못했습니다.', error)
      setIssue('invalid')
      void resync()
    }
  })
  stream.addEventListener('heartbeat', () => {
    lastStreamAt = Date.now()
  })
  stream.onerror = () => {
    if (issue !== 'server') setIssue('stream')
    void resync()
  }
  const watchdog = window.setInterval(() => {
    if (Date.now() - lastStreamAt > 45_000) {
      if (issue === 'none') setIssue('stream')
      void resync()
    } else if (issue !== 'none') void resync()
  }, 5_000)
  const visible = () => {
    if (!document.hidden) void resync(true)
  }
  document.addEventListener('visibilitychange', visible)
  return {
    apply,
    resync,
    dispose() {
      disposed = true
      window.clearInterval(watchdog)
      document.removeEventListener('visibilitychange', visible)
      stream.close()
    },
  }
}
