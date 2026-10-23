import { onBeforeUnmount, onMounted, ref, type Ref } from 'vue';

/**
 * The phone breakpoint, in the same terms the stylesheet uses.
 *
 * Kept next to the composable so components stop re-declaring the string; it
 * has to match the `max-width: 767px` media queries in `style.css`.
 */
export const PHONE_QUERY = '(max-width: 767px)';

function isPhoneLayout(): boolean {
  if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return false;
  return window.matchMedia(PHONE_QUERY).matches;
}

/**
 * Reactive "this window is phone-shaped" flag for code that has to pick a
 * different rendering rather than a different style.
 *
 * The article table is the motivating case: it is a desktop layout with a
 * 600px minimum width, so on a 411px screen it turns into a sideways-scrolling
 * table with the date column off-screen. Below the breakpoint the list renders
 * its normal rows instead, which carry the feed and the date already.
 */
export function usePhoneLayout(): Ref<boolean> {
  // Read once during setup so the first paint is already correct.
  const phone = ref(isPhoneLayout());
  let query: MediaQueryList | null = null;

  function onChange(event: MediaQueryListEvent) {
    phone.value = event.matches;
  }

  onMounted(() => {
    if (typeof window === 'undefined' || typeof window.matchMedia !== 'function') return;
    query = window.matchMedia(PHONE_QUERY);
    phone.value = query.matches;
    query.addEventListener('change', onChange);
  });

  onBeforeUnmount(() => {
    query?.removeEventListener('change', onChange);
    query = null;
  });

  return phone;
}

/**
 * Height in px of the phone's fixed bottom activity bar, or 0 when the rail
 * layout is in use.
 *
 * Menus and popovers clamp themselves to `window.innerHeight`, but on a phone
 * the bottom 56px (plus the gesture inset) stays covered by the bar, so a panel
 * that is clamped to the viewport still loses its last rows underneath it.
 * Measured from the element because the inset is part of the bar's height and
 * only the browser knows its value.
 */
export function bottomBarHeight(): number {
  if (typeof document === 'undefined') return 0;
  const bar = document.querySelector<HTMLElement>('.smart-activity-bar');
  if (!bar || getComputedStyle(bar).position !== 'fixed') return 0;
  return bar.getBoundingClientRect().height;
}
