<script lang="ts">
  import { Button } from '$lib/components/ui/button/index.js'
  import type { Snapshot } from '$lib/api'
  import type { ConnectionIssue } from '$lib/live-state'
  export let state: Snapshot
  export let connectionIssue: ConnectionIssue = 'none'
  export let error = ''
  export let openSettings: () => void
  export let startWhisperInstall: () => void
  export let resyncState: (force?: boolean) => void
  $: installPercent = state.whisperInstall?.total
    ? Math.min(
        100,
        Math.round((state.whisperInstall.downloaded * 100) / state.whisperInstall.total),
      )
    : null
</script>

<header class="topbar">
  <div class="brand">
    <span class="brand-mark" aria-hidden="true">M</span>
    <h1>회의록 만들기</h1>
  </div>
  <div class="statuses">
    <span class:warn={!state.whisperReady} class="status-pill"
      ><span class="dot"></span>{state.whisperInstalling
        ? (state.whisperInstall?.message || 'Whisper 설치 중') +
          (installPercent === null ? '' : ' · ' + installPercent + '%')
        : state.whisperReady
          ? 'Whisper 모델 준비됨'
          : 'Whisper 모델 미설치'}
      {#if !state.whisperReady && !state.whisperInstalling}<Button
          size="xs"
          onclick={startWhisperInstall}>설치</Button
        >{/if}</span
    >
    <span class:warn={!state.codexReady} class="status-pill"
      ><span class="dot"></span>{state.codexReady
        ? 'Codex 로컬 서비스 준비됨'
        : 'Codex 실행 파일 확인 필요'}</span
    >
    <Button aria-label="설정 열기" variant="outline" size="sm" onclick={openSettings}>설정</Button>
  </div>
</header>
{#if connectionIssue !== 'none'}<div role="status" class="connection-notice">
    <span
      >{connectionIssue === 'server'
        ? '서버 응답을 받지 못했습니다. 연결을 확인하고 있습니다.'
        : connectionIssue === 'invalid'
          ? '실시간 상태를 읽지 못했습니다. 작업 상태를 다시 확인하고 있습니다.'
          : '실시간 연결을 다시 확인하고 있습니다. 작업 상태는 자동으로 새로 읽습니다.'}</span
    ><Button variant="outline" size="xs" onclick={() => resyncState(true)}>지금 확인</Button>
  </div>{/if}
{#if error}<div role="alert" class="error page-error">
    {error}
    <button class="mini-button" onclick={() => (error = '')} aria-label="오류 닫기">닫기</button>
  </div>{/if}
{#if state.storageError}<div role="alert" class="error page-error">
    {state.storageError} 저장을 자동으로 다시 시도합니다.
  </div>{/if}
{#if state.whisperError}<div role="alert" class="error page-error">
    Whisper 설치 실패: {state.whisperError}
  </div>{/if}
