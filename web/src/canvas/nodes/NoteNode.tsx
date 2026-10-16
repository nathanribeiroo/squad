import { useEffect, useRef, useState } from 'react';
import type { Node, NodeProps } from '@xyflow/react';
import { api } from '../../api/client';
import type { WNode } from '../../api/types';
import { Icon } from '../../components/Icon';
import { roleColor, useRoleLookup } from '../../lib/roles';
import { Markdown, changedLines, type MarkdownHighlight } from '../../lib/markdown';
import { toast } from '../../lib/toast';
import { useNow } from '../../lib/useNow';
import { useSquad } from '../../state/store';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu } from './NodeMenu';

export type NoteFlowNode = Node<{ node: WNode }, 'note'>;

/** How long an agent's edit stays highlighted (handoff §3). */
const HIGHLIGHT_MS = 30_000;

export function NoteNode({ id, data, selected }: NodeProps<NoteFlowNode>) {
  const note = data.node.note!;
  const edit = useSquad((s) => s.noteEdits[id]);
  const nodes = useSquad((s) => s.workspace?.nodes);
  const roleOf = useRoleLookup();
  const target = useConnectTarget(id);
  const [content, setContent] = useState<string | null>(null);
  const [mode, setMode] = useState<'preview' | 'edit'>('preview');
  const [draft, setDraft] = useState('');
  const [stale, setStale] = useState(false);
  const [mark, setMark] = useState<{ from: number; to: number; author: string; at: number } | null>(null);
  const shown = useRef<string | null>(null);
  const saveTimer = useRef<number | undefined>(undefined);
  const now = useNow(1000, !!mark);

  // Load, and reload when someone else edits the file.
  useEffect(() => {
    let live = true;
    api
      .note(id)
      .then((n) => {
        if (!live) return;
        const prev = shown.current;
        if (edit && prev !== null && edit.author !== 'Você') {
          const r = changedLines(prev, n.content);
          if (r) setMark({ ...r, author: edit.author, at: edit.at });
        }
        if (mode === 'edit' && prev !== null && edit?.author !== 'Você') {
          setStale(true);
          return;
        }
        shown.current = n.content;
        setContent(n.content);
      })
      .catch(() => live && setContent(''));
    return () => {
      live = false;
    };
  }, [id, edit?.at]);

  useEffect(() => {
    if (mark && now - mark.at > HIGHLIGHT_MS) setMark(null);
  }, [mark, now]);

  const save = (text: string) => {
    shown.current = text;
    setContent(text);
    api.saveNote(id, text).catch((e: Error) => toast(e.message.includes('locked') ? 'Nota travada: destrave para editar.' : e.message));
  };

  const startEdit = () => {
    setDraft(content ?? '');
    setStale(false);
    setMode('edit');
  };
  const finishEdit = () => {
    window.clearTimeout(saveTimer.current);
    if (draft !== content) save(draft);
    setMode('preview');
  };

  const toggleTask = (line: number) => {
    if (content === null || note.locked) return;
    const lines = content.split('\n');
    lines[line] = lines[line].replace(/\[( |x|X)\]/, (m) => (m === '[ ]' ? '[x]' : '[ ]'));
    save(lines.join('\n'));
  };

  const setLocked = (locked: boolean) => api.updateNode(id, { note: { ...note, locked } }).catch((e: Error) => toast(e.message));
  const remove = () => {
    if (window.confirm(`Remover a nota "${note.title}" do canvas? O arquivo ${note.file} continua no disco.`))
      api.removeNode(id).catch((e: Error) => toast(e.message));
  };

  const authorRole = (name: string) => roleColor(roleOf(nodes?.find((n) => n.terminal?.name === name)?.terminal?.role));
  const highlight: MarkdownHighlight | null = mark
    ? { from: mark.from, to: mark.to, role: authorRole(mark.author), label: `${mark.author} escreveu ${ago(now - mark.at)}` }
    : null;

  return (
    <div className={`nnode${selected ? ' selected' : ''}${connectClass(target)}`}>
      <NodeHandles id={id} />
      <header className="nn-head node-drag">
        <Icon name="description" size={18} style={{ color: 'var(--text-2)' }} />
        <span className="nn-title">{note.title}</span>
        <span className="nn-file mono">{note.file}</span>
        <span className="tn-spacer" />
        <button
          className="icon-btn nodrag nn-lock"
          onClick={() => setLocked(!note.locked)}
          title={note.locked ? 'Travada: ninguém escreve. Clique para destravar.' : 'Clique para travar a nota'}
        >
          <Icon name={note.locked ? 'lock' : 'lock_open'} size={16} />
        </button>
        <div className="nn-seg nodrag" role="tablist">
          <button className={mode === 'preview' ? 'on' : ''} onClick={() => (mode === 'edit' ? finishEdit() : undefined)}>
            Prévia
          </button>
          <button className={mode === 'edit' ? 'on' : ''} onClick={() => (mode === 'preview' ? startEdit() : undefined)} disabled={note.locked}>
            Editar
          </button>
        </div>
        <NodeMenu items={[{ icon: 'delete', label: 'Remover do canvas', action: remove, danger: true }]} />
      </header>
      {mode === 'edit' ? (
        <>
          {stale && (
            <div className="nn-stale">
              <Icon name="sync_problem" size={15} />
              {edit?.author ?? 'Alguém'} alterou a nota.
              <button
                onClick={() => {
                  setStale(false);
                  void api.note(id).then((n) => {
                    shown.current = n.content;
                    setContent(n.content);
                    setDraft(n.content);
                  });
                }}
              >
                Recarregar
              </button>
            </div>
          )}
          <textarea
            className="nn-editor nodrag nowheel mono"
            value={draft}
            autoFocus
            spellCheck={false}
            onChange={(e) => {
              const v = e.target.value;
              setDraft(v);
              window.clearTimeout(saveTimer.current);
              saveTimer.current = window.setTimeout(() => save(v), 800);
            }}
            onBlur={finishEdit}
            onKeyDown={(e) => {
              if (e.key === 'Escape') {
                e.stopPropagation();
                finishEdit();
              }
            }}
          />
        </>
      ) : (
        <div className="nn-body nodrag nowheel" onDoubleClick={() => !note.locked && startEdit()}>
          {content === null ? null : content.trim() === '' ? (
            <span className="nn-empty">Nota vazia. Clique em Editar, ou ligue um agente a ela para ele escrever com squad note.</span>
          ) : (
            <Markdown text={content} highlight={highlight} onToggleTask={note.locked ? undefined : toggleTask} />
          )}
        </div>
      )}
    </div>
  );
}

function ago(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  return s < 60 ? `há ${s} s` : `há ${Math.round(s / 60)} min`;
}
