import { useEffect, useState } from 'react';
import type { Node, NodeProps } from '@xyflow/react';
import { api } from '../../api/client';
import type { WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { toast } from '../../lib/toast';
import { useSquad } from '../../state/store';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu } from './NodeMenu';

export type NoteFlowNode = Node<{ node: WNode }, 'note'>;

export function NoteNode({ id, data, selected }: NodeProps<NoteFlowNode>) {
  const note = data.node.note!;
  const edit = useSquad((s) => s.noteEdits[id]);
  const target = useConnectTarget(id);
  const [content, setContent] = useState<string | null>(null);

  useEffect(() => {
    let live = true;
    api
      .note(id)
      .then((n) => live && setContent(n.content))
      .catch(() => live && setContent(''));
    return () => {
      live = false;
    };
  }, [id, edit?.at]);

  const remove = () => {
    if (window.confirm(`Remover a nota "${note.title}" do canvas? O arquivo ${note.file} continua no disco.`))
      api.removeNode(id).catch((e: Error) => toast(e.message));
  };

  return (
    <div className={`nnode${selected ? ' selected' : ''}${connectClass(target)}`}>
      <NodeHandles id={id} />
      <header className="nn-head node-drag">
        <Icon name="description" size={18} style={{ color: 'var(--text-2)' }} />
        <span className="nn-title">{note.title}</span>
        <span className="nn-file mono">{note.file}</span>
        <span className="tn-spacer" />
        <Icon name={note.locked ? 'lock' : 'lock_open'} size={16} style={{ color: 'var(--text-3)' }} />
        <NodeMenu items={[{ icon: 'delete', label: 'Remover do canvas', action: remove, danger: true }]} />
      </header>
      <div className="nn-body nodrag nowheel">
        {content === null ? '' : content.trim() === '' ? <span className="nn-empty">Nota vazia. Agentes ligados a ela podem escrever com squad note.</span> : content}
      </div>
    </div>
  );
}
