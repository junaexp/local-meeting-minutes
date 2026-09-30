<script lang="ts">
  import { Button } from '$lib/components/ui/button/index.js'
  import type { Job } from '$lib/api'
  import { isMedia } from '$lib/cart'
  export let activeJobs: Job[] = []
  export let finishedJobs: Job[] = []
  export let lastStateAt = 0
  export let notificationsEnabled = false
  export let clearingFinished = false
  export let cancellingIds: Set<string>
  export let toggleNotifications: () => void
  export let openDetail: (job: Job) => void
  export let cancelJob: (job: Job) => void
  export let clearFinishedJobs: () => void
</script>

<aside class="panel sidebar" aria-label="작업 상태">
  <div class="sidebar-columns">
    <div class="notice">
      <div class="section-title">작업 완료 알림</div>
      <p class="compact muted" style="margin:7px 0 0">
        정리가 끝나면 브라우저 알림으로 알려드릴게요.
      </p>
      <Button
        size="sm"
        variant={notificationsEnabled ? 'outline' : 'default'}
        aria-pressed={notificationsEnabled}
        onclick={toggleNotifications}>{notificationsEnabled ? '알림 끄기' : '알림 켜기'}</Button
      >
    </div>
    <section>
      <div class="section-top">
        <h2 class="section-title">작업 큐</h2>
        <span class="tiny muted">{activeJobs.length}건</span>
      </div>
      {#if activeJobs.length && lastStateAt}<p class="tiny muted state-checked">
          마지막 상태 확인 {new Date(lastStateAt).toLocaleTimeString()}
        </p>{/if}
      <div class="job-list">
        {#each activeJobs as job (job.id)}<div class="job-row">
            <button class="job-card" onclick={() => openDetail(job)}
              ><div class="job-active">
                <span class="truncate">{job.name}</span><span class="tiny"
                  >{job.kind === 'transcription' ? '전사' : '회의록'} · {job.status === 'queued'
                    ? '대기 중'
                    : '진행 중'}</span
                >
              </div>
              <div class="job-meta truncate">{job.phase}</div>
              {#if job.status !== 'queued'}<div
                  class="indeterminate"
                  aria-label="작업 진행 중"
                ></div>{/if}</button
            >{#if job.kind === 'transcription' || (isMedia(job.path) && ['extracting', 'transcribing'].includes(job.stage || ''))}<Button
                size="xs"
                variant="destructive"
                disabled={cancellingIds.has(job.id)}
                title={job.kind === 'minutes'
                  ? '전사와 회의록 작업을 함께 중지합니다.'
                  : '전사를 중지합니다.'}
                aria-label={`${job.name} ${job.kind === 'transcription' ? '전사' : '작업'} 중지`}
                onclick={() => cancelJob(job)}
                >{cancellingIds.has(job.id) ? '중지 중' : '중지'}</Button
              >{/if}
          </div>{:else}<p class="empty">대기 중인 작업이 없습니다.</p>{/each}
      </div>
    </section>
    <section>
      <div class="section-top">
        <h2 class="section-title">완료 목록</h2>
        <div class="section-actions">
          <span class="tiny muted">{finishedJobs.length}건</span><Button
            size="xs"
            variant="destructive"
            aria-label="완료 기록 지우기"
            disabled={!finishedJobs.length || clearingFinished}
            onclick={clearFinishedJobs}>{clearingFinished ? '지우는 중' : '지우기'}</Button
          >
        </div>
      </div>
      <div class="job-list">
        {#each finishedJobs.slice(0, 12) as job (job.id)}<button
            class="job-card"
            onclick={() => openDetail(job)}
            ><div class="job-active">
              <span class="truncate">{job.name}</span><span class="tiny"
                >{job.kind === 'transcription' ? '전사' : '회의록'} · {job.status === 'completed'
                  ? '완료'
                  : job.status === 'failed'
                    ? '실패'
                    : '중단'}</span
              >
            </div>
            <div class="job-meta truncate">
              {job.outputPath ||
                (job.transcriptPath
                  ? `SRT 저장됨 · ${job.error || job.phase}`
                  : job.error || job.phase)}
            </div></button
          >{:else}<p class="empty">완료한 작업이 없습니다.</p>{/each}
      </div>
    </section>
  </div>
</aside>
