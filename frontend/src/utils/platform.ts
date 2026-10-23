/**
 * Runtime platform detection for the reader interface.
 *
 * The same bundle runs in a desktop window, an Android app and a plain browser.
 * Desktop-only settings (startup registration, tray behaviour, in-app updates,
 * data directory pickers) must stay hidden where the underlying capability does
 * not exist.
 */

type WailsBridge = { environment?: unknown };

/** True when the bundle is running inside the Wails shell rather than a tab. */
export function isWailsShell(): boolean {
  if (typeof window === 'undefined') return false;
  return Boolean((window as unknown as { _wails?: WailsBridge })._wails);
}

/** True when the Wails shell is running on a phone or tablet. */
export function isMobileShell(): boolean {
  if (!isWailsShell()) return false;
  return /Android|iPhone|iPad|iPod/i.test(navigator.userAgent);
}

/**
 * True when the reader has a desktop window: tray, startup registration,
 * in-app update installation and native file pickers are meaningful.
 */
export function isDesktopShell(): boolean {
  return isWailsShell() && !isMobileShell();
}
