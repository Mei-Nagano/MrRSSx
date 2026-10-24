import { describe, expect, it } from 'vitest';
import gallerySource from './index.vue?raw';
import viewerSource from './components/ImageViewerModal.vue?raw';

/**
 * The gallery viewer is a layer above the gallery. Its back handling lives in the
 * parent because that is what owns `showImageViewer`, and the viewer component has
 * no handler of its own — so removing this registration silently drops the user out
 * of the gallery on the first back press.
 */
describe('gallery back handling', () => {
  it('registers a back handler that closes the viewer first', () => {
    expect(gallerySource).toMatch(/useAndroidBack\(/);
    expect(gallerySource).toMatch(/BACK_PRIORITY\.viewer/);
    expect(gallerySource).toMatch(/if \(!showImageViewer\.value\) return false;/);
    expect(gallerySource).toMatch(/closeImageViewer\(\);/);
  });

  it('keeps the handler above the article and drawer layers', () => {
    // A lower priority would let the app reset the gallery scope instead of closing
    // the viewer, which is the failure the acceptance run reported.
    const priority = gallerySource.match(/BACK_PRIORITY\.viewer/);
    expect(priority).not.toBeNull();
    expect(gallerySource).not.toMatch(/BACK_PRIORITY\.(view|drawer|articleDetail)\)/);
  });
});

describe('viewer keyboard handling', () => {
  it('closes on Escape and removes the listener on unmount', () => {
    expect(viewerSource).toMatch(/if \(event\.key !== 'Escape'\) return;/);
    expect(viewerSource).toMatch(/emit\('close'\)/);
    expect(viewerSource).toMatch(
      /onUnmounted\(\(\) => window\.removeEventListener\('keydown', handleEscape\)\)/
    );
  });
});