import { useEffect, useMemo, useRef, useState } from 'react';
import { NodeResizeControl, NodeResizer, type Node, type NodeProps, type ResizeParams } from '@xyflow/react';
import { ApiError, api } from '../../api/client';
import type { NoteData, WNode } from '../../api/types';
import { confirmDialog } from '../../components/ConfirmDialog';
import { Icon } from '../../components/Icon';
import { roleColor, useCreator, useRoleLookup } from '../../lib/roles';
import { Markdown, changedLines, type MarkdownHighlight } from '../../lib/markdown';
import { callsign } from '../../lib/model';
import { toast } from '../../lib/toast';
import { stackOf } from '../../lib/stacks';
import { useNow } from '../../lib/useNow';
import { useSquad } from '../../state/store';
import { NodeHandles, connectClass, useConnectTarget } from './NodeHandles';
import { NodeMenu } from './NodeMenu';

export type NoteFlowNode = Node<{ node: WNode }, 'note'>;

/** How long an agent's edit stays highlighted (handoff §3). */
const HIGHLIGHT_MS = 30_000;

const NO_NODES: WNode[] = [];

/**
 * A NOTAM on the canvas, or a stack of them (the node is the base's): the
 * stack shows one note at a time, with its tabs, and its wires are the
 * base's (internal/store/stacks.go).
 */
export function NoteNode({ id, data, selected }: NodeProps<NoteFlowNode>) {
  const nodes = useSquad((s) => s.workspace?.nodes) ?? NO_NODES;
  const members = useMemo(() => stackOf(nodes, data.node), [nodes, data.node]);
  const stacked = members.length > 1;
  const [pick, setPick] = useState(id);
  const shown = members.find((m) => m.id === pick) ?? members[0];
  const target = useConnectTarget(id);

  const onResizeEnd = (_: unknown, p: ResizeParams) => {
    useSquad.getState().moveNodeLocal(id, Math.round(p.x), Math.round(p.y));
    api
      .updateNode(id, { x: Math.round(p.x), y: Math.round(p.y), w: Math.round(p.width), h: Math.round(p.height) })
      .catch((e: Error) => toast(e.message, 'err'));
  };

  return (
    <div className={`nnode${selected ? ' selected' : ''}${shown.note!.locked ? ' locked' : ''}${stacked ? ' stacked' : ''}${connectClass(target)}`}>
      <NodeResizer isVisible={selected} minWidth={280} minHeight={200} onResizeEnd={onResizeEnd} />
      {/* Unselected, a grip in the corner resizes it too (selecting first wasn't obvious). */}
      {!selected && (
        <NodeResizeControl position="bottom-right" minWidth={280} minHeight={200} onResizeEnd={onResizeEnd} className="nn-grip">
          <span title="Arraste para mudar o tamanho" />
        </NodeResizeControl>
      )}
      <NodeHandles id={id} />
      {stacked && <StackStrip members={members} shown={shown.id} onPick={setPick} />}
      <NoteCard key={shown.id} node={shown} stacked={stacked} />
    </div>
  );
}

/**
 * PILHA · 3 NOTAMs, a tab per note and ‹ 1/3 ›. A note an agent just wrote
 * while another one is shown gets a dot, and a line to go to it.
 */
function StackStrip({ members, shown, onPick }: { members: WNode[]; shown: string; onPick: (id: string) => void }) {
  const edits = useSquad((s) => s.noteEdits);
  const nodes = useSquad((s) => s.workspace?.nodes);
  const now = useNow(1000);
  // Who wrote, by callsign (IMP), as everywhere else.
  const who = (name: string) => {
    const t = nodes?.find((n) => n.terminal?.name === name)?.terminal;
    return t ? callsign(t) : name;
  };
  const at = members.findIndex((m) => m.id === shown);
  const fresh = members.filter((m) => m.id !== shown && edits[m.id] && edits[m.id].author !== 'Você' && now - edits[m.id].at < HIGHLIGHT_MS);
  const go = (step: number) => onPick(members[(at + step + members.length) % members.length].id);
  return (
    <div className="nn-stack node-drag">
      <span className="nn-stack-tag">Pilha · {members.length} NOTAMs</span>
      <span className="nn-stack-tabs nodrag" role="tablist" aria-label="NOTAMs da pilha">
        {members.map((m, k) => (
          <button
            key={m.id}
            role="tab"
            aria-selected={m.id === shown}
            className={`nn-stack-tab${m.id === shown ? ' on' : ''}${fresh.includes(m) ? ' fresh' : ''}`}
            title={m.note!.title}
            onClick={() => onPick(m.id)}
          >
            {k + 1}
          </button>
        ))}
      </span>
      {fresh[0] && (
        <button className="nn-stack-fresh nodrag" onClick={() => onPick(fresh[0].id)} title="Ver a NOTAM">
          {who(edits[fresh[0].id].author)} escreveu em {fresh[0].note!.title}
        </button>
      )}
      <span className="tn-spacer" />
      <button className="icon-btn nodrag" onClick={() => go(-1)} aria-label="NOTAM anterior">
        <Icon name="chevron_left" size={16} />
      </button>
      <span className="nn-stack-pos mono">
        {at + 1}/{members.length}
      </span>
      <button className="icon-btn nodrag" onClick={() => go(1)} aria-label="Próxima NOTAM">
        <Icon name="chevron_right" size={16} />
      </button>
    </div>
  );
}

