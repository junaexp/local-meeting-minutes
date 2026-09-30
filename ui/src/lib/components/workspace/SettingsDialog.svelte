<script lang="ts">
  import { Button } from '$lib/components/ui/button/index.js'
  import { Input } from '$lib/components/ui/input/index.js'
  import { NativeSelect } from '$lib/components/ui/native-select/index.js'
  import * as Dialog from '$lib/components/ui/dialog/index.js'
  import { api, type Config, type Environment, type Snapshot } from '$lib/api'
  import { scrollOutput } from '$lib/scroll'
  export let open = false
  export let config: Config | null = null
  export let state: Snapshot
  export let onSaved: (value: Config) => Promise<void>
  export let onError: (message: string) => void
  let settingsDraft: Config | null = null
  let environment: Environment | null = null
  let environmentLoading = false
  let environmentError = ''
  let testLog = ''
  let testResult = ''
  let testRunning = false
  let busy = false
  let wasOpen = false
  let previousInstall = false
  let environmentRequest = 0
  $: installPercent = state.whisperInstall?.total
    ? Math.min(
        100,
        Math.round((state.whisperInstall.downloaded * 100) / state.whisperInstall.total),
      )
    : null
  $: if (open !== wasOpen) {
    wasOpen = open
    if (open) {
      settingsDraft = config ? { ...config } : null
      testLog = ''
      testResult = ''
      environment = null
      void refreshEnvironment()
    } else environmentRequest++
  }
  $: if (state.whisperInstalling !== previousInstall) {
    const finished = previousInstall && !state.whisperInstalling
    previousInstall = state.whisperInstalling
    if (open && finished) void refreshEnvironment()
  }
  function formatBytes(value: number) {
    return (value / (1024 * 1024)).toFixed(1) + ' MB'
  }
  async function refreshEnvironment() {
    const request = ++environmentRequest
    environmentLoading = true
    environmentError = ''
    try {
      const next = await api.environment()
      if (request === environmentRequest) environment = next
    } catch (error) {
      if (request === environmentRequest) environmentError = (error as Error).message
    } finally {
      if (request === environmentRequest) environmentLoading = false
    }
  }
  async function startWhisperInstall() {
    try {
      await api.installWhisper()
    } catch (error) {
      onError((error as Error).message)
    }
  }
  async function saveSettings() {
    if (!settingsDraft || busy) return
    busy = true
    try {
      await onSaved(await api.saveConfig(settingsDraft))
      open = false
    } catch (error) {
      onError((error as Error).message)
    } finally {
      busy = false
    }
  }
  async function testCodex() {
    if (testRunning) return
    testRunning = true
    testResult = ''
    testLog = 'Codex app-server 연결\nGPT-5.6 Luna / low · Hello world! 전송 중...'
    try {
      const result = await api.testCodex('gpt-5.6-luna')
      testResult = result.result
      testLog += '\n응답: ' + result.result + '\n테스트 완료'
    } catch (error) {
      testLog += '\n오류: ' + (error as Error).message
    } finally {
      testRunning = false
    }
  }
</script>

