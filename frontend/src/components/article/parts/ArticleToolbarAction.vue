<script setup lang="ts">
import { computed } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  PhArrowClockwise,
  PhArrowSquareOut,
  PhArticle,
  PhBookOpenText,
  PhCircle,
  PhClockCountdown,
  PhGlobe,
  PhLinkSimple,
  PhSpinnerGap,
  PhStar,
  PhTextT,
  PhTranslate,
} from '@phosphor-icons/vue';
import SiYuanIcon from '@/components/common/SiYuanIcon.vue';
import { withShortcut } from '@/composables/ui/shortcutBindings';
import type { Article } from '@/types/models';
import type { ToolbarActionID } from '@/utils/articleToolbar';

const props = withDefaults(
  defineProps<{
    item: ToolbarActionID;
    article: Article;
    showContent: boolean;
    showTranslations: boolean;
    isContentLoading?: boolean;
    isReadingModeLoading?: boolean;
    isExporting?: boolean;
    /** `icon` is the toolbar button, `menu` is the same action as a labelled row. */
    variant?: 'icon' | 'menu';
  }>(),
  {
    isContentLoading: false,
    isReadingModeLoading: false,
    isExporting: false,
    variant: 'icon',
  }
);

const emit = defineEmits<{ run: [] }>();

const { t } = useI18n();

const pluginIcons: Record<string, { src: string; alt: string }> = {
  obsidian: { src: '/assets/plugin_icons/obsidian.svg', alt: 'Obsidian' },
  notion: { src: '/assets/plugin_icons/notion.svg', alt: 'Notion' },
  zotero: { src: '/assets/plugin_icons/zotero.png', alt: 'Zotero' },
};

const plainLabel = computed(() => {
  switch (props.item) {
    case 'readingMode':
      return t('article.action.readingMode');
    case 'view':
      return props.showContent ? t('article.action.viewOriginal') : t('article.action.viewContent');
    case 'translation':
      return props.showTranslations
        ? t('setting.reading.hideTranslations')
        : t('setting.reading.showTranslations');
    case 'read':
      return props.article.is_read
        ? t('article.action.markAsUnread')
        : t('article.action.markAsRead');
    case 'favorite':
      return props.article.is_favorite
        ? t('article.action.removeFromFavorite')
        : t('article.toolbar.addToFavorite');
    case 'readLater':
      return props.article.is_read_later
        ? t('article.action.removeFromReadLater')
        : t('article.toolbar.addToReadLater');
    case 'browser':
      return t('article.action.openInBrowser');
    case 'copyTitle':
      return t('common.contextMenu.copyTitle');
    case 'copyLink':
      return t('common.contextMenu.copyLink');
    case 'reload':
      return t('article.action.reloadContent');
    case 'obsidian':
      return t('setting.plugins.obsidian.exportTo');
    case 'notion':
      return t('setting.plugins.notion.exportTo');
    case 'zotero':
      return t('setting.plugins.zotero.exportTo');
    case 'siyuan':
      return t('setting.plugins.siyuan.exportTo');
    default:
      return '';
  }
});

// Shortcut hints belong in the tooltip, not in a menu row.
const label = computed(() => {
  switch (props.item) {
    case 'view':
      return withShortcut(plainLabel.value, 'toggleContentView');
    case 'read':
      return withShortcut(plainLabel.value, 'toggleReadStatus');
    case 'favorite':
      return withShortcut(plainLabel.value, 'toggleFavoriteStatus');
    case 'readLater':
      return withShortcut(plainLabel.value, 'toggleReadLaterStatus');
    case 'browser':
      return withShortcut(plainLabel.value, 'openInBrowser');
    default:
      return plainLabel.value;
  }
});

const icon = computed(() => {
  switch (props.item) {
    case 'readingMode':
      return PhBookOpenText;
    case 'view':
      return props.showContent ? PhGlobe : PhArticle;
    case 'translation':
      return PhTranslate;
    case 'read':
      return PhCircle;
    case 'favorite':
      return PhStar;
    case 'readLater':
      return PhClockCountdown;
    case 'browser':
      return PhArrowSquareOut;
    case 'copyTitle':
      return PhTextT;
    case 'copyLink':
      return PhLinkSimple;
    case 'reload':
      return PhArrowClockwise;
    default:
      return undefined;
  }
});

