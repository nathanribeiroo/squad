import type { WNode } from '../api/types';

// Stacks of notes (internal/store/stacks.go): the note at the base holds the
// stack's place and wires; the others are drawn inside it, one at a time.

/** A note stacked on another: not drawn on its own. */
export function hiddenInStack(n: WNode): boolean {
  return !!n.note?.stack && n.note.stack !== n.id;
}

/** The notes of the stack based on base, in order (just the base when it isn't a stack). */
export function stackOf(nodes: WNode[], base: WNode): WNode[] {
  if (base.note?.stack !== base.id) return [base];
  return nodes.filter((n) => n.note?.stack === base.id).sort((a, b) => (a.note?.stackPos ?? 0) - (b.note?.stackPos ?? 0));
}

/** ids plus the notes stacked on the ones that are stack bases. */
export function withStacks(nodes: WNode[], ids: string[]): string[] {
  const out = new Set(ids);
  for (const n of nodes) if (n.note?.stack && ids.includes(n.note.stack)) out.add(n.id);
  return [...out];
}

/**
 * The flights that would see more notes if moving (a note or a stack) went
 * on top of onto: the ones wired to one side and not to the other.
 */
export function stackGains(nodes: WNode[], edges: { a: string; b: string }[], moving: string, onto: string): WNode[] {
  const wired = (id: string) => new Set(edges.flatMap((e) => (e.a === id ? [e.b] : e.b === id ? [e.a] : [])));
  const a = wired(moving);
  const b = wired(onto);
  const ids = [...a].filter((x) => !b.has(x)).concat([...b].filter((x) => !a.has(x)));
  return nodes.filter((n) => n.terminal && ids.includes(n.id));
}
