<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  PhArrowClockwise,
  PhCheck,
  PhCheckCircle,
  PhCircle,
  PhDotsThreeVertical,
  PhList,
  PhSortAscending,
  PhSortDescending,
  PhTextT,
  PhTextTSlash,
} from '@phosphor-icons/vue';
import type { MediaTypeFilter } from '../types';
import type { ArticleSortOrder } from '@/stores/app';
import { useDismissOnBack } from '@/composables/ui/useAndroidBack';

interface Props {
  title: string;
  isRefreshing: boolean;
  showTextOverlay: boolean;
  showOnlyUnread: boolean;
  mediaType: MediaTypeFilter;
  sortOrder: ArticleSortOrder;
}

defineProps<Props>();

const emit = defineEmits<{
  toggleSidebar: [];
  refresh: [];
  toggleTextOverlay: [];
  toggleShowOnlyUnread: [];
  updateMediaType: [mediaType: MediaTypeFilter];
  markAllRead: [];
  toggleSortOrder: [];
}>();

const { t } = useI18n();

// Phones cannot fit the full action row (it is wider than a 390px screen), so
// everything except sort moves behind a menu. The desktop header keeps the row.
const moreOpen = ref(false);
const headerRoot = ref<HTMLElement | null>(null);

function closeMoreMenu(event?: Event) {
  if (!moreOpen.value) return;
  if (event?.target instanceof Node && headerRoot.value?.contains(event.target)) return;
  moreOpen.value = false;
}

function runAndClose(action: () => void) {
  action();
  moreOpen.value = false;
}

onMounted(() => document.addEventListener('pointerdown', closeMoreMenu, true));
onUnmounted(() => document.removeEventListener('pointerdown', closeMoreMenu, true));

// Back press closes the overflow menu rather than leaving the app.
useDismissOnBack(moreOpen, () => {
  moreOpen.value = false;
});
</script>

