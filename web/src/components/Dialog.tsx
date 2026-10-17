import { useEffect } from 'react';
import type { IconName } from '../lib/icons';
import { Icon } from './Icon';
import './Dialog.css';

/** A modal dialog: icon, title, subtitle, body and an optional footer;
 * Escape or a click outside closes it. */
export function Dialog({
  icon,
  title,
  sub,
  onClose,
  children,
  foot,
  wide,
  tone,
}: {
  icon: IconName;
  title: string;
  sub?: React.ReactNode;
  onClose: () => void;
  children: React.ReactNode;
  foot?: React.ReactNode;
  wide?: boolean;
  /** The border: danger (loses work), warn (interrupts it), info. */
  tone?: 'danger' | 'warn' | 'info';
}) {
  useEffect(() => {
    const esc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopImmediatePropagation(); // the dialog under it, or a panel, keeps open
        onClose();
      }
    };
    window.addEventListener('keydown', esc, true);
    return () => window.removeEventListener('keydown', esc, true);
  }, [onClose]);
  return (
    <div className={`dlg-backdrop${tone ? ' top' : ''}`} onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={`dlg${wide ? ' wide' : ''}${tone ? ` tone-${tone}` : ''}`} role="dialog" aria-modal="true" aria-label={title}>
        <header className="dlg-head">
          <Icon name={icon} size={20} />
          <div>
            <strong>{title}</strong>
            {sub && <span>{sub}</span>}
          </div>
          <span className="tn-spacer" />
          <button className="icon-btn" onClick={onClose} aria-label="Fechar">
            <Icon name="close" size={18} />
          </button>
        </header>
        <div className="dlg-body">{children}</div>
        {foot && <footer className="dlg-foot">{foot}</footer>}
      </div>
    </div>
  );
}
