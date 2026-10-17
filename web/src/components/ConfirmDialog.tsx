import { useEffect, useRef, useState } from 'react';
import { create } from 'zustand';
import type { IconName } from '../lib/icons';
import { Dialog } from './Dialog';
import { Icon } from './Icon';

/**
 * A confirmation (docs/torre/TELAS.md, quadro 15): the question as the title,
 * what is lost (✕) and what stays (✓), and the verb on the button. The tone
 * colors the border: danger loses work, warn interrupts it, info only
 * restarts. Esc cancels; Enter confirms, except a danger one.
 */
export interface ConfirmOptions {
  title: string;
  icon?: IconName;
  tone?: 'danger' | 'warn' | 'info';
  text?: React.ReactNode;
  lose?: React.ReactNode[];
  keep?: React.ReactNode[];
  /** The button: a verb ("Remover voo"). */
  action: string;
  /** A box to tick before the button works ("Entendo que esse trabalho se perde."). */
  ack?: string;
}

type Pending = ConfirmOptions & { resolve: (ok: boolean) => void };

const useConfirm = create<{ pending: Pending | null }>(() => ({ pending: null }));

/** Asks; resolves true when confirmed. A second question cancels the first. */
export function confirmDialog(o: ConfirmOptions): Promise<boolean> {
  return new Promise((resolve) => {
    useConfirm.getState().pending?.resolve(false);
    useConfirm.setState({ pending: { ...o, resolve } });
  });
}

/** A confirmation is on screen (other Escape handlers leave it the key). */
export function confirmOpen(): boolean {
  return useConfirm.getState().pending !== null;
}

/** Where the confirmations show (once, in App). */
export function ConfirmHost() {
  const p = useConfirm((s) => s.pending);
  if (!p) return null;
  return <Confirm key={p.title + p.action} p={p} />;
}

function Confirm({ p }: { p: Pending }) {
  const [acked, setAcked] = useState(!p.ack);
  const actionRef = useRef<HTMLButtonElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const tone = p.tone ?? 'warn';
  const done = (ok: boolean) => {
    useConfirm.setState({ pending: null });
    p.resolve(ok);
  };
  useEffect(() => {
    (tone === 'danger' ? cancelRef : actionRef).current?.focus();
  }, [tone]);
  return (
    <Dialog
      icon={p.icon ?? (tone === 'danger' ? 'delete' : tone === 'warn' ? 'warning' : 'restart_alt')}
      title={p.title}
      tone={tone}
      onClose={() => done(false)}
      foot={
        <>
          <span className="tn-spacer" />
          <button ref={cancelRef} className="btn" onClick={() => done(false)}>
            Cancelar
          </button>
          <button
            ref={actionRef}
            className={`btn ${tone === 'danger' ? 'danger' : 'filled'}`}
            disabled={!acked}
            onClick={() => done(true)}
            onKeyDown={(e) => tone === 'danger' && e.key === 'Enter' && e.preventDefault()}
          >
            {p.action}
          </button>
        </>
      }
    >
      {p.text && <p className="cf-text">{p.text}</p>}
      {(p.lose?.length || p.keep?.length) && (
        <ul className="cf-list">
          {p.lose?.filter(Boolean).map((l, i) => (
            <li key={`l${i}`} className="cf-lose">
              <Icon name="close" size={15} />
              <span>{l}</span>
            </li>
          ))}
          {p.keep?.filter(Boolean).map((l, i) => (
            <li key={`k${i}`} className="cf-keep">
              <Icon name="check" size={15} />
              <span>{l}</span>
            </li>
          ))}
        </ul>
      )}
      {p.ack && (
        <label className="cf-ack">
          <input type="checkbox" checked={acked} onChange={(e) => setAcked(e.target.checked)} />
          <span>{p.ack}</span>
        </label>
      )}
    </Dialog>
  );
}
