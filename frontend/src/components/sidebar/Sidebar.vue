<script setup lang="ts">
import { computed, ref, watch, onMounted, onUnmounted } from 'vue';
import ActivityBar from './ActivityBar.vue';
import FeedList from './FeedList.vue';
import { PHONE_QUERY } from '@/composables/ui/usePhoneLayout';
import { useAndroidBack, BACK_PRIORITY } from '@/composables/ui/useAndroidBack';

interface Props {
  isOpen: boolean;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  toggle: [];
}>();

const isFeedListPinned = ref(localStorage.getItem('FeedListPinned') !== 'false');
const sidebarRoot = ref<HTMLElement | null>(null);
watch(isFeedListPinned, (pinned) => {
  localStorage.setItem('FeedListPinned', String(pinned));
});

// Phones swap the icon rail for a bottom bar and show the feed list as a
// full-height sheet. A pinned 280px drawer would leave no room for the list,
// so the pin only applies above the phone breakpoint.
const phoneMedia = typeof window !== 'undefined' ? window.matchMedia(PHONE_QUERY) : null;
const isPhone = ref(phoneMedia?.matches ?? false);
const isEffectivelyPinned = computed(() => isFeedListPinned.value && !isPhone.value);

function handlePhoneChange(event: MediaQueryListEvent) {
  isPhone.value = event.matches;
}

// The bottom bar stays visible over the sheet, so picking a filter should also
// dismiss it instead of leaving the drawer covering the result.
function handleNavSelection() {
  if (isPhone.value && props.isOpen) emit('toggle');
}

// Back on Android dismisses a floating drawer. It does not touch a pinned one:
// that is a column in the layout, not something stacked over the reader. The
// pin state lives here, so the handler does too — App cannot tell the two apart.
useAndroidBack(() => {
  if (!props.isOpen || isEffectivelyPinned.value) return false;
  emit('toggle');
  return true;
}, BACK_PRIORITY.drawer);

function handleFeedListExpand() {
  if (!props.isOpen) emit('toggle');
}

function handleFeedListCollapse() {
  if (props.isOpen) emit('toggle');
}

function handlePinFeedList() {
  isFeedListPinned.value = true;
  handleFeedListExpand();
}

function handleUnpinFeedList() {
  isFeedListPinned.value = false;
}

const emitShowAddFeed = () => window.dispatchEvent(new CustomEvent('show-add-feed'));
const emitShowSettings = () => window.dispatchEvent(new CustomEvent('show-settings'));
const toggleFeedList = () => emit('toggle');

// Drawer footer actions. A floating drawer (phones, unpinned) closes so the
// dialog it opens is not sitting on top of a stale sheet; a pinned desktop list
// stays put because it is part of the normal layout, not an overlay.
function handleDrawerAddFeed() {
  if (!isEffectivelyPinned.value) handleFeedListCollapse();
  emitShowAddFeed();
}
function handleDrawerSettings() {
  if (!isEffectivelyPinned.value) handleFeedListCollapse();
  emitShowSettings();
}
function collapseFloatingFeedList(event?: Event) {
  // `isFeedListPinned` alone is not enough: on phones the pin is inert (the
  // drawer is always a sheet), so relying on it would make outside taps stop
  // closing the sheet for anyone who had pinned it on desktop.
  if (isEffectivelyPinned.value || !props.isOpen) return;
  if (event?.target instanceof Node && sidebarRoot.value?.contains(event.target)) return;
  handleFeedListCollapse();
}
onMounted(() => {
  window.addEventListener('toggle-feed-list', toggleFeedList);
  document.addEventListener('pointerdown', collapseFloatingFeedList, true);
  document.addEventListener('focusin', collapseFloatingFeedList);
  window.addEventListener('blur', collapseFloatingFeedList);
  phoneMedia?.addEventListener('change', handlePhoneChange);
});
onUnmounted(() => {
  window.removeEventListener('toggle-feed-list', toggleFeedList);
  document.removeEventListener('pointerdown', collapseFloatingFeedList, true);
  document.removeEventListener('focusin', collapseFloatingFeedList);
  window.removeEventListener('blur', collapseFloatingFeedList);
  phoneMedia?.removeEventListener('change', handlePhoneChange);
});
</script>

