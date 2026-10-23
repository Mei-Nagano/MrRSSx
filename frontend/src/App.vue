<script setup lang="ts">
import { getAutoRefreshInterval, useAppStore } from './stores/app';
import { useI18n } from 'vue-i18n';
import Sidebar from './components/sidebar/Sidebar.vue';
import ArticleList from './components/article/ArticleList.vue';
import ArticleDetail from './components/article/ArticleDetail.vue';
import ImageGalleryView from './components/article/imageGallery/index.vue';
const AddFeedModal = defineAsyncComponent(
  () => import('./components/modals/feed/AddFeedModal.vue')
);
const EditFeedModal = defineAsyncComponent(
  () => import('./components/modals/feed/EditFeedModal.vue')
);
// Settings is the heaviest screen in the app (every tab plus their panels) and
// is always opened after the reader, so it loads on demand instead of shipping
// in the startup bundle.
const SettingsModal = defineAsyncComponent(() => import('./components/modals/SettingsModal.vue'));
const DiscoverFeedsModal = defineAsyncComponent(
  () => import('./components/modals/discovery/DiscoverFeedsModal.vue')
);
const UpdateAvailableDialog = defineAsyncComponent(
  () => import('./components/modals/update/UpdateAvailableDialog.vue')
);
import ContextMenu from './components/common/ContextMenu.vue';
import ConfirmDialog from './components/modals/common/ConfirmDialog.vue';
import InputDialog from './components/modals/common/InputDialog.vue';
import MultiSelectDialog from './components/modals/common/MultiSelectDialog.vue';
import Toast from './components/common/Toast.vue';
import PartnerAdvertisement from './components/common/PartnerAdvertisement.vue';
import { useAdvertisementsStore } from './stores/advertisements';
import {
  defineAsyncComponent,
  onMounted,
  onUnmounted,
  ref,
  computed,
  watch,
  watchEffect,
} from 'vue';
import { useNotifications } from './composables/ui/useNotifications';
import { useKeyboardShortcuts } from './composables/ui/useKeyboardShortcuts';
import { useContextMenu } from './composables/ui/useContextMenu';
import { useResizablePanels } from './composables/ui/useResizablePanels';
import { useArticleTableSplit } from './composables/ui/useArticleTableSplit';
import { useWindowState } from './composables/core/useWindowState';
import { useAppUpdates } from './composables/core/useAppUpdates';
import { useSettings } from './composables/core/useSettings';
import { useCustomCSS } from './composables/ui/useCustomCSS';
import { useAndroidBack, BACK_PRIORITY } from './composables/ui/useAndroidBack';
import { PHONE_QUERY } from './composables/ui/usePhoneLayout';
import { resolveFontFamily } from './utils/fontDetector';
import { isDesktopShell } from './utils/platform';
import type { Feed } from './types/models';
import type { TabName } from './types/settings';

const store = useAppStore();
const advertisements = useAdvertisementsStore();
const { t } = useI18n();
const { settings } = useSettings();
useCustomCSS(() => settings.value.custom_css_file);

const uiFontSize = computed(() => {
  const value = Number(settings.value.ui_font_size);
  return Number.isFinite(value) ? Math.min(20, Math.max(12, value)) : 16;
});

watchEffect(() => {
  const rootStyle = document.documentElement.style;
  rootStyle.setProperty('--ui-font-family', resolveFontFamily(settings.value.ui_font_family));
  rootStyle.setProperty('--ui-font-size', `${uiFontSize.value}px`);
  rootStyle.setProperty('--ui-font-scale', String(uiFontSize.value / 16));
});

onUnmounted(() => {
  advertisements.stop();
  store.startAutoRefresh(0);
  store.stopProgressPolling();
  store.stopFreshRSSStatusPolling();
  const rootStyle = document.documentElement.style;
  rootStyle.removeProperty('--ui-font-family');
  rootStyle.removeProperty('--ui-font-size');
  rootStyle.removeProperty('--ui-font-scale');
});

