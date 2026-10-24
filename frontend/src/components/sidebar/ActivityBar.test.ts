import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { ref } from 'vue';

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>();
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) };
});
vi.mock('@/composables/ui/shortcutBindings', () => ({
  withShortcut: (label: string) => label,
}));

const store = {
  currentFilter: 'all',
  currentArticleId: 7 as number | null,
  setFilter: vi.fn(),
};

vi.mock('@/stores/app', () => ({ useAppStore: () => store }));
vi.mock('@/composables/article/useArticleFilter', () => ({
  useArticleFilter: () => ({ clearAllFilters: vi.fn() }),
}));
vi.mock('@/composables/core/useSettings', () => ({
  useSettings: () => ({ settings: ref({}), fetchSettings: async () => undefined }),
}));

import ActivityBar from './ActivityBar.vue';
import { PHONE_QUERY } from '@/composables/ui/usePhoneLayout';

const originalMatchMedia = window.matchMedia;

function useViewport(phone: boolean) {
  window.matchMedia = vi.fn((query: string) => ({
    matches: phone && query === PHONE_QUERY,
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
}

function mountBar() {
  return mount(ActivityBar, { global: { stubs: { TransitionGroup: false } } });
}

afterEach(() => {
  window.matchMedia = originalMatchMedia;
  store.currentArticleId = 7;
  store.setFilter.mockClear();
});

describe('bottom bar navigation', () => {
  // On the phone layout the article detail covers the list, so picking another
  // filter must also leave the article; otherwise the pressed target stays
  // hidden behind it.
  it('leaves the open article when a filter is chosen on a phone', async () => {
    useViewport(true);
    const wrapper = mountBar();

    await wrapper.findAll('button')[0].trigger('click');

    expect(store.setFilter).toHaveBeenCalled();
    expect(store.currentArticleId).toBeNull();
    wrapper.unmount();
  });

  it('keeps the split-layout reading pane intact', async () => {
    useViewport(false);
    const wrapper = mountBar();

    await wrapper.findAll('button')[0].trigger('click');

    expect(store.setFilter).toHaveBeenCalled();
    expect(store.currentArticleId).toBe(7);
    wrapper.unmount();
  });
});