<template>
  <div
    ref="headerRoot"
    class="relative flex-shrink-0 bg-bg-primary border-b border-border p-2 sm:p-4 flex items-center gap-2 sm:gap-3"
  >
    <!-- Sidebar toggle button (mobile only) -->
    <button
      class="p-2 rounded-lg hover:bg-bg-tertiary text-text-primary transition-colors md:hidden"
      :title="t('shortcut.toggle.sidebar')"
      :aria-label="t('sidebar.feedList.feeds')"
      @click="emit('toggleSidebar')"
    >
      <PhList :size="24" />
    </button>

    <!-- Title -->
    <div class="flex items-center gap-2 flex-1 min-w-0">
      <h1 class="text-base sm:text-lg font-bold text-text-primary line-height-fixed-32 truncate">
        {{ title }}
      </h1>
    </div>

    <div class="flex items-center gap-1 sm:gap-2 shrink-0">
      <!-- Actions that need the full row; phones reach them through the menu -->
      <div class="hidden md:flex items-center gap-2">
        <div
          class="flex items-center rounded-md bg-bg-secondary p-0.5"
          role="group"
          :aria-label="t('article.imageGallery.mediaFilter')"
        >
          <button
            v-for="filter in ['all', 'images', 'videos'] as MediaTypeFilter[]"
            :key="filter"
            type="button"
            class="cursor-pointer rounded px-2 py-1 text-xs text-text-secondary transition-colors hover:bg-bg-tertiary hover:text-text-primary"
            :class="mediaType === filter ? 'bg-bg-tertiary text-text-primary' : ''"
            @click="emit('updateMediaType', filter)"
          >
            {{ t(`article.imageGallery.filter.${filter}`) }}
          </button>
        </div>

        <!-- Show only unread toggle button -->
        <button
          class="p-1 sm:p-1.5 rounded hover:bg-bg-tertiary text-text-secondary transition-colors cursor-pointer"
          :class="showOnlyUnread ? 'text-accent' : ''"
          :title="
            showOnlyUnread
              ? t('setting.reading.showAllArticles')
              : t('setting.reading.showOnlyUnread')
          "
          @click="emit('toggleShowOnlyUnread')"
        >
          <PhCircle :size="20" :weight="showOnlyUnread ? 'fill' : 'regular'" />
        </button>

        <!-- Toggle text overlay button -->
        <button
          class="p-1 sm:p-1.5 rounded hover:bg-bg-tertiary text-text-secondary transition-colors cursor-pointer"
          :title="showTextOverlay ? t('setting.reading.hideText') : t('setting.reading.showText')"
          @click="emit('toggleTextOverlay')"
        >
          <PhTextTSlash v-if="showTextOverlay" :size="20" />
          <PhTextT v-else :size="20" />
        </button>

        <button
          class="p-1 sm:p-1.5 rounded hover:bg-bg-tertiary text-text-secondary transition-colors cursor-pointer"
          :title="t('article.imageGallery.markAllRead')"
          @click="emit('markAllRead')"
        >
          <PhCheckCircle :size="20" />
        </button>

        <button
          class="p-1 sm:p-1.5 rounded hover:bg-bg-tertiary text-text-secondary transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-60"
          :title="t('article.action.refresh')"
          :disabled="isRefreshing"
          @click="emit('refresh')"
        >
          <PhArrowClockwise :size="20" :class="isRefreshing ? 'animate-spin' : ''" />
        </button>
      </div>

      <!-- Publication date sort order -->
      <button
        class="flex h-10 w-10 cursor-pointer items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-bg-tertiary hover:text-text-primary sm:h-auto sm:w-auto sm:p-1.5"
        :title="
          sortOrder === 'newest'
            ? t('article.action.sortOldestFirst')
            : t('article.action.sortNewestFirst')
        "
        @click="emit('toggleSortOrder')"
      >
        <PhSortDescending v-if="sortOrder === 'newest'" :size="20" />
        <PhSortAscending v-else :size="20" />
      </button>

      <!-- Phone-only overflow menu -->
      <button
        class="flex h-10 w-10 cursor-pointer items-center justify-center rounded-md text-text-secondary transition-colors hover:bg-bg-tertiary md:hidden"
        :class="moreOpen ? 'bg-bg-tertiary text-text-primary' : ''"
        :title="t('article.list.more')"
        :aria-expanded="moreOpen"
        @click="moreOpen = !moreOpen"
      >
        <PhDotsThreeVertical :size="20" />
      </button>

      <div
        v-if="moreOpen"
        class="gallery-more-menu md:hidden absolute right-2 top-full z-50 w-60 overflow-hidden rounded-xl border border-border bg-bg-primary p-1 shadow-xl"
      >
        <div class="px-2 pb-1 pt-1.5 text-[11px] font-medium text-text-secondary">
          {{ t('article.imageGallery.mediaFilter') }}
        </div>
        <button
          v-for="filter in ['all', 'images', 'videos'] as MediaTypeFilter[]"
          :key="filter"
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-sm transition-colors hover:bg-bg-tertiary"
          :class="mediaType === filter ? 'text-accent' : 'text-text-primary'"
          @click="runAndClose(() => emit('updateMediaType', filter))"
        >
          <PhCheck v-if="mediaType === filter" :size="16" />
          <span v-else class="w-4"></span>
          {{ t(`article.imageGallery.filter.${filter}`) }}
        </button>

        <div class="my-1 h-px bg-border"></div>

        <button
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-sm text-text-primary transition-colors hover:bg-bg-tertiary"
          @click="runAndClose(() => emit('toggleShowOnlyUnread'))"
        >
          <PhCircle
            :size="16"
            :weight="showOnlyUnread ? 'fill' : 'regular'"
            :class="showOnlyUnread ? 'text-accent' : ''"
          />
          {{ t('setting.reading.showOnlyUnread') }}
        </button>

        <button
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-sm text-text-primary transition-colors hover:bg-bg-tertiary"
          @click="runAndClose(() => emit('toggleTextOverlay'))"
        >
          <PhTextTSlash v-if="showTextOverlay" :size="16" />
          <PhTextT v-else :size="16" />
          {{ showTextOverlay ? t('setting.reading.hideText') : t('setting.reading.showText') }}
        </button>

        <div class="my-1 h-px bg-border"></div>

        <button
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-sm text-text-primary transition-colors hover:bg-bg-tertiary"
          @click="runAndClose(() => emit('markAllRead'))"
        >
          <PhCheckCircle :size="16" />
          {{ t('article.imageGallery.markAllRead') }}
        </button>

        <button
          type="button"
          class="flex w-full items-center gap-2 rounded-lg px-2 py-2 text-sm text-text-primary transition-colors hover:bg-bg-tertiary disabled:cursor-not-allowed disabled:opacity-60"
          :disabled="isRefreshing"
          @click="runAndClose(() => emit('refresh'))"
        >
          <PhArrowClockwise :size="16" :class="isRefreshing ? 'animate-spin' : ''" />
          {{ t('article.action.refresh') }}
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* The menu rows were 36px — a mouse height. The menu only exists below 768px,
   where a thumb is the only input, so every row gets a 40px floor. */
.gallery-more-menu button {
  min-height: 2.5rem;
}
</style>
