import type { Job } from './api'
const key = 'meet-to-md-notifications-enabled'
/** Permission belongs to the browser; this preference only controls this app's notifications. */
export function createNotifications() {
  let known = new Set<string>()
  let initialized = false
  return {
    read() {
      try {
        return (
          localStorage.getItem(key) === '1' &&
          'Notification' in window &&
          Notification.permission === 'granted'
        )
      } catch {
        return false
      }
    },
    async toggle(enabled: boolean) {
      if (enabled) {
        try {
          localStorage.removeItem(key)
        } catch {}
        return false
      }
      if (!('Notification' in window)) throw new Error('이 브라우저는 알림을 지원하지 않습니다.')
      const permission =
        Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission()
      if (permission !== 'granted')
        throw new Error(
          permission === 'denied'
            ? '브라우저 설정에서 알림 권한을 허용해 주세요.'
            : '브라우저 알림 권한이 허용되지 않았습니다.',
        )
      try {
        localStorage.setItem(key, '1')
      } catch {}
      return true
    },
    accept(jobs: Job[], enabled: boolean) {
      const completed = jobs.filter((job) => job.status === 'completed')
      if (
        initialized &&
        enabled &&
        typeof Notification !== 'undefined' &&
        Notification.permission === 'granted'
      ) {
        for (const job of completed)
          if (!known.has(job.id)) {
            try {
              new Notification(job.kind === 'transcription' ? '전사 완료' : '회의록 생성 완료', {
                body: job.name + ' · 저장 완료',
              })
            } catch (error) {
              console.warn('작업 완료 알림을 표시하지 못했습니다.', error)
            }
          }
      }
      known = new Set(completed.map((job) => job.id))
      initialized = true
    },
  }
}
