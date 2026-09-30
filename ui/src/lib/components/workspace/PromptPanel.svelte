<script lang="ts">
  import { Button } from '$lib/components/ui/button/index.js'
  import { Textarea } from '$lib/components/ui/textarea/index.js'
  import { NativeSelect } from '$lib/components/ui/native-select/index.js'
  import type { Model } from '$lib/api'
  import { effortName } from '$lib/models'
  export let promptValue = ''
  export let promptEditing = false
  export let model = 'gpt-5.6-sol'
  export let effort = 'medium'
  export let models: Model[] = []
  export let readyCount = 0
  export let busy = false
  export let codexReady = false
  export let savePrompt: () => void
  export let copy: (value: string) => void
  export let startJobs: () => void
  $: selectedModel = models.find((item) => item.model === model || item.id === model)
  $: efforts = selectedModel?.supportedReasoningEfforts
    ?.map((item) => item.reasoningEffort)
    .filter((value) => ['low', 'medium', 'high', 'xhigh'].includes(value)) ?? [
    'low',
    'medium',
    'high',
    'xhigh',
  ]
  function modelAvailable(id: string) {
    return !models.length || models.some((item) => item.model === id || item.id === id)
  }
</script>

<section class="panel" aria-labelledby="settings-title">
  <div class="section-top"><h2 id="settings-title" class="section-number">03 / SETTINGS</h2></div>
  <div class="section-top">
    <h3 class="section-title">회의록 프롬프트</h3>
    <div class="button-row" style="flex:0 0 auto">
      <Button
        variant="outline"
        size="xs"
        onclick={() => (promptEditing ? savePrompt() : (promptEditing = true))}
        >{promptEditing ? '저장' : '수정'}</Button
      ><Button variant="outline" size="xs" onclick={() => copy(promptValue)}>복사</Button>
    </div>
  </div>
  <Textarea
    class="prompt-area"
    aria-label="회의록 프롬프트"
    readonly={!promptEditing}
    bind:value={promptValue}
  />
  <div class="panel-footer">
    <label class="field-label" for="model-select">GPT 모델</label><NativeSelect
      class="w-full"
      id="model-select"
      bind:value={model}
      ><option value="gpt-6-astra" disabled={!modelAvailable('gpt-6-astra')}
        >GPT-6 Astra{modelAvailable('gpt-6-astra') ? '' : ' · CLI 업데이트 필요'}</option
      ><option value="gpt-6-sol" disabled={!modelAvailable('gpt-6-sol')}
        >GPT-6 Sol{modelAvailable('gpt-6-sol') ? '' : ' · CLI 업데이트 필요'}</option
      ><option value="gpt-5.6-sol" disabled={!modelAvailable('gpt-5.6-sol')}>GPT-5.6 Sol</option
      ></NativeSelect
    >
    <label class="field-label" for="effort-select">추론 수준</label><NativeSelect
      class="w-full"
      id="effort-select"
      bind:value={effort}
      >{#each efforts as value}<option {value}>{effortName[value]}</option>{/each}</NativeSelect
    >
    <Button
      class="w-full mt-3"
      size="lg"
      disabled={!readyCount || busy || !codexReady}
      onclick={startJobs}>회의록 정리 시작</Button
    >
    <p class="tiny muted" style="text-align:center;margin-top:8px">
      {readyCount}개 파일을 위에서 정한 순서대로 처리합니다.
    </p>
  </div>
</section>
