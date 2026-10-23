import { computed, onBeforeUnmount, ref, watch, type Ref } from 'vue';

interface Options {
  /** Pull distance (px) that arms the refresh. */
  threshold?: number;
  /** Hard cap for the resisted pull distance. */
  max?: number;
  /** External refresh state; the indicator stays up until this turns false. */
  isRefreshing: Ref<boolean>;
}

/**
 * Pull-to-refresh for a vertically scrolling element.
 *
 * Only gestures that start at the very top are considered, and the first few
 * pixels are ignored so a normal upward scroll never gets captured. Once the
 * pull passes the threshold the browser's own overscroll is suppressed
 * (`preventDefault`), which needs a non-passive listener — hence the manual
 * binding instead of Vue's `@touchmove`.
 */
export function usePullToRefresh(
  container: Ref<HTMLElement | null>,
  onRefresh: () => void,
  options: Options
) {
  const threshold = options.threshold ?? 64;
  const max = options.max ?? 96;
  const { isRefreshing } = options;

  const distance = ref(0);
  const pulling = ref(false);
  const armed = computed(() => distance.value >= threshold);

  let startY = 0;
  let tracking = false;
  let bound: HTMLElement | null = null;

  function reset() {
    distance.value = 0;
    pulling.value = false;
  }

  function touchstart(event: TouchEvent) {
    if (isRefreshing.value || event.touches.length !== 1) return;
    const element = container.value;
    if (!element || element.scrollTop > 0) return;
    startY = event.touches[0].clientY;
    tracking = true;
  }

  function touchmove(event: TouchEvent) {
    if (!tracking || isRefreshing.value) return;
    const element = container.value;
    if (!element) return;
    const delta = event.touches[0].clientY - startY;
    // The gesture turned into a normal upward scroll; let it go.
    if (delta < 0 || element.scrollTop > 0) {
      tracking = false;
      reset();
      return;
    }
    // Ignore the first few pixels so a tap or a jitter does not move the list.
    if (delta < 8) return;
    if (event.cancelable) event.preventDefault();
    pulling.value = true;
    distance.value = Math.min(max, delta * 0.5);
  }

  function touchend() {
    if (!tracking) return;
    tracking = false;
    if (distance.value < threshold) {
      reset();
      return;
    }
    distance.value = threshold;
    onRefresh();
    // The refresh handler sets its own flag synchronously; if it bailed out
    // (already refreshing, nothing to fetch) drop the indicator right away.
    requestAnimationFrame(() => {
      if (!isRefreshing.value) reset();
    });
  }

  function bind(element: HTMLElement | null) {
    if (bound === element) return;
    unbind();
    bound = element;
    if (!bound) return;
    bound.addEventListener('touchstart', touchstart, { passive: true });
    bound.addEventListener('touchmove', touchmove, { passive: false });
    bound.addEventListener('touchend', touchend, { passive: true });
    bound.addEventListener('touchcancel', reset, { passive: true });
  }

  function unbind() {
    if (!bound) return;
    bound.removeEventListener('touchstart', touchstart);
    bound.removeEventListener('touchmove', touchmove);
    bound.removeEventListener('touchend', touchend);
    bound.removeEventListener('touchcancel', reset);
    bound = null;
  }

  // `flush: 'post'` so the element exists by the time the watcher runs: template
  // refs are assigned during render, and a pre-flush watcher would still see null.
  watch(container, bind, { immediate: true, flush: 'post' });
  watch(
    isRefreshing,
    (value) => {
      if (!value) reset();
    },
    { flush: 'sync' }
  );
  onBeforeUnmount(unbind);

  return { distance, pulling, armed };
}
