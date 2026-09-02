import {useCallback, useEffect, useRef} from 'react';
import {isHoverCapable} from '@/lib/utils';

/** Grace period before a hover-opened table aside closes itself. */
export const HOVER_PANEL_CLOSE_DELAY_MS = 320;

/** Hover open/close for the table's docked asides (chat, reactions, winners).
 *
 * A strict `mouseleave` closes them the instant the pointer crosses the
 * toggle's own edge, which is easy to do by accident: the toggle is a 45px
 * circle and the panel it opens is a detached box a gap away, so the natural
 * diagonal from the button to the panel's content leaves the element. The
 * close is therefore deferred, and re-entering anywhere in the aside cancels
 * it. The other half of the fix is CSS: `.table-aside-skirt` in globals.css
 * widens the element's own hit area so the pointer has room to travel. */
export function useHoverPanel(onOpenChangeAction: (open: boolean) => void, enabled = true) {
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const cancel = useCallback(() => {
    if (timer.current) clearTimeout(timer.current);
    timer.current = null;
  }, []);
  useEffect(() => cancel, [cancel]);

  return {
    onMouseEnter: () => {
      if (!enabled || !isHoverCapable()) return;
      cancel();
      onOpenChangeAction(true);
    },
    onMouseLeave: () => {
      if (!enabled || !isHoverCapable()) return;
      cancel();
      timer.current = setTimeout(() => onOpenChangeAction(false), HOVER_PANEL_CLOSE_DELAY_MS);
    }
  };
}
