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
  if (isOpen) return null;
  return (
    <button className="composer-pill" onClick={() => open(true, target)} title="Transmitir (⌥K)">
      <span className="cp-label">Transmitir →</span>
      {t && target && (
        <span className="cp-target mono">
          <i style={{ background: `var(--r-${roleOf(target)})` }} />
          {callsign(t)}
        </span>
      )}
      <span className="cp-placeholder">{t ? 'escreva para o voo…' : 'escolha um voo e escreva…'}</span>
      <span className="cp-kbd mono">⌥K</span>
      <span className="cp-send">
        <Icon name="arrow_upward" size={17} />
      </span>
    </button>
  );
}
