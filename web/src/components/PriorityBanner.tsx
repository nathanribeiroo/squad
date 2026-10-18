import { useStore } from '@xyflow/react';
import type { WNode } from '../api/types';
import { askPhrase, goToNextAsk } from '../lib/attention';
import { flightLevel } from '../lib/floors';
import { callsign } from '../lib/model';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './PriorityBanner.css';

/**
 * PRIORIDADE out of sight (docs/torre/NOVIDADES-E-BACKEND.md, item 1): the
 * oldest flight asking you that isn't on screen (another level, or outside
 * the view), with IR ATÉ ELE (⌥⏎).
 */
export function PriorityBanner() {
  const ids = useSquad((s) => s.askingIds);
  const nodes = useSquad((s) => s.workspace?.nodes);
  const floors = useSquad((s) => s.workspace?.floors);
  const activeFloor = useSquad((s) => s.activeFloor);
  const runtime = useSquad((s) => s.runtime);
  const [tx, ty, zoom] = useStore((s) => s.transform);
  const width = useStore((s) => s.width);
  const height = useStore((s) => s.height);

  const onScreen = (n: WNode) => {
    if ((n.floor ?? '') !== activeFloor) return false;
    const cx = (n.x + n.w / 2) * zoom + tx;
    const cy = (n.y + n.h / 2) * zoom + ty;
    return cx > 0 && cx < width && cy > 0 && cy < height;
  };
  const asking = ids.map((id) => nodes?.find((n) => n.id === id)).filter((n): n is WNode => !!n?.terminal);
  const off = asking.find((n) => !onScreen(n));
  if (!off) return null;
  const phrase = askPhrase(runtime[off.id]?.attention);
  const level = off.floor ? ` em ${flightLevel(floors?.find((f) => f.id === off.floor))}` : '';
  return (
    <div className="pb" role="alert">
      <Icon name="front_hand" size={17} />
      <b className="pb-tag">Prioridade</b>
      <span className="pb-text">
        <b className="mono">{callsign(off.terminal!)}</b>
        {level} {phrase.charAt(0).toLowerCase() + phrase.slice(1)}
      </span>
      {asking.length > 1 && <span className="pb-count mono">1 de {asking.length}</span>}
      <span style={{ flex: 1 }} />
      <button className="pb-go" onClick={() => goToNextAsk()}>
        Ir até ele
        <span className="kbd">⌥⏎</span>
      </button>
    </div>
  );
}