const showAddFeed = ref(false);
const showEditFeed = ref(false);
const feedToEdit = ref<Feed | null>(null);
const showSettings = ref(false);
const settingsInitialTab = ref<TabName>('general');
const showDiscoverBlogs = ref(false);
const feedToDiscover = ref<Feed | null>(null);
// On phone-width screens the feed drawer covers the article list, so it starts
// closed unless the user has explicitly opened or closed it before.
const storedSidebarState = localStorage.getItem('FeedListExpanded');
const isSidebarOpen = ref(
  storedSidebarState === null
    ? !window.matchMedia(PHONE_QUERY).matches
    : storedSidebarState !== 'false'
);
watch(isSidebarOpen, (expanded) => {
  localStorage.setItem('FeedListExpanded', String(expanded));
});

// Check if we're in image gallery mode
const isImageGalleryMode = computed(() => store.currentFilter === 'imageGallery');

// Check if we're in card mode
const isCardMode = ref(false);
const isTableMode = ref(false);
const readerPanes = ref<HTMLElement | null>(null);
const tableSplit = useArticleTableSplit(readerPanes);
watch([isTableMode, isImageGalleryMode], () => tableSplit.stop());

// Use composables
const {
  confirmDialog,
  inputDialog,
  multiSelectDialog,
  toasts,
  removeToast,
  installGlobalHandlers,
} = useNotifications();

const { contextMenu, openContextMenu, handleContextMenuAction } = useContextMenu();

const { sidebarWidth, articleListWidth, startResizeArticleList, setCompactMode } =
  useResizablePanels();

// Use app updates composable
const {
  updateInfo,
  checkForUpdates,
  downloadAndInstallUpdate,
  downloadingUpdate,
  installingUpdate,
  downloadProgress,
  downloadProgressKnown,
  downloadBytesWritten,
  downloadTotalBytes,
  downloadErrorCode,
} = useAppUpdates();

// Update dialog state
const showUpdateDialog = ref(false);

// Warm the settings chunk once the reader has settled. Loading it up front cost
// every launch 420 kB of parse for a screen most sessions never open; loading it
// only on click would make the first open wait. Idle prefetch splits the
// difference, with a timer for WebViews without `requestIdleCallback`.
const prefetchSettings = () => void import('./components/modals/SettingsModal.vue');
if ('requestIdleCallback' in window) {
  window.requestIdleCallback(prefetchSettings, { timeout: 4000 });
} else {
  setTimeout(prefetchSettings, 2500);
}

// Android hardware back. Sheets and dialogs are dismissed before the article
// behind them (registered by ArticleDetail at a lower priority) and before the
// feed drawer, so a press always reverses the most recent thing on screen.
useAndroidBack(() => {
  if (showSettings.value) {
    showSettings.value = false;
    return true;
  }
  if (showAddFeed.value) {
    showAddFeed.value = false;
    return true;
  }
  if (showEditFeed.value) {
    showEditFeed.value = false;
    return true;
  }
  if (showDiscoverBlogs.value) {
    showDiscoverBlogs.value = false;
    return true;
  }
  if (showUpdateDialog.value) {
    showUpdateDialog.value = false;
    return true;
  }
  return false;
}, BACK_PRIORITY.sheet);

// A scoped list — a feed, a category, or any view other than All — is one step
// into the app, so back returns to All instead of finishing the activity. Only
// the Android shell calls this; on the root view the press falls through and
// the app closes as usual.
useAndroidBack(() => {
  const scoped =
    store.currentFilter !== 'all' || store.currentFeedId !== null || store.currentCategory !== null;
  if (!scoped) return false;
  void store.setFilter('all');
  return true;
}, BACK_PRIORITY.view);

// Initialize window state management
const windowState = useWindowState();
windowState.init();

// Initialize keyboard shortcuts
const { shortcuts, shortcutsEnabled } = useKeyboardShortcuts({
  onOpenSettings: () => {
    showSettings.value = true;
  },
  onAddFeed: () => {
    showAddFeed.value = true;
  },
  onMarkAllRead: async () => {
    // Reuse the article-list action so the shortcut has the same scope,
    // confirmation dialog, and filtered-article handling as the toolbar button.
    window.dispatchEvent(new CustomEvent('mark-all-as-read'));
  },
});

