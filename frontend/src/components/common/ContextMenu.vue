<script setup lang="ts">
import {
  ref,
  onMounted,
  onUnmounted,
  computed,
  nextTick,
  watch,
  type Ref,
  type Component,
} from 'vue';
import {
  PhArrowBendLeftDown,
  PhArrowBendRightUp,
  PhArrowSquareOut,
  PhArticle,
  PhArrowsClockwise,
  PhBinoculars,
  PhCheckCircle,
  PhCircle,
  PhClockCountdown,
  PhCopy,
  PhDownloadSimple,
  PhEnvelope,
  PhEnvelopeOpen,
  PhEye,
  PhEyeSlash,
  PhFolderMinus,
  PhGlobe,
  PhLinkSimple,
  PhMagnifyingGlass,
  PhMagnifyingGlassPlus,
  PhPencil,
  PhPushPin,
  PhRssSimple,
  PhStar,
  PhTextT,
  PhTrash,
} from '@phosphor-icons/vue';
import type { ContextMenuItem } from '@/types/context-menu';
import { useAndroidBack, BACK_PRIORITY } from '@/composables/ui/useAndroidBack';
import { bottomBarHeight } from '@/composables/ui/usePhoneLayout';

interface Props {
  items: ContextMenuItem[];
  x: number;
  y: number;
}

const props = defineProps<Props>();

const emit = defineEmits<{
  close: [];
  action: [action: string];
}>();

const menuRef: Ref<HTMLDivElement | null> = ref(null);
const adjustedPosition = ref({ top: 0, left: 0 });
let listenerTimer: ReturnType<typeof setTimeout>;

function handleKeyDown(event: KeyboardEvent) {
  if (event.key === 'Escape') {
    event.preventDefault();
    event.stopPropagation();
    emit('close');
  }
}

// The menu is mounted only while it is open, so a mount-scoped handler is
// enough. Without it a back press on a phone would leave the app from under an
// open menu instead of dismissing the menu.
useAndroidBack(() => {
  emit('close');
  return true;
}, BACK_PRIORITY.popover);

// Only the icons the menus actually ask for. Importing the package namespace
// (`import * as PhosphorIcons`) disabled tree-shaking: every icon in the
// catalogue shipped in the bundle, on every launch. The cost of the registry is
// that a new menu icon has to be added here; a miss renders no icon and warns
// in development.
const iconComponents: Record<string, Component> = {
  PhArrowBendLeftDown,
  PhArrowBendRightUp,
  PhArrowSquareOut,
  PhArticle,
  PhArrowsClockwise,
  PhBinoculars,
  PhCheckCircle,
  PhCircle,
  PhClockCountdown,
  PhCopy,
  PhDownloadSimple,
  PhEnvelope,
  PhEnvelopeOpen,
  PhEye,
  PhEyeSlash,
  PhFolderMinus,
  PhGlobe,
  PhLinkSimple,
  PhMagnifyingGlass,
  PhMagnifyingGlassPlus,
  PhPencil,
  PhPushPin,
  PhRssSimple,
  PhStar,
  PhTextT,
  PhTrash,
};

// Map old icon names to registry keys
const iconMap: Record<string, string> = {
  'ph-link': 'PhLinkSimple',
  'ph-link-simple': 'PhLinkSimple',
  'ph-text-t': 'PhTextT',
  'ph-check-circle': 'PhCheckCircle',
  'ph-circle': 'PhCircle',
  'ph-globe': 'PhGlobe',
  'ph-pencil': 'PhPencil',
  'ph-trash': 'PhTrash',
  'ph-envelope': 'PhEnvelope',
  'ph-envelope-open': 'PhEnvelopeOpen',
  'ph-star': 'PhStar',
  'ph-article': 'PhArticle',
  'ph-eye': 'PhEye',
  'ph-eye-slash': 'PhEyeSlash',
  'ph-arrow-square-out': 'PhArrowSquareOut',
  'ph-clock-countdown': 'PhClockCountdown',
  'ph-arrow-bend-right-up': 'PhArrowBendRightUp',
  'ph-arrow-bend-left-down': 'PhArrowBendLeftDown',
  'ph-rss-simple': 'PhRssSimple',
  PhMagnifyingGlass: 'PhMagnifyingGlass',
  PhArrowsClockwise: 'PhArrowsClockwise',
  PhMagnifyingGlassPlus: 'PhMagnifyingGlassPlus',
  PhDownloadSimple: 'PhDownloadSimple',
};

