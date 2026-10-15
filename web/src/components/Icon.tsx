import type { CSSProperties } from 'react';
import type { IconName } from '../lib/icons';

/** Material Symbols Outlined glyph. */
export function Icon({ name, size = 20, fill = false, style, className = '' }: {
  name: IconName;
  size?: number;
  fill?: boolean;
  style?: CSSProperties;
  className?: string;
}) {
  return (
    <span className={`msi${fill ? ' fill' : ''} ${className}`} style={{ fontSize: size, ...style }} aria-hidden="true">
      {name}
    </span>
  );
}