onMounted(async () => {
  // Install global notification handlers
  installGlobalHandlers();
  advertisements.start();

  // Initialize theme system immediately (lightweight)
  store.initTheme();
  store.pollProgress();
  void store.startFreshRSSStatusPolling();

  // Load remaining settings (theme and other settings are already loaded in main.ts)
  let updateInterval = 10;
  let refreshMode = 'fixed';
  let lastGlobalRefresh = '';
  let updateCheckEnabled = true;

  try {
    const res = await fetch('/api/settings');
    const data = await res.json();

    // Set initial article list width based on layout mode setting
    const layoutMode = data.layout_mode || 'normal';
    const isCompactModeLayout = layoutMode === 'compact';
    isCardMode.value = layoutMode === 'card';
    isTableMode.value = layoutMode === 'table';
    // Restore the user's width for this layout.
    setCompactMode(isCompactModeLayout);

    // Notify all components that settings have been loaded
    window.dispatchEvent(new CustomEvent('settings-loaded'));

    // Apply saved theme preference (already applied in main.ts, but ensure it's set)
    if (data.theme) {
      store.setTheme(data.theme);
    }

    // Apply other settings
    refreshMode = data.refresh_mode || 'fixed';
    if (data.update_interval) {
      updateInterval = parseInt(data.update_interval);
    }
    store.startAutoRefresh(getAutoRefreshInterval(refreshMode, updateInterval));

    if (data.last_global_refresh) {
      lastGlobalRefresh = data.last_global_refresh;
    }

    updateCheckEnabled = data.update_check_enabled !== 'false';

    shortcutsEnabled.value = data.shortcuts_enabled !== 'false';

    // Load saved shortcuts
    if (data.shortcuts) {
      try {
        const parsed = JSON.parse(data.shortcuts);
        shortcuts.value = { ...shortcuts.value, ...parsed };
      } catch (e) {
        console.error('Error parsing shortcuts:', e);
      }
    }
  } catch (e) {
    console.error('Error loading initial settings:', e);
  }

  if (updateCheckEnabled && isDesktopShell()) {
    // Check for updates on startup (silent mode - don't show toast if up to date)
    setTimeout(async () => {
      try {
        await checkForUpdates(true);

        // If update is available, show dialog for user to manually confirm
        if (updateInfo.value && updateInfo.value.has_update) {
          showUpdateDialog.value = true;
        }
      } catch (e) {
        console.error('Error checking for updates:', e);
      }
    }, 3000); // Check 3 seconds after startup
  }

  // Defer heavy operations to allow UI to render first
  setTimeout(() => {
    // Load feeds and articles in background
    store.fetchFeeds();
    store.fetchArticles();

    // Check if backend is already refreshing (e.g., from auto-refresh on startup)
    // and start polling progress if so
    setTimeout(async () => {
      try {
        const progressRes = await fetch('/api/progress');
        const progressData = await progressRes.json();

        if (progressData.is_running) {
          // Backend is already refreshing, start polling
          store.refreshProgress = {
            ...store.refreshProgress,
            isRunning: true,
            pool_task_count: progressData.pool_task_count,
            article_click_count: progressData.article_click_count,
            queue_task_count: progressData.queue_task_count,
          };
          store.pollProgress();
          return; // Don't trigger another refresh
        }
      } catch (e) {
        console.error('Error checking initial refresh progress:', e);
      }

      // Only trigger feed refresh if enough time has passed since last update
      // and backend is not already refreshing

      // Re-fetch the latest last_global_refresh from backend to ensure we have
      // the most recent value (in case a previous update just completed)
      let latestLastGlobalRefresh = lastGlobalRefresh;
      try {
        const settingsRes = await fetch('/api/settings');
        const settingsData = await settingsRes.json();
        if (settingsData.last_global_refresh) {
          latestLastGlobalRefresh = settingsData.last_global_refresh;
        }
      } catch (e) {
        console.error('Error fetching latest last_global_refresh:', e);
      }

      const shouldRefresh =
        refreshMode === 'fixed' && shouldTriggerRefresh(latestLastGlobalRefresh, updateInterval);
      if (shouldRefresh) {
        store.refreshFeeds();
      }
    }, 500);
  }, 100);
});