// Get icon component from icon string
function getIconComponent(iconName?: string): Component | null {
  if (!iconName) return null;
  const componentName = iconMap[iconName] || iconName;
  const component = iconComponents[componentName];
  if (!component && import.meta.env.DEV) {
    console.warn(`ContextMenu: no icon registered for "${iconName}"`);
  }
  return component ?? null;
}

function handleClickOutside(event: MouseEvent) {
  if (menuRef.value && event.target instanceof Node && !menuRef.value.contains(event.target)) {
    emit('close');
  }
}

// Adjust position to keep menu within viewport
function adjustMenuPosition() {
  nextTick(() => {
    if (!menuRef.value) {
      adjustedPosition.value = { top: props.y, left: props.x };
      return;
    }

    // `offset*` rather than `getBoundingClientRect()`: the menu opens with a
    // `scale(0.95)` fade-in, and a rect measured during that animation is 5%
    // short — it left the last row below the clamp, under the phone's bottom
    // bar. Offset sizes are the layout box and ignore the transform.
    const menuWidth = menuRef.value.offsetWidth;
    const menuHeight = menuRef.value.offsetHeight;
    const viewportWidth = window.innerWidth;
    // The phone's bottom bar covers the last slice of the viewport; a menu
    // clamped to `innerHeight` would tuck its bottom rows underneath it.
    const viewportHeight = window.innerHeight - bottomBarHeight();

    let newTop = props.y;
    let newLeft = props.x;

    // Check if menu goes beyond right edge
    if (props.x + menuWidth > viewportWidth - 10) {
      newLeft = props.x - menuWidth;
      if (newLeft < 10) newLeft = 10;
    }

    // Check if menu goes beyond bottom edge
    if (props.y + menuHeight > viewportHeight - 10) {
      newTop = props.y - menuHeight;
      if (newTop < 10) newTop = 10;
    }

    adjustedPosition.value = { top: newTop, left: newLeft };
  });
}

onMounted(() => {
  // Adjust position immediately
  adjustMenuPosition();

  // Use setTimeout to avoid catching the event that opened the menu
  document.addEventListener('keydown', handleKeyDown, true);
  listenerTimer = setTimeout(() => {
    // Capture outside presses before card/detail overlays stop propagation.
    document.addEventListener('click', handleClickOutside, true);
    document.addEventListener('contextmenu', handleClickOutside, true);
  }, 0);
});

// Watch for position changes and re-adjust
watch(
  () => [props.x, props.y],
  () => {
    adjustMenuPosition();
  }
);

onUnmounted(() => {
  clearTimeout(listenerTimer);
  document.removeEventListener('keydown', handleKeyDown, true);
  document.removeEventListener('click', handleClickOutside, true);
  document.removeEventListener('contextmenu', handleClickOutside, true);
});

function handleAction(item: ContextMenuItem) {
  if (item.disabled) return;
  if (item.action) {
    emit('action', item.action);
  }
  emit('close');
}

const menuStyle = computed(() => ({
  top: `${adjustedPosition.value.top}px`,
  left: `${adjustedPosition.value.left}px`,
}));
</script>

<template>
  <div
    ref="menuRef"
    class="fixed z-[1000] bg-bg-primary border border-border rounded-lg shadow-xl py-1 min-w-[180px] animate-fade-in"
    :style="menuStyle"
  >
    <template v-for="(item, index) in items" :key="index">
      <div v-if="item.separator" class="h-px bg-border my-1"></div>
      <div
        v-else
        class="px-4 py-2 min-h-11 flex items-center gap-3 cursor-pointer hover:bg-bg-tertiary text-sm transition-colors"
        :class="[
          item.disabled ? 'opacity-50 cursor-not-allowed' : '',
          item.danger
            ? 'text-red-600 dark:text-red-400 hover:bg-red-50 dark:hover:bg-red-900/20'
            : 'text-text-primary',
        ]"
        @click="handleAction(item)"
      >
        <component
          :is="getIconComponent(item.icon)"
          v-if="item.icon && getIconComponent(item.icon)"
          :size="20"
          :weight="item.iconWeight || 'regular'"
          :class="
            item.iconColor ||
            (item.danger ? 'text-red-600 dark:text-red-400' : 'text-text-secondary')
          "
        />
        <span>{{ item.label }}</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.animate-fade-in {
  animation: fadeIn 0.1s ease-out;
}
@keyframes fadeIn {
  from {
    opacity: 0;
    transform: scale(0.95);
  }
  to {
    opacity: 1;
    transform: scale(1);
  }
}
</style>
