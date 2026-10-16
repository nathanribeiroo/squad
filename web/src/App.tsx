import { useCallback, useEffect } from 'react';
import { ReactFlowProvider, useReactFlow } from '@xyflow/react';
import { api } from './api/client';
import { connectEvents } from './api/events';
import { connectWorkspaces } from './api/workspaces';
import type { WNode } from './api/types';
import { Canvas } from './canvas/Canvas';
import { ActivityDrawer } from './components/ActivityDrawer';
import { Composer } from './components/Composer';
import { ComposerPill } from './components/ComposerPill';
import { RolesPanel } from './components/RolesPanel';
import { Toolbar } from './components/Toolbar';
import { TopBar } from './components/TopBar';
import { Toast, toast } from './lib/toast';
import { useSquad } from './state/store';
import './App.css';

export function App() {
  const theme = useSquad((s) => s.theme);
  const term = useSquad((s) => s.termTheme);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);
  useEffect(() => {
    document.documentElement.dataset.term = term;
  }, [term]);
  // The workspace list picks the workspace on screen; its /ws/events starts
  // with "hello", the full state, and is reopened when switching.
  const activeWs = useSquad((s) => s.activeWs);
  useEffect(() => connectWorkspaces(), []);
  useEffect(() => (activeWs ? connectEvents() : undefined), [activeWs]);

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
      const body: Partial<WNode> = { kind, x: Math.round(c.x - w / 2), y: Math.round(c.y - h / 2) };
      if (kind === 'note') body.note = { title: 'Nota', file: '' };
      api.addNode(body).catch((e: Error) => toast(e.message));
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
          KeyP: () => void (st.extra.paused ? api.resume() : api.pause()).catch((err: Error) => toast(err.message)),
          ...Object.fromEntries(
            st.workspaces.slice(0, 9).map((w, i) => [`Digit${i + 1}`, () => st.switchWorkspace(w.id)]),
          ),
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
      if (e.metaKey || e.ctrlKey || typing(e.target) || st.rolesPanel !== false) return;
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
      <Canvas />
      <Toolbar onCreateNote={() => create('note')} onCreatePortal={() => create('portal')} />
      <ComposerPill />
      <Composer />
      <ActivityDrawer />
      <RolesPanel />
      <Toast />
    </main>
  );
}
