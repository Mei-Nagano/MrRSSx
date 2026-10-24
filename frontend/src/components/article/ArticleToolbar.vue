<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { useSettings } from '@/composables/core/useSettings';
import { computed, nextTick, onBeforeUnmount, onMounted, ref } from 'vue';
import ArticleToolbarSettings from './ArticleToolbarSettings.vue';
import ArticleToolbarAction from './parts/ArticleToolbarAction.vue';
import { parseToolbarLayout, type ToolbarActionID } from '@/utils/articleToolbar';
import { withShortcut } from '@/composables/ui/shortcutBindings';
import { PHONE_QUERY } from '@/composables/ui/usePhoneLayout';
import { useAndroidBack, BACK_PRIORITY } from '@/composables/ui/useAndroidBack';
import { useSiYuanExport } from '@/composables/article/useSiYuanExport';
import { PhArrowLeft, PhDotsThree, PhSlidersHorizontal, PhX } from '@phosphor-icons/vue';
import type { Article } from '@/types/models';
import { copyArticleLink, copyArticleTitle } from '@/utils/clipboard';

const { t } = useI18n();
const { settings, fetchSettings } = useSettings();
const showToolbarSettings = ref(false);
const { isExporting: isExportingToSiYuan, exportToSiYuan } = useSiYuanExport();

const props = withDefaults(
  defineProps<{
    article: Article;
    showContent: boolean;
    showTranslations?: boolean;
    isModal?: boolean;
    isLoadingContent?: boolean;
    isReadingModeLoading?: boolean;
  }>(),
  {
    showTranslations: true,
    isModal: false,
    isLoadingContent: false,
    isReadingModeLoading: false,
  }
);

const emit = defineEmits<{
  close: [];
  toggleContentView: [];
  readingMode: [];
  toggleRead: [];
  toggleFavorite: [];
  toggleReadLater: [];
  openOriginal: [];
  toggleTranslations: [];
  reloadContent: [];
  exportToObsidian: [];
  exportToNotion: [];
  exportToZotero: [];
}>();

// Phones get the first four actions inline and the rest in a menu, so every
// button can be a 40px touch target instead of a 26px icon. The user's order
// from the toolbar customizer decides what stays visible.
const PHONE_INLINE_ACTIONS = 4;

const isPhone = ref(
  typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia(PHONE_QUERY).matches
);
let phoneQuery: MediaQueryList | null = null;

function onPhoneChange(event: MediaQueryListEvent) {
  isPhone.value = event.matches;
}

const availableActions = computed<ToolbarActionID[]>(() =>
  parseToolbarLayout(settings.value.article_toolbar_layout)
    .filter((item) => {
      if (!item.visible) return false;
      switch (item.id) {
        case 'translation':
          return (
            props.showContent &&
            settings.value.translation_enabled &&
            !settings.value.translation_only_mode
          );
        case 'obsidian':
          return settings.value.obsidian_enabled;
        case 'notion':
          return settings.value.notion_enabled;
        case 'zotero':
          return settings.value.zotero_enabled;
        case 'siyuan':
          return settings.value.siyuan_enabled;
        default:
          return true;
      }
    })
    .map((item) => item.id)
);

const inlineActions = computed(() =>
  isPhone.value ? availableActions.value.slice(0, PHONE_INLINE_ACTIONS) : availableActions.value
);
const overflowActions = computed(() =>
  isPhone.value ? availableActions.value.slice(PHONE_INLINE_ACTIONS) : []
);

const menuOpen = ref(false);
const menuTrigger = ref<HTMLButtonElement | null>(null);
const menuPanel = ref<HTMLElement | null>(null);
const menuPosition = ref({ top: '0px', right: '8px', maxHeight: '60vh' });

