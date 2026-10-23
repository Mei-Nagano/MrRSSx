<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import { useI18n } from 'vue-i18n';
import {
  PhArrowClockwise,
  PhFolderOpen,
  PhGlobe,
  PhKey,
  PhUser,
} from '@phosphor-icons/vue';
import type { SettingsData } from '@/types/settings';
import { useAppStore } from '@/stores/app';
import {
  NestedSettingsContainer,
  SubSettingItem,
  InputControl,
  SelectControl,
  ToggleControl,
} from '@/components/settings';
import { openInBrowser } from '@/utils/browser';

const props = defineProps<{ settings: SettingsData }>();
const emit = defineEmits<{ 'update:settings': [settings: SettingsData] }>();

const { t } = useI18n();
const store = useAppStore();

const account = ref<{ name: string; email: string } | null>(null);
const lastImport = ref<string | null>(null);
const lastSync = ref<string | null>(null);
const mappedCount = ref(0);
const credential = ref('');
const signingIn = ref(false);
const importing = ref(false);
const syncing = ref(false);
const busy = ref(false);

const controller = new AbortController();
let poll: ReturnType<typeof setInterval> | undefined;

const apiUrl = computed(() => props.settings.folo_api_url);
const webUrl = computed(() => props.settings.folo_web_url);
const folderMode = computed(() => props.settings.folo_folder_mode);
const enabled = computed(() => props.settings.folo_enabled);

function setEnabled(value: boolean) {
  emit('update:settings', { ...props.settings, folo_enabled: value });
}

// Turning the integration off hides the panel and stops the sign-in poll, so no
// request reaches Folo while it is disabled.
watch(enabled, (on) => {
  if (on) {
    void fetchAccount();
    return;
  }
  stopPolling();
  signingIn.value = false;
});

function update(field: 'api_url' | 'web_url' | 'folder_mode', value: string) {
  emit('update:settings', { ...props.settings, [`folo_${field}`]: value });
}

async function fetchAccount() {
  try {
    const res = await fetch('/api/folo/account', { signal: controller.signal });
    // 401 is the only answer that means "signed out". A 502 means the account
    // could not be checked right now (offline, Folo unreachable), so keep the
    // last known state instead of pretending the user was logged out.
    if (res.status === 401) {
      account.value = null;
      return;
    }
    if (!res.ok) return;
    const data = await res.json();
    account.value = { name: data.name || '', email: data.email || '' };
    lastImport.value = data.last_import_time || null;
    lastSync.value = data.last_sync_time || null;
    mappedCount.value = Number(data.mapped_count) || 0;
  } catch {
    /* Aborted or offline; the status is refreshed when the tab is opened again. */
  }
}

function stopPolling() {
  if (poll) clearInterval(poll);
  poll = undefined;
}

// A rejected token means the stored session is gone (expired, revoked, or a
// different account). Showing "signed in" after that is worse than a fresh
// sign-in prompt, so the panel drops back to the signed-out state.
function expireSession() {
  account.value = null;
  stopPolling();
  signingIn.value = false;
}

async function request(path: string, init?: RequestInit) {
  const res = await fetch(path, { ...init, signal: controller.signal });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) {
    throw new Error(data.code || 'error');
  }
  return data;
}

// The browser owns the actual sign-in, so the UI follows the loopback callback by
// asking the backend what happened to it.
async function checkLoginStatus() {
  try {
    const data = await request('/api/folo/login/status');
    if (data.state === 'connected') {
      stopPolling();
      signingIn.value = false;
      await fetchAccount();
      window.showToast(t('setting.folo.signInSuccess'), 'success');
    } else if (data.state === 'failed' || data.state === 'expired') {
      stopPolling();
      signingIn.value = false;
      window.showToast(t('setting.folo.signInFailed'), 'error');
    }
  } catch {
    /* Keep waiting; the attempt may still land. */
  }
}

function watchLogin() {
  stopPolling();
  let ticks = 0;
  signingIn.value = true;
  poll = setInterval(async () => {
    if (ticks++ > 100) {
      stopPolling();
      signingIn.value = false;
      return;
    }
    await checkLoginStatus();
  }, 2000);
}

async function signIn() {
  if (busy.value) return;
  busy.value = true;
  try {
    const data = await request('/api/folo/login/start', { method: 'POST' });
    await openInBrowser(data.login_url);
    watchLogin();
  } catch {
    window.showToast(t('setting.folo.signInFailed'), 'error');
  } finally {
    busy.value = false;
  }
}

