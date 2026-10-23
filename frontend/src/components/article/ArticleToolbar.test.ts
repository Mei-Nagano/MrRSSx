import { mount } from '@vue/test-utils';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { ref } from 'vue';
import { settingsDefaults } from '@/config/defaults';
import type { SettingsData } from '@/types/settings';
import type { Article } from '@/types/models';
import ArticleToolbar from './ArticleToolbar.vue';

const settings = ref({ ...settingsDefaults, article_toolbar_layout: '[]' } as SettingsData);

vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }));
vi.mock('@/composables/core/useSettings', () => ({
  useSettings: () => ({ settings, fetchSettings: async () => {} }),
}));
vi.mock('@/composables/ui/shortcutBindings', () => ({
  withShortcut: (label: string) => label,
}));
vi.mock('@/composables/article/useSiYuanExport', () => ({
  useSiYuanExport: () => ({ isExporting: false, exportToSiYuan: vi.fn() }),
}));
vi.mock('@/utils/clipboard', () => ({
  copyArticleLink: async () => true,
  copyArticleTitle: async () => true,
}));

const article = {
  id: 1,
  title: 'A title',
  url: 'https://example.com/a',
  is_read: true,
  is_favorite: false,
  is_read_later: false,
} as Article;

const originalMatchMedia = window.matchMedia;

function mountToolbar() {
  return mount(ArticleToolbar, {
    props: { article, showContent: true, showTranslations: true },
  });
}

function usePhoneViewport() {
  window.matchMedia = vi.fn((query: string) => ({
    matches: query === '(max-width: 767px)',
    media: query,
    onchange: null,
    addListener: vi.fn(),
    removeListener: vi.fn(),
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    dispatchEvent: vi.fn(),
  })) as unknown as typeof window.matchMedia;
}

beforeEach(() => {
  window.showToast = vi.fn();
});
afterEach(() => {
  window.matchMedia = originalMatchMedia;
  document.body.innerHTML = '';
});

describe('Article toolbar', () => {
  it('keeps every action inline on a wide viewport', () => {
    const wrapper = mountToolbar();

    // view, readingMode, read, favorite, readLater, browser, copyTitle, copyLink, reload
    expect(wrapper.findAll('.toolbar-action')).toHaveLength(9);
    expect(wrapper.find('[aria-label="article.toolbar.moreActions"]').exists()).toBe(false);
    wrapper.unmount();
  });

  it('respects actions hidden in the toolbar customizer', () => {
    settings.value = {
      ...settingsDefaults,
      article_toolbar_layout: JSON.stringify([
        { id: 'view', visible: true },
        { id: 'favorite', visible: false },
      ]),
    } as SettingsData;
    const wrapper = mountToolbar();

    const labels = wrapper
      .findAll('.toolbar-action')
      .map((button) => button.attributes('aria-label'));
    expect(labels).not.toContain('article.toolbar.addToFavorite');
    expect(labels).toContain('article.action.viewOriginal');

    settings.value = { ...settingsDefaults, article_toolbar_layout: '[]' } as SettingsData;
    wrapper.unmount();
  });

  it('moves the extra actions into a menu on a phone', async () => {
    usePhoneViewport();
    const wrapper = mountToolbar();
    await wrapper.vm.$nextTick();

    expect(wrapper.findAll('.toolbar-action')).toHaveLength(4);
    const more = wrapper.find('[aria-label="article.toolbar.moreActions"]');
    expect(more.exists()).toBe(true);

    await more.trigger('click');
    await wrapper.vm.$nextTick();

    const items = document.querySelectorAll('[role="menuitem"]');
    // readLater, browser, copyTitle, copyLink, reload
    expect(items).toHaveLength(5);
    expect(items[0].textContent).toContain('article.toolbar.addToReadLater');

    (items[0] as HTMLElement).click();
    await wrapper.vm.$nextTick();

    expect(wrapper.emitted('toggleReadLater')).toHaveLength(1);
    // Choosing an action closes the menu so the article is visible again.
    expect(document.querySelectorAll('[role="menuitem"]')).toHaveLength(0);
    wrapper.unmount();
  });
});