async function openMenu() {
  menuOpen.value = true;
  await nextTick();
  const anchor = menuTrigger.value?.getBoundingClientRect();
  if (!anchor) return;
  const top = anchor.bottom + 6;
  // On the phone layout the bottom bar is fixed over the article, so the menu has
  // to stop above it instead of running to the viewport edge underneath it. Its
  // height is measured (56px plus the safe-area inset) rather than copied here as
  // a second constant that could drift.
  const bottomBar = isPhone.value
    ? document.querySelector<HTMLElement>('.smart-activity-bar')
    : null;
  const reservedBottom = bottomBar?.offsetHeight ?? 0;
  menuPosition.value = {
    top: `${Math.round(top)}px`,
    right: `${Math.round(Math.max(8, window.innerWidth - anchor.right))}px`,
    maxHeight: `${Math.max(160, Math.round(window.innerHeight - top - 8 - reservedBottom))}px`,
  };
  menuPanel.value?.querySelector<HTMLButtonElement>('button:not(:disabled)')?.focus();
}

function closeMenu(restoreFocus = false) {
  menuOpen.value = false;
  if (restoreFocus) menuTrigger.value?.focus();
}

// The overflow menu is a layer of its own: back while it is open belongs to the
// menu, not to the article underneath it. Without this the press fell through to
// the detail layer and closed the article with the menu still on screen.
useAndroidBack(() => {
  if (!menuOpen.value) return false;
  closeMenu(true);
  return true;
}, BACK_PRIORITY.popover);

function onDocumentPointerDown(event: Event) {
  if (!menuOpen.value) return;
  const target = event.target;
  if (
    target instanceof Node &&
    (menuPanel.value?.contains(target) || menuTrigger.value?.contains(target))
  )
    return;
  closeMenu();
}

function onViewportChange(event?: Event) {
  if (!menuOpen.value) return;
  // The scroll listener runs in the capture phase, so it also sees the panel's
  // own scrolling. Treating that as a viewport change closed the menu while the
  // user was reaching for the last action in an overflowing menu.
  if (event?.target instanceof Node && menuPanel.value?.contains(event.target)) return;
  closeMenu();
}

onMounted(async () => {
  phoneQuery = window.matchMedia?.(PHONE_QUERY) ?? null;
  if (phoneQuery) {
    isPhone.value = phoneQuery.matches;
    phoneQuery.addEventListener('change', onPhoneChange);
  }
  document.addEventListener('pointerdown', onDocumentPointerDown);
  window.addEventListener('resize', onViewportChange);
  window.addEventListener('scroll', onViewportChange, true);
  try {
    await fetchSettings();
  } catch (e) {
    console.error('Error loading settings:', e);
  }
});

onBeforeUnmount(() => {
  phoneQuery?.removeEventListener('change', onPhoneChange);
  document.removeEventListener('pointerdown', onDocumentPointerDown);
  window.removeEventListener('resize', onViewportChange);
  window.removeEventListener('scroll', onViewportChange, true);
});

function runAction(id: ToolbarActionID) {
  closeMenu();
  switch (id) {
    case 'readingMode':
      emit('readingMode');
      break;
    case 'view':
      emit('toggleContentView');
      break;
    case 'translation':
      emit('toggleTranslations');
      break;
    case 'read':
      emit('toggleRead');
      break;
    case 'favorite':
      emit('toggleFavorite');
      break;
    case 'readLater':
      emit('toggleReadLater');
      break;
    case 'browser':
      emit('openOriginal');
      break;
    case 'copyTitle':
      void copyTitle(props.article);
      break;
    case 'copyLink':
      void copyLink(props.article);
      break;
    case 'reload':
      emit('reloadContent');
      break;
    case 'obsidian':
      emit('exportToObsidian');
      break;
    case 'notion':
      emit('exportToNotion');
      break;
    case 'zotero':
      emit('exportToZotero');
      break;
    case 'siyuan':
      void exportToSiYuan(props.article.id);
      break;
  }
}

async function copyLink(article: Article) {
  const success = await copyArticleLink(article.url);
  if (success) {
    window.showToast(t('common.toast.copiedToClipboard'), 'success');
  } else {
    window.showToast(t('common.errors.failedToCopy'), 'error');
  }
}

