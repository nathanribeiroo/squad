import type { Node, NodeProps } from '@xyflow/react';
import { api } from '../../api/client';
import type { WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { toast } from '../../lib/toast';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu } from './NodeMenu';

export type PortalFlowNode = Node<{ node: WNode }, 'portal'>;

/** Frame of the browser portal. Streaming over CDP is a later stage (docs/PLAN.md). */
export function PortalNode({ id, data, selected }: NodeProps<PortalFlowNode>) {
  const url = data.node.portal?.url ?? '';
  const target = useConnectTarget(id);
  const remove = () => api.removeNode(id).catch((e: Error) => toast(e.message));
  return (
    <div className={`pnode${selected ? ' selected' : ''}${connectClass(target)}`}>
      <NodeHandles id={id} />
      <div className="node-stripe" style={{ background: 'var(--line-2)' }} />
      <header className="pn-head node-drag">
        <Icon name="public" size={18} style={{ color: 'var(--text-2)' }} />
        <span className="pn-title">Portal</span>
        <Icon name="refresh" size={18} style={{ color: 'var(--text-3)' }} />
        <span className="pn-url mono">{url || 'sem endereço'}</span>
        <span className="pn-live">
          <i />
          indisponível
        </span>
        <NodeMenu items={[{ icon: 'delete', label: 'Remover', action: remove, danger: true }]} />
      </header>
      <div className="pn-body">
        <Icon name="public_off" size={28} />
        <span>O navegador do portal ainda não está disponível nesta versão.</span>
      </div>
    </div>
  );
}
