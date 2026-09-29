<script lang="ts">
  import { onMount, tick } from 'svelte'
  import { Button } from '$lib/components/ui/button/index.js'
  import { Input } from '$lib/components/ui/input/index.js'
  import { Textarea } from '$lib/components/ui/textarea/index.js'
  import { NativeSelect } from '$lib/components/ui/native-select/index.js'
  import * as Dialog from '$lib/components/ui/dialog/index.js'
  import { api, type BrowseResult, type Config, type Environment, type Job, type Model, type Snapshot } from '$lib/api'
  import { addPaths, moveFile, removeFile, type CartFile } from '$lib/cart'

  let state: Snapshot = { jobs: [], codexReady: false, whisperReady: false, whisperInstalling: false, whisperError: '' }
  let config: Config | null = null
  let promptValue = ''
  let cart: CartFile[] = []
  let selectedPath = ''
  let preview = '파일을 선택하면 여기에 원문을 표시합니다.'
  let previewTranscribed = false
  let previewLoading = false
  let previewRequest = 0
  let transcriptionBusyPath = ''
  let whisperLogArea: HTMLTextAreaElement
  let installLogArea: HTMLTextAreaElement
  let logAtBottom = true
  let detailLogArea: HTMLTextAreaElement
  let detailLogAtBottom = true
  let detailTranscript = ''
  let model = 'gpt-5.6-sol'
  let effort = 'medium'
  let models: Model[] = []
  let error = ''
  let busy = false
  let settingsOpen = false
  let browserOpen = false
  let browserMode: 'file' | 'folder' = 'file'
  let browser: BrowseResult | null = null
  let browserPath = ''
  let browserBusy = false
  let detailId = ''
  let promptEditing = false
  let settingsDraft: Config | null = null
  let environment: Environment | null = null
  let environmentLoading = false
  let environmentError = ''
  let testLog = ''
  let testRunning = false
  let testResult = ''
  let uploadInput: HTMLInputElement
  let dragIndex = -1
  let dropActive = false
  let notificationsEnabled = false
  let knownCompleted = new Set<string>()
  $: detailJob = state.jobs.find(job => job.id === detailId)
  $: installPercent = state.whisperInstall?.total ? Math.min(100, Math.round(state.whisperInstall.downloaded * 100 / state.whisperInstall.total)) : null
  $: selectedFile = cart.find(file => file.path === selectedPath)
  $: selectedMediaJob = [...state.jobs].reverse().find(job => job.path === selectedPath && (job.transcriptionModel || config?.whisperModel) === config?.whisperModel && (job.kind === 'transcription' || !!job.transcriptionLog))
  $: selectedLog = selectedMediaJob?.transcriptionLog || ''
  $: detailTranscribing = detailJob?.kind === 'transcription' || ['extracting', 'transcribing', 'transcribed'].includes(detailJob?.stage || '')
  $: detailLog = detailTranscribing ? detailJob?.transcriptionLog || '' : detailJob?.recentOutput || ''
  $: activeJobs = state.jobs.filter(job => ['queued', 'preparing', 'running'].includes(job.status))
  $: finishedJobs = state.jobs.filter(job => !['queued', 'preparing', 'running'].includes(job.status)).slice().reverse()
  $: selectedModel = models.find(item => item.model === model || item.id === model)
  $: efforts = selectedModel?.supportedReasoningEfforts?.map(item => item.reasoningEffort).filter(value => ['low','medium','high','xhigh'].includes(value)) ?? ['low','medium','high','xhigh']
  const effortName: Record<string,string> = { low: 'Light', medium: 'Medium', high: 'High', xhigh: 'Extra high' }
  function isMedia(path: string) { return /\.(wav|mp3|m4a|mp4|mov|ogg)$/i.test(path) }
  function isActive(job?: Job) { return !!job && ['queued', 'preparing', 'running'].includes(job.status) }
  function hasTranscript(job?: Job) { return !!job && (job.stage === 'transcribed' || (job.kind === 'transcription' && job.status === 'completed') || (!!job.transcriptionLog && ['drafting', 'completed'].includes(job.stage || ''))) }
  function updateScrollPin(event: Event, detail = false) {
    const area = event.currentTarget as HTMLTextAreaElement
    const pinned = area.scrollTop + area.clientHeight >= area.scrollHeight - 24
    if (detail) detailLogAtBottom = pinned
    else logAtBottom = pinned
  }
  $: if (selectedLog) { void tick().then(() => { if (logAtBottom && whisperLogArea) whisperLogArea.scrollTop = whisperLogArea.scrollHeight }) }
  $: if (detailLog) { void tick().then(() => { if (detailLogAtBottom && detailLogArea) detailLogArea.scrollTop = detailLogArea.scrollHeight }) }
  $: if (state.whisperInstall?.log) { void tick().then(() => { if (installLogArea) installLogArea.scrollTop = installLogArea.scrollHeight }) }
  function formatBytes(value: number) { return `${(value / (1024 * 1024)).toFixed(1)} MB` }
  function modelAvailable(id: string) { return !models.length || models.some(item => item.model === id || item.id === id) }
  async function loadModels() {
    const response = await api.models(); models = response.models ?? []
    if (!modelAvailable(model)) model = modelAvailable('gpt-5.6-sol') ? 'gpt-5.6-sol' : (models[0]?.model || model)
  }

  function acceptState(next: Snapshot) {
    next = { ...next, jobs: next.jobs ?? [] }
    const whisperChanged = state.whisperReady !== next.whisperReady || (state.whisperInstalling && !next.whisperInstalling)
    const newlyTranscribed = isMedia(selectedPath) && next.jobs.some(job => job.path === selectedPath && hasTranscript(job) && !hasTranscript(state.jobs.find(previous => previous.id === job.id)))
    const detailCompleted = next.jobs.some(job => job.id === detailId && job.kind === 'transcription' && job.status === 'completed' && state.jobs.find(previous => previous.id === job.id)?.status !== 'completed')
    if (notificationsEnabled && typeof Notification !== 'undefined' && Notification.permission === 'granted') {
      for (const job of next.jobs) if (job.status === 'completed' && !knownCompleted.has(job.id)) new Notification(job.kind === 'transcription' ? '전사 완료' : '회의록 생성 완료', { body: `${job.name} · ${job.kind === 'transcription' ? '전사문 준비됨' : '저장 완료'}` })
    }
    knownCompleted = new Set(next.jobs.filter(job => job.status === 'completed').map(job => job.id))
    state = next
    if (settingsOpen && whisperChanged) void refreshEnvironment()
    if (newlyTranscribed) void loadPreview(selectedPath)
    if (detailCompleted) void loadDetailTranscript(detailId)
  }
  onMount(() => {
    Promise.all([api.state(), api.config()]).then(([next, cfg]) => {
      acceptState(next); config = cfg; promptValue = cfg.prompt
    }).catch(err => error = String(err.message || err))
    loadModels().catch(() => {})
    const stream = new EventSource('/api/events')
    stream.addEventListener('state', event => { try { acceptState(JSON.parse((event as MessageEvent).data) as Snapshot) } catch { /* retain last state */ } })
    stream.onerror = () => { error = '서버 연결을 확인 중입니다. 잠시 후 다시 연결합니다.' }
    stream.onopen = () => { if (error.startsWith('서버 연결')) error = '' }
    return () => stream.close()
  })
  async function loadPreview(path: string) {
    const request = ++previewRequest
    previewLoading = true
    try {
      const result = await api.preview(path)
      if (request === previewRequest && selectedPath === path) { preview = result.text; previewTranscribed = result.transcribed }
    } catch (err) { if (request === previewRequest && selectedPath === path) preview = (err as Error).message }
    finally { if (request === previewRequest && selectedPath === path) previewLoading = false }
  }
  function selectFile(path: string) {
    selectedPath = path; preview = ''; previewTranscribed = false; logAtBottom = true
    void loadPreview(path)
  }
  function addToCart(paths: string[]) { cart = addPaths(cart, paths); if (!selectedPath && cart.length) void selectFile(cart[0].path) }
  function removeFromCart(path: string) { cart = removeFile(cart, path); if (selectedPath === path) { ++previewRequest; selectedPath = ''; preview = '파일을 선택하면 여기에 원문을 표시합니다.'; previewTranscribed = false; if (cart.length) void selectFile(cart[0].path) } }
  async function startTranscription(path: string) {
    if (transcriptionBusyPath) return
    transcriptionBusyPath = path; error = ''
    selectFile(path)
    try { await api.createTranscription(path) }
    catch (err) { error = (err as Error).message }
    finally { transcriptionBusyPath = '' }
  }
  async function importFiles(files: FileList | File[]) {
    if (!files.length) return; busy = true; error = ''
    try { addToCart((await api.importFiles(files)).paths) } catch (err) { error = (err as Error).message }
    finally { busy = false; if (uploadInput) uploadInput.value = '' }
  }
  async function openBrowser(mode: 'file' | 'folder') { browserMode = mode; browserOpen = true; await browseTo(browser?.path || '') }
  async function browseTo(path: string) {
    browserBusy = true; error = ''
    try { browser = await api.browse(path); browserPath = browser.path } catch (err) { error = (err as Error).message }
    finally { browserBusy = false }
  }
  async function addBrowserPath(path: string) {
    browserBusy = true; error = ''
    try { addToCart((await api.expand([path])).paths); browserOpen = false } catch (err) { error = (err as Error).message }
    finally { browserBusy = false }
  }
  async function startJobs() {
    if (!cart.length || busy) return
    if (!window.confirm(`${cart.length}개 파일을 표시된 순서대로 회의록으로 정리할까요?`)) return
    busy = true; error = ''
    try { await api.createJobs(cart.map(file => file.path), model, effort); cart = []; selectedPath = ''; ++previewRequest; preview = '파일을 선택하면 여기에 원문을 표시합니다.'; previewTranscribed = false }
    catch (err) { error = (err as Error).message } finally { busy = false }
  }
  async function refreshEnvironment() {
    environmentLoading = true; environmentError = ''
    try { environment = await api.environment() }
    catch (err) { environmentError = (err as Error).message }
    finally { environmentLoading = false }
  }
  function openSettings() { settingsDraft = config ? { ...config } : null; testLog = ''; testResult = ''; environment = null; settingsOpen = true; void refreshEnvironment() }
  async function startWhisperInstall() {
    if (!settingsOpen && config) openSettings()
    try { await api.installWhisper() }
    catch (err) { error = (err as Error).message }
  }
  async function saveSettings() {
    if (!settingsDraft) return; busy = true; error = ''
    try { const previousModel = config?.whisperModel; config = await api.saveConfig(settingsDraft); promptValue = config.prompt; settingsOpen = false; if (selectedPath && isMedia(selectedPath) && previousModel !== config.whisperModel) await loadPreview(selectedPath); await loadModels() }
    catch (err) { error = (err as Error).message } finally { busy = false }
  }
  async function savePrompt() {
    if (!config) return; busy = true
    try { config = await api.saveConfig({ ...config, prompt: promptValue }); promptEditing = false }
    catch (err) { error = (err as Error).message } finally { busy = false }
  }
  async function testCodex() {
    testRunning = true; testResult = ''; testLog = `${new Date().toLocaleTimeString()} · Codex app-server 연결\n${new Date().toLocaleTimeString()} · gpt-5.6-luna / low · Hello world! 전송 중...`
    try { const result = await api.testCodex('gpt-5.6-luna'); testResult = result.result; testLog += `\n${new Date().toLocaleTimeString()} · 응답: ${result.result}\n테스트 완료` }
    catch (err) { testLog += `\n${new Date().toLocaleTimeString()} · 오류: ${(err as Error).message}` }
    finally { testRunning = false }
  }
  async function enableNotifications() { if (!('Notification' in window)) { error = '이 브라우저는 알림을 지원하지 않습니다.'; return }; notificationsEnabled = (await Notification.requestPermission()) === 'granted'; if (!notificationsEnabled) error = '브라우저 알림 권한이 허용되지 않았습니다.' }
  async function copy(value: string) { try { await navigator.clipboard.writeText(value) } catch { error = '복사할 수 없습니다. 브라우저 권한을 확인해 주세요.' } }
  async function deleteJob(job: Job) { try { await api.deleteJob(job.id); if (detailId === job.id) detailId = '' } catch (err) { error = (err as Error).message } }
  async function cancelJob(job: Job) { try { await api.cancelJob(job.id) } catch (err) { error = (err as Error).message } }
  async function loadDetailTranscript(id: string) {
    try { const result = await api.jobTranscript(id); if (detailId === id) detailTranscript = result.text }
    catch { if (detailId === id) detailTranscript = '' }
  }
  function openDetail(job: Job) { detailId = job.id; detailTranscript = ''; detailLogAtBottom = true; if (job.kind === 'transcription' || ['extracting', 'transcribing', 'transcribed'].includes(job.stage || '')) void loadDetailTranscript(job.id) }
  function onDrop(event: DragEvent) { event.preventDefault(); dropActive = false; if (event.dataTransfer?.files.length) void importFiles(event.dataTransfer.files) }
  function onCartDrop(event: DragEvent, index: number) { event.preventDefault(); cart = moveFile(cart, dragIndex, index); dragIndex = -1 }
