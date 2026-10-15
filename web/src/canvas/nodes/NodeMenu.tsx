import { useEffect, useRef, useState } from 'react';
import { Icon } from '../../components/Icon';

export interface MenuItem {
  icon: string;
  label: string;
  action: () => void;
  danger?: boolean;
  disabled?: boolean;
  checked?: boolean;
}

/** The `more_horiz` menu in node headers. */
export function NodeMenu({ items }: { items: (MenuItem | 'divider')[] }) {
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
                {it.checked && <Icon name="check" size={16} style={{ marginLeft: 'auto' }} />}
              </button>
            ),
          )}
        </div>
      )}
    </div>
  );
}
