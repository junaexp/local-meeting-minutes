<script lang="ts">
  import type { CartFile } from '$lib/cart'
  import { isMedia } from '$lib/cart'
  import { scrollOutput } from '$lib/scroll'
  export let selectedFile: CartFile | undefined
  export let previewLoading = false
  export let preview = ''
  export let selectedPreviewText = ''
  export let selectedLog = ''
  let previewPinned = true
  let logPinned = true
  let previousPath = ''
  $: if (selectedFile?.path !== previousPath) {
    previousPath = selectedFile?.path || ''
    previewPinned = true
    logPinned = true
  }
</script>

<section class="panel" aria-labelledby="preview-title">
  <div class="section-top"><h2 id="preview-title" class="section-number">02 / PREVIEW</h2></div>
  <div class="preview-file">
    <div class="section-title truncate">{selectedFile?.name || '선택한 파일 없음'}</div>
    <div class="tiny muted truncate">{selectedFile?.path || '왼쪽에서 파일을 선택해 주세요.'}</div>
  </div>
  <div class="section-top">
    <h3 class="section-title">
      {selectedFile && isMedia(selectedFile.path) ? '전사문 미리보기' : '텍스트 미리보기'}
    </h3>
    {#if selectedFile && isMedia(selectedFile.path)}<button
        class:active={previewPinned}
        class="scroll-pin"
        aria-label="전사문 미리보기 하단 고정"
        aria-pressed={previewPinned}
        onclick={() => (previewPinned = !previewPinned)}
        >하단 고정 {previewPinned ? '켬' : '끔'}</button
      >{:else}<span class="tiny muted">앞 64KB 표시</span>{/if}
  </div>
  {#if selectedFile && isMedia(selectedFile.path)}
    <textarea
      class="preview-box media-preview-box"
      aria-label="전사문 미리보기"
      readonly
      value={selectedPreviewText}
      use:scrollOutput={{
        text: selectedPreviewText,
        pinned: previewPinned,
        key: selectedFile.path,
      }}></textarea>
    <div class="section-top media-log-title">
      <h3 class="section-title">Whisper 전사 로그</h3>
      <button
        class:active={logPinned}
        class="scroll-pin"
        aria-label="Whisper 전사 로그 하단 고정"
        aria-pressed={logPinned}
        onclick={() => (logPinned = !logPinned)}>하단 고정 {logPinned ? '켬' : '끔'}</button
      >
    </div>
    <textarea
      class="media-log"
      aria-label="Whisper 전사 로그"
      readonly
      value={selectedLog || '전사를 시작하면 FFmpeg와 Whisper 실행 내용이 여기에 표시됩니다.'}
      use:scrollOutput={{ text: selectedLog, pinned: logPinned, key: selectedFile.path }}
    ></textarea>
    <p class="tiny muted media-log-hint">다른 파일을 선택하면 해당 파일의 로그를 표시합니다.</p>
  {:else}
    <textarea
      class="preview-box text-preview-box"
      aria-label="텍스트 미리보기"
      readonly
      value={previewLoading ? '불러오는 중...' : preview}
      use:scrollOutput={{ text: preview, key: selectedFile?.path || '', resetTop: true }}
    ></textarea>
  {/if}
</section>
