import {useCallback, useEffect, useRef} from 'react';
import {isHoverCapable} from '@/lib/utils';

/** Grace period before a hover-opened table aside closes itself. */
export const HOVER_PANEL_CLOSE_DELAY_MS = 320;

/** How long a hover-open keeps absorbing the toggle's click. Long enough to
 * cover "moved onto the button and pressed it" as one gesture, short enough
 * that a deliberate later click still closes the panel the way the toggle's
 * own "Fechar…" label promises. */
export const HOVER_PANEL_CLICK_PIN_MS = 300;

/** Hover open/close for the table's docked asides (chat, reactions, winners).
 *
 * A strict `mouseleave` closes them the instant the pointer crosses the
 * toggle's own edge, which is easy to do by accident: the toggle is a 45px
 * circle and the panel it opens is a detached box a gap away, so the natural
 * diagonal from the button to the panel's content leaves the element. The
 * close is therefore deferred, and re-entering anywhere in the aside cancels
 * it. The other half of the fix is CSS: `.table-aside-skirt` in globals.css
 * widens the element's own hit area so the pointer has room to travel. */
export function useHoverPanel(onOpenChangeAction: (open: boolean) => void, enabled = true, isOpen = false) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  // When hover last opened the aside (0 once the pointer left or a click
  // consumed it). Without this the toggle's own `!open` click fights the
  // hover that the very same pointer movement just performed: whether the click
  // handler still sees `open === false` (so it opens) or already sees
  // `open === true` (so it closes the panel hover had just opened) depends on
  // whether the browser committed the mouseenter render before dispatching the
  // click. Chromium did, Firefox did not — clicking the reactions toggle in
  // Firefox opened and immediately re-closed the panel, so a click-first player
  // could never reach the reaction catalog at all. The first click after a
  // hover-open therefore pins the panel instead of toggling it.
  const hoverOpenedAt = useRef(0);
  const cancel = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  }, []);
  useEffect(() => cancel, [cancel]);

  return {
    onMouseEnter: () => {
      if (!enabled || !isHoverCapable()) return;
      cancel();
      // Only a hover that actually opens the aside can absorb a click. A
      // pointer re-entering an already-open aside (the panel's own layout
      // shift moves the box under a stationary cursor) must not, or the
      // toggle would stop closing anything.
      if (!isOpen) hoverOpenedAt.current = Date.now();
      onOpenChangeAction(true);
    },
    onMouseLeave: () => {
      if (!enabled || !isHoverCapable()) return;
      cancel();
      hoverOpenedAt.current = 0;
      timer.current = setTimeout(() => onOpenChangeAction(false), HOVER_PANEL_CLOSE_DELAY_MS);
    },
    /** The toggle button's click, made independent of hover/click ordering.
     * `open` is the render's current state; pass it as the component reads it. */
    toggleFromClick: (open: boolean) => {
      if (Date.now() - hoverOpenedAt.current < HOVER_PANEL_CLICK_PIN_MS) {
        hoverOpenedAt.current = 0;
        onOpenChangeAction(true);
        return;
      }
      onOpenChangeAction(!open);
    }
  };
}
