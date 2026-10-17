import { flightLevel, useActiveFloor, useBranch } from '../lib/floors';
import { shortPath } from '../lib/paths';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './FloorDialogs.css';

/**
 * A floor with nothing on it yet: what it is, its setup's state and how to
 * put agents on it.
 */
export function FloorEmpty() {
  const floor = useActiveFloor();
  const empty = useSquad((s) => !!floor && !s.workspace?.nodes.some((n) => n.floor === floor.id));
  const setup = useSquad((s) => s.workspace?.floorConfig?.setup);
  const branch = useBranch(floor?.id ?? '');
  if (!floor || !empty) return null;
  const open = (kind: 'log') => useSquad.getState().setFloorDialog({ kind, id: floor.id });
  return (
    <div className="fl-empty">
      <div className="fl-empty-card">
        <span className="label-caps">Nível vazio</span>
        <strong>
          {flightLevel(floor)} · {floor.name}
        </strong>
        <span className="mono fl-empty-path">
          ⎇ {branch || floor.branch} · {shortPath(floor.cwd)}
        </span>
        {floor.status === 'preparing' && (
          <span className="fl-empty-state">
            <Icon name="progress_activity" size={15} className="spin" />
            <span>
              Preparando{setup ? ': ' : '…'}
              {setup && <span className="mono">{setup}</span>}
            </span>
            <span style={{ flex: 1 }} />
            <button className="link-btn" onClick={() => open('log')}>
              ver log
            </button>
          </span>
        )}
        {floor.status === 'failed' && (
          <span className="fl-empty-state err">
            <Icon name="error" size={15} />
            <span>O preparo falhou; o nível funciona, mas pode faltar dependência.</span>
            <span style={{ flex: 1 }} />
            <button className="link-btn" onClick={() => open('log')}>
              ver log
            </button>
          </span>
        )}
        <p>
          Ponha voos aqui com <span className="kbd">T</span>, ou peça a um líder:
        </p>
        <code className="fl-empty-cmd mono">torre new "Bia" --role impl --floor "{floor.name}"</code>
        <button className="btn filled fl-empty-btn" onClick={() => useSquad.getState().setNewTerm(true)}>
          <Icon name="flight_takeoff" />
          Novo voo neste nível
        </button>
      </div>
    </div>
  );
}
