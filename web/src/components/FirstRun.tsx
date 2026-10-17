import { useEffect, useState } from 'react';
import { hub } from '../api/client';
import type { AgentsDetection, DirInfo } from '../api/types';
import { shortPath, trimDir } from '../lib/paths';
import { toast } from '../lib/toast';
import { useSquad } from '../state/store';
import { DirInput } from './DirInput';
import { Icon } from './Icon';
import './FirstRun.css';

const DONE_KEY = 'torre.welcomed';

function welcomed(): boolean {
  try {
    return localStorage.getItem(DONE_KEY) === '1';
  } catch {
    return true;
  }
}

/**
 * The first open (docs/torre/TELAS.md, quadro 19), on a new installation:
 * 1 Enlace (done: the login page), 2 Pasta (the workspace's folder, with
 * what git says about it), 3 Agentes (what this computer has).
 */
export function FirstRun() {
  const [show, setShow] = useState(false);
  const [step, setStep] = useState<2 | 3>(2);
  useEffect(() => {
    if (welcomed()) return;
    hub.welcome().then(
      (w) => setShow(w.firstRun),
      () => {},
    );
  }, []);
  if (!show) return null;
  const finish = () => {
    try {
      localStorage.setItem(DONE_KEY, '1');
    } catch {
      /* storage may be unavailable */
    }
    setShow(false);
  };
  return (
    <div className="fr" role="dialog" aria-modal="true" aria-label="Primeira abertura">
      <svg className="fr-radar" viewBox="0 0 600 600" aria-hidden="true">
        <g fill="none" stroke="#0e2015">
          <circle cx="300" cy="300" r="290" />
          <circle cx="300" cy="300" r="218" />
          <circle cx="300" cy="300" r="145" />
          <circle cx="300" cy="300" r="72" />
          <path d="M10 300H590M300 10V590" />
        </g>
      </svg>
      <main className="fr-main">
        <div className="fr-steps mono">
          <span className="done">1 Enlace ✓</span>
          <span className={step === 2 ? 'on' : 'done'}>2 Pasta{step === 3 && ' ✓'}</span>
          <span className={step === 3 ? 'on' : ''}>3 Agentes</span>
        </div>
        <div className="fr-card">{step === 2 ? <Folder onNext={() => setStep(3)} /> : <Agents onDone={finish} />}</div>
        <button className="link-btn fr-skip" onClick={finish}>
          Pular
        </button>
      </main>
    </div>
  );
}

function Folder({ onNext }: { onNext: () => void }) {
  const ws = useSquad((s) => s.workspaces.find((w) => w.id === s.activeWs));
  const [cwd, setCwd] = useState(ws?.cwd ?? '');
  const [info, setInfo] = useState<DirInfo | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (!cwd && ws?.cwd) setCwd(ws.cwd);
  }, [ws?.cwd, cwd]);
  useEffect(() => {
    const p = trimDir(cwd);
    if (!p) return setInfo(null);
    const t = window.setTimeout(() => hub.dirInfo(p).then(setInfo, () => setInfo(null)), 250);
    return () => window.clearTimeout(t);
  }, [cwd]);
  const next = async () => {
    if (!ws) return onNext();
    const p = trimDir(cwd);
    if (info && info.path !== ws.cwd) {
      setBusy(true);
      try {
        await hub.update(ws.id, { cwd: p, name: p.split(/[\\/]/).filter(Boolean).pop() ?? ws.name });
      } catch (e) {
        toast((e as Error).message, 'err');
        setBusy(false);
        return;
      }
      setBusy(false);
    }
    onNext();
  };
  const recent = (info?.recent ?? []).filter((r) => r !== info?.path);
  return (
    <>
      <h1>Pasta do projeto</h1>
      <p>Onde os voos trabalham. Cada pasta vira um workspace; dá para abrir outros depois, no seletor do topo.</p>
      <label className="field">
        Pasta
        <DirInput value={cwd} onChange={setCwd} placeholder="~/projetos/meu-app" suggest={hub.dirs} />
      </label>
      {info && (
        <ul className="fr-checks mono">
          {!info.exists ? (
            <li className="bad">✕ pasta não encontrada</li>
          ) : (
            <>
              <li className={info.git ? 'ok' : 'off'}>
                {info.git
                  ? `✓ repositório git · ${info.branch || 'HEAD'} · ${info.changes ? `${info.changes} ${info.changes === 1 ? 'alteração' : 'alterações'}` : 'limpo'}`
                  : '○ não é um repositório git'}
              </li>
              <li className={info.git && info.commits ? 'ok' : 'off'}>
                {info.git && info.commits ? '✓ níveis de voo disponíveis' : '○ níveis de voo precisam de um repositório git com um commit'}
              </li>
              <li className={info.envFiles.length ? 'ok' : 'off'}>
                {info.envFiles.length ? `✓ ${info.envFiles.join(', ')}: copiado para os níveis` : '○ nenhum .env'}
              </li>
            </>
          )}
        </ul>
      )}
      {recent.length > 0 && (
        <div className="fr-recent">
          <span className="label-caps">Recentes</span>
          {recent.map((r) => (
            <button key={r} className="fr-recent-item mono" onClick={() => setCwd(r)}>
              {shortPath(r)}
            </button>
          ))}
        </div>
      )}
      <div className="fr-actions">
        <button className="btn filled" disabled={busy || !info?.exists} onClick={() => void next()}>
          Continuar
          <Icon name="arrow_forward" size={16} />
        </button>
      </div>
    </>
  );
}

function Agents({ onDone }: { onDone: () => void }) {
  const [d, setD] = useState<AgentsDetection | null>(null);
  useEffect(() => {
    hub.agents(true).then(setD, (e: Error) => toast(e.message, 'err'));
  }, []);
  const labels: Record<string, string> = { claude: 'Claude Code', codex: 'Codex', devin: 'Devin', gemini: 'Gemini' };
  return (
    <>
      <h1>Agentes</h1>
      <p>Os agentes que a Torre achou neste computador. Os outros podem ser instalados quando você quiser.</p>
      {!d ? (
        <p>Procurando…</p>
      ) : (
        <ul className="fr-agents">
          {d.agents.map((a) => (
            <li key={a.agent}>
              <b>{labels[a.agent]}</b>
              {a.found ? <span className="mono ok">✓ {a.version || 'instalado'}</span> : <span className="mono off">✕ não achado · {a.installHint}</span>}
            </li>
          ))}
          <li>
            <b>Navegador</b>
            {d.browser.found ? <span className="mono ok">✓ para os portais</span> : <span className="mono off">✕ não achado: os portais precisam do Chrome, Edge ou Chromium</span>}
          </li>
        </ul>
      )}
      <div className="fr-actions">
        <button
          className="btn"
          onClick={() => {
            onDone();
            useSquad.getState().setPlansDialog('gallery');
          }}
        >
          Com um plano de voo
        </button>
        <button className="btn filled" onClick={onDone}>
          Começar vazio
          <Icon name="flight_takeoff" size={16} />
        </button>
      </div>
    </>
  );
}
