export interface ToolbarMenuAnchor {
  top: number;
  bottom: number;
  right: number;
}

export interface ToolbarMenuViewport {
  width: number;
  height: number;
  top?: number;
  bottom?: number;
}

export interface ToolbarMenuPosition {
  top: number;
  right: number;
  maxHeight: number;
  placement: 'above' | 'below';
}

const MENU_GAP = 6;
const VIEWPORT_MARGIN = 8;
const MAX_HEIGHT_RATIO = 0.6;

/**
 * Keep the toolbar menu inside the usable viewport while retaining its own
 * scroll container when all actions cannot fit.
 */
export function getToolbarMenuPosition(
  anchor: ToolbarMenuAnchor,
  viewport: ToolbarMenuViewport,
  contentHeight = 0
): ToolbarMenuPosition {
  const viewportTop = Math.max(0, viewport.top ?? 0);
  const viewportBottom = Math.max(
    viewportTop,
    Math.min(viewport.height, viewport.bottom ?? viewport.height)
  );
  const belowTop = anchor.bottom + MENU_GAP;
  const aboveBottom = anchor.top - MENU_GAP;
  const belowSpace = Math.max(0, viewportBottom - VIEWPORT_MARGIN - belowTop);
  const aboveSpace = Math.max(0, aboveBottom - viewportTop - VIEWPORT_MARGIN);
  const viewportLimit = Math.max(1, Math.floor(viewport.height * MAX_HEIGHT_RATIO));
  const measuredHeight =
    Number.isFinite(contentHeight) && contentHeight > 0 ? contentHeight : viewportLimit;
  const desiredHeight = Math.max(1, Math.min(viewportLimit, Math.ceil(measuredHeight)));
  const placeAbove = belowSpace < desiredHeight && aboveSpace > belowSpace;
  const availableSpace = placeAbove ? aboveSpace : belowSpace;
  const maxHeight = Math.max(1, Math.floor(Math.min(desiredHeight, availableSpace)));

  return {
    top: placeAbove ? Math.max(viewportTop + VIEWPORT_MARGIN, aboveBottom - maxHeight) : belowTop,
    right: Math.max(VIEWPORT_MARGIN, viewport.width - anchor.right),
    maxHeight,
    placement: placeAbove ? 'above' : 'below',
  };
}
