<script lang="ts">
  import { Button } from '$lib/components/ui/button/index.js'
  import { Textarea } from '$lib/components/ui/textarea/index.js'
  import * as Dialog from '$lib/components/ui/dialog/index.js'
  import type { Job } from '$lib/api'
  import { isActive } from '$lib/cart'
  import { effortName } from '$lib/models'
  import { scrollOutput } from '$lib/scroll'
  export let detailJob: Job | undefined
  export let detailTranscript = ''
  export let cancellingIds: Set<string>
  export let copy: (value: string) => void
  export let closeDetail: () => void
  export let cancelJob: (job: Job) => void
  export let deleteJob: (job: Job) => void
  let detailPreviewAtBottom = true
  let detailLogAtBottom = true
  let previousID = ''
  $: if (detailJob?.id !== previousID) {
    previousID = detailJob?.id || ''
    detailPreviewAtBottom = true
    detailLogAtBottom = true
  }
  $: detailTranscribing =
    detailJob?.kind === 'transcription' ||
    ['extracting', 'transcribing', 'transcribed'].includes(detailJob?.stage || '')
  $: detailPreviewText = detailTranscribing
    ? detailJob?.transcriptPreview || detailTranscript || '전사문을 기다리는 중입니다.'
    : detailJob?.result || '결과를 기다리는 중입니다.'
  $: detailLog = detailTranscribing
    ? detailJob?.transcriptionLog || ''
    : detailJob?.recentOutput || ''
  function toggleScrollPin(area: string) {
    if (area === 'detail-preview') detailPreviewAtBottom = !detailPreviewAtBottom
    else detailLogAtBottom = !detailLogAtBottom
  }
</script>

<Dialog.Root
  open={!!detailJob}
  onOpenChange={(open) => {
    if (!open) closeDetail()
  }}
  ><Dialog.Content class="!max-w-[1280px] w-[calc(100vw-2rem)] max-h-[92vh] overflow-auto">
    {#if detailJob}<Dialog.Header
        ><Dialog.Title>{detailJob.name}</Dialog.Title><Dialog.Description
          >{detailJob.phase} · {detailJob.kind === 'transcription'
            ? `Whisper ${detailJob.transcriptionModel || ''}`
            : `${detailJob.model} / ${effortName[detailJob.effort]}`}</Dialog.Description
        ></Dialog.Header
      >
      <div class="modal-grid">
        <div>
          <div class="section-top">
            <h3 class="section-title">{detailTranscribing ? '전사문' : 'Codex 결과'}</h3>
            {#if detailTranscribing}<button
                class:active={detailPreviewAtBottom}
                class="scroll-pin"
                aria-label="상세 전사문 하단 고정"
                aria-pressed={detailPreviewAtBottom}
                onclick={() => toggleScrollPin('detail-preview')}
                >하단 고정 {detailPreviewAtBottom ? '켬' : '끔'}</button
              >{:else}<Button variant="outline" size="xs" onclick={() => copy(detailJob!.result)}
                >복사</Button
              >{/if}
          </div>
          <textarea
            class="modal-textarea"
            aria-label={detailTranscribing ? '전사문' : 'Codex 결과'}
            readonly
            use:scrollOutput={{
              text: detailPreviewText,
              pinned: detailPreviewAtBottom,
              key: detailJob.id,
            }}
            value={detailPreviewText}></textarea>
        </div>
        <div>
          <div class="section-top">
            <h3 class="section-title">{detailTranscribing ? 'Whisper 전사 로그' : '최근 출력'}</h3>
            {#if detailTranscribing}<button
                class:active={detailLogAtBottom}
                class="scroll-pin"
                aria-label="상세 Whisper 전사 로그 하단 고정"
                aria-pressed={detailLogAtBottom}
                onclick={() => toggleScrollPin('detail-log')}
                >하단 고정 {detailLogAtBottom ? '켬' : '끔'}</button
              >{:else}<span class="tiny muted">{isActive(detailJob) ? '실시간' : '작업 로그'}</span
              >{/if}
          </div>
          <textarea
            class="modal-textarea"
            aria-label={detailTranscribing ? 'Whisper 전사 로그' : '최근 출력'}
            readonly
            use:scrollOutput={{ text: detailLog, pinned: detailLogAtBottom, key: detailJob.id }}
            value={detailLog || detailJob.phase}></textarea>
        </div>
      </div>
      {#if detailJob.error}<div class="error">{detailJob.error}</div>{/if}
      {#if detailJob.transcriptPath}<div class="success">
          SRT 저장 위치: {detailJob.transcriptPath}
          <Button size="xs" variant="outline" onclick={() => copy(detailJob!.transcriptPath!)}
            >경로 복사</Button
          >
        </div>{/if}
      {#if detailJob.outputPath}<div class="success">
          회의록 저장 위치: {detailJob.outputPath}
          <Button size="xs" variant="outline" onclick={() => copy(detailJob!.outputPath)}
            >경로 복사</Button
          >
        </div>{/if}
      <details class="modal-section">
        <summary>요청 정보</summary>
        <p class="compact muted">원본: {detailJob.path}</p>
        <p class="compact muted">Whisper 모델: {detailJob.transcriptionModel || '설정값'}</p>
        {#if detailJob.kind !== 'transcription'}<p class="compact muted">
            Codex 모델: {detailJob.model} · {effortName[detailJob.effort]}
          </p>
          <Textarea
            readonly
            value={detailJob.prompt}
            aria-label="전송 프롬프트"
            class="w-full min-h-36"
          />{#if detailJob.transcriptionLog}<p class="compact muted">전사 로그</p>
            <Textarea
              readonly
              value={detailJob.transcriptionLog}
              aria-label="전사 로그 기록"
              class="w-full min-h-36"
            />{/if}{/if}
      </details>
      <Dialog.Footer
        >{#if ['running', 'preparing', 'queued'].includes(detailJob.status)}<Button
            variant="destructive"
            disabled={cancellingIds.has(detailJob.id)}
            onclick={() => cancelJob(detailJob!)}
            >{cancellingIds.has(detailJob.id) ? '중지 요청 중' : '작업 중지'}</Button
          >{:else}<Button variant="outline" onclick={() => deleteJob(detailJob!)}
            >목록에서 삭제</Button
          >{/if}<Button onclick={closeDetail}>닫기</Button></Dialog.Footer
      >
    {/if}
  </Dialog.Content></Dialog.Root
>
