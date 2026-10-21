import { useEffect, useState } from 'react';
import { api, hub } from '../api/client';
import type { AgentInfo, AgentsDetection, LimitsView, WorkspaceSettings } from '../api/types';
import type { IconName } from '../lib/icons';
import { noticePrefs, setNoticePrefs, beep, type NoticePrefs } from '../lib/notify';
import { toast } from '../lib/toast';
import { useSquad, type SettingsTab } from '../state/store';
import { Icon } from './Icon';
import { SummarySettingsBody } from './SummaryDialogs';
import './SettingsPanel.css';

const TABS: { id: SettingsTab | 'roles'; label: string; icon: IconName }[] = [
  { id: 'appearance', label: 'Aparência', icon: 'contrast' },
  { id: 'agents', label: 'Agentes', icon: 'smart_toy' },
  { id: 'workspaces', label: 'Workspaces', icon: 'bedtime' },
  { id: 'limits', label: 'Limites', icon: 'swap_horiz' },
  { id: 'summaries', label: 'Resumos', icon: 'summarize' },
  { id: 'roles', label: 'Papéis…', icon: 'badge' },
  { id: 'shortcuts', label: 'Atalhos', icon: 'keyboard' },
  { id: 'notices', label: 'Avisos', icon: 'notifications' },
];

const AGENT_LABELS: Record<string, string> = { claude: 'Claude Code', codex: 'Codex', devin: 'Devin', gemini: 'Gemini' };

/** Configurações (docs/torre/TELAS.md, quadro 18): 1200×780, tabs on the left. */
export function SettingsPanel() {
  const tab = useSquad((s) => s.settingsTab);
  if (tab === null) return null;
  return <Panel tab={tab} />;
}

function Panel({ tab }: { tab: SettingsTab }) {
  const close = () => useSquad.getState().openSettings(null);
  useEffect(() => {
    const esc = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !document.querySelector('.dlg-backdrop.top')) close();
    };
    window.addEventListener('keydown', esc);
    return () => window.removeEventListener('keydown', esc);
  }, []);
  return (
    <div className="st-backdrop" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div className="st-panel" role="dialog" aria-modal="true" aria-label="Configurações">
        <header className="st-head">
          <Icon name="settings" size={20} />
          <strong>Configurações</strong>
          <span style={{ flex: 1 }} />
          <button className="icon-btn" onClick={close} aria-label="Fechar">
            <Icon name="close" size={18} />
          </button>
        </header>
        <div className="st-body">
          <nav className="st-tabs" aria-label="Seções">
            {TABS.map((t) => (
              <button
                key={t.id}
                className={`st-tab${tab === t.id ? ' on' : ''}`}
                onClick={() => {
                  if (t.id === 'roles') {
                    close();
                    useSquad.getState().openRoles();
                  } else useSquad.getState().openSettings(t.id);
                }}
              >
                <Icon name={t.icon} size={17} />
                {t.label}
              </button>
            ))}
          </nav>
          <section className="st-content">
            {tab === 'appearance' && <Appearance />}
            {tab === 'agents' && <Agents />}
            {tab === 'workspaces' && <WorkspacesTab />}
            {tab === 'limits' && <LimitsTab />}
            {tab === 'summaries' && (
              <>
                <h2>Resumos e briefing</h2>
                <p className="st-lead">Neste workspace.</p>
                <SummarySettingsBody />
              </>
            )}
            {tab === 'shortcuts' && <Shortcuts />}
            {tab === 'notices' && <Notices />}
          </section>
        </div>
      </div>
    </div>
  );
}

function Choice<T extends string>({
  value,
  options,
  onChange,
  disabled,
}: {
  value: T;
  options: { id: T; label: string; icon: IconName }[];
  onChange: (v: T) => void;
  disabled?: boolean;
}) {
  return (
    <div className="st-choice" role="radiogroup" aria-disabled={disabled}>
      {options.map((o) => (
        <button key={o.id} role="radio" aria-checked={value === o.id} className={value === o.id ? 'on' : ''} disabled={disabled} onClick={() => onChange(o.id)}>
          <Icon name={o.icon} size={18} />
          {o.label}
        </button>
      ))}
    </div>
  );
}

const IDLE_MINUTES = ['1', '5', '15', '30', '60'] as const;

