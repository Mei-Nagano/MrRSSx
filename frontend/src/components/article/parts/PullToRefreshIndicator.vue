<script setup lang="ts">
import { useI18n } from 'vue-i18n';
import { PhArrowDown, PhSpinner } from '@phosphor-icons/vue';

// The bar above a list that is being pulled down. It grows with the gesture so
// the pull reads as the list being dragged rather than as a banner appearing;
// the caller moves the list itself.
defineProps<{
  distance: number;
  armed: boolean;
  refreshing: boolean;
}>();

const { t } = useI18n();
</script>

<template>
  <div
    class="pointer-events-none absolute inset-x-0 top-0 z-10 flex items-center justify-center gap-2 overflow-hidden bg-bg-primary text-xs text-text-secondary"
    :style="{ height: `${distance}px` }"
    aria-hidden="true"
  >
    <PhSpinner v-if="refreshing" :size="16" class="animate-spin" />
    <PhArrowDown
      v-else
      :size="16"
      class="transition-transform duration-150"
      :class="{ 'rotate-180': armed }"
    />
    <span v-if="distance > 28">
      {{
        refreshing
          ? t('article.list.refreshing')
          : armed
            ? t('article.list.releaseToRefresh')
            : t('article.list.pullToRefresh')
      }}
    </span>
  </div>
</template>
