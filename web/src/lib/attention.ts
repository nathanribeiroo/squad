// The PRIORIDADE state (docs/torre/NOVIDADES-E-BACKEND.md, item 1): what a
// flight asks the user, in words, and the answers the RESPONDER bar sends.
import { api } from '../api/client';
import type { Attention, AttentionChoice } from '../api/types';
import { useSquad } from '../state/store';
import { toast } from './toast';

export { askPhrase } from './askText';

export const CHOICES: { choice: AttentionChoice; label: string; key: string }[] = [
  { choice: 'allow', label: 'Autorizar', key: '1' },
  { choice: 'always', label: 'Sempre', key: '2' },
  { choice: 'deny', label: 'Negar', key: '3' },
];

/** The agent's own words for an answer, for its button ("Sempre: Yes, always allow `npm test` commands in `app`"). */
export function choiceTitle(a: Attention | undefined, c: { choice: AttentionChoice; label: string }): string | undefined {
  const words = a?.labels?.[c.choice];
  return words ? `${c.label}: ${words}` : undefined;
}

/** Sends an answer; a refusal (the prompt changed…) becomes a toast. */
export function respond(id: string, choice: AttentionChoice) {
  api.respond(id, choice).catch((e: Error) => toast(e.message, 'err'));
}

/** Picks one of the agent's options (with the user's text on its text field); false when refused (a toast says why). */
export function pickOption(id: string, option: string, text?: string): Promise<boolean> {
  return api
    .respondOption(id, option, text)
    .then(() => true)
    .catch((e: Error) => {
      toast(e.message, 'err');
      return false;
    });
}

/** Answers the flight's prompt with key 1–9: on a question or a plan, the agent's option of that number; on a permission, 1 allow, 2 always, 3 deny. */
export function answerKey(id: string, a: Attention | undefined, digit: string): boolean {
  if (!a) return false;
  if (a.options?.length) {
    const o = a.options.find((x) => x.key === digit && !x.input);
    if (o) void pickOption(id, o.key);
    return !!o;
  }
  const c = CHOICES.find((x) => x.key === digit);
  if (!c || !a.choices?.includes(c.choice)) return false;
  respond(id, c.choice);
  return true;
}

/** Goes to the oldest flight asking the user: its level, centered, with the keyboard (⌥⏎). */
export function goToNextAsk(): boolean {
  const st = useSquad.getState();
  const id = st.askingIds[0];
  const n = id && st.workspace?.nodes.find((x) => x.id === id);
  if (!n) return false;
  st.setFloor(n.floor ?? '', n.id);
  st.setFocusedTerminal(n.id);
  return true;
}