/** When workspaces off screen stop their agents (internal/hub sleep.go). */
function WorkspacesTab() {
  const [s, setS] = useState<WorkspaceSettings | null>(null);
  useEffect(() => {
    hub.workspaceSettings().then(setS, (err: Error) => toast(err.message, 'err'));
  }, []);
  if (!s) return <h2>Workspaces</h2>;
  const save = (next: WorkspaceSettings) => {
    const before = s;
    setS(next);
    hub.setWorkspaceSettings(next).then(setS, (err: Error) => {
      setS(before);
      toast(err.message, 'err');
    });
  };
  const minutes = String(s.idleMinutes);
  return (
    <>
      <h2>Workspaces</h2>
      <p className="st-lead">
        Os agentes rodam no workspace que está na tela. Fora da tela e sem nada acontecendo, um workspace para os agentes: cada voo guarda a conversa e a última tela, e
        volta, continuando de onde estava, quando o workspace aparecer de novo. Os shells e os comandos próprios continuam rodando.
      </p>
      <label className="st-check">
        <input type="checkbox" checked={s.keepAll} onChange={(e) => save({ ...s, keepAll: e.target.checked })} />
        <span>
          <b>Manter todos os workspaces ativos</b>
          <span>
            Os agentes de todos os workspaces ficam rodando, mesmo fora da tela, e sobem quando a Torre abre. Ligar não sobe agora os que estão inativos: cada um sobe
            quando você entrar nele.
          </span>
        </span>
      </label>
      <div className={`st-row${s.keepAll ? ' st-off' : ''}`}>
        <div className="st-label">
          <b>Inativar depois de</b>
          <span>
            Quanto tempo um workspace fora da tela espera para parar os agentes. A conta começa quando você sai dele, ou quando tudo nele termina: um voo no ar, pedindo
            você ou esperando outro, e um nível em preparo, seguram o workspace ativo.
          </span>
        </div>
        <Choice
          value={(IDLE_MINUTES as readonly string[]).includes(minutes) ? minutes : ''}
          disabled={s.keepAll}
          onChange={(v) => save({ ...s, idleMinutes: Number(v) })}
          options={IDLE_MINUTES.map((m) => ({ id: m, label: m === '60' ? '1 h' : `${m} min`, icon: 'bedtime' as IconName }))}
        />
      </div>
      <p className="st-lead">
        Um workspace também pode ficar sempre ativo sozinho, pelo alfinete no seletor de workspaces, onde "Inativar agora" para os agentes de um que está fora da tela.
      </p>
    </>
  );
}

function Appearance() {
  const pref = useSquad((s) => s.themePref);
  const term = useSquad((s) => s.termTheme);
  return (
    <>
      <h2>Aparência</h2>
      <div className="st-row">
        <div className="st-label">
          <b>Tema da interface</b>
          <span>"Sistema" segue o tema do computador.</span>
        </div>
        <Choice
          value={pref}
          onChange={(v) => useSquad.getState().setTheme(v)}
          options={[
            { id: 'dark', label: 'Escuro', icon: 'dark_mode' },
            { id: 'light', label: 'Claro', icon: 'light_mode' },
            { id: 'system', label: 'Sistema', icon: 'contrast' },
          ]}
        />
      </div>
      <div className="st-row">
        <div className="st-label">
          <b>Tema dos terminais</b>
          <span>Muda com o da interface; dá para trocar depois.</span>
        </div>
        <Choice
          value={term}
          onChange={(v) => useSquad.getState().setTermTheme(v)}
          options={[
            { id: 'noite', label: 'Noite', icon: 'terminal' },
            { id: 'papel', label: 'Papel', icon: 'description' },
          ]}
        />
      </div>
    </>
  );
}

function Agents() {
  const [d, setD] = useState<AgentsDetection | null>(null);
  const [busy, setBusy] = useState(false);
  const load = (refresh: boolean) => {
    setBusy(true);
    hub
      .agents(refresh)
      .then(setD, (e: Error) => toast(e.message, 'err'))
      .finally(() => setBusy(false));
  };
  useEffect(() => load(false), []);
  return (
    <>
      <div className="st-title-row">
        <h2>Agentes</h2>
        <button className="btn" onClick={() => load(true)} disabled={busy}>
          <Icon name={busy ? 'progress_activity' : 'refresh'} className={busy ? 'spin' : undefined} />
          Procurar de novo
        </button>
      </div>
      <p className="st-lead">
        O comando padrão abre o agente nos voos sem comando próprio, em todos os workspaces. A Torre acrescenta os dela (hooks, MCP, papel, conversa).
      </p>
      {!d ? (
        <p className="st-lead">Procurando…</p>
      ) : (
        <table className="st-table">
          <thead>
            <tr>
              <th>Agente</th>
              <th>Versão</th>
              <th>Comando padrão</th>
            </tr>
          </thead>
          <tbody>
            {d.agents.map((a) => (
              <AgentRow key={a.agent} a={a} onSaved={setD} />
            ))}
            <tr>
              <td>
                <b>Navegador</b>
                <span className="st-sub">para os portais</span>
              </td>
              <td className={d.browser.found ? 'st-ok mono' : 'st-err mono'}>{d.browser.found ? '✓ achado' : '✕ não achado'}</td>
              <td className="mono st-path">{d.browser.path ?? 'Chrome, Edge, Chromium ou Brave (TORRE_BROWSER aponta outro)'}</td>
            </tr>
          </tbody>
        </table>
      )}
    </>
  );
}

