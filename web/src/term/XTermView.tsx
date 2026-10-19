import { useEffect, useRef } from 'react';
import { termSession } from './session';

/**
 * Slot for a terminal inside a world-sized node body. The canvas scales the
 * node by `zoom`; the session sizes and scales the slot for the zoom the
 * terminal is laid out for (see TermSession.setZoom).
 */
export function XTermView({ id, zoom }: { id: string; zoom: number }) {
  const ref = useRef<HTMLDivElement>(null);
  const zoomRef = useRef(zoom);
  zoomRef.current = zoom;
  useEffect(() => {
    const el = ref.current!;
    const s = termSession(id);
    s.attach(el, zoomRef.current);
    return () => s.detach(el);
  }, [id]);
  useEffect(() => termSession(id).setZoom(zoom), [id, zoom]);
  return <div ref={ref} className="xterm-slot" />;
}

/**
 * Lays its children out in screen pixels over a world-sized node, optionally
 * shrunk by `fit` (< 1) when the node is smaller on screen than the design.
 */
export function ScreenSpace({ zoom, fit = 1, className = '', children }: { zoom: number; fit?: number; className?: string; children: React.ReactNode }) {
  const k = zoom / fit;
  return (
    <div className={`screen-space ${className}`} style={{ width: `${k * 100}%`, height: `${k * 100}%`, transform: `scale(${1 / k})` }}>
      {children}
    </div>
  );
}
