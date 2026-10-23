<script setup lang="ts">
import { withShortcut } from '@/composables/ui/shortcutBindings';
import { PhNewspaper, PhCaretLeft, PhCaretRight } from '@phosphor-icons/vue';
import { useArticleDetail } from '@/composables/article/useArticleDetail';
import { useAndroidBack, BACK_PRIORITY } from '@/composables/ui/useAndroidBack';
import { PHONE_QUERY } from '@/composables/ui/usePhoneLayout';
import ArticleToolbar from './ArticleToolbar.vue';
import ArticleContent from './ArticleContent.vue';
import ImageViewer from '../common/ImageViewer.vue';
import FindInPage from '../common/FindInPage.vue';

import { ref, onMounted, onBeforeUnmount, nextTick } from 'vue';

const {
  article,
  showContent,
  articleContent,
  isLoadingContent,
  imageViewerSrc,
  imageViewerAlt,
  imageViewerImages,
  imageViewerInitialIndex,
  hasPreviousArticle,
  hasNextArticle,
  close,
  toggleRead,
  toggleFavorite,
  toggleReadLater,
  openOriginal,
  toggleContentView,
  reloadArticleContent,
  closeImageViewer,
  attachImageEventListeners,
  exportToObsidian,
  exportToNotion,
  exportToZotero,
  handleRetryLoadContent,
  goToPreviousArticle,
  goToNextArticle,
  t,
} = useArticleDetail();

const showTranslations = ref(true);
const showFindInPage = ref(false);
const contentView = ref<InstanceType<typeof ArticleContent> | null>(null);

// Android hardware back: the image viewer and find-in-page are overlays inside
// this pane, so they close before the article itself does. The pane is the
// bottom layer — the feed drawer and app sheets sit on top of it — so it takes
// the lowest priority; anything above it gets the press first.
useAndroidBack(() => {
  if (!article.value) return false;
  if (imageViewerSrc.value) {
    closeImageViewer();
    return true;
  }
  if (showFindInPage.value) {
    closeFindInPage();
    return true;
  }
  close();
  return true;
}, BACK_PRIORITY.articleDetail);

// Swipe right from the left edge to leave the article — the gesture Android
// users expect from a full-screen reader, and the reason the back button alone
// feels wrong on a phone. Limited to the left edge so it never competes with
// horizontal scrolling inside the content (code blocks, wide tables).
let backSwipe: { x: number; y: number; at: number } | null = null;

// A wide code block or table is scrolled back to its first column with exactly
// this gesture, and that drag also starts near the left edge. Leave those alone;
// the pane only owns swipes that start on non-scrolling content.
function startsInHorizontalScroller(target: EventTarget | null): boolean {
  let node = target instanceof Element ? target : null;
  while (node) {
    if (node instanceof HTMLElement) {
      const overflowX = getComputedStyle(node).overflowX;
      const scrollsSideways =
        (overflowX === 'auto' || overflowX === 'scroll') && node.scrollWidth > node.clientWidth + 1;
      if (scrollsSideways) return true;
    }
    node = node.parentElement;
  }
  return false;
}

function onDetailTouchStart(event: TouchEvent) {
  if (!article.value || imageViewerSrc.value) return;
  if (!window.matchMedia(PHONE_QUERY).matches) return;
  if (startsInHorizontalScroller(event.target)) return;
  const touch = event.touches[0];
  backSwipe = touch.clientX <= 40 ? { x: touch.clientX, y: touch.clientY, at: Date.now() } : null;
}

function onDetailTouchEnd(event: TouchEvent) {
  const start = backSwipe;
  backSwipe = null;
  if (!start) return;
  const touch = event.changedTouches[0];
  const dx = touch.clientX - start.x;
  const dy = touch.clientY - start.y;
  if (dx >= 72 && Math.abs(dy) <= 60 && Date.now() - start.at <= 700) close();
}

async function enterReadingMode() {
  const id = article.value?.id;
  showContent.value = true;
  await nextTick();
  if (id === article.value?.id) await contentView.value?.enterReadingMode();
}

function toggleTranslations() {
  showTranslations.value = !showTranslations.value;
}

function openFindInPage() {
  showFindInPage.value = true;
}

function closeFindInPage() {
  showFindInPage.value = false;
}

function handleKeydown(e: KeyboardEvent) {
  // Open find in page with Ctrl+F or Cmd+F
  if ((e.ctrlKey || e.metaKey) && e.key === 'f') {
    // Only if we're showing an article in content mode (not webpage view)
    if (article.value && showContent.value) {
      e.preventDefault();
      openFindInPage();
    }
  }

  // Note: FindInPage component handles its own ESC key to close
  // We don't handle ESC here to avoid conflicts - FindInPage will stopPropagation
  // when it needs to handle the key (when search is focused or has content)

  // Note: Arrow key navigation is now handled by the global keyboard shortcuts system
  // See useKeyboardShortcuts.ts which properly checks for editable elements
}

