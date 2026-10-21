import type { IconName } from '../lib/icons';
import { togglePlace } from '../lib/place';
import { groupSelection } from '../lib/visual';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import { TerminalForm } from './TerminalForm';
import './Toolbar.css';

type Tool = { icon: IconName; label: string; key: string; action: () => void; active?: boolean; disabled?: boolean };

export function Toolbar() {
  const tool = useSquad((s) => s.tool);
  const setTool = useSquad((s) => s.setTool);
  const placing = useSquad((s) => (s.tool === 'place' ? s.placeKind : null));
  const newTerm = useSquad((s) => s.newTermOpen);
  const setNewTerm = useSquad((s) => s.setNewTerm);
  const setEditTerm = useSquad((s) => s.setEditTerm);
  const editNode = useSquad((s) => (s.editTermId ? s.workspace?.nodes.find((n) => n.id === s.editTermId && n.terminal) : undefined));

  const nav: Tool[] = [
    { icon: 'near_me', label: 'Selecionar', key: 'V', action: () => setTool('select'), active: tool === 'select' },
    { icon: 'pan_tool', label: 'Mover', key: 'H', action: () => setTool('pan'), active: tool === 'pan' },
  ];
  const create: Tool[] = [
    // Each item: pick it, then click or drag on the canvas where it goes.
    { icon: 'flight_takeoff', label: 'Novo voo', key: 'T', action: () => (newTerm ? setNewTerm(false) : togglePlace('terminal')), active: newTerm || placing === 'terminal' },
    { icon: 'sticky_note_2', label: 'NOTAM', key: 'N', action: () => togglePlace('note'), active: placing === 'note' },
    { icon: 'public', label: 'Portal', key: 'P', action: () => togglePlace('portal'), active: placing === 'portal' },
    { icon: 'inventory_2', label: 'Carga', key: 'C', action: () => togglePlace('cargo'), active: placing === 'cargo' },
    { icon: 'gesture', label: 'Desenho', key: 'D', action: () => setTool(tool === 'draw' ? 'select' : 'draw'), active: tool === 'draw' },
    { icon: 'select', label: 'Grupo', key: 'G', action: groupSelection, active: tool === 'frame' },
    { icon: 'flight_class', label: 'Planos de voo', key: '⌥M', action: () => useSquad.getState().setPlansDialog('gallery') },
  ];

  const render = (t: Tool) => (
    <button
      key={t.icon}
      className={`tbx-btn${t.active ? ' active' : ''}`}
      onClick={t.action}
      disabled={t.disabled}
      aria-label={t.label}
    >
      <Icon name={t.icon} size={19} />
      <span className="tbx-tip">
        {t.label} <span className="mono">{t.key}</span>
      </span>
    </button>
  );

  return (
    <>
      <nav className="toolbar" aria-label="Ferramentas">
        {nav.map(render)}
        <span className="tbx-divider" />
        {create.map(render)}
      </nav>
      {newTerm && <TerminalForm onClose={() => setNewTerm(false)} />}
      {editNode && <TerminalForm key={editNode.id} node={editNode} onClose={() => setEditTerm(null)} />}
    </>
  );
}
