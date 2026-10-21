import { useCargoDrop } from '../lib/cargoDrop';
import { callsign } from '../lib/model';
import { useNodeRoleColor } from '../lib/roles';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './ComposerPill.css';

/** "TRANSMITIR → ■ IMP · escreva para o voo… ⌥K ↑": the Transmitir panel, closed. */
export function ComposerPill() {
  // Only flights receive messages; a selected NOTAM or portal is not a target.
  const target = useSquad((s) => {
    const isTerm = (id: string | null) => !!id && !!s.workspace?.nodes.find((n) => n.id === id && n.terminal);
    return [s.focusedTerminal, s.selectedNode, s.composerTarget].find(isTerm) ?? null;
  });
  const t = useSquad((s) => (target ? s.workspace?.nodes.find((n) => n.id === target)?.terminal : undefined));
  const roleOf = useNodeRoleColor();
  const open = useSquad((s) => s.setComposer);
  const isOpen = useSquad((s) => s.composerOpen);
  // A cargo's files dropped here open Transmitir with them, to the flight
  // picked (clicking the cargo unpicks it) or the level's only flight.
  const ready = useSquad((s) => s.cargoDrag);
  const drop = useCargoDrop((items) => open(true, target ?? onlyFlight(), undefined, items));
  if (isOpen) return null;
  return (
    <button
      className={`composer-pill${ready ? ' drop-ready' : ''}${drop.over ? ' drop-over' : ''}`}
      onClick={() => open(true, target)}
      title="Transmitir (⌥K)"
      {...drop.props}
    >
      <span className="cp-label">Transmitir →</span>
      {t && target && (
        <span className="cp-target mono">
          <i style={{ background: `var(--r-${roleOf(target)})` }} />
          {callsign(t)}
        </span>
      )}
      <span className="cp-placeholder">{ready ? 'solte aqui: abre com o caminho' : t ? 'escreva para o voo…' : 'escolha um voo e escreva…'}</span>
      <span className="cp-kbd mono">⌥K</span>
      <span className="cp-send">
        <Icon name="arrow_upward" size={17} />
      </span>
    </button>
  );
}

/** The flight of the level on screen, when it is the only one. */
function onlyFlight(): string | null {
  const st = useSquad.getState();
  const flights = st.workspace?.nodes.filter((n) => n.terminal && (n.floor ?? '') === st.activeFloor) ?? [];
  return flights.length === 1 ? flights[0].id : null;
}