async function useToken() {
  const value = credential.value.trim();
  if (!value || busy.value) return;
  busy.value = true;
  try {
    const data = await request('/api/folo/login/complete', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ credential: value }),
    });
    account.value = { name: data.name || '', email: data.email || '' };
    credential.value = '';
    stopPolling();
    signingIn.value = false;
    window.showToast(t('setting.folo.signInSuccess'), 'success');
  } catch {
    window.showToast(t('setting.folo.signInFailed'), 'error');
  } finally {
    busy.value = false;
  }
}

async function signOut() {
  busy.value = true;
  try {
    await request('/api/folo/logout', { method: 'POST' });
    account.value = null;
    stopPolling();
    signingIn.value = false;
    window.showToast(t('setting.folo.signOutSuccess'), 'success');
  } catch {
    window.showToast(t('setting.folo.actionFailed'), 'error');
  } finally {
    busy.value = false;
  }
}

async function importSubscriptions() {
  if (busy.value) return;
  if (!account.value) {
    window.showToast(t('setting.folo.accountRequired'), 'error');
    return;
  }
  busy.value = true;
  importing.value = true;
  try {
    const data = await request('/api/folo/import', { method: 'POST' });
    window.showToast(t('setting.folo.importDone', { imported: data.imported, total: data.total }), 'success');
    await fetchAccount();
    store.fetchFeeds();
    store.pollProgress();
  } catch (error) {
    const code = error instanceof Error ? error.message : '';
    if (code === 'not_authenticated') {
      expireSession();
      window.showToast(t('setting.folo.sessionExpired'), 'error');
    } else {
      window.showToast(t('setting.folo.importFailed'), 'error');
    }
  } finally {
    busy.value = false;
    importing.value = false;
  }
}

async function syncState() {
  if (busy.value) return;
  if (!account.value) {
    window.showToast(t('setting.folo.accountRequired'), 'error');
    return;
  }
  busy.value = true;
  syncing.value = true;
  try {
    const data = await request('/api/folo/sync', { method: 'POST' });
    window.showToast(
      t('setting.folo.syncDone', { pushed: data.pushed, pulled: data.pulled }),
      'success',
    );
    await fetchAccount();
    // Pulled state changes the read/favorite flags the list is rendering.
    window.dispatchEvent(new CustomEvent('refresh-articles'));
  } catch (error) {
    const code = error instanceof Error ? error.message : '';
    if (code === 'not_authenticated') {
      expireSession();
      window.showToast(t('setting.folo.sessionExpired'), 'error');
    } else {
      window.showToast(
        t(code === 'sync_busy' ? 'setting.folo.syncBusy' : 'setting.folo.syncFailed'),
        'error',
      );
    }
  } finally {
    busy.value = false;
    syncing.value = false;
  }
}

// Returning from the system browser (or from another app) is when the account and
// sign-in state may have changed behind our back. WebView timers are throttled
// while the page is hidden, so ask immediately instead of waiting for the next
// poll tick.
function onVisibilityChange() {
  if (document.visibilityState !== 'visible' || !enabled.value) return;
  if (signingIn.value) void checkLoginStatus();
  else void fetchAccount();
}

onMounted(() => {
  if (enabled.value) void fetchAccount();
  document.addEventListener('visibilitychange', onVisibilityChange);
});
onBeforeUnmount(() => {
  controller.abort();
  stopPolling();
  document.removeEventListener('visibilitychange', onVisibilityChange);
});
</script>

