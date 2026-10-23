import type { SupportedLocale } from './types';

/**
 * Maps whatever is stored in the settings onto a locale the bundle ships.
 *
 * The dropdown writes `en-US` / `zh-CN`, but a config written by an older build
 * or by hand can hold `zh`, `en` or `zh-Hans`. vue-i18n then finds no messages
 * for that key, falls back to English without saying so, and the dropdown shows
 * its placeholder because no option matches the stored value — an empty
 * "Select..." on the General tab while the UI is English.
 *
 * Lives in its own module, away from the i18n instance, so callers that only
 * need to normalise a string do not drag `createI18n` into their import graph.
 */
export function resolveLocale(value?: string | null): SupportedLocale {
  if (value && value.toLowerCase().startsWith('zh')) return 'zh-CN';
  return 'en-US';
}
