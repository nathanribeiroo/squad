import type { Node, NodeProps } from '@xyflow/react';
import { api } from '../../api/client';
import type { Role, WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { ROLE_META, roleKey } from '../../lib/model';
import { toast } from '../../lib/toast';
import { useSquad } from '../../state/store';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu, type MenuItem } from './NodeMenu';
import { StatePill } from './StatePill';

export type TerminalFlowNode = Node<{ node: WNode }, 'terminal'>;

function shortPath(p?: string): string {
  if (!p) return '';
  const m = p.match(/^\/(Users|home)\/[^/]+/);
  return m ? '~' + p.slice(m[0].length) : p;
}

export function TerminalNode({ id, data, selected }: NodeProps<TerminalFlowNode>) {
  const t = data.node.terminal!;
  const rt = useSquad((s) => s.runtime[id]);
  const cwd = useSquad((s) => t.cwd || s.workspace?.cwd);
  const focused = useSquad((s) => s.focusedTerminal === id);
  const target = useConnectTarget(id);
  const state = rt?.activity ?? 'boot';
  const role = roleKey(t.role);
  const running = rt?.status === 'running' || rt?.status === 'starting';

  const act = (a: 'start' | 'stop' | 'restart') => api.nodeAction(id, a).catch((e: Error) => toast(e.message));
  const setRole = (r: Role) => api.updateNode(id, { terminal: { ...t, role: r } }).catch((e: Error) => toast(e.message));
  const remove = () => {
    if (window.confirm(`Remover "${t.name}"? O processo do terminal é encerrado.`)) api.removeNode(id).catch((e: Error) => toast(e.message));
  };
  const items: (MenuItem | 'divider')[] = [
    running
      ? { icon: 'stop_circle', label: 'Parar', action: () => act('stop') }
      : { icon: 'play_circle', label: 'Iniciar', action: () => act('start') },
    { icon: 'restart_alt', label: 'Reiniciar', action: () => act('restart') },
    'divider',
    ...(Object.keys(ROLE_META) as Exclude<Role, ''>[]).map((r) => ({
      icon: ROLE_META[r].icon,
      label: ROLE_META[r].label,
      checked: t.role === r,
      action: () => setRole(t.role === r ? '' : r),
    })),
    'divider',
    { icon: 'delete', label: 'Remover', action: remove, danger: true },
  ];

  return (
    <div className={`tnode${focused ? ' focused' : ''}${selected ? ' selected' : ''}${connectClass(target)}`}>
      <NodeHandles id={id} />
      <div className="node-stripe" style={{ background: `var(--r-${role})` }} />
      <header className="tn-head node-drag">
        <span className="tn-name">{t.name}</span>
        <span className="tn-agent mono">{t.agent}</span>
        {role !== 'none' && (
          <span className="tn-role" style={{ background: `var(--rb-${role})`, color: `var(--r-${role})` }}>
            <Icon name={ROLE_META[role].icon} size={14} />
            {ROLE_META[role].label}
          </span>
        )}
        <span className="tn-spacer" />
        {focused && (
          <span className="tn-kbd-here">
            <Icon name="keyboard" size={15} />
            teclado aqui
          </span>
        )}
        <StatePill state={state} since={rt?.activitySince} exitCode={rt?.exitCode} />
        <NodeMenu items={items} />
      </header>
      <div className="tn-body nodrag nowheel">
        {rt?.status === 'failed' || rt?.error ? (
          <div className="tn-msg err">
            <Icon name="error" size={18} />
            {rt?.error || 'O processo terminou com erro.'}
          </div>
        ) : rt?.status === 'exited' || rt?.status === 'stopped' ? (
          <div className="tn-msg">
            <Icon name="bedtime" size={18} />
            Processo encerrado{rt.exitCode ? ` (saída ${rt.exitCode})` : ''}. Use o menu para iniciar.
          </div>
        ) : (
          <div className="tn-msg dim mono">{rt?.title || t.command || t.agent}</div>
        )}
      </div>
      <footer className="tn-foot mono">
        <span>⎇ main</span>
        <span>
          {t.cols ?? 80}×{t.rows ?? 24}
        </span>
        <span className="tn-spacer" />
        <span className="tn-cwd">{shortPath(cwd)}</span>
      </footer>
    </div>
  );
}
