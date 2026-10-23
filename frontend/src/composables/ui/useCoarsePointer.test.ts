import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { defineComponent, h } from 'vue';
import { COARSE_POINTER_QUERY, useCoarsePointer } from './useCoarsePointer';

const originalMatchMedia = window.matchMedia;

let listeners: Array<(event: MediaQueryListEvent) => void> = [];

/**
 * Stubs `matchMedia` for one query and remembers the change listener so a test
 * can play a device switch (a tablet losing its trackpad, a phone going into
 * DeX mode) without a real media query.
 */
function stubMatchMedia(matches: boolean) {
  listeners = [];
  window.matchMedia = vi.fn((query: string) => ({
    matches: query === COARSE_POINTER_QUERY ? matches : false,
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
        const coarse = useCoarsePointer();
        return () => h('div', { 'data-testid': 'probe' }, String(coarse.value));
      },
    })
  );
}

afterEach(() => {
  window.matchMedia = originalMatchMedia;
});

describe('useCoarsePointer', () => {
  it('reports a touch device before mount, so nothing flashes in on first paint', () => {
    stubMatchMedia(true);
    const wrapper = mountProbe();
    expect(wrapper.get('[data-testid="probe"]').text()).toBe('true');
    wrapper.unmount();
  });

  it('reports a pointer device when the primary input is a mouse', () => {
    stubMatchMedia(false);
    const wrapper = mountProbe();
    expect(wrapper.get('[data-testid="probe"]').text()).toBe('false');
    wrapper.unmount();
  });

  it('follows the device switching between coarse and fine pointers', async () => {
    stubMatchMedia(true);
    const wrapper = mountProbe();
    expect(wrapper.get('[data-testid="probe"]').text()).toBe('true');

    listeners.forEach((listener) => listener({ matches: false } as MediaQueryListEvent));
    await wrapper.vm.$nextTick();
    expect(wrapper.get('[data-testid="probe"]').text()).toBe('false');

    wrapper.unmount();
  });
});
