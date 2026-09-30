import { tick } from 'svelte'
type ScrollOptions = { text: string; pinned?: boolean; key?: string; resetTop?: boolean }

/** DOM-only scrolling: it never writes component state or starts another render cycle. */
export function scrollOutput(node: HTMLElement, initial: ScrollOptions) {
  let previous: ScrollOptions | undefined
  let revision = 0
  function update(options: ScrollOptions) {
    const reset = options.key !== previous?.key
    const changed =
      !previous || reset || options.text !== previous.text || options.pinned !== previous.pinned
    previous = options
    if (!changed) return
    const current = ++revision
    void tick().then(() => {
      if (current !== revision) return
      if (reset && options.resetTop) node.scrollTop = 0
      else if (options.pinned) node.scrollTop = node.scrollHeight
    })
  }
  update(initial)
  return {
    update,
    destroy() {
      revision++
    },
  }
}