<template>
  <div ref="sidebarRoot" class="compact-sidebar-wrapper flex h-full relative">
    <div class="sidebar-toggle-container">
      <ActivityBar
        :is-feed-list-expanded="isOpen"
        @add-feed="emitShowAddFeed"
        @settings="emitShowSettings"
        @toggle-feed-drawer="emit('toggle')"
        @select-filter="handleNavSelection"
      />
    </div>

    <!-- Feed Drawer -->
    <Transition name="drawer-position">
      <div v-if="isOpen" class="feed-drawer-wrapper" :class="{ pinned: isEffectivelyPinned }">
        <FeedList
          :is-expanded="isOpen"
          :is-pinned="isEffectivelyPinned"
          @expand="handleFeedListExpand"
          @collapse="handleFeedListCollapse"
          @pin="handlePinFeedList"
          @unpin="handleUnpinFeedList"
          @add-feed="handleDrawerAddFeed"
          @settings="handleDrawerSettings"
        />
      </div>
    </Transition>

    <!-- Overlay for mobile -->
    <Transition name="overlay-fade">
      <div
        v-if="isOpen"
        class="fixed inset-0 bg-black/50 z-[35] md:hidden"
        @click="handleFeedListCollapse"
      ></div>
    </Transition>
  </div>
</template>

<style scoped>
.compact-sidebar-wrapper {
  position: relative;
  z-index: 20;
  display: flex;
  align-items: stretch;
}

/* Fixed-width navigation stays available while the feed drawer is closed. */
.sidebar-toggle-container {
  position: relative;
  width: 56px;
  min-width: 56px;
  height: 100%;
  flex-shrink: 0;
}

/* Smaller screens (laptops, tablets) */
@media (max-width: 1400px) {
  .sidebar-toggle-container {
    width: 48px;
    min-width: 48px;
  }
}

/* Mobile devices: the bar is fixed to the viewport, so the wrapper keeps no
   width and no stacking context of its own — otherwise it would trap the fixed
   bar underneath the article pane. */
@media (max-width: 767px) {
  .compact-sidebar-wrapper {
    z-index: auto;
  }
  .sidebar-toggle-container {
    width: 0;
    min-width: 0;
  }
}

.feed-drawer-wrapper {
  position: relative;
  z-index: 20;
  height: 100%;
  flex-shrink: 0;
}

.feed-drawer-wrapper:not(.pinned) {
  position: absolute;
  left: 56px;
  top: 0;
  bottom: 0;
  z-index: 20;
}

/* Smaller screens (laptops, tablets) */
@media (max-width: 1400px) {
  .feed-drawer-wrapper:not(.pinned) {
    left: 48px;
  }
}

/* Mobile devices: with no rail to clear, the sheet starts at the screen edge. */
@media (max-width: 767px) {
  .feed-drawer-wrapper:not(.pinned) {
    left: 0;
    /* The sheet has to stay usable while an article is open, and the article
       pane sits at z-30 on phones. It stays below the bottom bar (z-40) so the
       bar keeps answering taps and can dismiss the drawer. */
    z-index: 36;
  }
}

/* Drawer position transition */
.drawer-position-enter-active {
  transition:
    transform 0.3s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.2s ease;
  will-change: transform, opacity;
}

.drawer-position-leave-active {
  transition:
    transform 0.25s cubic-bezier(0.4, 0, 0.2, 1),
    opacity 0.2s ease;
  will-change: transform, opacity;
}

.drawer-position-enter-from {
  opacity: 0;
  transform: translateX(-16px);
}

.drawer-position-leave-to {
  opacity: 0;
  transform: translateX(-16px);
}

.drawer-position-enter-to,
.drawer-position-leave-from {
  opacity: 1;
  transform: translateX(0);
}

/* Optimize feed drawer rendering */
.feed-drawer-wrapper {
  backface-visibility: hidden;
  -webkit-font-smoothing: antialiased;
  transform: translateZ(0);
}

/* Overlay transition */
.overlay-fade-enter-active,
.overlay-fade-leave-active {
  transition: opacity 0.25s cubic-bezier(0.4, 0, 0.2, 1);
  will-change: opacity;
}

.overlay-fade-enter-from,
.overlay-fade-leave-to {
  opacity: 0;
}

.overlay-fade-enter-to,
.overlay-fade-leave-from {
  opacity: 1;
}
</style>