// Listen for events from Sidebar (moved outside onMounted to ensure proper capture)
window.addEventListener('show-add-feed', () => {
  showAddFeed.value = true;
});
window.addEventListener('show-edit-feed', (e) => {
  const customEvent = e as CustomEvent<any>;
  feedToEdit.value = customEvent.detail;
  showEditFeed.value = true;
});
window.addEventListener('show-settings', (e) => {
  const requestedTab = (e as CustomEvent<{ tab?: TabName }>).detail?.tab;
  settingsInitialTab.value = requestedTab || 'general';
  showSettings.value = true;
});
window.addEventListener('show-discover-blogs', (e) => {
  const customEvent = e as CustomEvent<any>;
  feedToDiscover.value = customEvent.detail;
  showDiscoverBlogs.value = true;
});

// Listen for compact mode changes to update article list width
window.addEventListener('layout-mode-changed', (e) => {
  const customEvent = e as CustomEvent<{ mode: string }>;
  const mode = customEvent.detail.mode;
  const isCompactModeLayout = mode === 'compact';
  isCardMode.value = mode === 'card';
  isTableMode.value = mode === 'table';
  setCompactMode(isCompactModeLayout);
});

// A refresh the user asked for reports the feeds that failed; the count is
// enough to act on, and the drawer's per-feed error badges say which ones.
window.addEventListener('refresh-finished', (e) => {
  const failed = (e as CustomEvent<{ failed?: number }>).detail?.failed ?? 0;
  if (failed > 0) {
    window.showToast(t('article.list.refreshFailed', { count: failed }), 'error');
  }
});

// Global Context Menu Event Listener
window.addEventListener('open-context-menu', (e) => {
  openContextMenu(e as CustomEvent<any>);
});

// Check if we should trigger refresh based on last update time and interval
function shouldTriggerRefresh(lastUpdate: string, intervalMinutes: number): boolean {
  if (!lastUpdate) {
    return true; // Never updated, should refresh
  }

  try {
    const lastUpdateTime = new Date(lastUpdate).getTime();
    const now = Date.now();
    const intervalMs = intervalMinutes * 60 * 1000;

    // Refresh if more than interval time has passed since last update
    return now - lastUpdateTime >= intervalMs;
  } catch {
    return true; // Invalid date, should refresh
  }
}

function toggleSidebar(): void {
  isSidebarOpen.value = !isSidebarOpen.value;
}

async function onFeedAdded(feedId?: number): Promise<void> {
  await store.fetchFeeds();
  if (feedId) {
    await store.setFilter('all');
    store.setFeed(feedId);
  }
  // Start polling for progress as the backend is now fetching articles for the new feed
  if (!feedId) {
    store.pollProgress();
  }
}

function onFeedUpdated(): void {
  store.fetchFeeds();
  // Refresh articles to immediately apply hide_from_timeline changes
  store.fetchArticles();
}
</script>

