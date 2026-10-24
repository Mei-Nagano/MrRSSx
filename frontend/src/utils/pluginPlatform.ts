/**
 * Which plugin integrations belong on a phone.
 *
 * The settings page lists every integration in one place, but four of them are
 * note/knowledge tools rather than feed tools: Obsidian writes into a local vault
 * path, SiYuan talks to a local API, and Notion/Zotero are note destinations. None
 * of that has a meaning on Android, where the app has no desktop filesystem and no
 * local service to reach. They stay on desktop; the RSS-side integrations (reader
 * sync, Folo, RSSHub) are the ones that work with the feeds the phone is showing.
 */
export const DESKTOP_ONLY_PLUGINS = ['obsidian', 'notion', 'siyuan', 'zotero'] as const;

export type DesktopOnlyPlugin = (typeof DESKTOP_ONLY_PLUGINS)[number];

const DESKTOP_ONLY = new Set<string>(DESKTOP_ONLY_PLUGINS);

/** True when the integration only makes sense in the desktop app. */
export function isDesktopOnlyPlugin(id: string): boolean {
  return DESKTOP_ONLY.has(id);
}

/** True when the settings page should render this integration at all. */
export function isPluginVisible(id: string, isPhone: boolean): boolean {
  return !isPhone || !isDesktopOnlyPlugin(id);
}
