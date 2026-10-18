import { useEffect, useRef, useState } from 'react';
import { Icon } from '../../components/Icon';
import type { IconName } from '../../lib/icons';

export interface MenuItem {
  icon: IconName;
  label: string;
  action: () => void;
  danger?: boolean;
  disabled?: boolean;
  checked?: boolean;
  /** Said after the label, smaller ("vai ao hangar"). */
  hint?: string;
}

/** A row of color swatches (a group's color). */
export interface MenuColors {
  colors: readonly string[];
  value?: string;
  /** The CSS color of a key. */
  css: (key: string) => string;
  pick: (key: string) => void;
}

/** A divider, or a label over the items that follow ("Papel"). */
export type MenuEntry = MenuItem | MenuColors | 'divider' | { section: string };

/** The `more_horiz` menu in node headers. */
export function NodeMenu({ items }: { items: MenuEntry[] }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    window.addEventListener('pointerdown', close, true);
    return () => window.removeEventListener('pointerdown', close, true);
  }, [open]);
  return (
    <div className="node-menu nodrag" ref={ref}>
      <button className="icon-btn node-menu-btn" onClick={() => setOpen(!open)} aria-label="Mais ações" aria-expanded={open}>
        <Icon name="more_horiz" size={18} />
      </button>
      {open && (
        <div className="popover node-menu-pop" role="menu">
          {items.map((it, i) =>
            it === 'divider' ? (
              <span key={i} className="node-menu-div" />
            ) : 'section' in it ? (
              <span key={i} className="node-menu-section">
                {it.section}
              </span>
            ) : 'colors' in it ? (
              <span key={i} className="node-menu-colors">
                {it.colors.map((c) => (
                  <button
                    key={c}
                    className={`node-menu-swatch${c === (it.value || 'none') ? ' on' : ''}`}
                    style={{ background: it.css(c) }}
                    aria-label={`Cor ${c}`}
                    onClick={() => {
                      setOpen(false);
                      it.pick(c);
                    }}
                  />
                ))}
              </span>
            ) : (
              <button
                key={it.label}
                role="menuitem"
                className={`node-menu-item${it.danger ? ' danger' : ''}`}
                disabled={it.disabled}
                onClick={() => {
                  setOpen(false);
                  it.action();
                }}
              >
                <Icon name={it.icon} size={17} />
                <span>{it.label}</span>
                {it.hint && <span className="node-menu-hint mono">{it.hint}</span>}
                {it.checked && <Icon name="check" size={16} style={{ marginLeft: 'auto' }} />}
              </button>
            ),
          )}
        </div>
      )}
    </div>
  );
}