/** One NOTAM: header, preview or editor. In a stack, the one on top. */
function NoteCard({ node, stacked }: { node: WNode; stacked: boolean }) {
  const id = node.id;
  const note = node.note!;
  const edit = useSquad((s) => s.noteEdits[id]);
  const nodes = useSquad((s) => s.workspace?.nodes);
  const roleOf = useRoleLookup();
  const [content, setContent] = useState<string | null>(null);
  const [mode, setMode] = useState<'preview' | 'edit'>('preview');
  const [draft, setDraft] = useState('');
  const [stale, setStale] = useState(false);
  const [mark, setMark] = useState<{ from: number; to: number; author: string; at: number } | null>(null);
  const shown = useRef<string | null>(null);
  const saveTimer = useRef<number | undefined>(undefined);
  const now = useNow(1000, !!mark);
  const [renaming, setRenaming] = useState(false);
  const creator = useCreator(node.createdBy);
  // Notes agents make keep the name they gave them.
  const canRename = !node.createdBy;

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
    api.saveNote(id, text).catch((e: Error) => toast(e.message.includes('locked') ? 'NOTAM travada: destrave para editar.' : e.message, 'err'));
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

  const setLocked = (locked: boolean) => api.updateNode(id, { note: { ...note, locked } }).catch((e: Error) => toast(e.message, 'err'));
  const remove = async () => {
    const ok = await confirmDialog({
      title: `Remover a NOTAM ${note.title}?`,
      icon: 'description',
      tone: 'warn',
      lose: ['A NOTAM sai do canvas, e os voos na frequência dela deixam de vê-la'],
      keep: [
        note.file && (
          <>
            O arquivo <span className="mono">{note.file}</span>, no disco
          </>
        ),
      ],
      action: 'Remover NOTAM',
    });
    if (ok) api.removeNode(id).catch((e: Error) => toast(e.message, 'err'));
  };

  const authorRole = (name: string) => roleColor(roleOf(nodes?.find((n) => n.terminal?.name === name)?.terminal?.role));
  const highlight: MarkdownHighlight | null = mark
    ? { from: mark.from, to: mark.to, role: authorRole(mark.author), label: `${mark.author} escreveu ${ago(now - mark.at)}` }
    : null;

  const unstack = (all: boolean) =>
    api
      .unstackNote(id, all)
      .then(() => toast(all ? 'Pilha desfeita: cada NOTAM ficou com as frequências da pilha' : `${note.title} saiu da pilha, com as frequências dela`))
      .catch((e: Error) => toast(e.message, 'err'));

  return (
    <>
      <header className="nn-head node-drag">
        <span className="nn-tag">NOTAM</span>
        {note.draft || renaming ? (
          <NoteNameInput id={id} note={note} onDone={() => setRenaming(false)} />
        ) : (
          <span className="nn-title" title={titleHint(note, creator?.name, canRename)} onDoubleClick={() => canRename && setRenaming(true)}>
            {note.title}
          </span>
        )}
        {creator && (
          <span className="pn-creator" title={`${creator.name} criou esta NOTAM (torre note create); o nome é o que o voo deu`}>
            <i style={{ background: `var(--r-${creator.color})` }} />
            por {creator.callsign}
          </span>
        )}
        {note.file && <span className="nn-file mono">{note.file}</span>}
        <span className="tn-spacer" />
        <button
          className="icon-btn nodrag nn-lock"
          onClick={() => setLocked(!note.locked)}
          title={note.locked ? 'Travada: ninguém escreve. Clique para destravar.' : 'Clique para travar a NOTAM'}
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
        <NodeMenu
          items={[
            ...(canRename ? [{ icon: 'edit' as const, label: 'Renomear', action: () => setRenaming(true) }] : []),
            ...(stacked
              ? [
                  { icon: 'call_split' as const, label: 'Tirar da pilha', action: () => unstack(false) },
                  { icon: 'layers_clear' as const, label: 'Desfazer a pilha', action: () => unstack(true) },
                ]
              : []),
            { icon: 'delete', label: 'Remover do canvas', action: remove, danger: true },
          ]}
        />
      </header>
      {mode === 'edit' ? (
        <>
          {stale && (
            <div className="nn-stale">
              <Icon name="sync_problem" size={15} />
              {edit?.author ?? 'Alguém'} alterou a NOTAM.
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
            className="nn-editor nodrag nowheel"
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
          <footer className="nn-foot mono">grava sozinha 0,8 s depois · Esc volta à prévia</footer>
        </>
      ) : (
        <div className="nn-body nodrag nowheel" onDoubleClick={() => !note.locked && startEdit()}>
          {content === null ? null : content.trim() === '' ? (
            <span className="nn-empty">NOTAM vazia. Clique em Editar, ou ponha um voo na frequência dela para ele escrever.</span>
          ) : (
            <Markdown text={content} highlight={highlight} onToggleTask={note.locked ? undefined : toggleTask} />
          )}
        </div>
      )}
    </>
  );
}

/** The title's tooltip: the name agents use, and who may change it. */
function titleHint(note: NoteData, creator: string | undefined, canRename: boolean): string {
  const use = `Os agentes usam este nome: torre note read "${note.title}"`;
  const former = note.aliases?.length ? `\nNomes antigos, que continuam valendo: ${note.aliases.map((a) => `"${a}"`).join(', ')}` : '';
  if (!canRename) return `${use}\nNome dado por ${creator ?? 'um voo'}; não muda.${former}`;
  return `${use}${former}\nDuplo clique para renomear (o nome antigo continua valendo para os agentes).`;
}

/** The note's name, editable. A draft opens with it selected: Enter or a
 * click elsewhere confirms it (and names the file), Esc keeps the default. */
function NoteNameInput({ id, note, onDone }: { id: string; note: NoteData; onDone: () => void }) {
  const done = useRef(false);
  const input = useRef<HTMLInputElement>(null);
  const commit = (value: string, keep: boolean) => {
    if (done.current) return;
    const name = keep ? note.title : value.trim();
    if (!note.draft && (keep || name === note.title)) {
      done.current = true;
      onDone();
      return;
    }
    const problem = !name ? 'Dê um nome à NOTAM.' : name.startsWith('-') ? 'O nome não pode começar com "-".' : '';
    if (problem) {
      toast(problem, 'err');
      input.current?.focus();
      return;
    }
    done.current = true;
    api
      .renameNote(id, name)
      .then(onDone)
      .catch((e: Error) => {
        done.current = false;
        toast(e instanceof ApiError && e.status === 409 ? `Já existe uma NOTAM chamada "${name}" (ou que já teve esse nome).` : e.message, 'err');
        input.current?.focus();
        input.current?.select();
      });
  };
  return (
    <input
      ref={input}
      className="nn-name nodrag"
      defaultValue={note.title}
      autoFocus
      spellCheck={false}
      maxLength={60}
      aria-label="Nome da NOTAM"
      placeholder="Nome da NOTAM"
      onFocus={(e) => e.target.select()}
      onBlur={(e) => commit(e.target.value, false)}
      onKeyDown={(e) => {
        e.stopPropagation(); // canvas shortcuts (N, T…) stay out of the name
        if (e.key === 'Enter') commit(e.currentTarget.value, false);
        if (e.key === 'Escape') commit(e.currentTarget.value, true);
      }}
    />
  );
}

function ago(ms: number): string {
  const s = Math.max(0, Math.round(ms / 1000));
  return s < 60 ? `há ${s} s` : `há ${Math.round(s / 60)} min`;
}
