import { createI18n } from 'vue-i18n';
import { watch } from 'vue';
import type { SupportedLocale } from './types';

// Import translation messages
import en from './locales/en';
import zh from './locales/zh';

const i18n = createI18n({
  legacy: false, // Use Composition API mode
  locale: 'en-US', // Default to English, will be overridden by settings
  fallbackLocale: 'en-US' as SupportedLocale,
  messages: {
    'en-US': en,
    'zh-CN': zh,
  },
});

export default i18n;

// Export the i18n instance for direct access
export const { locale } = i18n.global;

type Translate = (key: string, named?: Record<string, unknown>) => string;

/**
 * Translate outside a component's setup. The global composer's own `t` type is
 * expanded from the whole message tree, which exceeds the compiler's
 * instantiation depth limit for literal keys, so utilities reach it through
 * this narrower signature instead.
 */
export function translate(key: string, named?: Record<string, unknown>): string {
  return (i18n as unknown as { global: { t: Translate } }).global.t(key, named);
}

/**
 * Maps whatever is stored in the settings onto a locale the bundle ships.
 *
 * The dropdown writes `en-US` / `zh-CN`, but a config written by an older build
 * or by hand can hold `zh`, `en` or `zh-Hans`. vue-i18n then finds no messages
 * for that key, falls back to English without saying so, and the dropdown shows
 * its placeholder because no option matches the stored value — an empty
 * "Select..." on the General tab while the UI is English.
 */
export { resolveLocale } from './locale';

// Keep <html lang> in step with the interface language. It drives hyphenation,
// font fallback and which voice a screen reader picks; the shell ships it as
// `en`, which is wrong the moment the user switches to Chinese.
watch(
  locale,
  (value) => {
    if (typeof document !== 'undefined') document.documentElement.lang = value;
  },
  { immediate: true }
);
