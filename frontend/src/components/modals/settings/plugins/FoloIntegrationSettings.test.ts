import { mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { settingsDefaults } from '@/config/defaults';
import { InputControl } from '@/components/settings';
import type { SettingsData } from '@/types/settings';
import FoloIntegrationSettings from './FoloIntegrationSettings.vue';

const openInBrowser = vi.fn(async (_url: string) => {});
vi.mock('@/utils/browser', () => ({
  openInBrowser: (...args: [string]) => openInBrowser(...args),
}));
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));

const store = { fetchFeeds: vi.fn(), pollProgress: vi.fn() };
vi.mock('@/stores/app', () => ({ useAppStore: () => store }));

type Route = (init?: RequestInit) => Response | Promise<Response>;

function stubFetch(routes: Record<string, Route>) {
  const calls: Array<{ url: string; init?: RequestInit }> = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(async (url: string, init?: RequestInit) => {
      calls.push({ url, init });
      const route = routes[url];
      if (!route) throw new Error(`unexpected request: ${url}`);
      return route(init);
    })
  );
  return calls;
}

function mountSettings(overrides: Partial<SettingsData> = {}) {
  const settings = { ...settingsDefaults, folo_enabled: true, ...overrides } as SettingsData;
  return mount(FoloIntegrationSettings, { props: { settings } });
}

function textOf(wrapper: ReturnType<typeof mountSettings>) {
  return wrapper.text();
}

beforeEach(() => {
  window.showToast = vi.fn();
  openInBrowser.mockClear();
  store.fetchFeeds.mockClear();
  store.pollProgress.mockClear();
});
afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

