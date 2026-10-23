import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue';

/**
 * True when the primary input is a finger rather than a mouse and keyboard.
 *
 * Width is the wrong signal for this: a phone in landscape is wider than the
 * phone breakpoint, and a narrow desktop window is not a phone. The pointer
 * media feature answers the question that actually matters — can the user
 * hover, and can they press a key.
 */
export const COARSE_POINTER_QUERY = '(pointer: coarse)';

function isCoarsePointer(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
  return window.matchMedia(COARSE_POINTER_QUERY).matches;
}

/**
 * Reactive "this is a touch device" flag for code that has to remove a feature
 * rather than restyle it.
 *
 * Keyboard shortcuts are the motivating case: on a phone there is no keyboard
 * to bind and no way to press one, so the shortcut settings page is dropped
 * from the tab strip and from settings search instead of being hidden with a
 * class, which would leave it reachable through search and leave a dead tab id
 * behind.
 */
export function useCoarsePointer(): Ref<boolean> {
  // Read once during setup so the first paint is already correct; waiting for
  // `onMounted` would flash the keyboard-only items on a phone.
  const coarse = ref(isCoarsePointer());
  let query: MediaQueryList | null = null;

  function onChange(event: MediaQueryListEvent) {
    coarse.value = event.matches;
  }

  onMounted(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
    query = window.matchMedia(COARSE_POINTER_QUERY);
    coarse.value = query.matches;
    query.addEventListener('change', onChange);
  });

  onBeforeUnmount(() => {
    query?.removeEventListener('change', onChange);
    query = null;
  });

  return coarse;
}
