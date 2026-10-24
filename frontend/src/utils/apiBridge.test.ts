import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const { callByName } = vi.hoisted(() => ({ callByName: vi.fn() }));
vi.mock('@wailsio/runtime', () => ({ Call: { ByName: callByName } }));

import { installAndroidApiBridge } from './apiBridge';

const originalFetch = globalThis.fetch;

// Node's Request needs an absolute URL; jsdom has no base to resolve against.
function apiUrl(path: string) {
  return new URL(path, window.location.href).href;
}

function installNativeShell() {
  Object.assign(window, { wails: { invokeAsync: () => undefined } });
}

beforeEach(() => {
  callByName.mockReset();
  installNativeShell();
});

afterEach(() => {
  globalThis.fetch = originalFetch;
  delete (window as { wails?: unknown }).wails;
});

describe('Android API transport cancellation', () => {
  it('never reaches the native side when the signal is already aborted', async () => {
    installAndroidApiBridge();
    const controller = new AbortController();
    controller.abort();

    await expect(window.fetch(apiUrl('/api/articles'), { signal: controller.signal })).rejects.toMatchObject(
      { name: 'AbortError' }
    );
    expect(callByName).not.toHaveBeenCalled();
  });

  it('cancels the same request in Go when the caller aborts mid-flight', async () => {
    callByName.mockImplementation((name: string) => {
      if (name === 'main.APIProxy.Do') return new Promise(() => {}); // never settles
      return Promise.resolve();
    });
    installAndroidApiBridge();

    const controller = new AbortController();
    const pending = window.fetch(apiUrl('/api/articles'), { signal: controller.signal });
    // The shim dispatches synchronously, so the native call is already in flight.
    controller.abort();

    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    expect(callByName).toHaveBeenCalledWith(
      'main.APIProxy.Cancel',
      expect.stringMatching(/^api-\d+$/)
    );
  });

  it('forwards a request id so a cancellation can be addressed', async () => {
    callByName.mockImplementation((name: string) =>
      name === 'main.APIProxy.Do'
        ? Promise.resolve({
            status: 200,
            headers: { 'content-type': 'application/json' },
            body: btoa('{"ok":true}'),
          })
        : Promise.resolve()
    );
    installAndroidApiBridge();

    const response = await window.fetch(apiUrl('/api/articles'));
    expect(response.status).toBe(200);
    await expect(response.json()).resolves.toEqual({ ok: true });

    const payload = callByName.mock.calls[0]?.[1] as { request_id?: string };
    expect(payload.request_id).toMatch(/^api-\d+$/);
  });

  // Reading the body is asynchronous, so a POST can be aborted after the request
  // is built but before dispatch. That window used to send the call anyway and
  // return 200 with no Cancel at all.
  it('never dispatches a POST aborted while its body was being read', async () => {
    callByName.mockImplementation(() => Promise.resolve({ status: 200, headers: {}, body: '' }));
    installAndroidApiBridge();

    const controller = new AbortController();
    const pending = window.fetch(apiUrl('/api/articles'), {
      method: 'POST',
      body: 'payload',
      signal: controller.signal,
    });
    controller.abort();

    await expect(pending).rejects.toMatchObject({ name: 'AbortError' });
    expect(callByName).not.toHaveBeenCalled();
  });
});