const imageIcon = computed(() => pluginIcons[props.item] ?? null);
const spinner = computed(() => props.item === 'readingMode' && props.isReadingModeLoading);
const busy = computed(() => spinner.value || (props.item === 'siyuan' && props.isExporting));

const disabled = computed(() => {
  switch (props.item) {
    case 'readingMode':
      return props.isContentLoading || props.isReadingModeLoading;
    case 'copyTitle':
      return !props.article.title;
    case 'copyLink':
      return !props.article.url;
    case 'siyuan':
      return props.isExporting;
    default:
      return false;
  }
});

const iconWeight = computed<'regular' | 'fill'>(() => {
  switch (props.item) {
    case 'read':
      return props.article.is_read ? 'regular' : 'fill';
    case 'favorite':
      return props.article.is_favorite ? 'fill' : 'regular';
    case 'readLater':
      return props.article.is_read_later ? 'fill' : 'regular';
    case 'translation':
      return props.showTranslations ? 'fill' : 'regular';
    default:
      return 'regular';
  }
});

const activeClass = computed(() => {
  switch (props.item) {
    case 'read':
      return props.article.is_read ? '' : 'text-accent';
    case 'favorite':
      return props.article.is_favorite ? 'text-yellow-500 hover:text-yellow-600' : '';
    case 'readLater':
      return props.article.is_read_later ? 'text-blue-500 hover:text-blue-600' : '';
    default:
      return '';
  }
});
</script>

<template>
  <button
    v-if="variant === 'icon'"
    type="button"
    class="toolbar-action"
    :class="activeClass"
    :title="label"
    :aria-label="plainLabel"
    :disabled="disabled"
    :aria-busy="busy || undefined"
    @click="emit('run')"
  >
    <PhSpinnerGap v-if="spinner" :size="18" class="animate-spin sm:h-5 sm:w-5" />
    <SiYuanIcon v-else-if="item === 'siyuan'" class="h-[18px] w-[18px] sm:h-5 sm:w-5" />
    <img
      v-else-if="imageIcon"
      :src="imageIcon.src"
      :alt="imageIcon.alt"
      class="h-[18px] w-[18px] sm:h-5 sm:w-5"
    />
    <component :is="icon" v-else-if="icon" :size="18" class="sm:h-5 sm:w-5" :weight="iconWeight" />
  </button>
  <button
    v-else
    type="button"
    role="menuitem"
    class="menu-action"
    :class="activeClass"
    :disabled="disabled"
    :aria-busy="busy || undefined"
    @click="emit('run')"
  >
    <PhSpinnerGap v-if="spinner" :size="16" class="shrink-0 animate-spin" />
    <SiYuanIcon v-else-if="item === 'siyuan'" class="h-4 w-4 shrink-0" />
    <img v-else-if="imageIcon" :src="imageIcon.src" :alt="imageIcon.alt" class="h-4 w-4 shrink-0" />
    <component :is="icon" v-else-if="icon" :size="16" class="shrink-0" :weight="iconWeight" />
    <span class="flex-1 truncate">{{ plainLabel }}</span>
  </button>
</template>

<style scoped>
@reference "../../../style.css";

/* 40px touch target on phones; the compact desktop padding returns at sm. */
.toolbar-action {
  @apply flex h-10 w-10 cursor-pointer items-center justify-center rounded-md text-lg text-text-secondary transition-colors hover:bg-bg-tertiary hover:text-text-primary sm:h-auto sm:w-auto sm:p-1.5 sm:text-xl disabled:cursor-not-allowed disabled:opacity-50;
}

.menu-action {
  @apply flex w-full cursor-pointer items-center gap-2.5 rounded-lg px-3 py-2.5 text-left text-sm text-text-primary transition-colors hover:bg-bg-tertiary disabled:cursor-not-allowed disabled:opacity-50;
}
</style>
