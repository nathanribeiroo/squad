import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './ComposerPill.css';

export function ComposerPill() {
  // Only terminals receive messages; a selected note or portal is not a target.
  const target = useSquad((s) => {
    const isTerm = (id: string | null) => !!id && !!s.workspace?.nodes.find((n) => n.id === id && n.terminal);
    return [s.focusedTerminal, s.selectedNode, s.composerTarget].find(isTerm) ?? null;
  });
  const name = useSquad((s) => (target ? (s.workspace?.nodes.find((n) => n.id === target)?.terminal?.name ?? '') : ''));
  const open = useSquad((s) => s.setComposer);
  const isOpen = useSquad((s) => s.composerOpen);
  if (isOpen) return null;
  return (
    <button className="composer-pill" onClick={() => open(true, target)}>
      <Icon name="edit_square" size={18} />
      <span className="cp-placeholder">{name ? `Transmitir a ${name}…` : 'Transmitir a um voo…'}</span>
      <span className="kbd">⌥K</span>
      <span className="cp-send">
        <Icon name="arrow_upward" size={18} />
      </span>
    </button>
  );
}
