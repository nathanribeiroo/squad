// A single transient message at the bottom of the stage ("A ↔ B ligados").
import { create } from 'zustand';

interface ToastState {
  text: string | null;
  seq: number;
  show(text: string): void;
}

let timer: number | undefined;

export const useToast = create<ToastState>((set, get) => ({
  text: null,
  seq: 0,
  show: (text) => {
    window.clearTimeout(timer);
    set({ text, seq: get().seq + 1 });
    timer = window.setTimeout(() => set({ text: null }), 2600);
  },
}));

export const toast = (text: string) => useToast.getState().show(text);

export function Toast() {
  const text = useToast((s) => s.text);
  const seq = useToast((s) => s.seq);
  if (!text) return null;
  return (
    <div key={seq} className="toast" role="status">
      {text}
    </div>
  );
}
