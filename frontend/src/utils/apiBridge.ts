/**
 * Android API transport.
 *
 * The desktop and browser builds talk to the reader's REST API over ordinary
 * HTTP: the Wails desktop window serves it on loopback and the browser build
 * runs the same handler behind a real server. The Android host is different.
 * It renders the frontend from `http://wails.localhost` and answers every
 * request in-process through `WebViewAssetLoader` -> `shouldInterceptRequest`.
 * That path cannot carry a request body (the WebView API simply does not expose
 * one) and drops query strings on the way to Go, so every POST/PUT/DELETE would
 * arrive empty and every filtered GET would lose its filters.
 *
 * The fix is to stop relying on HTTP for API traffic when the native Android
 * bridge is present: this module installs a `fetch` shim that serialises the
 * request and replays it through the Wails runtime transport, which does carry
 * the payload. Nothing else in the app changes - callers keep using `fetch`.
 */

import { Call } from '@wailsio/runtime';

type AndroidBridge = { invokeAsync?: (...args: unknown[]) => unknown };

type ProxyResponse = {
  status: number;
  headers?: Record<string, string>;
  body?: string;
};

/** True only inside the native Android host, not in a browser tab. */
function hasAndroidBridge(): boolean {
  if (typeof window === 'undefined') return false;
  const bridge = (window as unknown as { wails?: AndroidBridge }).wails;
  return typeof bridge?.invokeAsync === 'function';
}

function toBase64(bytes: Uint8Array): string {
  // String.fromCharCode(...bytes) blows the stack on large payloads, so feed it
  // in chunks.
  const chunkSize = 0x8000;
  let binary = '';
  for (let i = 0; i < bytes.length; i += chunkSize) {
    binary += String.fromCharCode(...bytes.subarray(i, i + chunkSize));
  }
  return btoa(binary);
}

function fromBase64(value: string): Uint8Array<ArrayBuffer> {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

const NO_BODY_STATUS = new Set([204, 205, 304]);

/**
 * Routes same-origin `/api/` traffic through the native bridge. Returns true
 * when the shim was installed, so callers can log the active transport.
 */
export function installAndroidApiBridge(): boolean {
  if (!hasAndroidBridge()) return false;

  const originalFetch = window.fetch.bind(window);

  window.fetch = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const request = new Request(input as RequestInfo, init);
    const url = new URL(request.url, window.location.href);

    // Anything that is not the reader's own API keeps the normal path: assets,
    // external URLs, and the /wails runtime route.
    if (url.origin !== window.location.origin || !url.pathname.startsWith('/api/')) {
      return originalFetch(input as RequestInfo, init);
    }

    const hasBody = request.method !== 'GET' && request.method !== 'HEAD';
    const buffer = hasBody ? await request.arrayBuffer() : null;

    const headers: Record<string, string> = {};
    request.headers.forEach((value, key) => {
      headers[key] = value;
    });

    const response = (await Call.ByName('main.APIProxy.Do', {
      method: request.method,
      path: url.pathname + url.search,
      headers,
      body: buffer && buffer.byteLength > 0 ? toBase64(new Uint8Array(buffer)) : '',
    })) as ProxyResponse;

    const status = response?.status ?? 500;
    const responseHeaders = response?.headers ?? {};

    if (NO_BODY_STATUS.has(status) || !response?.body) {
      return new Response(null, { status, headers: responseHeaders });
    }

    return new Response(fromBase64(response.body), { status, headers: responseHeaders });
  };

  return true;
}
