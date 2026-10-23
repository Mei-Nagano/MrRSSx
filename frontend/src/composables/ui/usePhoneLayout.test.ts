import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { defineComponent, h } from 'vue';
import { PHONE_QUERY, usePhoneLayout } from './usePhoneLayout';

const originalMatchMedia = window.matchMedia;

let listeners: Array<(event: MediaQueryListEvent) => void> = [];

/** Stubs `matchMedia` for the phone query and keeps the change listener. */
function stubMatchMedia(matches: boolean) {
  listeners = [];
  window.matchMedia = vi.fn((query: string) => ({
    matches: query === PHONE_QUERY ? matches : false,
    media: query,
    onchange: null,
    addEventListener: (_: string, listener: (event: MediaQueryListEvent) => void) => {
      listeners.push(listener);
    },
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
}

function mountProbe() {
  return mount(
    defineComponent({
      setup() {
        const phone = usePhoneLayout();
        return () => h('div', { 'data-testid': 'probe' }, String(phone.value));
      },
    })
  );
}

afterEach(() => {
  window.matchMedia = originalMatchMedia;
});

describe('usePhoneLayout', () => {
  it('reports a phone window before mount, so the first paint is not a table', () => {
    stubMatchMedia(true);
    const wrapper = mountProbe();
    expect(wrapper.get('[data-testid="probe"]').text()).toBe('true');
    wrapper.unmount();
  });

  it('reports a wide window when the breakpoint is not matched', () => {
    stubMatchMedia(false);
    const wrapper = mountProbe();
    expect(wrapper.get('[data-testid="probe"]').text()).toBe('false');
    wrapper.unmount();
  });

  it('follows a window being resized across the breakpoint', async () => {
    stubMatchMedia(false);
    const wrapper = mountProbe();
    expect(wrapper.get('[data-testid="probe"]').text()).toBe('false');

    listeners.forEach((listener) => listener({ matches: true } as MediaQueryListEvent));
    await wrapper.vm.$nextTick();
    expect(wrapper.get('[data-testid="probe"]').text()).toBe('true');

    wrapper.unmount();
  });
});
