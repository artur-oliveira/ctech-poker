import {useEffect} from 'react';

/** Closes an open panel on Escape or a pointerdown outside `ref`. No-ops while
 * `open` is false, so it never fights the toggle button's own click. */
export function useDismiss(ref: React.RefObject<HTMLElement | null>, open: boolean, onDismiss: () => void) {
  useEffect(() => {
    if (!open) return () => {
    };
    function onKeyDown(event: KeyboardEvent) {
      if (event.key !== 'Escape') return;
      onDismiss();
      // Escape is keyboard-only dismissal, so keyboard focus must land
      // somewhere; the outside-click path leaves it wherever the user clicked.
      ref.current?.querySelector<HTMLElement>('[aria-expanded]')?.focus();
    }
    function onPointerDown(event: PointerEvent) {
      if (ref.current && !ref.current.contains(event.target as Node)) onDismiss();
    }
    document.addEventListener('keydown', onKeyDown);
    document.addEventListener('pointerdown', onPointerDown);
    return () => {
      document.removeEventListener('keydown', onKeyDown);
      document.removeEventListener('pointerdown', onPointerDown);
    };
  }, [ref, open, onDismiss]);
}
