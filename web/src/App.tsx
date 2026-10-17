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
import { FloorEmpty } from './components/FloorEmpty';
import { InterruptedBar } from './components/InterruptedBar';
import { NoLink } from './components/NoLink';
import { PriorityBanner } from './components/PriorityBanner';
import { RolesPanel } from './components/RolesPanel';
import { SummaryDialogs } from './components/SummaryDialogs';
import { Toolbar } from './components/Toolbar';
import { TopBar } from './components/TopBar';
import { goToNextAsk, respond } from './lib/attention';
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
  useEffect(() => {
    const name = wsName ? `${wsName} · Torre` : 'Torre';
    document.title = asks > 0 ? `(${asks}) ${name}` : name;
  }, [wsName, asks]);
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
        const map: Record<string, () => void> = {
          KeyA: () => st.setDrawer(!st.drawerOpen),
          KeyP: () => void (st.extra.paused ? api.resume() : api.pause()).catch((err: Error) => toast(err.message, 'err')),
          ...Object.fromEntries(
            st.workspaces.slice(0, 9).map((w, i) => [`Digit${i + 1}`, () => st.switchWorkspace(w.id)]),
          ),
          // The next flight asking you: its level, centered, with the keyboard.
          Enter: () => void goToNextAsk(),
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
        if (st.newTermOpen) st.setNewTerm(false);
        else if (st.editTermId) st.setEditTerm(null);
        else if (st.composerOpen) st.setComposer(false);
        return;
      }
      // Plain letters only act on the canvas, never while typing (or in a dialog).
      if (e.metaKey || e.ctrlKey || typing(e.target) || st.rolesPanel !== false || document.querySelector('[aria-modal="true"]')) return;
      // 1, 2, 3 answer the selected flight that asks you (RESPONDER).
      const ask = st.selectedNode && st.askingIds.includes(st.selectedNode) ? st.runtime[st.selectedNode]?.attention : undefined;
      const answer = (choice: 'allow' | 'always' | 'deny') => () => {
        if (ask?.choices?.includes(choice)) respond(st.selectedNode!, choice);
      };
      const plain: Record<string, () => void> = {
        ...(ask ? { Digit1: answer('allow'), Digit2: answer('always'), Digit3: answer('deny') } : {}),
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
      <Canvas />
      <FloorEmpty />
      <AirspaceEmpty onNote={() => create('note')} />
      <InterruptedBar />
      <PriorityBanner />
      <Toolbar onCreateNote={() => create('note')} onCreatePortal={() => create('portal')} />
      <ComposerPill />
      <Composer />
      <ActivityDrawer />
      <RolesPanel />
      <FloorDialogs />
      <SummaryDialogs />
      <NoLink />
      <ConfirmHost />
      <Toast />
    </main>
  );
}
