import { describe, expect, it, vi } from 'vitest';
import { flushPromises, shallowMount } from '@vue/test-utils';
import { defineComponent, ref } from 'vue';
import ArticleDetail from './ArticleDetail.vue';
import ArticleToolbar from './ArticleToolbar.vue';
import modalSource from './ArticleDetailModal.vue?raw';

vi.mock('@/composables/article/useArticleDetail', () => ({
  useArticleDetail: () => ({
    article: ref({ id: 1, url: 'https://example.org/article' }),
    showContent: ref(false),
    articleContent: ref('<p>Feed content</p>'),
    isLoadingContent: ref(false),
    hasPreviousArticle: ref(false),
    hasNextArticle: ref(false),
    imageViewerSrc: ref(null),
    t: (key: string) => key,
  }),
}));

describe('reading mode entry', () => {
  it('replaces the webpage with the reader before requesting clean text', async () => {
    const enterReadingMode = vi.fn();
    const wrapper = shallowMount(ArticleDetail, {
      global: {
        stubs: {
          ArticleContent: defineComponent({
            setup(_, { expose }) {
              expose({ enterReadingMode, isFetchingFullArticle: false });
              return () => null;
            },
          }),
        },
      },
    });
    expect(wrapper.find('iframe').exists()).toBe(true);
    wrapper.findComponent(ArticleToolbar).vm.$emit('readingMode');
    await flushPromises();
    expect(wrapper.find('iframe').exists()).toBe(false);
    expect(enterReadingMode).toHaveBeenCalledTimes(1);
    wrapper.unmount();
  });
});

describe('original page isolation', () => {
  it('sandboxes the proxied original page so it cannot reach the app', () => {
    const wrapper = shallowMount(ArticleDetail, {
      global: { stubs: { ArticleContent: true } },
    });
    const frame = wrapper.find('iframe');
    expect(frame.exists()).toBe(true);

    const sandbox = frame.attributes('sandbox') ?? '';
    expect(sandbox).toMatch(/allow-scripts/);
    // `allow-scripts` together with `allow-same-origin` cancels the sandbox: the
    // untrusted page keeps the app origin and can reach window.parent, app storage
    // and the native bridge through the fetch shim. The pair must never come back.
    expect(sandbox).not.toMatch(/allow-same-origin/);
    expect(frame.attributes('referrerpolicy')).toBe('no-referrer');
    wrapper.unmount();
  });

  it('keeps the same isolation in the card reading modal', () => {
    expect(modalSource).toMatch(/sandbox="[^"]*allow-scripts/);
    expect(modalSource).not.toMatch(/allow-same-origin/);
  });
});
