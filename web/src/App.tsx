import { useCallback, useEffect } from 'react';
import { ReactFlowProvider, useReactFlow } from '@xyflow/react';
import { api } from './api/client';
import { connectEvents } from './api/events';
import { connectWorkspaces } from './api/workspaces';
import type { WNode } from './api/types';
import { Canvas } from './canvas/Canvas';
import { ActivityDrawer } from './components/ActivityDrawer';
import { AirspaceEmpty } from './components/AirspaceEmpty';
import { Composer } from './components/Composer';
import { ComposerPill } from './components/ComposerPill';
import { ConfirmHost } from './components/ConfirmDialog';
import { FloorDialogs } from './components/FloorDialogs';
import { FirstRun } from './components/FirstRun';
import { FloorEmpty } from './components/FloorEmpty';
import { InterruptedBar } from './components/InterruptedBar';
import { LoopCard } from './components/LoopCard';
import { NoLink } from './components/NoLink';
import { PlansDialogs } from './components/PlansDialogs';
import { PriorityBanner } from './components/PriorityBanner';
import { ScreenPane } from './components/ScreenPane';
import { RolesPanel } from './components/RolesPanel';
import { SettingsPanel } from './components/SettingsPanel';
import { Synoptic } from './components/synoptic/Synoptic';
import { SummaryDialogs } from './components/SummaryDialogs';
import { Toolbar } from './components/Toolbar';
import { TopBar } from './components/TopBar';
import { answerKey, goToNextAsk } from './lib/attention';
import { callsign } from './lib/model';
import { noticePrefs, remindAsks, remindYourTurns, unlockAudio } from './lib/notify';
import { dockAt, nextNode, toggleFocus } from './lib/screens';
import { termSession } from './term/session';
import { Toast, toast } from './lib/toast';
import { useSquad } from './state/store';
import './App.css';

export function App() {
  const theme = useSquad((s) => s.theme);
  const term = useSquad((s) => s.termTheme);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    // The title bar of an app window (and of the Safari web app) follows the
    // top bar's color.
    document.querySelector('meta[name="theme-color"]')?.setAttribute('content', theme === 'light' ? '#f6f2e4' : '#07100b');
  }, [theme]);
  // The window's name in the taskbar or the Dock: the workspace first, after
  // how many flights ask you, here and in the other workspaces.
  const wsName = useSquad((s) => s.workspace?.name);
  const asks = useSquad((s) => s.askingIds.length + s.workspaces.reduce((n, w) => n + (w.id !== s.activeWs ? (w.attention ?? 0) : 0), 0));
  const settingsOpen = useSquad((s) => s.settingsTab !== null); // Avisos may have changed
  useEffect(() => {
    const name = wsName ? `${wsName} · Torre` : 'Torre';
    document.title = asks > 0 && noticePrefs().title ? `(${asks}) ${name}` : name;
  }, [wsName, asks, settingsOpen]);
  // The sound, prepared at the first click; again while asks wait (Avisos).
  useEffect(() => {
    unlockAudio();
    const t = setInterval(() => {
      const s = useSquad.getState();
      remindAsks([
        ...s.askingIds.map((id) => `${id}@${s.runtime[id]?.attention?.since ?? ''}`),
        ...s.workspaces.filter((w) => w.id !== s.activeWs && (w.attention ?? 0) > 0).map((w) => `ws:${w.id}`),
      ]);
      remindYourTurns(
        Object.values(s.runtime)
          .filter((rt) => rt.yourTurn && rt.activity === 'done')
          .map((rt) => {
            const t = s.workspace?.nodes.find((n) => n.id === rt.id)?.terminal;
            return { key: `${rt.id}@${rt.activitySince}`, since: rt.activitySince, who: t ? callsign(t) : 'Um voo', said: (s.cards[rt.id]?.message ?? '').trim().split('\n')[0].slice(0, 160) };
          }),
      );
    }, 5_000);
    return () => clearInterval(t);
  }, []);
  // "Sistema" follows the system's theme as it changes.
  const themePref = useSquad((s) => s.themePref);
  useEffect(() => {
    if (themePref !== 'system' || typeof matchMedia !== 'function') return;
    const mq = matchMedia('(prefers-color-scheme: dark)');
    const on = () => useSquad.getState().setTheme('system');
    mq.addEventListener('change', on);
    return () => mq.removeEventListener('change', on);
  }, [themePref]);
  useEffect(() => {
    document.documentElement.dataset.term = term;
  }, [term]);
  // The workspace list picks the workspace on screen; its /ws/events starts
  // with "hello", the full state, and is reopened when switching workspace
  // or session.
  const activeWs = useSquad((s) => s.activeWs);
  const activeSession = useSquad((s) => s.activeSession);
  useEffect(() => connectWorkspaces(), []);
  useEffect(() => (activeWs ? connectEvents() : undefined), [activeWs, activeSession]);

  return (
    <ReactFlowProvider>
      <div className="app">
        <TopBar />
        <Stage />
        <FirstRun />
      </div>
    </ReactFlowProvider>
  );
}

