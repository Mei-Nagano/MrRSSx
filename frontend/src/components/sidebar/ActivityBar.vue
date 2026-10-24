<script setup lang="ts">
import { withShortcut } from '@/composables/ui/shortcutBindings';
import { PHONE_QUERY } from '@/composables/ui/usePhoneLayout';
import {
  PhListDashes,
  PhSquaresFour,
  PhTray,
  PhStar,
  PhClockCountdown,
  PhImages,
  PhPlus,
  PhGear,
  PhSidebar,
} from '@phosphor-icons/vue';
import { ref, onMounted } from 'vue';
import { useAppStore } from '@/stores/app';
import { useI18n } from 'vue-i18n';
import { useArticleFilter } from '@/composables/article/useArticleFilter';
import { useSettings } from '@/composables/core/useSettings';

const store = useAppStore();
const { t } = useI18n();
const { clearAllFilters } = useArticleFilter();
const { settings } = useSettings();

interface Props {
  isFeedListExpanded: boolean;
}

defineProps<Props>();

const emit = defineEmits<{
  'select-filter': [filterType: string];
  'add-feed': [];
  settings: [];
  'toggle-feed-drawer': [];
}>();

interface NavItem {
  id: string;
  icon: any;
  label: string;
  activeIcon?: any;
  shortcut?: (keyof import('@/composables/ui/shortcutBindings').KeyboardShortcuts)[];
  filterType: 'all' | 'unread' | 'favorites' | 'readLater' | 'imageGallery';
}

const navItems: NavItem[] = [
  {
    id: 'all',
    shortcut: ['goToAllArticles'],
    icon: PhListDashes,
    activeIcon: PhSquaresFour,
    label: t('sidebar.activity.allArticles'),
    filterType: 'all',
  },
  {
    id: 'unread',
    shortcut: ['goToUnread', 'toggleUnreadFilter'],
    icon: PhTray,
    label: t('sidebar.feedList.unread'),
    filterType: 'unread',
  },
  {
    id: 'favorites',
    shortcut: ['goToFavorites', 'toggleFavoritesFilter'],
    icon: PhStar,
    label: t('sidebar.activity.favorites'),
    filterType: 'favorites',
  },
  {
    id: 'readLater',
    shortcut: ['goToReadLater', 'toggleReadLaterFilter'],
    icon: PhClockCountdown,
    label: t('sidebar.activity.readLater'),
    filterType: 'readLater',
  },
  {
    id: 'imageGallery',
    icon: PhImages,
    label: t('sidebar.activity.imageGallery'),
    filterType: 'imageGallery',
  },
];

// Check if image gallery feature is enabled
const imageGalleryEnabled = ref(false);

async function loadImageGallerySetting() {
  try {
    const res = await fetch('/api/settings');
    if (res.ok) {
      const data = await res.json();
      imageGalleryEnabled.value = data.image_gallery_enabled === 'true';
    }
  } catch (e) {
    console.error('Failed to load settings:', e);
  }
}

onMounted(async () => {
  await loadImageGallerySetting();

  // Listen for settings changes
  window.addEventListener('image-gallery-setting-changed', (e: Event) => {
    const customEvent = e as CustomEvent;
    imageGalleryEnabled.value = customEvent.detail.enabled;
  });
});

function handleNavClick(item: NavItem) {
  // Clear any active saved filters when clicking main filter buttons
  clearAllFilters();
  store.setFilter(item.filterType);

  // On the phone layout the article detail covers the list, so switching the
  // bottom bar has to leave the article as well; otherwise the pressed target
  // stayed hidden behind the open article. The split layout keeps its reading
  // pane, which is the existing desktop convention.
  if (typeof window !== 'undefined' && window.matchMedia(PHONE_QUERY).matches) {
    store.currentArticleId = null;
  }

  emit('select-filter', item.filterType);

  // Don't auto-expand feed panel when clicking nav items
  // Only expand when clicking the Feed button
}
</script>

