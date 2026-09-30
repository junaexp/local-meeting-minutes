<script lang="ts">
  import { Button } from '$lib/components/ui/button/index.js'
  import type { CartFile } from '$lib/cart'
  import { isMedia, isActive, moveFile, minuteStatus } from '$lib/cart'
  import type { Job } from '$lib/api'
  // This panel owns only drag/drop state; the parent retains the input list.
  export let cart: CartFile[] = []
  export let selectedPath = ''
  export let previewTranscribed = false
  export let transcriptionBusyPath = ''
  export let busy = false
  export let codexReady = false
  export let whisperReady = false
  export let cancellingIds: Set<string>
  export let jobIndex: { transcriptions: Map<string, Job> }
  export let selectFile: (path: string) => void
  export let removeFromCart: (path: string) => void
  export let startTranscription: (path: string) => void
  export let cancelJob: (job: Job) => void
  export let rerunFile: (file: CartFile) => void
  export let clearCart: () => void
  export let openBrowser: (mode: 'file' | 'folder') => void
  export let importFiles: (files: FileList | File[]) => Promise<void>
  let dragIndex = -1
  let dropActive = false
  let uploadInput: HTMLInputElement
  function onDrop(event: DragEvent) {
    event.preventDefault()
    dropActive = false
    if (event.dataTransfer?.files.length) void importFiles(event.dataTransfer.files)
  }
  function onCartDrop(event: DragEvent, index: number) {
    event.preventDefault()
    cart = moveFile(cart, dragIndex, index)
    dragIndex = -1
  }
</script>

<section class="panel" aria-labelledby="input-title">
  <div class="section-top">
    <h2 id="input-title" class="section-number">01 / INPUT</h2>
    <span class="compact muted">{cart.length}개 파일</span>
  </div>
  <div class="button-row">
    <Button onclick={() => openBrowser('file')}>파일 선택</Button><Button
      variant="outline"
      onclick={() => openBrowser('folder')}>폴더 선택</Button
    >
  </div>
  <input
    aria-label="컴퓨터에서 파일 가져오기"
    bind:this={uploadInput}
    hidden
    type="file"
    multiple
    accept=".srt,.vtt,.txt,.md,.wav,.mp3,.m4a,.mp4,.mov,.ogg"
    onchange={(event) =>
      importFiles(event.currentTarget.files || []).finally(() => (uploadInput.value = ''))}
  />
  <button
    class:active={dropActive}
    class="drop-zone"
    ondragover={(event) => {
      event.preventDefault()
      dropActive = true
    }}
    ondragleave={() => (dropActive = false)}
    ondrop={onDrop}
    onclick={() => uploadInput?.click()}
    >파일을 여기에 끌어놓거나 컴퓨터에서 가져오기<br /><span class="tiny muted"
      >SRT · VTT · TXT · MD · 오디오 · 영상</span
    ></button
  >
  <div class="queue-title">
    <h3 class="section-title">처리 순서</h3>
    <div class="section-actions">
      <span class="tiny muted">끌어서 순서 변경</span><Button
        size="xs"
        variant="destructive"
        aria-label="처리 순서 지우기"
        disabled={!cart.length}
        onclick={clearCart}>지우기</Button
      >
    </div>
  </div>
  {#if cart.length}<ol class="cart-list">
      {#each cart as file, index (file.path)}<li
          class:selected={selectedPath === file.path}
          class:disabled={file.disabled}
          class="cart-item"
          draggable={!file.disabled}
          ondragstart={() => (dragIndex = index)}
          ondragover={(event) => event.preventDefault()}
          ondrop={(event) => onCartDrop(event, index)}
        >
          <span class="tiny muted"
            >{file.disabled ? '원본' : String(index + 1).padStart(2, '0')}</span
          ><button class="cart-main" onclick={() => selectFile(file.path)} title={file.path}
            ><span class="truncate">{file.name}</span><span class="tiny muted truncate"
              >{file.path}</span
            ></button
          >
          <span class="cart-controls"
            ><button
              class="mini-button"
              aria-label={`${file.name} 위로`}
              disabled={file.disabled || index === 0}
              onclick={() => (cart = moveFile(cart, index, index - 1))}>위</button
            ><button
              class="mini-button"
              aria-label={`${file.name} 아래로`}
              disabled={file.disabled || index === cart.length - 1}
              onclick={() => (cart = moveFile(cart, index, index + 1))}>아래</button
            ></span
          >
          <button
            class="mini-button"
            aria-label={`${file.name} 삭제`}
            onclick={() => removeFromCart(file.path)}>삭제</button
          >
          {#if file.disabled}<span class="cart-source-note">전사 완료 · 회의록 처리 제외</span>{/if}
          {#if file.minutesJob}<div class="cart-media-actions">
              <span class="tiny muted">회의록 · {minuteStatus(file.minutesJob.status)}</span
              >{#if file.minutesJob.status === 'completed' && !file.disabled}<Button
                  size="xs"
                  variant="outline"
                  disabled={busy || !codexReady}
                  onclick={() => rerunFile(file)}>다시 정리</Button
                >{/if}
            </div>{/if}
          {#if isMedia(file.path)}{@const fileJob = jobIndex.transcriptions.get(
              file.path,
            )}{@const fileReady = selectedPath === file.path && previewTranscribed}
            <div class="cart-media-actions">
              <span class="tiny muted"
                >{fileReady
                  ? '전사문 준비됨'
                  : fileJob?.status === 'failed'
                    ? '전사 실패 · 다시 시도 가능'
                    : isActive(fileJob)
                      ? fileJob?.phase
                      : '오디오·영상 전사'}</span
              >{#if fileJob && isActive(fileJob)}<Button
                  size="xs"
                  variant="destructive"
                  disabled={cancellingIds.has(fileJob.id)}
                  onclick={() => cancelJob(fileJob)}
                  >{cancellingIds.has(fileJob.id) ? '중지 요청 중' : '전사 중지'}</Button
                >{:else}<Button
                  size="xs"
                  disabled={transcriptionBusyPath === file.path ||
                    (!file.disabled && fileReady) ||
                    !whisperReady}
                  onclick={() => startTranscription(file.path)}
                  >{file.disabled ? '다시 전사' : fileReady ? '전사 완료' : '전사 시작'}</Button
                >{/if}
            </div>
          {/if}
        </li>{/each}
    </ol>{:else}<p class="empty">파일을 등록하면 여기에서 순서를 바꿀 수 있습니다.</p>{/if}
</section>