async function copyTitle(article: Article) {
  const success = await copyArticleTitle(article.title);
  if (success) {
    window.showToast(t('common.toast.copiedToClipboard'), 'success');
  } else {
    window.showToast(t('common.errors.failedToCopy'), 'error');
  }
}
</script>

<template>
  <div
    class="p-2 sm:p-4 border-b border-border flex justify-between items-center bg-bg-primary shrink-0"
  >
    <!-- Modal mode: X button always visible -->
    <button
      v-if="isModal"
      class="back-hit relative flex items-center gap-1.5 text-sm text-text-secondary hover:text-text-primary sm:gap-2 sm:text-base"
      :title="withShortcut(t('common.close'), 'closeArticle')"
      @click="emit('close')"
    >
      <PhX :size="20" class="sm:w-5 sm:h-5" />
    </button>
    <!-- Normal mode: Back button on mobile -->
    <button
      v-else
      class="back-hit relative md:hidden flex items-center gap-1.5 text-sm text-text-secondary hover:text-text-primary sm:gap-2 sm:text-base"
      @click="emit('close')"
    >
      <PhArrowLeft :size="18" class="sm:w-5 sm:h-5" />
      <span class="hidden min-[400px]:inline">{{ t('common.back') }}</span>
    </button>
    <div class="flex flex-wrap justify-end gap-1 sm:gap-2 ml-auto">
      <ArticleToolbarAction
        v-for="id in inlineActions"
        :key="id"
        :item="id"
        :article="article"
        :show-content="showContent"
        :show-translations="showTranslations"
        :is-content-loading="isLoadingContent"
        :is-reading-mode-loading="isReadingModeLoading"
        :is-exporting="isExportingToSiYuan"
        @run="runAction(id)"
      />
      <button
        v-if="overflowActions.length > 0"
        ref="menuTrigger"
        type="button"
        class="toolbar-button"
        :title="t('article.toolbar.moreActions')"
        :aria-label="t('article.toolbar.moreActions')"
        :aria-expanded="menuOpen"
        aria-haspopup="menu"
        @click="menuOpen ? closeMenu() : openMenu()"
      >
        <PhDotsThree :size="20" class="sm:h-5 sm:w-5" weight="bold" />
      </button>
      <button
        type="button"
        class="toolbar-button"
        :title="t('article.toolbar.customize')"
        :aria-label="t('article.toolbar.customize')"
        @click="showToolbarSettings = true"
      >
        <PhSlidersHorizontal :size="18" class="sm:w-5 sm:h-5" />
      </button>
    </div>
  </div>
  <Teleport to="body">
    <div
      v-if="menuOpen"
      ref="menuPanel"
      role="menu"
      class="fixed z-50 w-64 max-w-[calc(100vw-1rem)] overflow-y-auto rounded-xl border border-border bg-bg-primary p-1 shadow-xl"
      :style="menuPosition"
      @keydown.esc.stop.prevent="closeMenu(true)"
    >
      <ArticleToolbarAction
        v-for="id in overflowActions"
        :key="id"
        :item="id"
        variant="menu"
        :article="article"
        :show-content="showContent"
        :show-translations="showTranslations"
        :is-content-loading="isLoadingContent"
        :is-reading-mode-loading="isReadingModeLoading"
        :is-exporting="isExportingToSiYuan"
        @run="runAction(id)"
      />
    </div>
    <ArticleToolbarSettings v-if="showToolbarSettings" @close="showToolbarSettings = false" />
  </Teleport>
</template>

<style scoped>
@reference "../../style.css";
.toolbar-button {
  @apply flex h-10 w-10 cursor-pointer items-center justify-center rounded-md text-lg text-text-secondary transition-colors hover:bg-bg-tertiary hover:text-text-primary sm:h-auto sm:w-auto sm:p-1.5 sm:text-xl;
}

/* The back arrow is only 18px; grow its hit area without moving the icon. */
.back-hit::after {
  content: '';
  position: absolute;
  inset: -10px;
}
</style>