onMounted(() => {
  window.addEventListener('keydown', handleKeydown);
});

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKeydown);
});
</script>

<template>
  <main
    :class="[
      'flex-1 bg-bg-primary flex flex-col h-full absolute inset-0 md:static md:w-auto z-30 transition-transform duration-300',
      article ? 'translate-x-0' : 'translate-x-full md:translate-x-0',
    ]"
    @touchstart.passive="onDetailTouchStart"
    @touchend.passive="onDetailTouchEnd"
  >
    <div
      v-if="!article"
      class="hidden md:flex flex-col items-center justify-center h-full text-text-secondary text-center px-4"
    >
      <PhNewspaper :size="48" class="mb-4 sm:mb-5 opacity-50 sm:w-16 sm:h-16" />
      <p class="text-sm sm:text-base">{{ t('article.content.selectArticle') }}</p>
    </div>

    <div v-else class="flex flex-col h-full bg-bg-primary">
      <ArticleToolbar
        :article="article"
        :show-content="showContent"
        :show-translations="showTranslations"
        :is-loading-content="isLoadingContent"
        :is-reading-mode-loading="contentView?.isFetchingFullArticle ?? false"
        @reading-mode="enterReadingMode"
        @close="close"
        @toggle-content-view="toggleContentView"
        @toggle-read="toggleRead"
        @toggle-favorite="toggleFavorite"
        @toggle-read-later="toggleReadLater"
        @open-original="openOriginal"
        @toggle-translations="toggleTranslations"
        @reload-content="reloadArticleContent"
        @export-to-obsidian="exportToObsidian"
        @export-to-notion="exportToNotion"
        @export-to-zotero="exportToZotero"
      />

      <!-- Original webpage view -->
      <div v-if="!showContent" class="flex-1 bg-bg-primary w-full">
        <iframe
          :key="article.id"
          :src="`/api/webpage/proxy?url=${encodeURIComponent(article.url)}`"
          class="w-full h-full border-none"
          sandbox="allow-scripts allow-same-origin allow-popups"
        ></iframe>
      </div>

      <!-- RSS content view -->
      <ArticleContent
        v-else
        ref="contentView"
        :article="article"
        :article-content="articleContent"
        :is-loading-content="isLoadingContent"
        :attach-image-event-listeners="attachImageEventListeners"
        :show-translations="showTranslations"
        :show-content="showContent"
        @retry-load-content="handleRetryLoadContent"
        @previous="goToPreviousArticle"
        @next="goToNextArticle"
      />

      <!-- Navigation buttons -->
      <div
        v-if="hasPreviousArticle || hasNextArticle"
        class="flex items-center justify-between bg-bg-primary px-3 py-1.5"
      >
        <button
          v-if="hasPreviousArticle"
          :title="
            withShortcut(t('article.navigation.previousArticle') || 'Previous article', [
              'previousArticle',
              'previousArticleArrow',
            ])
          "
          class="flex min-h-10 items-center gap-1.5 px-2 py-1 rounded text-text-secondary/70 hover:text-text-primary hover:bg-bg-secondary/50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          @click="goToPreviousArticle"
        >
          <PhCaretLeft :size="16" />
          <span class="text-xs">{{ t('article.navigation.previousArticle') || 'Previous' }}</span>
        </button>

        <div v-else class="w-16"></div>

        <button
          v-if="hasNextArticle"
          :title="
            withShortcut(t('article.navigation.nextArticle') || 'Next article', [
              'nextArticle',
              'nextArticleArrow',
            ])
          "
          class="flex min-h-10 items-center gap-1.5 px-2 py-1 rounded text-text-secondary/70 hover:text-text-primary hover:bg-bg-secondary/50 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          @click="goToNextArticle"
        >
          <span class="text-xs">{{ t('article.navigation.nextArticle') || 'Next' }}</span>
          <PhCaretRight :size="16" />
        </button>

        <div v-else class="w-16"></div>
      </div>
    </div>

    <!-- Find in Page (only shown in content mode) -->
    <FindInPage
      v-if="showFindInPage && showContent"
      container-selector=".prose-content"
      :article-id="article?.id"
      @close="closeFindInPage"
    />

    <!-- Image Viewer Modal -->
    <ImageViewer
      v-if="imageViewerSrc"
      :src="imageViewerSrc"
      :alt="imageViewerAlt"
      :images="imageViewerImages"
      :initial-index="imageViewerInitialIndex"
      @close="closeImageViewer"
    />
  </main>
</template>
