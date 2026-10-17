import { useState } from 'react';
import { api } from '../api/client';
import { callsign } from '../lib/model';
import { toast } from '../lib/toast';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './InterruptedBar.css';

/**
 * Agents that were in the middle of a turn when their terminal stopped
 * (Torre closed or restarted): they're back in their conversations, waiting.
 * Continuar tells them to go on; Dispensar forgets it.
 */
export function InterruptedBar() {
  const names = useSquad((s) =>
    (s.workspace?.nodes ?? [])
      .filter((n) => n.terminal && s.runtime[n.id]?.interrupted)
      .map((n) => callsign(n.terminal!))
      .join('\u0000'),
  );
  const [busy, setBusy] = useState(false);
  if (!names) return null;
  const list = names.split('\u0000');
  const who = list.length === 1 ? `${list[0]} foi interrompido` : `${joinNames(list)} foram interrompidos`;
  const run = (action: 'continue' | 'dismiss') => {
    setBusy(true);
    api
      .interrupted(action)
      .catch((e: Error) => toast(e.message, 'err'))
      .finally(() => setBusy(false));
  };
  return (
    <div className="ib" role="status">
      <Icon name="u_turn_left" size={16} />
      <span className="ib-text">
        <strong>Arremetida</strong> · {who} no meio do turno. A conversa voltou; falta só continuar.
      </span>
      <button className="btn filled ib-btn" disabled={busy} onClick={() => run('continue')} title='Manda "Continue de onde parou" a cada um'>
        <Icon name="play_arrow" size={16} />
        Continuar
      </button>
      <button className="btn ib-btn" disabled={busy} onClick={() => run('dismiss')}>
        Dispensar
      </button>
    </div>
  );
}

function joinNames(names: string[]): string {
  return names.length < 2 ? names.join('') : `${names.slice(0, -1).join(', ')} e ${names[names.length - 1]}`;
}