<template>
  <div
    class="app-container flex h-screen w-full bg-bg-primary text-text-primary overflow-hidden"
    :style="{
      '--sidebar-width': sidebarWidth + 'px',
      '--article-list-width': articleListWidth + 'px',
    }"
  >
    <Sidebar :is-open="isSidebarOpen" @toggle="toggleSidebar" />

    <!-- Show ImageGalleryView when in image gallery mode -->
    <template v-if="isImageGalleryMode">
      <ImageGalleryView :is-sidebar-open="isSidebarOpen" @toggle-sidebar="toggleSidebar" />
    </template>

    <!-- Show ArticleList and ArticleDetail when not in image gallery mode -->
    <div
      v-else
      ref="readerPanes"
      class="reader-panes"
      :class="{ 'table-mode': isTableMode }"
      :style="{ '--table-list-height': tableSplit.split.value + '%' }"
    >
      <ArticleList :is-sidebar-open="isSidebarOpen" @toggle-sidebar="toggleSidebar" />

      <!-- Hide resizer and ArticleDetail when in card mode -->
      <template v-if="!isCardMode">
        <div
          v-if="isTableMode"
          class="table-resizer hidden md:block focus-visible:outline-2 focus-visible:outline-accent"
          role="separator"
          tabindex="0"
          aria-orientation="horizontal"
          :aria-label="t('article.table.resize')"
          :aria-valuenow="tableSplit.split.value"
          :aria-valuemin="25"
          :aria-valuemax="75"
          @pointerdown="tableSplit.start"
          @keydown="tableSplit.keydown"
        ></div>
        <div v-else class="resizer hidden md:block" @mousedown="startResizeArticleList"></div>

        <ArticleDetail />
      </template>
    </div>
    <div
      v-if="tableSplit.resizing.value"
      class="fixed inset-0 z-50 cursor-row-resize"
      aria-hidden="true"
    ></div>

    <PartnerAdvertisement
      v-if="!showSettings && !advertisements.popupDismissed && advertisements.visibleAds.length"
      :ad="advertisements.visibleAds[0]"
      popup
      class="partner-ad-popup fixed bottom-4 right-4 z-40 w-80 max-w-[calc(100vw-2rem)] max-h-[calc(100vh-2rem)] overflow-y-auto"
    />
    <AddFeedModal v-if="showAddFeed" @close="showAddFeed = false" @added="onFeedAdded" />
    <EditFeedModal
      v-if="showEditFeed && feedToEdit"
      :feed="feedToEdit"
      @close="showEditFeed = false"
      @updated="onFeedUpdated"
    />
    <SettingsModal
      v-if="showSettings"
      :initial-tab="settingsInitialTab"
      @close="showSettings = false"
    />
    <DiscoverFeedsModal
      v-if="showDiscoverBlogs && feedToDiscover"
      :feed="feedToDiscover"
      :show="showDiscoverBlogs"
      @close="showDiscoverBlogs = false"
    />

    <UpdateAvailableDialog
      v-if="showUpdateDialog && updateInfo"
      :update-info="updateInfo"
      :downloading-update="downloadingUpdate"
      :installing-update="installingUpdate"
      :download-progress="downloadProgress"
      :download-progress-known="downloadProgressKnown"
      :download-bytes-written="downloadBytesWritten"
      :download-total-bytes="downloadTotalBytes"
      :download-error-code="downloadErrorCode"
      @close="showUpdateDialog = false"
      @update="downloadAndInstallUpdate"
    />

    <ContextMenu
      v-if="contextMenu.show"
      :x="contextMenu.x"
      :y="contextMenu.y"
      :items="contextMenu.items"
      @close="contextMenu.show = false"
      @action="handleContextMenuAction"
    />

    <!-- Global Notification System -->
    <ConfirmDialog
      v-if="confirmDialog"
      :title="confirmDialog.title"
      :message="confirmDialog.message"
      :confirm-text="confirmDialog.confirmText"
      :cancel-text="confirmDialog.cancelText"
      :is-danger="confirmDialog.isDanger"
      @confirm="confirmDialog.onConfirm"
      @cancel="confirmDialog.onCancel"
      @close="confirmDialog = null"
    />

    <InputDialog
      v-if="inputDialog"
      :title="inputDialog.title"
      :message="inputDialog.message"
      :placeholder="inputDialog.placeholder"
      :default-value="inputDialog.defaultValue"
      :confirm-text="inputDialog.confirmText"
      :cancel-text="inputDialog.cancelText"
      :suggestions="inputDialog.suggestions"
      @confirm="inputDialog.onConfirm"
      @cancel="inputDialog.onCancel"
      @close="inputDialog = null"
    />

    <MultiSelectDialog
      v-if="multiSelectDialog"
      :title="multiSelectDialog.title"
      :message="multiSelectDialog.message"
      :options="multiSelectDialog.options"
      :confirm-text="multiSelectDialog.confirmText"
      :cancel-text="multiSelectDialog.cancelText"
      @confirm="multiSelectDialog.onConfirm"
      @cancel="multiSelectDialog.onCancel"
      @close="multiSelectDialog = null"
    />

    <div class="toast-container">
      <Toast
        v-for="toast in toasts"
        :key="toast.id"
        :message="toast.message"
        :type="toast.type"
        :duration="toast.duration"
        @close="removeToast(toast.id)"
      />
    </div>
  </div>
</template>

<style>
.toast-container {
  position: fixed;
  top: 10px;
  left: 50%;
  transform: translateX(-50%);
  z-index: 60;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  pointer-events: none;
}