function typing(t: EventTarget | null): boolean {
  const el = t as HTMLElement | null;
  if (!el) return false;
  return el.isContentEditable || el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT';
}

function Stage() {
  const view = useSquad((s) => s.view);
  // The docked flight (unless it is in the Foco, which has its screen).
  const dock = useSquad((s) => (s.dock && s.dock.id !== s.focusPane && s.workspace?.nodes.some((n) => n.id === s.dock!.id) ? s.dock : null));
  const focusPane = useSquad((s) => s.focusPane);
  const rf = useReactFlow();

  const create = useCallback(
    (kind: 'note' | 'portal') => {
      const w = kind === 'note' ? 400 : 560;
      const h = kind === 'note' ? 350 : 420;
      const c = rf.screenToFlowPosition({ x: window.innerWidth / 2, y: window.innerHeight / 2 });
      const floor = useSquad.getState().activeFloor;
      const body: Partial<WNode> = { kind, x: Math.round(c.x - w / 2), y: Math.round(c.y - h / 2), floor: floor || undefined };
      // A draft: it opens with its name selected; the name the user gives names the file.
      if (kind === 'note') body.note = { title: '', file: '', draft: true };
      api.addNode(body).catch((e: Error) => toast(e.message, 'err'));
    },
    [rf],
  );

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const st = useSquad.getState();
      // ⌥ shortcuts are global (e.code: Option changes e.key on macOS).
      if (e.altKey && !e.metaKey && !e.ctrlKey) {
        // ⌥⇧←/→ dock the flight at an edge (TELAS.md, quadro 04).
        if (e.shiftKey && (e.code === 'ArrowLeft' || e.code === 'ArrowRight')) {
          e.preventDefault();
          dockAt(e.code === 'ArrowLeft' ? 'left' : 'right');
          return;
        }
        // ⌥M flight plans; ⌥⇧M saves the selection as one.
        if (e.code === 'KeyM') {
          e.preventDefault();
          if (st.view !== 'space') st.setView('space');
          st.setPlansDialog(e.shiftKey ? 'save' : st.plansDialog === 'gallery' ? null : 'gallery');
          return;
        }
        const map: Record<string, () => void> = {
          KeyA: () => st.setDrawer(!st.drawerOpen),
          KeyP: () => void (st.extra.paused ? api.resume() : api.pause()).catch((err: Error) => toast(err.message, 'err')),
          ...Object.fromEntries(
            st.workspaces.slice(0, 9).map((w, i) => [`Digit${i + 1}`, () => st.switchWorkspace(w.id)]),
          ),
          // The next flight asking you: its level, centered, with the keyboard.
          Enter: () => {
            if (st.view !== 'space') st.setView('space');
            void goToNextAsk();
          },
          KeyS: () => st.setView(st.view === 'space' ? 'synoptic' : 'space'),
          KeyF: () => toggleFocus(),
          KeyK: () => {
            const isTerm = (id: string | null) => !!id && !!st.workspace?.nodes.find((n) => n.id === id && n.terminal);
            st.setComposer(!st.composerOpen, [st.focusedTerminal, st.selectedNode, st.composerTarget].find(isTerm) ?? null);
          },
        };
        const fn = map[e.code];
        if (fn) {
          e.preventDefault();
          fn();
        }
        return;
      }
      if (e.key === 'Escape') {
        if (st.focusPane && !typing(e.target)) st.setFocusPane(null);
        else if (st.newTermOpen) st.setNewTerm(false);
        else if (st.editTermId) st.setEditTerm(null);
        else if (st.composerOpen) st.setComposer(false);
        return;
      }
      // Plain letters only act on the canvas, never while typing (or in a dialog).
      if (e.metaKey || e.ctrlKey || typing(e.target) || st.rolesPanel !== false || document.querySelector('[aria-modal="true"]')) return;
      // 1, 2, 3 answer the selected flight that asks you (RESPONDER); in the
      // Sinóptico, the one on the PRIORIDADE card.
      const askId = st.view === 'synoptic' ? st.askingIds[0] : st.selectedNode && st.askingIds.includes(st.selectedNode) ? st.selectedNode : null;
      const ask = askId ? st.runtime[askId]?.attention : undefined;
      // 1–9: on a question or a plan, the agent's option of that number.
      const digit = /^Digit([1-9])$/.exec(e.code)?.[1];
      if (digit && askId && !e.shiftKey && answerKey(askId, ask, digit)) {
        e.preventDefault();
        return;
      }
      if (st.view !== 'space') return; // the canvas' keys need the canvas
      // Tab / ⇧Tab pass the selection along the nodes; Enter gives the
      // selected flight the keyboard. Not over a button or a link, which
      // keep their own Tab and Enter.
      const onPage = document.activeElement === document.body || !!document.activeElement?.closest('.react-flow');
      if (onPage && !st.focusPane && (e.code === 'Tab' || (e.code === 'Enter' && !e.shiftKey && !e.altKey))) {
        if (e.code === 'Tab') {
          e.preventDefault();
          nextNode(e.shiftKey);
          return;
        }
        const sel = st.selectedNode && st.workspace?.nodes.find((n) => n.id === st.selectedNode && n.terminal);
        if (sel) {
          e.preventDefault();
          // Too small to type into: bring it close first.
          if (st.zoomLevel !== 'live') st.setFloor(sel.floor ?? '', sel.id);
          window.setTimeout(() => termSession(sel.id).focus(), st.zoomLevel !== 'live' ? 450 : 0);
        }
        return;
      }
      const plain: Record<string, () => void> = {
        KeyV: () => st.setTool('select'),
        KeyH: () => st.setTool('pan'),
        KeyT: () => st.setNewTerm(!st.newTermOpen),
        KeyN: () => create('note'),
        KeyP: () => create('portal'),
      };
      const fn = plain[e.code];
      if (fn && !e.shiftKey) {
        e.preventDefault();
        fn();
      }
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [create]);

  return (
    <main className="stage">
      {view === 'synoptic' ? (
        <Synoptic />
      ) : (
        <div className="space">
          {dock && <ScreenPane key={`dock-${dock.id}`} id={dock.id} mode={dock.side} />}
          <div className="space-main">
            <Canvas />
            <FloorEmpty />
            <AirspaceEmpty onNote={() => create('note')} />
            <div className="bands">
              <InterruptedBar />
              <LoopCard />
              <PriorityBanner />
            </div>
            <Toolbar onCreateNote={() => create('note')} onCreatePortal={() => create('portal')} />
            <ComposerPill />
          </div>
        </div>
      )}
      {focusPane && <ScreenPane key={`focus-${focusPane}`} id={focusPane} mode="focus" />}
      <Composer />
      <ActivityDrawer />
      <RolesPanel />
      <SettingsPanel />
      <FloorDialogs />
      <PlansDialogs />
      <SummaryDialogs />
      <NoLink />
      <ConfirmHost />
      <Toast />
    </main>
  );
}