<template>
  <div
    class="smart-activity-bar flex flex-col items-center py-3 bg-bg-tertiary border-r border-border h-full select-none shrink-0 relative z-30"
  >
    <!-- Navigation Items -->
    <div
      class="flex-1 flex flex-col items-center gap-1 w-full overflow-y-auto overflow-x-hidden nav-items-container"
    >
      <TransitionGroup name="nav-item">
        <button
          v-for="item in navItems"
          v-show="item.id !== 'imageGallery' || imageGalleryEnabled"
          :key="item.id"
          :class="[
            'relative flex items-center justify-center text-text-secondary flex-shrink-0 transition-all hover:text-accent',
            store.currentFilter === item.filterType ? 'text-accent' : '',
          ]"
          style="width: 44px; height: 44px"
          :title="item.shortcut ? withShortcut(item.label, item.shortcut) : item.label"
          :aria-label="item.label"
          :aria-current="store.currentFilter === item.filterType ? 'page' : undefined"
          @click="handleNavClick(item)"
        >
          <!-- Icon -->
          <component
            :is="store.currentFilter === item.filterType ? item.activeIcon || item.icon : item.icon"
            :size="24"
            :weight="store.currentFilter === item.filterType ? 'fill' : 'regular'"
            :class="[
              store.currentFilter === item.filterType ? 'text-accent' : '',
              'transition-colors',
            ]"
          />

          <!-- Unread Badge (only for 'all' button) -->
          <span
            v-if="settings.show_unread_counts && item.id === 'all' && store.unreadCounts?.total > 0"
            class="absolute bottom-0.5 right-0.5 min-w-[14px] h-[14px] px-0.5 text-[9px] font-semibold flex items-center justify-center rounded-full bg-accent text-white ring-1 ring-bg-primary"
          >
            {{ store.unreadCounts?.total > 99 ? '99+' : store.unreadCounts?.total }}
          </span>
        </button>
      </TransitionGroup>
    </div>

    <!-- Bottom Actions -->
    <div class="action-items-container flex flex-col items-center gap-1 mt-auto w-full">
      <button
        class="relative flex items-center justify-center text-text-secondary flex-shrink-0 transition-all hover:text-accent"
        style="width: 44px; height: 44px"
        :title="withShortcut(t('sidebar.activity.addFeed'), 'addFeed')"
        @click="emit('add-feed')"
      >
        <PhPlus :size="24" weight="regular" class="transition-all" />
      </button>

      <!-- Feed List Button -->
      <button
        class="relative flex items-center justify-center text-text-secondary flex-shrink-0 transition-all hover:text-accent"
        style="width: 44px; height: 44px"
        :title="
          withShortcut(
            isFeedListExpanded
              ? t('sidebar.activity.collapseFeedList')
              : t('sidebar.activity.expandFeedList'),
            'toggleFeedList'
          )
        "
        :aria-expanded="isFeedListExpanded"
        @click="emit('toggle-feed-drawer')"
      >
        <PhSidebar :size="24" :weight="isFeedListExpanded ? 'fill' : 'regular'" />
      </button>

      <button
        class="relative flex items-center justify-center text-text-secondary flex-shrink-0 transition-all hover:text-accent"
        style="width: 44px; height: 44px"
        :title="withShortcut(t('setting.tab.settings'), 'openSettings')"
        @click="emit('settings')"
      >
        <PhGear :size="24" weight="regular" class="transition-all" />
      </button>
    </div>
  </div>
</template>

<style scoped>
.smart-activity-bar {
  width: 56px;
  min-width: 56px;
  position: absolute;
  left: 0;
  top: 0;
  bottom: 0;
  z-index: 15;
  /* Prevent layout shift during animations */
  backface-visibility: hidden;
  -webkit-font-smoothing: antialiased;
}

/* Navigation items smooth transitions */
.nav-items-container {
  /* Smooth height transition when items are added/removed */
  transition: height 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  /* Keep scrolling without reserving a native gutter in WKWebView. */
  scrollbar-width: none;
  -ms-overflow-style: none;
}

.nav-items-container::-webkit-scrollbar {
  display: none;
}

/* Nav item enter/leave transitions */
.nav-item-enter-active,
.nav-item-leave-active {
  transition: all 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: opacity, transform;
}

.nav-item-enter-from {
  opacity: 0;
  transform: scale(0.9) translateY(-10px);
}

.nav-item-leave-to {
  opacity: 0;
  transform: scale(0.9) translateY(10px);
}

.nav-item-move {
  transition: transform 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: transform;
}

/* Ensure smooth transitions for icon scale changes */
.smart-activity-bar button .ph,
.smart-activity-bar button svg {
  display: block;
  shape-rendering: geometricPrecision;
  transition: color 0.2s ease;
}

/* Improve button hover transition */
.smart-activity-bar button {
  transition:
    color 0.2s ease,
    background-color 0.2s ease;
  will-change: color, background-color;
}

/* Smaller screens (laptops, tablets) */
@media (max-width: 1400px) {
  .smart-activity-bar {
    width: 48px;
    min-width: 48px;
  }

  button[style*='width: 44px'] {
    width: 40px !important;
    height: 40px !important;
  }
}

/* Mobile devices: the rail becomes a bottom navigation bar. A permanent 44px
   column costs more than a tenth of a 390px-wide reading area, while a bar at
   the bottom sits where a thumb already is.

   The bar carries the five views and nothing else: it answers "which view am I
   on", not "what can I do". Add-feed and settings are actions and live in the
   feed drawer's footer; the drawer itself opens from the hamburger that both
   the list and the gallery headers lead with on phones. */
@media (max-width: 767px) {
  .smart-activity-bar {
    position: fixed;
    left: 0;
    right: 0;
    top: auto;
    bottom: 0;
    width: auto;
    min-width: 0;
    height: calc(56px + env(safe-area-inset-bottom, 0px));
    flex-direction: row;
    align-items: stretch;
    padding: 0 0 env(safe-area-inset-bottom, 0px);
    border-right-width: 0;
    border-top: 1px solid var(--color-border);
    /* Above the article pane (z-30), which slides over the list on phones. */
    z-index: 40;
  }

  .nav-items-container {
    flex: 1 1 auto;
    flex-direction: row;
    align-items: center;
    justify-content: space-around;
    gap: 0;
    height: 56px;
    overflow-x: auto;
    overflow-y: hidden;
  }

  .action-items-container {
    display: none;
  }

  /* 44px is the smallest comfortable touch target; the rail's compact 36px
     sizing is for a mouse and is too small for a thumb. */
  button[style*='width: 44px'] {
    width: 44px !important;
    height: 44px !important;
  }
}
</style>
