import { useEffect, useRef } from 'react';

/**
 * Closes a popover on a click outside it (or outside the button that opens
 * it, given as a selector, which toggles it itself) and on Escape.
 */
export function usePopoverClose<T extends HTMLElement = HTMLDivElement>(onClose: () => void, opener?: string) {
  const ref = useRef<T>(null);
  useEffect(() => {
    const away = (e: PointerEvent) => {
      const t = e.target as Element;
      if (ref.current?.contains(t)) return;
      if (opener && t.closest?.(opener)) return;
      onClose();
    };
    const esc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('pointerdown', away, true);
    window.addEventListener('keydown', esc);
    return () => {
      window.removeEventListener('pointerdown', away, true);
      window.removeEventListener('keydown', esc);
    };
  }, [onClose, opener]);
  return ref;
}
