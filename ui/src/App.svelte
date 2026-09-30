<script lang="ts">
  import { onMount } from 'svelte'
  import { createSelection } from '$lib/selection'
  import { createJobDetail } from '$lib/job-detail'
  import ApplicationHeader from '$lib/components/workspace/ApplicationHeader.svelte'
  import InputPanel from '$lib/components/workspace/InputPanel.svelte'
  import PreviewPanel from '$lib/components/workspace/PreviewPanel.svelte'
  import PromptPanel from '$lib/components/workspace/PromptPanel.svelte'
  import JobSidebar from '$lib/components/workspace/JobSidebar.svelte'
  import FileBrowser from '$lib/components/workspace/FileBrowser.svelte'
  import SettingsDialog from '$lib/components/workspace/SettingsDialog.svelte'
  import JobDetail from '$lib/components/workspace/JobDetail.svelte'
  import { api, type Config, type Job, type Model, type Snapshot } from '$lib/api'
  import {
    addPaths,
    attachTranscript,
    removeFile,
    canSubmit,
    syncCartJobs,
    indexJobs,
    isMedia,
    isActive,
    type CartFile,
  } from '$lib/cart'
  import { createCartStorage } from '$lib/cart-storage'
  import { createCartRecovery } from '$lib/cart-recovery'
  import { createNotifications } from '$lib/notifications'
  import { connectState, type ConnectionIssue } from '$lib/live-state'

  // The root coordinates server data and selection. Panels own their local form/scroll state.
  let state: Snapshot = {
    jobs: [],
    codexReady: false,
    whisperReady: false,
    whisperInstalling: false,
    whisperError: '',
  }
  let config: Config | null = null
  let promptValue = ''
  let promptEditing = false
  let cart: CartFile[] = []
  let mounted = false
  const cartStorage = createCartStorage()
  const cartRecovery = createCartRecovery(
    () => cart,
    (files) => (cart = files),
  )
  const notifications = createNotifications()
  let connection: ReturnType<typeof connectState> | undefined
  let connectionIssue: ConnectionIssue = 'none'
  let lastStateAt = 0
  let notificationsEnabled = false
  let transcriptionBusyPath = ''
  let model = 'gpt-5.6-sol'
  let effort = 'medium'
  let models: Model[] = []
  let error = ''
  let busy = false
  let settingsOpen = false
  let browserOpen = false
  let browserMode: 'file' | 'folder' = 'file'
  let clearingFinished = false
  let cancellingIds = new Set<string>()
  const cancelTimers = new Map<string, number>()
  const selection = createSelection()
  const details = createJobDetail(showError)
  const openDetail = details.open
  const closeDetail = details.close

  // These computations follow their own inputs; log updates never serialize the cart.
  $: if (mounted) cartStorage.write(cart)
  $: readyFiles = cart.filter(canSubmit)
  $: jobIndex = indexJobs(state.jobs, config?.whisperModel)
  $: selectedFile = cart.find((file) => file.path === $selection.path)
  $: selectedMediaJob = jobIndex.media.get($selection.path)
  $: selectedLog =
    (selectedMediaJob &&
    !isActive(selectedMediaJob) &&
    $selection.fullJob?.id === selectedMediaJob.id
      ? $selection.fullJob.transcriptionLog
      : selectedMediaJob?.transcriptionLog) || ''
  $: selectedPreviewText = $selection.transcribed
    ? $selection.preview
    : selectedMediaJob?.transcriptPreview ||
      ($selection.loading ? '불러오는 중...' : $selection.preview)
  $: activeJobs = state.jobs.filter(isActive)
  $: finishedJobs = state.jobs.filter((job) => !isActive(job)).reverse()
  $: stateDetailJob = state.jobs.find((job) => job.id === $details.id)
  $: detailJob = stateDetailJob
    ? isActive(stateDetailJob)
      ? { ...stateDetailJob, prompt: $details.full?.id === $details.id ? $details.full.prompt : '' }
      : $details.full?.id === $details.id
        ? $details.full
        : stateDetailJob
    : undefined
  function showError(message: string) {
    error = message
  }
  function resyncState(force = false) {
    return connection?.resync(force)
  }
  function hasTranscript(job?: Job) {
    return (
      !!job &&
      (job.stage === 'transcribed' ||
        (job.kind === 'transcription' && job.status === 'completed') ||
        (!!job.transcriptionLog && ['drafting', 'completed'].includes(job.stage || '')))
    )
  }

  function acceptState(next: Snapshot) {
    if (!next || typeof next !== 'object' || (next.jobs != null && !Array.isArray(next.jobs)))
      throw new Error('올바르지 않은 작업 상태입니다.')
    next = { ...next, jobs: next.jobs ?? [] }
    if (next.jobs.some((job) => !job || typeof job !== 'object'))
      throw new Error('올바르지 않은 작업 목록입니다.')
    const previous = new Map(state.jobs.map((job) => [job.id, job]))
    let nextCart = syncCartJobs(cart, next.jobs)
    for (const job of next.jobs) {
      if (
        job.transcriptPath &&
        isMedia(job.path) &&
        previous.get(job.id)?.transcriptPath !== job.transcriptPath
      )
        nextCart = attachTranscript(nextCart, job.path, job.transcriptPath)
      if (cancellingIds.has(job.id) && !isActive(job)) clearCancelling(job.id)
    }
    const updatedCart = syncCartJobs(nextCart, next.jobs)
    if (updatedCart !== cart) cart = updatedCart
    cartRecovery.accept(next.jobs)
    notifications.accept(next.jobs, notificationsEnabled)
    state = next
    if (
      isMedia($selection.path) &&
      next.jobs.some(
        (job) =>
          job.path === $selection.path &&
          hasTranscript(job) &&
          !hasTranscript(previous.get(job.id)),
      )
    )
      void selection.load($selection.path)
    const finished = next.jobs.filter(
      (job) => !isActive(job) && previous.get(job.id)?.status !== job.status,
    )
    const detail = finished.find((job) => job.id === $details.id)
    if (detail) details.refresh(detail)
    const selected = finished.filter((job) => job.path === $selection.path).pop()
    if (selected && isMedia($selection.path)) void selection.loadJob(selected)
  }

  onMount(() => {
    mounted = true
    cart = cartStorage.read()
    notificationsEnabled = notifications.read()
    if (cart.length) selectFile(cart.find((file) => !file.disabled)?.path || cart[0].path)
    connection = connectState(
      acceptState,
      (value) => (connectionIssue = value),
      (value) => (lastStateAt = value),
    )
    api
      .config()
      .then((value) => {
        if (mounted) {
          config = value
          promptValue = value.prompt
        }
      })
      .catch((cause) => {
        if (mounted) error = (cause as Error).message
      })
    void loadModels().catch(() => {})
    return () => {
      mounted = false
      connection?.dispose()
      cartRecovery.dispose()
      selection.dispose()
      details.dispose()
      for (const timer of cancelTimers.values()) window.clearTimeout(timer)
      cancelTimers.clear()
    }
  })

  async function loadModels() {
    const result = await api.models()
    if (!mounted) return
    models = result.models ?? []
    if (models.length && !models.some((item) => item.model === model || item.id === model))
      model = models.some((item) => item.model === 'gpt-5.6-sol') ? 'gpt-5.6-sol' : models[0].model
  }
  function addToCart(paths: string[]) {
    let next = syncCartJobs(addPaths(cart, paths), state.jobs)
    for (const path of paths) {
      const transcript = jobIndex.media.get(path)?.transcriptPath
      if (transcript) next = attachTranscript(next, path, transcript)
    }
    cart = next
    if (!$selection.path && cart.length) selectFile(cart[0].path)
  }
  function selectFile(path: string) {
    selection.select(path, jobIndex.media.get(path))
  }
  function clearCart() {
    cart = []
    selection.clear()
  }
  function removeFromCart(path: string) {
    cart = removeFile(cart, path)
    if ($selection.path !== path) return
    const remaining = cart
    clearCart()
    cart = remaining
    if (cart.length) selectFile(cart.find((file) => !file.disabled)?.path || cart[0].path)
  }
  async function importFiles(files: FileList | File[]) {
    if (!files.length || busy) return
    busy = true
    error = ''
    try {
      addToCart((await api.importFiles(files)).paths)
    } catch (cause) {
      error = (cause as Error).message
    } finally {
      busy = false
    }
  }
  function openBrowser(mode: 'file' | 'folder') {
    browserMode = mode
    browserOpen = true
  }
  async function startTranscription(path: string) {
    if (transcriptionBusyPath) return
    transcriptionBusyPath = path
    error = ''
    selectFile(path)
    try {
      await api.createTranscription(path)
      void resyncState(true)
    } catch (cause) {
      error = (cause as Error).message
    } finally {
      transcriptionBusyPath = ''
    }
  }
  async function enqueueMinutes(files: CartFile[]) {
    busy = true
    error = ''
    try {
      const result = await api.createJobs(
        files.map((file) => file.path),
        model,
        effort,
      )
      cart = syncCartJobs(cart, result.jobs)
      void resyncState(true)
    } catch (cause) {
      error = (cause as Error).message
    } finally {
      busy = false
    }
  }
  async function startJobs() {
    if (
      !readyFiles.length ||
      busy ||
      !window.confirm(readyFiles.length + '개 파일을 표시된 순서대로 회의록으로 정리할까요?')
    )
      return
    await enqueueMinutes(readyFiles)
  }
  async function rerunFile(file: CartFile) {
    if (
      busy ||
      isActive(file.minutesJob) ||
      !window.confirm(file.name + ' 파일을 다시 정리할까요?')
    )
      return
    await enqueueMinutes([file])
  }
  async function settingsSaved(value: Config) {
    const previousModel = config?.whisperModel
    config = value
    promptValue = value.prompt
    if (isMedia($selection.path) && previousModel !== value.whisperModel)
      await selection.load($selection.path)
    await loadModels()
  }
  async function savePrompt() {
    if (!config || busy) return
    busy = true
    try {
      config = await api.saveConfig({ ...config, prompt: promptValue })
      promptEditing = false
    } catch (cause) {
      error = (cause as Error).message
    } finally {
      busy = false
    }
  }
  async function startWhisperInstall() {
    settingsOpen = true
    try {
      await api.installWhisper()
    } catch (cause) {
      error = (cause as Error).message
    }
  }
  async function toggleNotifications() {
    error = ''
    try {
      notificationsEnabled = await notifications.toggle(notificationsEnabled)
    } catch (cause) {
      error = (cause as Error).message
    }
  }
  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value)
    } catch {
      error = '복사할 수 없습니다. 브라우저 권한을 확인해 주세요.'
    }
  }
  async function deleteJob(job: Job) {
    try {
      await api.deleteJob(job.id)
      if ($details.id === job.id) closeDetail()
      void resyncState(true)
    } catch (cause) {
      error = (cause as Error).message
    }
  }
  async function clearFinishedJobs() {
    if (
      !finishedJobs.length ||
      clearingFinished ||
      !window.confirm(
        '완료·실패·중단 기록을 모두 지울까요?\n생성된 SRT와 Markdown 파일은 삭제되지 않습니다.',
      )
    )
      return
    const close = !!detailJob && !isActive(detailJob)
    clearingFinished = true
    error = ''
    try {
      const result = await api.deleteFinishedJobs()
      if (close) closeDetail()
      connection?.apply(result.state)
    } catch (cause) {
      error = (cause as Error).message
    } finally {
      clearingFinished = false
    }
  }
  function clearCancelling(id: string) {
    cancellingIds = new Set([...cancellingIds].filter((value) => value !== id))
    const timer = cancelTimers.get(id)
    if (timer !== undefined) window.clearTimeout(timer)
    cancelTimers.delete(id)
  }
  async function cancelJob(job: Job) {
    if (cancellingIds.has(job.id)) return
    cancellingIds = new Set(cancellingIds).add(job.id)
    cancelTimers.set(
      job.id,
      window.setTimeout(() => clearCancelling(job.id), 15_000),
    )
    try {
      connection?.apply(await api.cancelJob(job.id))
      void resyncState(true)
    } catch (cause) {
      clearCancelling(job.id)
      error = (cause as Error).message
    }
  }
