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
}: {
  icon: IconName;
  title: string;
  sub?: React.ReactNode;
  onClose: () => void;
  children: React.ReactNode;
  foot?: React.ReactNode;
  wide?: boolean;
}) {
  useEffect(() => {
    const esc = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation();
        onClose();
      }
    };
    window.addEventListener('keydown', esc, true);
    return () => window.removeEventListener('keydown', esc, true);
  }, [onClose]);
  return (
    <div className="dlg-backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div className={`dlg${wide ? ' wide' : ''}`} role="dialog" aria-label={title}>
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