describe('Folo settings', () => {
  it('stays collapsed and makes no requests until enabled', async () => {
    const calls = stubFetch({});
    const wrapper = mountSettings({ folo_enabled: false });
    await wrapper.vm.$nextTick();

    expect(calls).toHaveLength(0);
    expect(textOf(wrapper)).not.toContain('setting.folo.notSignedIn');

    const toggle = wrapper.find('input.toggle');
    expect(toggle.attributes('aria-label')).toBe('setting.folo.integration');
    await toggle.setValue(true);

    const emitted = wrapper.emitted('update:settings')?.[0][0] as SettingsData;
    expect(emitted.folo_enabled).toBe(true);
    wrapper.unmount();
  });

  it('shows the signed-out state and blocks importing', async () => {
    stubFetch({ '/api/folo/account': () => new Response('{}', { status: 401 }) });
    const wrapper = mountSettings();
    await vi.waitFor(() => expect(textOf(wrapper)).toContain('setting.folo.notSignedIn'));

    const importButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'setting.folo.import');
    expect(importButton?.attributes('disabled')).toBeDefined();
    wrapper.unmount();
  });

  it('imports subscriptions and refreshes the feed list', async () => {
    const calls = stubFetch({
      '/api/folo/account': () =>
        Response.json({
          connected: true,
          name: 'Ada',
          email: 'ada@example.com',
          last_import_time: '',
        }),
      '/api/folo/import': () => Response.json({ success: true, imported: 2, total: 3 }),
    });
    const wrapper = mountSettings();
    await vi.waitFor(() => expect(textOf(wrapper)).toContain('setting.folo.signedInAs'));

    const importButton = wrapper
      .findAll('button')
      .find((button) => button.text() === 'setting.folo.import');
    await importButton?.trigger('click');
    await vi.waitFor(() => expect(window.showToast).toHaveBeenCalled());

    expect(
      calls.some((call) => call.url === '/api/folo/import' && call.init?.method === 'POST')
    ).toBe(true);
    expect(window.showToast).toHaveBeenCalledWith('setting.folo.importDone', 'success');
    expect(store.fetchFeeds).toHaveBeenCalled();
    expect(store.pollProgress).toHaveBeenCalled();
    wrapper.unmount();
  });

  it('opens the Folo login page and follows the loopback callback', async () => {
    vi.useFakeTimers();
    const loginURL = 'https://app.folo.is/login?cli_callback=http%3A%2F%2F127.0.0.1%3A1%2Fx';
    stubFetch({
      '/api/folo/account': () => new Response('{}', { status: 401 }),
      '/api/folo/login/start': () => Response.json({ success: true, login_url: loginURL }),
      '/api/folo/login/status': () => Response.json({ state: 'connected' }),
    });
    const wrapper = mountSettings();
    await vi.waitFor(() => expect(textOf(wrapper)).toContain('setting.folo.notSignedIn'));

    const signIn = wrapper
      .findAll('button')
      .find((button) => button.text() === 'setting.folo.signIn');
    await signIn?.trigger('click');
    await vi.waitFor(() => expect(openInBrowser).toHaveBeenCalledWith(loginURL));

    await vi.advanceTimersByTimeAsync(2100);
    await vi.waitFor(() =>
      expect(window.showToast).toHaveBeenCalledWith('setting.folo.signInSuccess', 'success')
    );
    wrapper.unmount();
  });

  it('accepts a pasted token for headless installs', async () => {
    const calls = stubFetch({
      '/api/folo/account': () => new Response('{}', { status: 401 }),
      '/api/folo/login/complete': () =>
        Response.json({ success: true, name: 'Ada', email: 'ada@example.com' }),
    });
    const wrapper = mountSettings();
    await vi.waitFor(() => expect(textOf(wrapper)).toContain('setting.folo.notSignedIn'));

    const input = wrapper.findAllComponents(InputControl)[0];
    input.vm.$emit('update:modelValue', 'one-time');
    await wrapper.vm.$nextTick();

    const useToken = wrapper
      .findAll('button')
      .find((button) => button.text() === 'setting.folo.manualUse');
    await useToken?.trigger('click');
    await vi.waitFor(() => expect(window.showToast).toHaveBeenCalled());

    const complete = calls.find((call) => call.url === '/api/folo/login/complete');
    expect(complete?.init?.body).toBe(JSON.stringify({ credential: 'one-time' }));
    expect(textOf(wrapper)).toContain('setting.folo.signedInAs');
    // The credential is cleared so it cannot be read back off the screen.
    const passwordInput = wrapper.findAllComponents(InputControl)[0];
    expect(passwordInput.props('modelValue')).toBe('');
    wrapper.unmount();
  });

  it('syncs read state and refreshes the article list', async () => {
    const calls = stubFetch({
      '/api/folo/account': () =>
        Response.json({ connected: true, name: 'Ada', email: 'ada@example.com' }),
      '/api/folo/sync': () =>
        Response.json({ success: true, pushed: 2, pulled: 3, matched: 3, skipped: 0, mapped: 3 }),
    });
    const refreshed = vi.fn();
    window.addEventListener('refresh-articles', refreshed);

    const wrapper = mountSettings();
    await vi.waitFor(() => expect(textOf(wrapper)).toContain('setting.folo.signedInAs'));

    const sync = wrapper.findAll('button').find((button) => button.text() === 'setting.folo.sync');
    await sync?.trigger('click');
    await vi.waitFor(() => expect(window.showToast).toHaveBeenCalled());

    expect(
      calls.some((call) => call.url === '/api/folo/sync' && call.init?.method === 'POST')
    ).toBe(true);
    expect(window.showToast).toHaveBeenCalledWith('setting.folo.syncDone', 'success');
    expect(refreshed).toHaveBeenCalled();

    window.removeEventListener('refresh-articles', refreshed);
    wrapper.unmount();
  });

  it('keeps the signed-in state when the account cannot be reached', async () => {
    let reachable = true;
    const calls = stubFetch({
      '/api/folo/account': () =>
        reachable
          ? Response.json({ connected: true, name: 'Ada', email: 'ada@example.com' })
          : new Response('{}', { status: 502 }),
    });
    const wrapper = mountSettings();
    await vi.waitFor(() => expect(textOf(wrapper)).toContain('setting.folo.signedInAs'));

    // Coming back to the app while offline must not look like a sign-out.
    reachable = false;
    document.dispatchEvent(new Event('visibilitychange'));
    await vi.waitFor(() =>
      expect(calls.filter((call) => call.url === '/api/folo/account')).toHaveLength(2)
    );

    expect(textOf(wrapper)).toContain('setting.folo.signedInAs');
    expect(textOf(wrapper)).not.toContain('setting.folo.notSignedIn');
    wrapper.unmount();
  });

  it('drops back to signed out when the stored session is rejected', async () => {
    stubFetch({
      '/api/folo/account': () =>
        Response.json({ connected: true, name: 'Ada', email: 'ada@example.com' }),
      '/api/folo/sync': () => Response.json({ code: 'not_authenticated' }, { status: 401 }),
    });
    const wrapper = mountSettings();
    await vi.waitFor(() => expect(textOf(wrapper)).toContain('setting.folo.signedInAs'));

    const sync = wrapper.findAll('button').find((button) => button.text() === 'setting.folo.sync');
    await sync?.trigger('click');
    await vi.waitFor(() =>
      expect(window.showToast).toHaveBeenCalledWith('setting.folo.sessionExpired', 'error')
    );

    // The panel must not keep claiming a signed-in account Folo no longer accepts.
    expect(textOf(wrapper)).toContain('setting.folo.notSignedIn');
    expect(textOf(wrapper)).not.toContain('setting.folo.signedInAs');
    wrapper.unmount();
  });

  it('edits the API URL without disturbing other settings', async () => {
    stubFetch({ '/api/folo/account': () => new Response('{}', { status: 401 }) });
    const wrapper = mountSettings({
      folo_api_url: 'https://api.folo.is',
      folo_folder_mode: 'category',
    });
    await vi.waitFor(() => expect(textOf(wrapper)).toContain('setting.folo.notSignedIn'));

    const apiInput = wrapper.findAllComponents(InputControl)[1];
    apiInput.vm.$emit('update:modelValue', 'https://folo.internal');
    await wrapper.vm.$nextTick();

    const emitted = wrapper.emitted('update:settings')?.[0][0] as SettingsData;
    expect(emitted.folo_api_url).toBe('https://folo.internal');
    expect(emitted.folo_folder_mode).toBe('category');
    expect(emitted.freshrss_api_password).toBe(settingsDefaults.freshrss_api_password);
    wrapper.unmount();
  });
});
