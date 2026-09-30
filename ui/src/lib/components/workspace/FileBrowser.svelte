<script lang="ts">
  import { Button } from '$lib/components/ui/button/index.js'
  import { Input } from '$lib/components/ui/input/index.js'
  import * as Dialog from '$lib/components/ui/dialog/index.js'
  import { api, type BrowseResult } from '$lib/api'
  export let open = false
  export let mode: 'file' | 'folder' = 'file'
  export let onPaths: (paths: string[]) => void
  export let onError: (message: string) => void
  let browser: BrowseResult | null = null
  let browserPath = ''
  let browserBusy = false
  let wasOpen = false
  let requestVersion = 0
  $: if (open !== wasOpen) {
    wasOpen = open
    if (open) void browseTo(browser?.path || '')
    else requestVersion++
  }
  // Ignore late navigation responses after another folder is opened or this dialog is closed.
  async function browseTo(path: string) {
    const request = ++requestVersion
    browserBusy = true
    try {
      const result = await api.browse(path)
      if (request === requestVersion) {
        browser = result
        browserPath = result.path
      }
    } catch (error) {
      if (request === requestVersion) onError((error as Error).message)
    } finally {
      if (request === requestVersion) browserBusy = false
    }
  }
  async function addBrowserPath(path: string) {
    if (browserBusy) return
    const request = ++requestVersion
    browserBusy = true
    try {
      const result = await api.expand([path])
      if (request === requestVersion && open) {
        onPaths(result.paths)
        open = false
      }
    } catch (error) {
      if (request === requestVersion) onError((error as Error).message)
    } finally {
      if (request === requestVersion) browserBusy = false
    }
  }
</script>

<Dialog.Root bind:open
  ><Dialog.Content class="!max-w-2xl max-h-[90vh] overflow-auto"
    ><Dialog.Header
      ><Dialog.Title>{mode === 'folder' ? '폴더 선택' : '파일 선택'}</Dialog.Title
      ><Dialog.Description
        >컴퓨터의 폴더를 탐색합니다. 폴더를 추가하면 지원 파일을 이름순으로 등록합니다.</Dialog.Description
      ></Dialog.Header
    >
    <div class="button-row">
      <Input
        aria-label="폴더 경로"
        bind:value={browserPath}
        onkeydown={(event) => {
          if (event.key === 'Enter') browseTo(browserPath)
        }}
      /><Button variant="outline" onclick={() => browseTo(browserPath)}>이동</Button>
    </div>
    {#if browser}<div class="button-row">
        <Button variant="outline" onclick={() => browseTo(browser!.parent)}>상위 폴더</Button
        >{#if mode === 'folder'}<Button
            onclick={() => addBrowserPath(browser!.path)}
            disabled={browserBusy}>현재 폴더 추가</Button
          >{/if}
      </div>
      <div class="browser-list">
        {#each browser.entries as entry (entry.path)}<div class="browser-row">
            <button
              class="mini-button"
              onclick={() => (entry.isDir ? browseTo(entry.path) : addBrowserPath(entry.path))}
              >{entry.isDir ? '폴더 · ' : '파일 · '}{entry.name}</button
            >{#if entry.isDir}<Button
                size="xs"
                variant="outline"
                onclick={() => addBrowserPath(entry.path)}>추가</Button
              >{/if}
          </div>{:else}<p class="empty">표시할 파일이 없습니다.</p>{/each}
      </div>{/if}
    {#if browserBusy}<p class="muted compact">불러오는 중...</p>{/if}
  </Dialog.Content></Dialog.Root
>
