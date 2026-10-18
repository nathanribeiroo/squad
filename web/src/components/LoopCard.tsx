import { Fragment, useState } from 'react';
import { api } from '../api/client';
import { loopOf } from '../lib/loop';
import { callsign } from '../lib/model';
import { toast } from '../lib/toast';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './LoopCard.css';

/**
 * ESPERA GERAL · LAÇO (docs/torre/TELAS.md, quadro 21): a chain of requests
 * went past the hop limit, so the server holds every conversation. The
 * chain (a flight's callsign goes to it: centered in the Espaço, shown in
 * the Sinóptico), LIBERAR TODOS and the limits. The user's own Espera geral
 * has no card.
 */
export function LoopCard({ inSynoptic = false }: { inSynoptic?: boolean }) {
  const loop = useSquad((s) => loopOf(s.extra.pause));
  const nodes = useSquad((s) => s.workspace?.nodes);
  const [busy, setBusy] = useState(false);
  if (!loop) return null;

  const node = (id: string) => nodes?.find((n) => n.id === id && n.terminal);
  const open = (id: string, space: boolean) => {
    const n = node(id);
    if (!n) return;
    const st = useSquad.getState();
    if (!space) return st.setSynPick(id);
    st.setView('space');
    st.setFloor(n.floor ?? '', id);
  };
  const release = () => {
    setBusy(true);
    api
      .resume()
      .catch((e: Error) => toast(e.message, 'err'))
      .finally(() => setBusy(false));
  };
  return (
    <div className={`lc${inSynoptic ? ' in-syn' : ''}`} role="alert">
      <div className="lc-head">
        <Icon name="all_inclusive" size={18} />
        <b>Espera geral · laço</b>
      </div>
      <p className="lc-text">
        {loop.chain.map((s, i) => {
          const n = node(s.id);
          const label = n ? callsign(n.terminal!) : s.callsign || s.name;
          const name = n?.terminal?.name ?? s.name;
          return (
            <Fragment key={i}>
              {i > 0 && <span className="lc-arrow"> → </span>}
              <button
                className="lc-cs mono"
                disabled={!n}
                title={n ? (inSynoptic ? `${name} · duplo clique abre no Espaço` : `${name} · centralizar`) : `${name} (fechado)`}
                onClick={() => open(s.id, !inSynoptic)}
                onDoubleClick={() => inSynoptic && open(s.id, true)}
              >
                {label}
              </button>
            </Fragment>
          );
        })}{' '}
        passou de {loop.limit} saltos. Tudo em espera até você liberar.
      </p>
      <div className="lc-actions">
        <button className="lc-release" disabled={busy} onClick={release} title="Os pedidos retidos seguem; o limite continua o mesmo">
          Liberar todos
          <span className="kbd">⌥P</span>
        </button>
        <button className="btn lc-limits" onClick={() => useSquad.getState().openSettings('limits')} title="Configurações → Limites">
          Limites…
        </button>
      </div>
    </div>
  );
}