.toast-container > * {
  top: 42px; /* Account for MacOS top padding */
}

.toast-container > * {
  pointer-events: auto;
}

/* Installed-app mode on phones: keep the interface clear of the notch and the
   home indicator. Desktop windows and browser tabs never match this query. */
@media (display-mode: standalone) {
  .app-container {
    padding-top: env(safe-area-inset-top);
    padding-bottom: env(safe-area-inset-bottom);
    padding-left: env(safe-area-inset-left);
    padding-right: env(safe-area-inset-right);
  }
}
@media (min-width: 640px) {
  .toast-container {
    top: 20px;
    gap: 10px;
  }
  .app-container.macos-padding .toast-container {
    top: 52px; /* Account for MacOS top padding on larger screens */
  }
}
/* Native shell: clear the status bar, the cutout and the gesture area in every
   orientation. `main.ts` adds the class, and the environment variables resolve
   to 0 in a desktop window or a browser tab, so nothing changes there. */
.mobile-shell .app-container {
  padding-top: env(safe-area-inset-top, 0px);
  padding-bottom: env(safe-area-inset-bottom, 0px);
  padding-left: env(safe-area-inset-left, 0px);
  padding-right: env(safe-area-inset-right, 0px);
}
/* Portrait phones additionally make room for the bottom activity bar (see
   ActivityBar.vue); landscape falls back to the desktop layout and the rail.
   This applies to the browser build too, not only the native shell: the bar is
   fixed to the viewport in both, so anything anchored to the bottom of the
   layout (the feed drawer's footer, the last article) would sit underneath it. */
@media (max-width: 767px) {
  /* `.mobile-shell .app-container` above is more specific and only reserves the
     gesture area, so it has to be listed here as well; otherwise the native
     shell keeps 24px of bottom padding instead of clearing the 56px bar. */
  .app-container,
  .mobile-shell .app-container {
    padding-bottom: calc(56px + env(safe-area-inset-bottom, 0px));
  }

  /* Toasts move to the bottom on phones, just above the activity bar. Pinned to
     the top they covered the article toolbar — the back button and the overflow
     menu — for the whole three seconds they are on screen. The bar is 56px and
     the container is padded by it, but the toast is fixed to the viewport. */
  .toast-container,
  .mobile-shell .toast-container {
    top: auto;
    bottom: calc(56px + env(safe-area-inset-bottom, 0px) + 12px);
  }

  /* The ad popup is anchored to the viewport corner, so at `bottom-4` it landed
     on the activity bar — dimming and swallowing taps meant for the last two
     views. Lift it above the bar and let it grow with the safe area. */
  .partner-ad-popup {
    bottom: calc(56px + env(safe-area-inset-bottom, 0px) + 12px);
    max-height: calc(100% - 56px - env(safe-area-inset-bottom, 0px) - 1.5rem);
  }
}
.resizer {
  width: 4px;
  cursor: col-resize;
  background-color: transparent;
  flex-shrink: 0;
  transition: background-color 0.2s;
  z-index: 10;
  margin-left: -2px;
  margin-right: -2px;
}
.reader-panes {
  display: flex;
  flex: 1;
  min-width: 0;
  min-height: 0;
  /* Narrow screens absolutely position the detail pane over the article list;
     this keeps that pane anchored to the panes area instead of the viewport. */
  position: relative;
}
@media (min-width: 768px) {
  .reader-panes.table-mode {
    display: flex;
    flex: 1;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
  }
  /* This block is global, not scoped, so `:deep(main)` is not valid here and
     the rule was silently dropped — the detail pane kept `h-full` and pushed
     past the split. Plain `> main` targets ArticleDetail's root. */
  .reader-panes.table-mode > main {
    height: auto;
    min-height: 0;
    flex: 1 1 0;
  }
  .table-resizer {
    height: 4px;
    flex-shrink: 0;
    cursor: row-resize;
    background-color: var(--color-border);
    touch-action: none;
  }
  .table-resizer:hover,
  .table-resizer:focus-visible {
    background-color: var(--color-accent);
  }
}
.resizer:hover,
.resizer:active {
  background-color: var(--color-accent, #3b82f6);
}
/* Global styles if needed */
</style>
