import { afterEach, describe, expect, it, vi } from 'vitest';
import { mount } from '@vue/test-utils';
import { ref } from 'vue';

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>();
  return { ...actual, useI18n: () => ({ t: (key: string) => key }) };
});

const store = {
  currentFilter: 'all',
  currentFeedId: null,
  currentCategory: null,
  setFilter: vi.fn(),
  feeds: [{ id: 1, article_view_mode: 'feed' }],
  navigableArticles: [{ id: 1 }],
};
vi.mock('@/stores/app', () => ({ useAppStore: () => store }));
vi.mock('@/composables/core/useSettings', () => ({
  useSettings: () => ({
    settings: ref({ default_view_mode: 'rendered' }),
    fetchSettings: async () => undefined,
  }),
}));

import ArticleDetailModal from './ArticleDetailModal.vue';
import { installAndroidBackBridge } from '@/composables/ui/useAndroidBack';

const article = {
  id: 1,
  title: 'Hello',
  url: 'https://example.org/a',
  feed_id: 1,
  published_at: '2026-01-01T00:00:00Z',
  is_read: false,
  is_favorite: false,
} as never;

function mountModal() {
  return mount(ArticleDetailModal, {
    props: { article, articleContent: '<p>body</p>', isLoadingContent: false },
    global: {
      stubs: {
        ArticleToolbar: true,
        ArticleContent: true,
        ImageViewer: true,
        FindInPage: true,
      },
    },
  });
}

afterEach(() => {
  delete window.__mrrssHandleBack;
});

describe('card reading modal back handling', () => {
  // The modal opened from a card layout is a dialog over the list; without a
  // registered handler the press fell through to the list-scope reset or left
  // the app with the article still open.
  it('closes the dialog when the system back press arrives', () => {
    const wrapper = mountModal();
    installAndroidBackBridge();

    expect(window.__mrrssHandleBack?.()).toBe(true);
    expect(wrapper.emitted('close')).toHaveLength(1);

    wrapper.unmount();
  });

  it('does not answer once unmounted', () => {
    const wrapper = mountModal();
    installAndroidBackBridge();
    wrapper.unmount();

    expect(window.__mrrssHandleBack?.()).toBe(false);
  });
});
