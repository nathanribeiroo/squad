import { useState } from 'react';
import type { Node, NodeProps } from '@xyflow/react';
import { api } from '../../api/client';
import type { WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { toast } from '../../lib/toast';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu } from './NodeMenu';

export type PortalFlowNode = Node<{ node: WNode }, 'portal'>;

/**
 * Frame of the browser portal: the address is editable and agents linked to
 * the portal see it in mentions. The live page (a browser driven over CDP)
 * is a later stage (docs/PLAN.md).
 */
export function PortalNode({ id, data, selected }: NodeProps<PortalFlowNode>) {
  const url = data.node.portal?.url ?? '';
  const target = useConnectTarget(id);
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(url);
  const remove = () => api.removeNode(id).catch((e: Error) => toast(e.message));

  const commit = () => {
    setEditing(false);
    let next = draft.trim();
    if (next && !/^[a-z]+:\/\//i.test(next)) next = `http://${next}`;
    if (next !== url) api.updateNode(id, { portal: { url: next } }).catch((e: Error) => toast(e.message));
  };

  return (
    <div className={`pnode${selected ? ' selected' : ''}${connectClass(target)}`}>
      <NodeHandles id={id} />
      <div className="node-stripe" style={{ background: 'var(--line-2)' }} />
      <header className="pn-head node-drag">
        <Icon name="public" size={18} style={{ color: 'var(--text-2)' }} />
        <span className="pn-title">Portal</span>
        {editing ? (
          <input
            className="pn-url mono nodrag"
            value={draft}
            autoFocus
            spellCheck={false}
            placeholder="http://localhost:5173"
            onChange={(e) => setDraft(e.target.value)}
            onBlur={commit}
            onKeyDown={(e) => {
              if (e.key === 'Enter') commit();
              if (e.key === 'Escape') {
                e.stopPropagation();
                setDraft(url);
                setEditing(false);
              }
            }}
          />
        ) : (
          <button
            className="pn-url mono nodrag"
            onClick={() => {
              setDraft(url);
              setEditing(true);
            }}
            title="Editar endereço"
          >
            {url || 'definir endereço…'}
          </button>
        )}
        {url && !editing && (
          <a className="icon-btn nodrag" href={url} target="_blank" rel="noreferrer noopener" title="Abrir no navegador">
            <Icon name="open_in_new" size={16} />
          </a>
        )}
        <span className="pn-live" title="A transmissão da página chega numa etapa futura">
          <i />
          sem transmissão
        </span>
        <NodeMenu items={[{ icon: 'delete', label: 'Remover', action: remove, danger: true }]} />
      </header>
      <div className="pn-body">
        <Icon name="public_off" size={28} />
        <span>A imagem ao vivo da página ainda não está disponível nesta versão.</span>
        {url && <span className="mono pn-body-url">{url}</span>}
      </div>
    </div>
  );
}
