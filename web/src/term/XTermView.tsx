import { useEffect, useRef } from 'react';
import { termSession } from './session';

/**
 * Slot for a terminal inside a world-sized node body. The canvas scales the
 * node by `zoom`; this slot is laid out `zoom` times larger and scaled back by
 * 1/zoom, so xterm sees no transform at all: text is drawn at screen
 * resolution and mouse cells map correctly (xterm does not support scaled
 * ancestors).
 */
export function XTermView({ id, zoom }: { id: string; zoom: number }) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const el = ref.current!;
    const s = termSession(id);
    s.attach(el);
    return () => s.detach(el);
  }, [id]);
  useEffect(() => termSession(id).setZoom(zoom), [id, zoom]);
  return (
    <div
      ref={ref}
      className="xterm-slot"
      style={{
        width: `calc((100% - 20px) * ${zoom})`,
        height: `calc((100% - 16px) * ${zoom})`,
        transform: `scale(${1 / zoom})`,
      }}
    />
  );
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
