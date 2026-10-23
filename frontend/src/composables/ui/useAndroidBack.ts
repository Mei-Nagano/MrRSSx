/**
 * Android hardware back button.
 *
 * The native shell forwards a back press into the page instead of finishing the
 * activity, because a single-page reader keeps most of its state in the
 * document: the open article, the feed drawer, a settings sheet. Without this
 * bridge the hardware button would exit the app from any of those screens,
 * which is the main thing that made the Android build feel unlike a phone app.
 *
 * Components register a handler with `useAndroidBack`; the highest priority
 * handler that reports the press as handled wins, and an unhandled press falls
 * back to the shell (history, then finish). Handlers run in priority order, so
 * a modal registered by the root component closes before the article behind it.
 */

import { onMounted, onUnmounted, watch, type Ref } from 'vue';

export type AndroidBackHandler = () => boolean;

/**
 * Priority ladder for back presses. A press must undo the most recent thing on
 * screen, so anything stacked on top registers higher: the article is the floor,
 * the feed drawer sits on it, app-level sheets (settings, add feed) on top of
 * that, dialogs above sheets, transient popovers above dialogs, and the
 * full-screen viewers above everything else.
 */
export const BACK_PRIORITY = {
  articleDetail: 0,
  /** Scoped list views (a feed, a category, unread…): back returns to All. */
  view: 5,
  drawer: 10,
  sheet: 100,
  dialog: 500,
  popover: 600,
  findInPage: 650,
  viewer: 700,
} as const;

interface BackEntry {
  handler: AndroidBackHandler;
  priority: number;
}

const stack: BackEntry[] = [];

function runBackHandlers(): boolean {
  // Copy before iterating: a handler may unmount other components while closing
  // something, which mutates the registered list.
  const ordered = [...stack].sort((a, b) => b.priority - a.priority);
  for (const entry of ordered) {
    try {
      if (entry.handler()) return true;
    } catch (error) {
      console.error('Android back handler failed:', error);
    }
  }
  return false;
}

/**
 * Registers a back handler for the lifetime of the calling component. Higher
 * priorities are consulted first; the default 0 layers behind overlays.
 */
export function useAndroidBack(handler: AndroidBackHandler, priority = 0): void {
  const entry: BackEntry = { handler, priority };
  onMounted(() => {
    stack.push(entry);
  });
  onUnmounted(() => {
    const index = stack.indexOf(entry);
    if (index >= 0) stack.splice(index, 1);
  });
}

/**
 * Same as `useAndroidBack`, but the handler only takes part in dispatch while
 * `isOpen` is true. Popovers — dropdown menus, the image viewer, the context
 * menu — stay mounted while closed (or open and close many times), so a
 * mount-scoped registration would either never fire or keep swallowing presses
 * after the popover is gone. Those components watch their own open flag instead.
 */
export function useDismissOnBack(
  isOpen: Ref<boolean> | (() => boolean),
  close: () => void,
  priority = 200
): void {
  const entry: BackEntry = {
    handler: () => {
      close();
      return true;
    },
    priority,
  };
  let registered = false;

  const read = typeof isOpen === 'function' ? isOpen : () => isOpen.value;

  watch(
    read,
    (open) => {
      if (open && !registered) {
        stack.push(entry);
        registered = true;
      } else if (!open && registered) {
        const index = stack.indexOf(entry);
        if (index >= 0) stack.splice(index, 1);
        registered = false;
      }
    },
    { immediate: true }
  );

  onUnmounted(() => {
    const index = stack.indexOf(entry);
    if (index >= 0) stack.splice(index, 1);
    registered = false;
  });
}

declare global {
  interface Window {
    /** Called by the Android shell; returns true when the page consumed it. */
    __mrrssHandleBack?: () => boolean;
  }
}

/**
 * Publishes the dispatcher the native shell calls. Installed once from
 * `main.ts`; harmless in a browser or desktop window because nothing calls it.
 */
export function installAndroidBackBridge(): void {
  if (typeof window === 'undefined') return;
  window.__mrrssHandleBack = runBackHandlers;
}
