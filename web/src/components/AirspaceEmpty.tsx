import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './AirspaceEmpty.css';

/**
 * The runway with nothing on it (docs/torre/TELAS.md, quadro 20): how to put
 * the first team in the air. Flight plans come later (F5d).
 */
export function AirspaceEmpty({ onNote }: { onNote: () => void }) {
  const empty = useSquad((s) => !!s.workspace && !s.activeFloor && !s.workspace.nodes.some((n) => !n.floor));
  const name = useSquad((s) => s.workspace?.name ?? '');
  if (!empty) return null;
  return (
    <div className="ae">
      <div className="ae-card">
        <span className="label-caps">Espaço aéreo vazio · {name}</span>
        <h1>Ponha a primeira equipe no ar.</h1>
        <div className="ae-actions">
          <button className="ae-btn" disabled title="Os planos de voo chegam numa próxima versão">
            <Icon name="flight_class" size={20} />
            <span>Usar um plano de voo</span>
            <span className="kbd">⌥M</span>
          </button>
          <button className="ae-btn primary" onClick={() => useSquad.getState().setNewTerm(true)}>
            <Icon name="flight_takeoff" size={20} />
            <span>Novo voo</span>
            <span className="kbd">T</span>
          </button>
          <button className="ae-btn" onClick={onNote}>
            <Icon name="edit_note" size={20} />
            <span>Escrever o plano numa NOTAM</span>
            <span className="kbd">N</span>
          </button>
        </div>
        <p>
          Voos na mesma frequência conversam: arraste da bolinha de um voo até outro, e eles pedem e respondem com <span className="mono">torre ask</span>.
          Um voo líder (o Planejador) monta a equipe sozinho.
        </p>
      </div>
    </div>
  );
}