</script>

<div class="app-shell">
  <header class="topbar"><div class="brand"><span class="brand-mark" aria-hidden="true">M</span><h1>회의록 만들기</h1></div><div class="statuses">
    <span class:warn={!state.whisperReady} class="status-pill"><span class="dot"></span>{state.whisperInstalling ? `${state.whisperInstall?.message || 'Whisper 설치 중'}${installPercent === null ? '' : ` · ${installPercent}%`}` : state.whisperReady ? 'Whisper 모델 준비됨' : 'Whisper 모델 미설치'} {#if !state.whisperReady && !state.whisperInstalling}<Button size="xs" onclick={startWhisperInstall}>설치</Button>{/if}</span>
    <span class:warn={!state.codexReady} class="status-pill"><span class="dot"></span>{state.codexReady ? 'Codex 로컬 서비스 준비됨' : 'Codex 실행 파일 확인 필요'}</span>
    <Button aria-label="설정 열기" variant="outline" size="sm" onclick={openSettings}>설정</Button>
  </div></header>
  {#if error}<div role="alert" class="error" style="margin-bottom:12px">{error} <button class="mini-button" onclick={() => error = ''} aria-label="오류 닫기">닫기</button></div>{/if}
  {#if state.whisperError}<div role="alert" class="error" style="margin-bottom:12px">Whisper 설치 실패: {state.whisperError}</div>{/if}
  <main class="workspace">
    <section class="panel" aria-labelledby="input-title">
      <div class="section-top"><h2 id="input-title" class="section-number">01 / INPUT</h2><span class="compact muted">{cart.length}개 파일</span></div>
      <div class="button-row"><Button onclick={() => openBrowser('file')}>파일 선택</Button><Button variant="outline" onclick={() => openBrowser('folder')}>폴더 선택</Button></div>
      <input aria-label="컴퓨터에서 파일 가져오기" bind:this={uploadInput} hidden type="file" multiple accept=".srt,.vtt,.txt,.md,.wav,.mp3,.m4a,.mp4,.mov,.ogg" onchange={event => importFiles(event.currentTarget.files || [])} />
      <button class:active={dropActive} class="drop-zone" ondragover={event => { event.preventDefault(); dropActive = true }} ondragleave={() => dropActive = false} ondrop={onDrop} onclick={() => uploadInput?.click()}>파일을 여기에 끌어놓거나 컴퓨터에서 가져오기<br /><span class="tiny muted">SRT · VTT · TXT · MD · 오디오 · 영상</span></button>
      <div class="queue-title"><h3 class="section-title">처리 순서</h3><span class="tiny muted">끌어서 순서 변경</span></div>
      {#if cart.length}<ol class="cart-list">{#each cart as file, index (file.path)}<li class:selected={selectedPath === file.path} class="cart-item" draggable="true" ondragstart={() => dragIndex = index} ondragover={event => event.preventDefault()} ondrop={event => onCartDrop(event, index)}>
        <span class="tiny muted">{String(index + 1).padStart(2, '0')}</span><button class="cart-main" onclick={() => selectFile(file.path)} title={file.path}><span class="truncate">{file.name}</span><span class="tiny muted truncate">{file.path}</span></button>
        <span class="cart-controls"><button class="mini-button" aria-label={`${file.name} 위로`} disabled={index === 0} onclick={() => cart = moveFile(cart, index, index - 1)}>↑</button><button class="mini-button" aria-label={`${file.name} 아래로`} disabled={index === cart.length - 1} onclick={() => cart = moveFile(cart, index, index + 1)}>↓</button></span>
        <button class="mini-button" aria-label={`${file.name} 삭제`} onclick={() => removeFromCart(file.path)}>×</button>
        {#if isMedia(file.path)}{@const fileJob = [...state.jobs].reverse().find(job => job.path === file.path && job.kind === 'transcription' && (job.transcriptionModel || config?.whisperModel) === config?.whisperModel)}{@const fileReady = selectedPath === file.path && previewTranscribed}
          <div class="cart-media-actions"><span class="tiny muted">{fileReady ? '전사문 준비됨' : fileJob?.status === 'failed' ? '전사 실패 · 다시 시도 가능' : isActive(fileJob) ? fileJob?.phase : '오디오·영상 전사'}</span><Button size="xs" disabled={transcriptionBusyPath === file.path || isActive(fileJob) || fileReady || !state.whisperReady} onclick={() => startTranscription(file.path)}>{isActive(fileJob) ? '전사 중' : fileReady ? '전사 완료' : '전사 시작'}</Button></div>
        {/if}
      </li>{/each}</ol>{:else}<p class="empty">파일을 등록하면 여기에서 순서를 바꿀 수 있습니다.</p>{/if}
    </section>
    <section class="panel" aria-labelledby="preview-title"><div class="section-top"><h2 id="preview-title" class="section-number">02 / PREVIEW</h2></div><div class="preview-file"><div class="section-title truncate">{selectedFile?.name || '선택한 파일 없음'}</div><div class="tiny muted truncate">{selectedFile?.path || '왼쪽에서 파일을 선택해 주세요.'}</div></div><div class="section-top"><h3 class="section-title">{selectedFile && isMedia(selectedFile.path) ? '전사문 미리보기' : '텍스트 미리보기'}</h3><span class="tiny muted">앞 64KB 표시</span></div>
      {#if selectedFile && isMedia(selectedFile.path)}
        <textarea class="preview-box media-preview-box" aria-label="전사문 미리보기" readonly value={previewTranscribed ? preview : selectedMediaJob?.transcriptPreview || (previewLoading ? '불러오는 중...' : preview)}></textarea>
        <div class="section-top media-log-title"><h3 class="section-title">Whisper 전사 로그</h3><span class="tiny muted">실시간 · 읽기 전용</span></div>
        <textarea class="media-log" aria-label="Whisper 전사 로그" readonly bind:this={whisperLogArea} onscroll={event => updateScrollPin(event)} value={selectedLog || '전사를 시작하면 FFmpeg와 Whisper 실행 내용이 여기에 표시됩니다.'}></textarea>
        <p class="tiny muted media-log-hint">다른 파일을 선택하면 해당 파일의 로그를 표시합니다.</p>
      {:else}<div class="preview-box" aria-live="polite">{previewLoading ? '불러오는 중...' : preview}</div>{/if}
    </section>
    <section class="panel" aria-labelledby="settings-title"><div class="section-top"><h2 id="settings-title" class="section-number">03 / SETTINGS</h2></div><div class="section-top"><h3 class="section-title">회의록 프롬프트</h3><div class="button-row" style="flex:0 0 auto"><Button variant="outline" size="xs" onclick={() => promptEditing ? savePrompt() : promptEditing = true}>{promptEditing ? '저장' : '수정'}</Button><Button variant="outline" size="xs" onclick={() => copy(promptValue)}>복사</Button></div></div>
      <Textarea class="prompt-area" aria-label="회의록 프롬프트" readonly={!promptEditing} bind:value={promptValue} />
      <div class="panel-footer"><label class="field-label" for="model-select">GPT 모델</label><NativeSelect class="w-full" id="model-select" bind:value={model}><option value="gpt-6-astra" disabled={!modelAvailable('gpt-6-astra')}>GPT-6 Astra{modelAvailable('gpt-6-astra') ? '' : ' · CLI 업데이트 필요'}</option><option value="gpt-6-sol" disabled={!modelAvailable('gpt-6-sol')}>GPT-6 Sol{modelAvailable('gpt-6-sol') ? '' : ' · CLI 업데이트 필요'}</option><option value="gpt-5.6-sol" disabled={!modelAvailable('gpt-5.6-sol')}>GPT-5.6 Sol</option></NativeSelect>
        <label class="field-label" for="effort-select">추론 수준</label><NativeSelect class="w-full" id="effort-select" bind:value={effort}>{#each efforts as value}<option value={value}>{effortName[value]}</option>{/each}</NativeSelect>
        <Button class="w-full mt-3" size="lg" disabled={!cart.length || busy || !state.codexReady} onclick={startJobs}>회의록 정리 시작</Button><p class="tiny muted" style="text-align:center;margin-top:8px">{cart.length}개 파일을 위에서 정한 순서대로 처리합니다.</p>
      </div></section>
    <aside class="panel sidebar" aria-label="작업 상태"><div class="sidebar-columns">
      <div class="notice"><div class="section-title">작업 완료 알림</div><p class="compact muted" style="margin:7px 0 0">정리가 끝나면 브라우저 알림으로 알려드릴게요.</p><Button size="sm" disabled={notificationsEnabled} onclick={enableNotifications}>{notificationsEnabled ? '알림 켜짐' : '알림 켜기'}</Button></div>
      <section><div class="section-top"><h2 class="section-title">작업 큐</h2><span class="tiny muted">{activeJobs.length}건</span></div><div class="job-list">{#each activeJobs as job (job.id)}<button class="job-card" onclick={() => openDetail(job)}><div class="job-active"><span class="truncate">{job.name}</span><span class="tiny">{job.kind === 'transcription' ? '전사' : '회의록'} · {job.status === 'queued' ? '대기 중' : '진행 중'}</span></div><div class="job-meta truncate">{job.phase}</div>{#if job.status !== 'queued'}<div class="indeterminate" aria-label="작업 진행 중"></div>{/if}</button>{:else}<p class="empty">대기 중인 작업이 없습니다.</p>{/each}</div></section>
      <section><div class="section-top"><h2 class="section-title">완료 목록</h2><span class="tiny muted">{finishedJobs.length}건</span></div><div class="job-list">{#each finishedJobs.slice(0, 12) as job (job.id)}<button class="job-card" onclick={() => openDetail(job)}><div class="job-active"><span class="truncate">{job.name}</span><span class="tiny">{job.kind === 'transcription' ? '전사' : '회의록'} · {job.status === 'completed' ? '완료' : job.status === 'failed' ? '실패' : '중단'}</span></div><div class="job-meta truncate">{job.outputPath || (job.transcriptPath ? `SRT 저장됨 · ${job.error || job.phase}` : job.error || job.phase)}</div></button>{:else}<p class="empty">완료한 작업이 없습니다.</p>{/each}</div></section>
    </div></aside>
  </main>
</div>

<Dialog.Root bind:open={browserOpen}><Dialog.Content class="!max-w-2xl max-h-[90vh] overflow-auto"><Dialog.Header><Dialog.Title>{browserMode === 'folder' ? '폴더 선택' : '파일 선택'}</Dialog.Title><Dialog.Description>컴퓨터의 폴더를 탐색합니다. 폴더를 추가하면 지원 파일을 이름순으로 등록합니다.</Dialog.Description></Dialog.Header>
  <div class="button-row"><Input aria-label="폴더 경로" bind:value={browserPath} onkeydown={event => { if (event.key === 'Enter') browseTo(browserPath) }} /><Button variant="outline" onclick={() => browseTo(browserPath)}>이동</Button></div>
  {#if browser}<div class="button-row"><Button variant="outline" onclick={() => browseTo(browser!.parent)}>상위 폴더</Button>{#if browserMode === 'folder'}<Button onclick={() => addBrowserPath(browser!.path)} disabled={browserBusy}>현재 폴더 추가</Button>{/if}</div><div class="browser-list">{#each browser.entries as entry (entry.path)}<div class="browser-row"><button class="mini-button" onclick={() => entry.isDir ? browseTo(entry.path) : addBrowserPath(entry.path)}>{entry.isDir ? '폴더 · ' : '파일 · '}{entry.name}</button>{#if entry.isDir}<Button size="xs" variant="outline" onclick={() => addBrowserPath(entry.path)}>추가</Button>{/if}</div>{:else}<p class="empty">표시할 파일이 없습니다.</p>{/each}</div>{/if}
  {#if browserBusy}<p class="muted compact">불러오는 중...</p>{/if}
</Dialog.Content></Dialog.Root>

<Dialog.Root bind:open={settingsOpen}><Dialog.Content class="settings-dialog !max-w-[900px] w-[calc(100vw-2rem)] max-h-[90vh] overflow-auto"><Dialog.Header><Dialog.Title>설정</Dialog.Title></Dialog.Header>
  {#if settingsDraft}
    <div class="settings-heading"><h3>실행 환경</h3><Button variant="outline" size="xs" disabled={environmentLoading} onclick={refreshEnvironment}>{environmentLoading ? '확인 중' : '재검색'}</Button></div>
    {#if environmentError}<div class="error" role="alert">환경 정보를 읽지 못했습니다: {environmentError}</div>{/if}
    <div class="settings-fields">
      <div class="settings-field">
        <div class="settings-field-head"><label class="field-label" for="codex-path">Codex 실행 파일</label><span class="env-status" class:ready={environment?.codex.ready}>{environmentLoading ? '확인 중' : environment?.codex.ready ? '탐색됨' : '확인 필요'}</span></div>
        <Input id="codex-path" bind:value={settingsDraft.codexBinary} placeholder="codex 또는 실행 파일 절대경로" />
        <p class="settings-hint">PATH에서 자동 탐색합니다. 경로를 직접 지정할 수도 있습니다.</p>
        <div class="resolved-info"><span>현재 경로</span><code>{environment?.codex.path || environment?.codex.error || '확인 중'}</code></div>
      </div>
      <div class="settings-field">
        <label class="field-label" for="output-dir">회의록 저장 폴더</label>
        <Input id="output-dir" bind:value={settingsDraft.outputDir} placeholder="비워두면 원본 파일과 같은 폴더" />
        <p class="settings-hint">비워두면 각 원본 파일 옆에 회의록을 저장합니다.</p>
      </div>
    </div>
    <div class="settings-fields settings-tools">
      <div class="settings-field tool-card">
        <div class="settings-field-head"><label class="field-label" for="whisper-model">Whisper 모델</label><span class="env-status" class:ready={environment?.whisper.model === settingsDraft.whisperModel && environment?.whisper.modelReady && environment?.whisper.binaryReady}>{environmentLoading ? '확인 중' : !environment ? '확인 필요' : environment.whisper.model !== settingsDraft.whisperModel ? '저장 후 확인' : environment.whisper.modelReady && environment.whisper.binaryReady ? '준비됨' : '설치 필요'}</span></div>
        <NativeSelect class="w-full" id="whisper-model" bind:value={settingsDraft.whisperModel}><option value="large-v3-turbo">Large V3 Turbo · 다국어</option><option value="base">Base · 다국어</option><option value="small">Small · 다국어</option></NativeSelect>
        {#if environment && environment.whisper.model !== settingsDraft.whisperModel}<p class="settings-hint">모델 변경은 설정 저장 후 적용됩니다. 현재 적용: {environment.whisper.model}</p>{:else}<p class="settings-hint">현재 적용: {environment?.whisper.model || '확인 중'} · 자동 설치 기준 whisper.cpp {environment?.whisper.installerVersion || '확인 중'}</p>{/if}
        <div class="resolved-info"><span>모델 파일</span><code>{environment?.whisper.modelPath || '확인 중'}</code></div>
        <div class="resolved-info"><span>Whisper 실행 파일</span><code>{environment?.whisper.binaryPath || '미설치'}</code></div>
      </div>
      <div class="settings-field tool-card">
        <div class="settings-field-head"><label class="field-label" for="ffmpeg-path">ffmpeg 실행 파일</label><span class="env-status" class:ready={environment?.ffmpeg.ready}>{environmentLoading ? '확인 중' : environment?.ffmpeg.ready ? '탐색됨' : '확인 필요'}</span></div>
        <Input id="ffmpeg-path" bind:value={settingsDraft.ffmpegBinary} placeholder="ffmpeg 또는 실행 파일 절대경로" />
        <p class="settings-hint">설치된 버전: {environment?.ffmpeg.version || environment?.ffmpeg.error || '확인 중'}</p>
        <div class="resolved-info"><span>현재 경로</span><code>{environment?.ffmpeg.path || '찾지 못했습니다'}</code></div>
      </div>
    </div>
    {#if state.whisperInstalling || state.whisperInstall?.stage === 'completed' || state.whisperInstall?.stage === 'failed'}
      <div class="install-panel" aria-live="polite">
        <div class="settings-field-head"><h3>Whisper 설치</h3><span class="tiny muted">모델: {state.whisperInstall?.model || '확인 중'}</span></div>
        <p class="settings-hint">{state.whisperInstall?.message || '설치 준비 중'}{#if state.whisperInstalling && state.whisperInstall?.downloaded}{` · ${formatBytes(state.whisperInstall.downloaded)}${state.whisperInstall.total ? ` / ${formatBytes(state.whisperInstall.total)}` : ''}`}{/if}</p>
        {#if state.whisperInstalling}
          {#if installPercent !== null}<div class="install-progress" role="progressbar" aria-label="Whisper 설치 다운로드 진행률" aria-valuenow={installPercent} aria-valuemin="0" aria-valuemax="100"><span style:width={`${installPercent}%`}></span></div>
          {:else}<div class="indeterminate install-indeterminate" aria-label="Whisper 설치 단계 진행 중"></div>{/if}
        {/if}
        <textarea class="install-log" aria-label="Whisper 설치 로그" readonly bind:this={installLogArea} value={state.whisperInstall?.log || '설치를 시작하면 로그가 표시됩니다.'}></textarea>
        {#if state.whisperInstall?.stage === 'failed'}<Button size="sm" onclick={startWhisperInstall}>다시 시도</Button>{/if}
      </div>
    {/if}
    <div class="settings-test"><div class="settings-field-head"><h3>실제 응답 테스트</h3><Button size="sm" disabled={testRunning} onclick={testCodex}>{testRunning ? '테스트 중' : 'GPT-5.6 Luna로 테스트'}</Button></div><p class="settings-hint">Hello world!를 보내 실제 응답을 확인합니다. 모델 사용량이 발생할 수 있습니다.</p><div class="log-box" role="log" aria-live="polite">{testLog || '테스트를 실행하면 로그가 여기에 표시됩니다.'}</div>{#if testResult}<div class="success" style="margin-top:8px">모델 응답: {testResult}</div>{/if}</div>
    <Dialog.Footer class="settings-footer"><Button variant="outline" onclick={() => settingsOpen = false}>취소</Button><Button disabled={busy} onclick={saveSettings}>설정 저장</Button></Dialog.Footer>
  {/if}
</Dialog.Content></Dialog.Root>

<Dialog.Root open={!!detailJob} onOpenChange={open => { if (!open) detailId = '' }}><Dialog.Content class="!max-w-[1280px] w-[calc(100vw-2rem)] max-h-[92vh] overflow-auto">
  {#if detailJob}<Dialog.Header><Dialog.Title>{detailJob.name}</Dialog.Title><Dialog.Description>{detailJob.phase} · {detailJob.kind === 'transcription' ? `Whisper ${detailJob.transcriptionModel || ''}` : `${detailJob.model} / ${effortName[detailJob.effort]}`}</Dialog.Description></Dialog.Header>
    <div class="modal-grid"><div><div class="section-top"><h3 class="section-title">{detailTranscribing ? '전사문' : 'Codex 결과'}</h3>{#if !detailTranscribing}<Button variant="outline" size="xs" onclick={() => copy(detailJob!.result)}>복사</Button>{/if}</div><textarea class="modal-textarea" aria-label={detailTranscribing ? '전사문' : 'Codex 결과'} readonly value={detailTranscribing ? detailJob.transcriptPreview || detailTranscript || '전사문을 기다리는 중입니다.' : detailJob.result || '결과를 기다리는 중입니다.'}></textarea></div><div><div class="section-top"><h3 class="section-title">{detailTranscribing ? 'Whisper 전사 로그' : '최근 출력'}</h3><span class="tiny muted">{isActive(detailJob) ? '실시간' : '작업 로그'}</span></div><textarea class="modal-textarea" aria-label={detailTranscribing ? 'Whisper 전사 로그' : '최근 출력'} readonly bind:this={detailLogArea} onscroll={event => updateScrollPin(event, true)} value={detailLog || detailJob.phase}></textarea></div></div>
    {#if detailJob.error}<div class="error">{detailJob.error}</div>{/if}
    {#if detailJob.transcriptPath}<div class="success">SRT 저장 위치: {detailJob.transcriptPath} <Button size="xs" variant="outline" onclick={() => copy(detailJob!.transcriptPath!)}>경로 복사</Button></div>{/if}
    {#if detailJob.outputPath}<div class="success">회의록 저장 위치: {detailJob.outputPath} <Button size="xs" variant="outline" onclick={() => copy(detailJob!.outputPath)}>경로 복사</Button></div>{/if}
    <details class="modal-section"><summary>요청 정보</summary><p class="compact muted">원본: {detailJob.path}</p><p class="compact muted">Whisper 모델: {detailJob.transcriptionModel || '설정값'}</p>{#if detailJob.kind !== 'transcription'}<p class="compact muted">Codex 모델: {detailJob.model} · {effortName[detailJob.effort]}</p><Textarea readonly value={detailJob.prompt} aria-label="전송 프롬프트" class="w-full min-h-36" />{#if detailJob.transcriptionLog}<p class="compact muted">전사 로그</p><Textarea readonly value={detailJob.transcriptionLog} aria-label="전사 로그 기록" class="w-full min-h-36" />{/if}{/if}</details>
    <Dialog.Footer>{#if ['running','preparing','queued'].includes(detailJob.status)}<Button variant="destructive" onclick={() => cancelJob(detailJob!)}>작업 취소</Button>{:else}<Button variant="outline" onclick={() => deleteJob(detailJob!)}>목록에서 삭제</Button>{/if}<Button onclick={() => detailId = ''}>닫기</Button></Dialog.Footer>
  {/if}
</Dialog.Content></Dialog.Root>
