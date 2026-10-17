import { useActiveFloor, useBranch } from '../lib/floors';
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
        <Icon name="layers" size={24} />
        <strong>Andar {floor.name}</strong>
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
            <button className="link-btn" onClick={() => open('log')}>
              ver log
            </button>
          </span>
        )}
        {floor.status === 'failed' && (
          <span className="fl-empty-state err">
            <Icon name="error" size={15} />
            O preparo falhou; o andar funciona, mas pode faltar dependência.
            <button className="link-btn" onClick={() => open('log')}>
              ver log
            </button>
          </span>
        )}
        <p>
          Os terminais daqui trabalham nessa cópia, no branch dela. Crie um terminal (<span className="kbd">T</span>), ou peça a um gerente:{' '}
          <span className="mono">torre new "Bia" --role impl --floor "{floor.name}"</span>.
        </p>
        <button className="btn" onClick={() => useSquad.getState().setNewTerm(true)}>
          <Icon name="terminal" />
          Novo terminal neste andar
        </button>
      </div>
    </div>
  );
}
