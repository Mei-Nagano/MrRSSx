/**
 * PWA helpers for the browser (server) build.
 *
 * The desktop and Android shells load the same bundle from a WebView, where a
 * service worker would cache the app shell across upgrades and keep serving
 * stale assets, so registration is limited to real browser tabs.
 */

import { isWailsShell } from './platform';

/** Registers the service worker when the app runs in a browser over http(s). */
export function registerServiceWorker(): void {
  if (!('serviceWorker' in navigator)) return;
  if (!window.isSecureContext && window.location.hostname !== 'localhost') return;
  if (isWailsShell()) return;

  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((error) => {
      console.warn('[PWA] Service worker registration failed:', error);
    });
  });
}
