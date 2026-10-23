import { describe, expect, it } from 'vitest';
import { resolveLocale } from './locale';

describe('resolveLocale', () => {
  it('keeps the values the settings dropdown writes', () => {
    expect(resolveLocale('en-US')).toBe('en-US');
    expect(resolveLocale('zh-CN')).toBe('zh-CN');
  });

  it('maps older or hand-written tags onto a shipped locale', () => {
    expect(resolveLocale('zh')).toBe('zh-CN');
    expect(resolveLocale('zh-Hans')).toBe('zh-CN');
    expect(resolveLocale('en')).toBe('en-US');
    expect(resolveLocale('en-GB')).toBe('en-US');
  });

  it('falls back to English when nothing is stored', () => {
    expect(resolveLocale('')).toBe('en-US');
    expect(resolveLocale(undefined)).toBe('en-US');
    expect(resolveLocale(null)).toBe('en-US');
  });
});