function AgentRow({ a, onSaved }: { a: AgentInfo; onSaved: (d: AgentsDetection) => void }) {
  const [cmd, setCmd] = useState(a.defaultCommand);
  useEffect(() => setCmd(a.defaultCommand), [a.defaultCommand]);
  const save = (value: string) => {
    if (value.trim() === a.defaultCommand) return;
    hub.setAgentCommand(a.agent, value).then(
      (d) => {
        onSaved(d);
        toast(`Comando do ${AGENT_LABELS[a.agent]} salvo: vale para os voos que decolarem agora.`, 'ok');
      },
      (e: Error) => {
        setCmd(a.defaultCommand);
        toast(e.message, 'err');
      },
    );
  };
  return (
    <tr>
      <td>
        <b>{AGENT_LABELS[a.agent]}</b>
        {a.path && <span className="st-sub mono">{a.path}</span>}
      </td>
      <td className={a.found ? 'st-ok mono' : 'st-err mono'}>
        {a.found ? `✓ ${a.version || 'instalado'}` : '✕ não achado'}
        {!a.found && a.installHint && <span className="st-sub mono">{a.installHint}</span>}
      </td>
      <td>
        <span className="st-cmd">
          <input
            className="mono"
            value={cmd}
            aria-label={`Comando padrão do ${AGENT_LABELS[a.agent]}`}
            onChange={(e) => setCmd(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') save(cmd);
              if (e.key === 'Escape') {
                e.stopPropagation();
                setCmd(a.defaultCommand);
              }
            }}
            onBlur={() => save(cmd)}
          />
          {a.custom && (
            <button className="icon-btn" title={`Voltar para "${a.agent}"`} aria-label="Restaurar" onClick={() => save('')}>
              <Icon name="undo" size={16} />
            </button>
          )}
        </span>
      </td>
    </tr>
  );
}

function LimitsTab() {
  const [v, setV] = useState<LimitsView | null>(null);
  const wsName = useSquad((s) => s.workspace?.name ?? '');
  useEffect(() => {
    api.limits().then(setV, (e: Error) => toast(e.message, 'err'));
  }, []);
  const save = (patch: Partial<LimitsView['limits']>) => {
    if (!v) return;
    const next = { ...v.limits, ...patch };
    setV({ ...v, limits: next });
    api.setLimits(next).then(setV, (e: Error) => toast(e.message, 'err'));
  };
  if (!v) return <p className="st-lead">Lendo…</p>;
  const row = (key: keyof LimitsView['limits'], label: string, hint: string, min: number, max: number, slider: boolean) => (
    <div className="st-row" key={key}>
      <div className="st-label">
        <b>{label}</b>
        <span>
          {hint} Padrão: {v.defaults[key]}.
        </span>
      </div>
      <span className="st-limit">
        {slider && <input type="range" min={min} max={max} value={v.limits[key]} onChange={(e) => save({ [key]: Number(e.target.value) })} aria-label={label} />}
        <input
          className="mono st-num"
          type="number"
          min={min}
          max={max}
          value={v.limits[key]}
          onChange={(e) => {
            const n = Number(e.target.value);
            if (n >= min && n <= max) save({ [key]: n });
          }}
          aria-label={`${label} (número)`}
        />
      </span>
    </div>
  );
  return (
    <>
      <div className="st-title-row">
        <h2>Limites</h2>
        <button className="btn" onClick={() => save({ maxHops: 0, perMinute: 0, maxTeam: 0, portalsPerAgent: 0 })}>
          <Icon name="undo" />
          Restaurar padrões
        </button>
      </div>
      <p className="st-lead">Neste workspace ({wsName}), em todas as sessões. Valem na hora.</p>
      {row('maxHops', 'Saltos encadeados', 'Quantos pedidos uma conversa pode encadear (A pede a B, que pede a C…) antes da espera geral.', 1, 20, true)}
      {row('perMinute', 'Pedidos por minuto', 'Acima disso, os pedidos ficam retidos até o minuto passar.', 1, 120, true)}
      {row('maxTeam', 'Voos por líder', 'Quantos voos um líder pode criar com torre new.', 1, 30, false)}
      {row('portalsPerAgent', 'Portais por voo', 'Quantos portais um voo pode abrir com torre portal create.', 1, 10, false)}
    </>
  );
}