<Dialog.Root bind:open
  ><Dialog.Content
    class="settings-dialog !max-w-[900px] w-[calc(100vw-2rem)] max-h-[90vh] overflow-auto"
    ><Dialog.Header><Dialog.Title>설정</Dialog.Title></Dialog.Header>
    {#if settingsDraft}
      <div class="settings-heading">
        <h3>실행 환경</h3>
        <Button
          variant="outline"
          size="xs"
          disabled={environmentLoading}
          onclick={refreshEnvironment}>{environmentLoading ? '확인 중' : '재검색'}</Button
        >
      </div>
      {#if environmentError}<div class="error" role="alert">
          환경 정보를 읽지 못했습니다: {environmentError}
        </div>{/if}
      <div class="settings-fields">
        <div class="settings-field">
          <div class="settings-field-head">
            <label class="field-label" for="codex-path">Codex 실행 파일</label><span
              class="env-status"
              class:ready={environment?.codex.ready}
              >{environmentLoading
                ? '확인 중'
                : environment?.codex.ready
                  ? '탐색됨'
                  : '확인 필요'}</span
            >
          </div>
          <Input
            id="codex-path"
            bind:value={settingsDraft.codexBinary}
            placeholder="codex 또는 실행 파일 절대경로"
          />
          <p class="settings-hint">PATH에서 자동 탐색합니다. 경로를 직접 지정할 수도 있습니다.</p>
          <div class="resolved-info">
            <span>현재 경로</span><code
              >{environment?.codex.path || environment?.codex.error || '확인 중'}</code
            >
          </div>
        </div>
        <div class="settings-field">
          <label class="field-label" for="output-dir">회의록 저장 폴더</label>
          <Input
            id="output-dir"
            bind:value={settingsDraft.outputDir}
            placeholder="비워두면 원본 파일과 같은 폴더"
          />
          <p class="settings-hint">비워두면 각 원본 파일 옆에 회의록을 저장합니다.</p>
        </div>
      </div>
      <div class="settings-fields settings-tools">
        <div class="settings-field tool-card">
          <div class="settings-field-head">
            <label class="field-label" for="whisper-model">Whisper 모델</label><span
              class="env-status"
              class:ready={environment?.whisper.model === settingsDraft.whisperModel &&
                environment?.whisper.modelReady &&
                environment?.whisper.binaryReady}
              >{environmentLoading
                ? '확인 중'
                : !environment
                  ? '확인 필요'
                  : environment.whisper.model !== settingsDraft.whisperModel
                    ? '저장 후 확인'
                    : environment.whisper.modelReady && environment.whisper.binaryReady
                      ? '준비됨'
                      : '설치 필요'}</span
            >
          </div>
          <NativeSelect class="w-full" id="whisper-model" bind:value={settingsDraft.whisperModel}
            ><option value="large-v3-turbo">Large V3 Turbo · 다국어</option><option value="base"
              >Base · 다국어</option
            ><option value="small">Small · 다국어</option></NativeSelect
          >
          {#if environment && environment.whisper.model !== settingsDraft.whisperModel}<p
              class="settings-hint"
            >
              모델 변경은 설정 저장 후 적용됩니다. 현재 적용: {environment.whisper.model}
            </p>{:else}<p class="settings-hint">
              현재 적용: {environment?.whisper.model || '확인 중'} · 자동 설치 기준 whisper.cpp {environment
                ?.whisper.installerVersion || '확인 중'}
            </p>{/if}
          <div class="resolved-info">
            <span>모델 파일</span><code>{environment?.whisper.modelPath || '확인 중'}</code>
          </div>
          <div class="resolved-info">
            <span>Whisper 실행 파일</span><code>{environment?.whisper.binaryPath || '미설치'}</code>
          </div>
        </div>
        <div class="settings-field tool-card">
          <div class="settings-field-head">
            <label class="field-label" for="ffmpeg-path">ffmpeg 실행 파일</label><span
              class="env-status"
              class:ready={environment?.ffmpeg.ready}
              >{environmentLoading
                ? '확인 중'
                : environment?.ffmpeg.ready
                  ? '탐색됨'
                  : '확인 필요'}</span
            >
          </div>
          <Input
            id="ffmpeg-path"
            bind:value={settingsDraft.ffmpegBinary}
            placeholder="ffmpeg 또는 실행 파일 절대경로"
          />
          <p class="settings-hint">
            설치된 버전: {environment?.ffmpeg.version || environment?.ffmpeg.error || '확인 중'}
          </p>
          <div class="resolved-info">
            <span>현재 경로</span><code>{environment?.ffmpeg.path || '찾지 못했습니다'}</code>
          </div>
        </div>
      </div>
      {#if state.whisperInstalling || state.whisperInstall?.stage === 'completed' || state.whisperInstall?.stage === 'failed'}
        <div class="install-panel" aria-live="polite">
          <div class="settings-field-head">
            <h3>Whisper 설치</h3>
            <span class="tiny muted">모델: {state.whisperInstall?.model || '확인 중'}</span>
          </div>
          <p class="settings-hint">
            {state.whisperInstall?.message ||
              '설치 준비 중'}{#if state.whisperInstalling && state.whisperInstall?.downloaded}{` · ${formatBytes(state.whisperInstall.downloaded)}${state.whisperInstall.total ? ` / ${formatBytes(state.whisperInstall.total)}` : ''}`}{/if}
          </p>
          {#if state.whisperInstalling}
            {#if installPercent !== null}<div
                class="install-progress"
                role="progressbar"
                aria-label="Whisper 설치 다운로드 진행률"
                aria-valuenow={installPercent}
                aria-valuemin="0"
                aria-valuemax="100"
              >
                <span style:transform={`scaleX(${installPercent / 100})`}></span>
              </div>
            {:else}<div
                class="indeterminate install-indeterminate"
                aria-label="Whisper 설치 단계 진행 중"
              ></div>{/if}
          {/if}
          <textarea
            class="install-log"
            aria-label="Whisper 설치 로그"
            readonly
            use:scrollOutput={{ text: state.whisperInstall?.log || '', pinned: true }}
            value={state.whisperInstall?.log || '설치를 시작하면 로그가 표시됩니다.'}></textarea>
          {#if state.whisperInstall?.stage === 'failed'}<Button
              size="sm"
              onclick={startWhisperInstall}>다시 시도</Button
            >{/if}
        </div>
      {/if}
      <div class="settings-test">
        <div class="settings-field-head">
          <h3>실제 응답 테스트</h3>
          <Button size="sm" disabled={testRunning} onclick={testCodex}
            >{testRunning ? '테스트 중' : 'GPT-5.6 Luna로 테스트'}</Button
          >
        </div>
        <p class="settings-hint">
          Hello world!를 보내 실제 응답을 확인합니다. 모델 사용량이 발생할 수 있습니다.
        </p>
        <div class="log-box" role="log" aria-live="polite">
          {testLog || '테스트를 실행하면 로그가 여기에 표시됩니다.'}
        </div>
        {#if testResult}<div class="success" style="margin-top:8px">
            모델 응답: {testResult}
          </div>{/if}
      </div>
      <Dialog.Footer class="settings-footer"
        ><Button variant="outline" onclick={() => (open = false)}>취소</Button><Button
          disabled={busy}
          onclick={saveSettings}>설정 저장</Button
        ></Dialog.Footer
      >
    {/if}
  </Dialog.Content></Dialog.Root
>
