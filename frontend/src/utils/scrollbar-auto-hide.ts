/**
 * 滚动条自动隐藏：滚动时给滚动容器挂上 is-scrolling，停下约 0.9 秒后摘掉。
 * 全局样式靠这个类把滚动条从「半透明待机」淡入成「清晰可见」，再自己淡回去。
 */
const SCROLLING_CLASS = 'is-scrolling'
const IDLE_DELAY_MS = 900

const idleTimers = new WeakMap<Element, number>()

function resolveScroller(target: EventTarget | null): Element | null {
  if (!target) {
    return null
  }
  if (target === document || target === window) {
    return document.documentElement
  }
  return target instanceof Element ? target : null
}

function markScrolling(event: Event) {
  const scroller = resolveScroller(event.target)
  if (!scroller) {
    return
  }
  scroller.classList.add(SCROLLING_CLASS)
  const pending = idleTimers.get(scroller)
  if (pending) {
    window.clearTimeout(pending)
  }
  idleTimers.set(
    scroller,
    window.setTimeout(() => {
      scroller.classList.remove(SCROLLING_CLASS)
      idleTimers.delete(scroller)
    }, IDLE_DELAY_MS),
  )
}

let installed = false

/** installScrollbarAutoHide 全局只需装一次（App 根组件挂载时调用）。 */
export function installScrollbarAutoHide() {
  if (installed || typeof window === 'undefined') {
    return
  }
  installed = true
  // scroll 事件不冒泡，但捕获阶段可以收到页面里任意滚动容器的事件
  document.addEventListener('scroll', markScrolling, { capture: true, passive: true })
}
