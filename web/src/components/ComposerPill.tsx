import { useNodeName, useSquad } from '../state/store';
import { Icon } from './Icon';
import './ComposerPill.css';

export function ComposerPill() {
  const target = useSquad((s) => s.composerTarget ?? s.focusedTerminal ?? s.selectedNode);
  const name = useNodeName(target ?? undefined);
  const open = useSquad((s) => s.setComposer);
  return (
    <button className="composer-pill" onClick={() => open(true, target)}>
      <Icon name="edit_square" size={18} />
      <span className="cp-placeholder">{name ? `Escrever para ${name}…` : 'Escrever para um agente…'}</span>
      <span className="kbd">⌥K</span>
      <span className="cp-send">
        <Icon name="arrow_upward" size={18} />
      </span>
    </button>
  );
}
