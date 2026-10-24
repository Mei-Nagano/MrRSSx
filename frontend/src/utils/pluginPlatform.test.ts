import { describe, expect, it } from 'vitest';
import {
  DESKTOP_ONLY_PLUGINS,
  isDesktopOnlyPlugin,
  isPluginVisible,
} from './pluginPlatform';

describe('plugin platform visibility', () => {
  it('keeps note-app integrations off the phone', () => {
    for (const id of ['obsidian', 'notion', 'siyuan', 'zotero']) {
      expect(isDesktopOnlyPlugin(id)).toBe(true);
      expect(isPluginVisible(id, true)).toBe(false);
      expect(isPluginVisible(id, false)).toBe(true);
    }
  });

  it('keeps the RSS-side integrations on the phone', () => {
    for (const id of ['freshrss', 'miniflux', 'folo', 'rsshub']) {
      expect(isDesktopOnlyPlugin(id)).toBe(false);
      expect(isPluginVisible(id, true)).toBe(true);
    }
  });

  it('lists exactly the four note integrations as desktop-only', () => {
    expect([...DESKTOP_ONLY_PLUGINS].sort()).toEqual(['notion', 'obsidian', 'siyuan', 'zotero']);
  });
});
