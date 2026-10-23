import { describe, expect, it } from 'vitest';
import { createI18n } from 'vue-i18n';
import en from '../i18n/locales/en';
import zh from '../i18n/locales/zh';
import { formatDate, formatExactDateTime } from './date';

describe('article date preferences', () => {
  const date = '2026-12-31T15:04:00';

  it('formats calendar dates without converting the local day to UTC', () => {
    for (const [format, expected] of [
      ['yyyy-mm-dd', '2026-12-31'],
      ['mm/dd/yyyy', '12/31/2026'],
      ['dd/mm/yyyy', '31/12/2026'],
      ['dd.mm.yyyy', '31.12.2026'],
    ]) {
      expect(formatExactDateTime(date, 'en-US', { date_format: format, time_format: '24h' })).toBe(
        `${expected} 15:04`
      );
    }
  });

  it('supports 12-hour time and disabling relative dates', () => {
    expect(
      formatDate(date, 'en-US', undefined, {
        date_format: 'yyyy-mm-dd',
        time_format: '12h',
        relative_time: false,
      })
    ).toBe('2026-12-31 03:04 PM');
  });

  it('uses midnight rather than 24:00 in the 24-hour clock', () => {
    expect(
      formatExactDateTime('2026-12-31T00:00:00', 'en-US', {
        date_format: 'yyyy-mm-dd',
        time_format: '24h',
      })
    ).toBe('2026-12-31 00:00');
  });

  it('falls back to locale defaults for unknown stored formats', () => {
    const invalid = { date_format: 'unknown', time_format: 'unknown' };
    expect(formatExactDateTime(date, 'en-US', invalid)).toContain('2026');
    expect(formatDate('invalid', 'en-US', undefined, invalid)).toBe('');
    expect(formatExactDateTime('invalid', 'en-US', invalid)).toBe('');
  });
});

describe('relative time plural forms', () => {
  // The English strings used to hard-code the plural ("{count} days ago"), so a
  // single day read "1 days ago". They now use vue-i18n pipe plurals, which the
  // formatter selects from the `count` argument it already passes. Wire the real
  // locale files through a real i18n instance so a regression to the flat string
  // fails here instead of only on a phone.
  const i18n = createI18n({
    legacy: false,
    locale: 'en-US',
    fallbackLocale: 'en-US',
    messages: { 'en-US': en, 'zh-CN': zh },
  });
  type LocalTranslate = (key: string, params?: Record<string, unknown>) => string;
  const t: LocalTranslate = (key, params) =>
    (i18n as unknown as { global: { t: LocalTranslate } }).global.t(key, params ?? {});

  const relative = (msAgo: number) =>
    formatDate(new Date(Date.now() - msAgo).toISOString(), 'en-US', t);

  const DAY = 86_400_000;
  const HOUR = 3_600_000;
  const MINUTE = 60_000;

  it('uses singular English forms for a single unit', () => {
    expect(relative(1 * DAY)).toBe('one day ago');
    expect(relative(1 * HOUR)).toBe('one hour ago');
    expect(relative(1 * MINUTE)).toBe('one minute ago');
  });

  it('uses plural English forms above one', () => {
    expect(relative(3 * DAY)).toBe('3 days ago');
    expect(relative(5 * HOUR)).toBe('5 hours ago');
    expect(relative(10 * MINUTE)).toBe('10 minutes ago');
  });

  it('leaves Chinese unchanged because it has no grammatical number', () => {
    // Chinese has no singular/plural distinction; its key keeps the plain
    // `{count} 天前`. Select the locale on the instance (the `t` third argument
    // is options, not a locale) so the `{count}` interpolation is exercised.
    i18n.global.locale.value = 'zh-CN';
    try {
      expect(formatDate(new Date(Date.now() - DAY).toISOString(), 'zh-CN', t)).toBe('1 天前');
      expect(formatDate(new Date(Date.now() - 3 * DAY).toISOString(), 'zh-CN', t)).toBe('3 天前');
    } finally {
      i18n.global.locale.value = 'en-US';
    }
  });
});