<template>
  <div>
    <div
      class="flex items-center justify-between gap-3 rounded-lg border border-border bg-bg-secondary p-2 sm:items-start sm:gap-4 sm:p-3"
    >
      <div class="flex min-w-0 flex-1 items-start gap-2 sm:gap-3">
        <img
          src="/assets/plugin_icons/folo.svg"
          alt="Folo"
          class="mt-0.5 h-5 w-5 shrink-0 sm:h-6 sm:w-6"
        />
        <div class="min-w-0">
          <div class="text-sm font-medium sm:mb-1 sm:text-base">
            {{ t('setting.folo.integration') }}
          </div>
          <div class="text-xs text-text-secondary">
            {{ t('setting.folo.integrationDesc') }}
          </div>
        </div>
      </div>
      <div class="flex shrink-0 items-center gap-1.5 sm:gap-2">
        <ToggleControl
          :model-value="enabled"
          :aria-label="t('setting.folo.integration')"
          @update:model-value="setEnabled"
        />
      </div>
    </div>

    <NestedSettingsContainer v-if="enabled">
      <SubSettingItem
        :icon="PhUser"
        :title="
          account
            ? t('setting.folo.signedInAs', { name: account.name || account.email })
            : t('setting.folo.notSignedIn')
        "
        :description="account ? account.email : t('setting.folo.signInHint')"
      >
        <button v-if="account" class="btn-secondary" :disabled="busy" @click="signOut">
          {{ t('setting.folo.signOut') }}
        </button>
        <button v-else class="btn-primary" :disabled="busy || signingIn" @click="signIn">
          <PhArrowClockwise :size="16" :class="{ 'animate-spin': signingIn }" />
          {{ t(signingIn ? 'setting.folo.waiting' : 'setting.folo.signIn') }}
        </button>
      </SubSettingItem>
      <SubSettingItem
        :icon="PhFolderOpen"
        :title="t('setting.folo.importTitle')"
        :description="t('setting.folo.importDesc')"
      >
        <template #extraInfo>
          {{ t('setting.folo.lastImport') }}:
          {{
            lastImport
              ? new Date(lastImport).toLocaleString()
              : t('setting.folo.never')
          }}
        </template>
        <button class="btn-secondary" :disabled="busy || !account" @click="importSubscriptions">
          <PhArrowClockwise :size="16" :class="{ 'animate-spin': importing }" />
          {{ t(importing ? 'setting.folo.importing' : 'setting.folo.import') }}
        </button>
      </SubSettingItem>

      <SubSettingItem
        :icon="PhArrowClockwise"
        :title="t('setting.folo.syncTitle')"
        :description="t('setting.folo.syncDesc')"
      >
        <template #extraInfo>
          {{ t('setting.folo.lastSync') }}:
          {{ lastSync ? new Date(lastSync).toLocaleString() : t('setting.folo.never') }}
          <span class="whitespace-nowrap text-xs text-text-secondary">
            · {{ t('setting.folo.mapped', { count: mappedCount }) }}
          </span>
        </template>
        <button class="btn-secondary" :disabled="busy || !account" @click="syncState">
          <PhArrowClockwise :size="16" :class="{ 'animate-spin': syncing }" />
          {{ t(syncing ? 'setting.folo.syncing' : 'setting.folo.sync') }}
        </button>
      </SubSettingItem>

      <SubSettingItem
        :icon="PhKey"
        :title="t('setting.folo.manualTitle')"
        :description="t('setting.folo.manualDesc')"
        stack-narrow
      >
        <div class="flex w-full items-center gap-2 lg:w-auto">
          <InputControl
            type="password"
            :model-value="credential"
            :placeholder="t('setting.folo.tokenPlaceholder')"
            width="w-full lg:w-48"
            @update:model-value="credential = String($event)"
          />
          <button class="btn-secondary" :disabled="busy || !credential.trim()" @click="useToken">
            {{ t('setting.folo.manualUse') }}
          </button>
        </div>
      </SubSettingItem>

      <SubSettingItem
        :icon="PhFolderOpen"
        :title="t('setting.folo.folderMode')"
        :description="t('setting.folo.folderModeDesc')"
      >
        <SelectControl
          :model-value="folderMode"
          width="md"
          :options="[
            { value: 'category', label: t('setting.folo.folderModeCategory') },
            { value: 'view', label: t('setting.folo.folderModeView') },
          ]"
          @update:model-value="update('folder_mode', String($event))"
        />
      </SubSettingItem>

      <SubSettingItem :icon="PhGlobe" :title="t('setting.folo.apiUrl')" :description="t('setting.folo.apiUrlDesc')">
        <InputControl
          type="url"
          :model-value="apiUrl"
          placeholder="https://api.folo.is"
          width="md"
          @update:model-value="update('api_url', String($event))"
        />
      </SubSettingItem>

      <SubSettingItem :icon="PhGlobe" :title="t('setting.folo.webUrl')" :description="t('setting.folo.webUrlDesc')">
        <InputControl
          type="url"
          :model-value="webUrl"
          placeholder="https://app.folo.is"
          width="md"
          @update:model-value="update('web_url', String($event))"
        />
      </SubSettingItem>
    </NestedSettingsContainer>
  </div>
</template>

<style scoped>
@reference "../../../../style.css";
.btn-primary {
  @apply bg-accent text-white px-3 sm:px-4 py-1.5 sm:py-2 rounded-md cursor-pointer flex items-center gap-1.5 sm:gap-2 font-medium hover:opacity-90 transition-colors disabled:opacity-50 disabled:cursor-wait shrink-0;
}
.btn-secondary {
  @apply bg-bg-tertiary border border-border text-text-primary px-3 sm:px-4 py-1.5 sm:py-2 rounded-md cursor-pointer flex items-center gap-1.5 sm:gap-2 font-medium hover:bg-bg-secondary transition-colors disabled:opacity-50 disabled:cursor-wait shrink-0;
}
</style>
