// A single transient notice at the bottom center ("PLN ↔ IMP na mesma
// frequência"): a confirmation, a wait (amber) or an error (red), with an
// icon (docs/torre/TELAS.md, quadro 21).
import { create } from 'zustand';
import { Icon } from '../components/Icon';

export type ToastKind = 'ok' | 'wait' | 'err';

interface ToastState {
  text: string | null;
  kind: ToastKind;
  seq: number;
  show(text: string, kind?: ToastKind): void;
}

let timer: number | undefined;

export const useToast = create<ToastState>((set, get) => ({
  text: null,
  kind: 'ok',
  seq: 0,
  show: (text, kind = 'ok') => {
    window.clearTimeout(timer);
    set({ text, kind, seq: get().seq + 1 });
    timer = window.setTimeout(() => set({ text: null }), 4000);
  },
}));

export const toast = (text: string, kind?: ToastKind) => useToast.getState().show(text, kind);

/** An error from the server or the network, in red. */
export const toastError = (e: unknown) => toast(e instanceof Error ? e.message : String(e), 'err');

const ICON = { ok: 'check', wait: 'hourglass_top', err: 'error' } as const;

export function Toast() {
  const text = useToast((s) => s.text);
  const kind = useToast((s) => s.kind);
  const seq = useToast((s) => s.seq);
  if (!text) return null;
  return (
    <div key={seq} className={`toast ${kind}`} role="status">
      <Icon name={ICON[kind]} size={16} />
      <span>{text}</span>
    </div>
  );
}
