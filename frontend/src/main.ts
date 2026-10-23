import { createApp } from 'vue';
import { createPinia } from 'pinia';
import i18n, { locale } from './i18n';
import { resolveLocale } from './i18n/locale';
import './style.css';
import App from './App.vue';
import { setSettingsFromRawData } from './composables/core/useSettings';
import { registerServiceWorker } from './utils/pwa';
import { installAndroidApiBridge } from './utils/apiBridge';
import { installAndroidBackBridge } from './composables/ui/useAndroidBack';
import { isMobileShell } from './utils/platform';

// Must run before the first request is issued: inside the Android host the
// WebView asset loader cannot carry request bodies, so /api/ traffic is routed
// through the native bridge instead. No-op everywhere else.
installAndroidApiBridge();

// The Android shell forwards the hardware back button here instead of closing
// the activity; the registered handlers decide whether the press was consumed.
installAndroidBackBridge();

// Mark the document so the stylesheet can reserve the system bars. Neither
// `display-mode: standalone` nor a media query identifies this host: the native
// WebView reports a browser display mode, and a landscape phone is wider than
// the phone breakpoint while still having a status bar and cutout to clear.
if (isMobileShell()) {
  document.documentElement.classList.add('mobile-shell');
}

const app = createApp(App);
const pinia = createPinia();

// Add global error handler for Vue errors
app.config.errorHandler = (err, instance, info) => {
  console.error('[Vue Error Handler] Error:', err);
  console.error('[Vue Error Handler] Component:', instance?.$?.type?.name || 'Unknown');
  console.error('[Vue Error Handler] Info:', info);
  // Log the full stack trace
  if (err instanceof Error) {
    console.error('[Vue Error Handler] Stack:', err.stack);
  }
};

app.use(pinia);
app.use(i18n);

// Initialize language setting before mounting
async function initializeApp() {
  try {
    const res = await fetch('/api/settings');
    if (!res.ok) {
      throw new Error(`HTTP ${res.status}: ${res.statusText}`);
    }

    // Get response text first to debug JSON parsing issues
    const text = await res.text();
    let data;

    try {
      data = JSON.parse(text);
    } catch (jsonError) {
      console.error('JSON parse error:', jsonError);
      console.error('Response text (first 500 chars):', text.substring(0, 500));
      // Use default empty object if JSON is invalid
      data = {};
    }

    if (data.language) {
      locale.value = resolveLocale(data.language);
    }

    setSettingsFromRawData(data);

    // Start FreshRSS status polling if enabled
    // Note: Don't use store here - it will be initialized after mount
    // Store initialization will handle FreshRSS polling in App.vue
  } catch (e) {
    console.error('Error loading language setting:', e);
  }

  app.mount('#app');
}

// Initialize and mount
initializeApp();
registerServiceWorker();