</script>

<div class="app-shell">
  <ApplicationHeader
    {state}
    {connectionIssue}
    bind:error
    openSettings={() => (settingsOpen = true)}
    {startWhisperInstall}
    {resyncState}
  />
  <main class="workspace">
    <InputPanel
      bind:cart
      selectedPath={$selection.path}
      previewTranscribed={$selection.transcribed}
      {transcriptionBusyPath}
      {busy}
      codexReady={state.codexReady}
      whisperReady={state.whisperReady}
      {cancellingIds}
      {jobIndex}
      {selectFile}
      {removeFromCart}
      {startTranscription}
      {cancelJob}
      {rerunFile}
      {clearCart}
      {openBrowser}
      {importFiles}
    />
    <PreviewPanel
      {selectedFile}
      previewLoading={$selection.loading}
      preview={$selection.preview}
      {selectedPreviewText}
      {selectedLog}
    />
    <PromptPanel
      bind:promptValue
      bind:promptEditing
      bind:model
      bind:effort
      {models}
      readyCount={readyFiles.length}
      {busy}
      codexReady={state.codexReady}
      {savePrompt}
      {copy}
      {startJobs}
    />
    <JobSidebar
      {activeJobs}
      {finishedJobs}
      {lastStateAt}
      {notificationsEnabled}
      {clearingFinished}
      {cancellingIds}
      {toggleNotifications}
      {openDetail}
      {cancelJob}
      {clearFinishedJobs}
    />
  </main>
</div>
<FileBrowser bind:open={browserOpen} mode={browserMode} onPaths={addToCart} onError={showError} />
<SettingsDialog
  bind:open={settingsOpen}
  {config}
  {state}
  onSaved={settingsSaved}
  onError={showError}
/>
<JobDetail
  {detailJob}
  detailTranscript={$details.transcript}
  {cancellingIds}
  {copy}
  {closeDetail}
  {cancelJob}
  {deleteJob}
/>
