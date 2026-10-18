// The Rádio's lines come from the server (internal/radio) as data; they
// are worded here, in the interface's language.
import type { Attention, RadioItem } from '../api/types';
import type { ActivityItem } from '../state/store';
import { askPhrase } from './askText';

/** "hop limit 6 reached PLN → IMP → PLN" → the Rádio's sentence. */
function loopText(reason = ''): string {
  const m = /^hop limit (\d+) reached (.*)$/.exec(reason);
  return m ? `Laço: limite de ${m[1]} saltos atingido ${m[2]}. Espera geral até você liberar.` : `Laço: ${reason}`;
}

/** A line of the Rádio, as the drawer shows it. */
export function toActivity(r: RadioItem): ActivityItem {
  const a: ActivityItem = {
    key: r.key,
    time: r.time,
    id: r.id,
    from: r.from,
    to: r.to,
    fromName: r.fromName ?? '',
    toName: r.toName ?? '',
    text: r.text ?? '',
    outcome: r.outcome,
    elapsedMs: r.elapsedMs,
    source: r.source,
    hold: r.hold,
    choice: r.choice,
    hasReply: r.hasReply,
  };
  switch (r.outcome) {
    case 'held':
      if (r.hold === 'loop') return { ...a, fromName: 'Torre', toName: 'todos', text: loopText(r.text) };
      return { ...a, text: r.hold === 'paused' ? 'Retido: espera geral' : 'Retido: limite de pedidos por minuto' };
    case 'delivered':
      return { ...a, fromName: 'Você', text: a.text + (r.images ? ` (+${r.images} imagem${r.images > 1 ? 'ns' : ''})` : '') };
    case 'note':
      return { ...a, fromName: r.by === 'user' ? 'Você' : r.by === 'external' ? 'Edição externa' : a.fromName, toName: a.toName || a.text };
    case 'answered': {
      const ask: Attention = { kind: r.kind ?? 'permission', tool: r.tool, source: 'hook', since: r.time };
      return { ...a, toName: 'Você', text: [askPhrase(ask), r.detail || (r.kind === 'question' ? r.text : '')].filter(Boolean).join(': ') };
    }
  }
  return a;
}

/** Puts lines in the Rádio, a line with the same key replaced: newest first. */
export function mergeActivity(cur: ActivityItem[], add: ActivityItem[]): ActivityItem[] {
  const byKey = new Map(cur.map((a) => [a.key, a]));
  for (const a of add) byKey.set(a.key, a);
  return [...byKey.values()].sort((x, y) => Date.parse(y.time) - Date.parse(x.time));
}
