import type { LoopStop, Pause } from '../api/types';

export type Loop = Pause & { chain: LoopStop[] };

/** The loop holding conversations; null when nothing or the user holds them. */
export function loopOf(pause?: Pause | null): Loop | null {
  return pause?.by === 'loop' && pause.chain?.length ? (pause as Loop) : null;
}

/** Whether the wire between a and b is a hop of the loop (either way). */
export function onLoop(loop: Loop | null, a: string, b: string): boolean {
  return !!loop?.chain.some((s, i) => {
    const next = loop.chain[i + 1];
    return !!next && ((s.id === a && next.id === b) || (s.id === b && next.id === a));
  });
}