const SHORTCUTS: [string, string][] = [
  ['V · H', 'Selecionar · Mover'],
  ['T · N · P', 'Novo voo · NOTAM · Portal'],
  ['⌥K', 'Transmitir'],
  ['⌥Q', 'Sair da Torre'],
  ['⌥A', 'Rádio'],
  ['⌥P', 'Espera geral · Liberar todos'],
  ['⌥⏎', 'Ir ao próximo voo que pede você'],
  ['1 · 2 · 3', 'Autorizar · Sempre · Negar (com o voo que pede você selecionado)'],
  ['⌥S', 'Sinóptico ↔ Espaço'],
  ['⌥F', 'Foco: o voo ocupa a tela inteira (de novo, volta)'],
  ['⌥⇧← · ⌥⇧→', 'Encaixar o voo numa borda, com o canvas ao lado (de novo, desencaixa)'],
  ['Tab · ⇧Tab', 'Próximo · anterior nó, na ordem de leitura'],
  ['⏎', 'Dar o teclado ao voo selecionado'],
  ['⌥M · ⌥⇧M', 'Planos de voo · Salvar a seleção como plano'],
  ['⌥1…⌥9', 'Trocar de workspace'],
  ['⌥Esc', 'Devolver o teclado ao canvas'],
  ['Delete', 'Desligar a frequência selecionada'],
  ['⌘/Ctrl + roda', 'Zoom no canvas'],
];

function Shortcuts() {
  return (
    <>
      <h2>Atalhos</h2>
      <table className="st-table st-keys">
        <tbody>
          {SHORTCUTS.map(([k, what]) => (
            <tr key={k}>
              <td>
                {k.split(' · ').map((x) => (
                  <span key={x} className="kbd">
                    {x}
                  </span>
                ))}
              </td>
              <td>{what}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </>
  );
}

function Notices() {
  const [p, setP] = useState<NoticePrefs>(noticePrefs);
  const [perm, setPerm] = useState(typeof Notification === 'function' ? Notification.permission : 'denied');
  const set = (patch: Partial<NoticePrefs>) => {
    setP({ ...p, ...patch });
    setNoticePrefs(patch);
  };
  const askSystem = async (on: boolean) => {
    if (on && typeof Notification === 'function' && Notification.permission === 'default') {
      setPerm(await Notification.requestPermission());
    }
    set({ system: on });
  };
  return (
    <>
      <h2>Avisos</h2>
      <p className="st-lead">Quando um voo pede você (PRIORIDADE), em qualquer workspace; e quando termina um turno seu. Valem neste navegador.</p>
      <label className="st-check">
        <input type="checkbox" checked={p.title} onChange={(e) => set({ title: e.target.checked })} />
        <span>
          <b>No título da janela</b>
          <span>"(1) squad · Torre": aparece na barra de tarefas e no Dock.</span>
        </span>
      </label>
      <label className="st-check">
        <input type="checkbox" checked={p.system} onChange={(e) => void askSystem(e.target.checked)} />
        <span>
          <b>Notificação do sistema</b>
          <span>
            Só quando a janela da Torre não está na frente.
            {perm === 'denied' && ' O navegador bloqueou as notificações desta página: libere nas configurações dele.'}
          </span>
        </span>
      </label>
      <label className="st-check">
        <input type="checkbox" checked={p.sound} onChange={(e) => set({ sound: e.target.checked })} />
        <span>
          <b>Som curto</b>
          <span>
            Dois toques.{' '}
            <button className="link-btn" onClick={(e) => (e.preventDefault(), beep())}>
              Ouvir
            </button>
          </span>
        </span>
      </label>
      <label className="st-check">
        <input type="checkbox" checked={p.repeat} disabled={!p.sound} onChange={(e) => set({ repeat: e.target.checked })} />
        <span>
          <b>Repetir enquanto esperar</b>
          <span>O som de novo a cada 2 minutos, até 3 vezes, enquanto o voo continuar pedindo você.</span>
        </span>
      </label>
      <div className="st-row">
        <div className="st-label">
          <b>Quando um voo termina e espera você</b>
          <span>
            O turno era seu (você escreveu, usou Transmitir ou Continuar) e terminou: o voo mostra SUA VEZ. Com a janela da Torre atrás, avisa uma vez,
            com a notificação e um toque só, como escolhido acima.
          </span>
        </div>
        <Choice
          value={p.turnAfter}
          onChange={(v) => set({ turnAfter: v })}
          options={[
            { id: 'off', label: 'Não', icon: 'block' },
            { id: '0', label: 'Na hora', icon: 'notifications' },
            { id: '1', label: '1 min', icon: 'hourglass_top' },
            { id: '3', label: '3 min', icon: 'hourglass_top' },
            { id: '10', label: '10 min', icon: 'hourglass_top' },
          ]}
        />
      </div>
    </>
  );
}
