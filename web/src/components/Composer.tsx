import { useEffect, useMemo, useRef, useState } from 'react';
import { api } from '../api/client';
import type { UIState, WNode } from '../api/types';
import { STATE_META } from '../lib/model';
import { roleColor, roleIcon, useRole, useRoleLookup } from '../lib/roles';
import { toast } from '../lib/toast';
import { useNodes, useSquad } from '../state/store';
import { Icon } from './Icon';
import './Composer.css';

interface Attachment {
  name: string;
  data: string;
  w: number;
  h: number;
}

interface MenuState {
  query: string;
  index: number;
}

const nodeName = (n: WNode) => n.terminal?.name ?? n.note?.title ?? n.portal?.name ?? 'Portal';

/** Nodes linked to `id` (edges are undirected). */
function neighbours(edges: { a: string; b: string }[], id: string): Set<string> {
  const out = new Set<string>();
  for (const e of edges) {
    if (e.a === id) out.add(e.b);
    if (e.b === id) out.add(e.a);
  }
  return out;
}

/** Composer text: chips become @Name, line breaks become \n. */
function serialize(root: HTMLElement): { text: string; mentions: string[] } {
  const mentions: string[] = [];
  let text = '';
  const walk = (n: Node) => {
    if (n.nodeType === Node.TEXT_NODE) {
      text += n.textContent ?? '';
      return;
    }
    const el = n as HTMLElement;
    if (el.dataset?.mention) {
      mentions.push(el.dataset.mention);
      text += `@${el.dataset.name}`;
      return;
    }
    if (el.tagName === 'BR') {
      text += '\n';
      return;
    }
    const block = el !== root && (el.tagName === 'DIV' || el.tagName === 'P');
    if (block && text && !text.endsWith('\n')) text += '\n';
    el.childNodes.forEach(walk);
  };
  walk(root);
  return { text: text.replace(/ /g, ' '), mentions: [...new Set(mentions)] };
}

/** Text between an "@" (at a word start) and the caret, if the caret is in a mention. */
function mentionQuery(): { node: Text; start: number; query: string } | null {
  const sel = window.getSelection();
  if (!sel || sel.rangeCount === 0 || !sel.isCollapsed) return null;
  const node = sel.anchorNode;
  if (!node || node.nodeType !== Node.TEXT_NODE) return null;
  const before = (node.textContent ?? '').slice(0, sel.anchorOffset);
  const m = before.match(/(?:^|[\s ])@([^\s@]*)$/);
  if (!m) return null;
  return { node: node as Text, start: before.length - m[1].length - 1, query: m[1] };
}

function chipIcon(n: WNode): string {
  if (n.note) return 'description';
  if (n.portal) return 'public';
  return '';
}

function readImage(file: File): Promise<Attachment> {
  return new Promise((resolve, reject) => {
    const r = new FileReader();
    r.onerror = () => reject(r.error);
    r.onload = () => {
      const data = r.result as string;
      const img = new Image();
      img.onload = () => resolve({ name: file.name || 'captura.png', data, w: img.naturalWidth, h: img.naturalHeight });
      img.onerror = () => resolve({ name: file.name || 'imagem', data, w: 0, h: 0 });
      img.src = data;
    };
    r.readAsDataURL(file);
  });
}

export function Composer() {
  const open = useSquad((s) => s.composerOpen);
  if (!open) return null;
  return <ComposerPanel />;
}

function ComposerPanel() {
  const nodes = useNodes();
  const roleOf = useRoleLookup();
  const edges = useSquad((s) => s.workspace?.edges);
  const runtime = useSquad((s) => s.runtime);
  const initial = useSquad((s) => s.composerTarget ?? s.focusedTerminal ?? s.selectedNode);
  const setComposer = useSquad((s) => s.setComposer);
  const editor = useRef<HTMLDivElement>(null);
  const fileInput = useRef<HTMLInputElement>(null);
  const [targets, setTargets] = useState<string[]>(() => {
    const n = nodes.find((x) => x.id === initial);
    return n?.terminal ? [n.id] : [];
  });
  const [menu, setMenu] = useState<MenuState | null>(null);
  const [picker, setPicker] = useState(false);
  const [files, setFiles] = useState<Attachment[]>([]);
  const [busy, setBusy] = useState(false);
  const [empty, setEmpty] = useState(true);

  useEffect(() => {
    const el = editor.current;
    if (!el) return;
    el.focus();
    // A suggested next step opens the Composer with its text.
    const draft = useSquad.getState().takeComposerDraft();
    if (draft) {
      el.textContent = draft;
      const range = document.createRange();
      range.selectNodeContents(el);
      range.collapse(false);
      const sel = window.getSelection();
      sel?.removeAllRanges();
      sel?.addRange(range);
      setEmpty(false);
    }
  }, []);

  const byId = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const terminals = nodes.filter((n) => n.terminal);

  // Mentionable: nodes linked to every target (handoff §7).
  const { linked, unlinked } = useMemo(() => {
    const es = edges ?? [];
    let common: string[] | null = null;
    for (const t of targets) {
      const nb = neighbours(es, t);
      const prev: string[] = common ?? [...nb];
      common = prev.filter((x) => nb.has(x));
    }
    const ok = (common ?? []).filter((id) => !targets.includes(id)).map((id) => byId.get(id)!).filter(Boolean);
    const rest = nodes.filter((n) => !targets.includes(n.id) && !ok.includes(n));
    return { linked: ok, unlinked: rest };
  }, [edges, targets, nodes, byId]);

  const q = menu?.query.toLowerCase() ?? '';
  // Names that start with the typed text come first.
  const rank = (n: WNode) => (nodeName(n).toLowerCase().startsWith(q) ? 0 : 1);
  const matches = linked.filter((n) => nodeName(n).toLowerCase().includes(q)).sort((a, b) => rank(a) - rank(b));
  const others = unlinked.filter((n) => nodeName(n).toLowerCase().includes(q));

  const close = () => setComposer(false);

  const refresh = () => {
    const root = editor.current;
    if (!root) return;
    setEmpty(serialize(root).text.trim() === '' && !root.querySelector('[data-mention]'));
    const mq = mentionQuery();
    setMenu(mq && targets.length > 0 ? { query: mq.query, index: 0 } : null);
  };

  const insertMention = (n: WNode) => {
    const mq = mentionQuery();
    if (!mq) return;
    const sel = window.getSelection()!;
    const range = document.createRange();
    range.setStart(mq.node, mq.start);
    range.setEnd(mq.node, sel.anchorOffset);
    range.deleteContents();
    const chip = document.createElement('span');
    chip.className = 'mention-chip';
    chip.contentEditable = 'false';
    chip.dataset.mention = n.id;
    chip.dataset.name = nodeName(n);
    const icon = chipIcon(n);
    if (icon) {
      const i = document.createElement('span');
      i.className = 'msi';
      i.textContent = icon;
      chip.appendChild(i);
    } else {
      const sq = document.createElement('i');
      sq.className = 'mention-square';
      sq.style.background = `var(--r-${roleColor(roleOf(n.terminal?.role))})`;
      chip.appendChild(sq);
    }
    chip.appendChild(document.createTextNode(nodeName(n)));
    const space = document.createTextNode(' ');
    range.insertNode(space);
    range.insertNode(chip);
    const after = document.createRange();
    after.setStartAfter(space);
    after.collapse(true);
    sel.removeAllRanges();
    sel.addRange(after);
    setMenu(null);
    refresh();
  };

  const send = async () => {
    const root = editor.current;
    if (!root || targets.length === 0 || busy) return;
    const { text, mentions } = serialize(root);
    if (!text.trim() && files.length === 0) return;
    setBusy(true);
    const images = files.map((f) => ({ name: f.name, data: f.data }));
    const results = await Promise.allSettled(targets.map((t) => api.send(t, { text, mentions, images })));
    setBusy(false);
    const failed = results.flatMap((r, i) => (r.status === 'rejected' ? [`${nodeName(byId.get(targets[i])!)}: ${(r.reason as Error).message}`] : []));
    if (failed.length) {
      toast(`Não transmitido · ${failed.join(' · ')}`);
      return;
    }
    toast(targets.length === 1 ? `Transmitido a ${nodeName(byId.get(targets[0])!)}` : `Transmitido a ${targets.length} voos`);
    root.innerHTML = '';
    setFiles([]);
    close();
  };

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (menu && (matches.length > 0 || e.key === 'Escape')) {
      if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
        e.preventDefault();
        const d = e.key === 'ArrowDown' ? 1 : -1;
        setMenu({ ...menu, index: (menu.index + d + matches.length) % Math.max(1, matches.length) });
        return;
      }
      if (e.key === 'Enter' || e.key === 'Tab') {
        e.preventDefault();
        insertMention(matches[Math.min(menu.index, matches.length - 1)]);
        return;
      }
      if (e.key === 'Escape') {
        e.preventDefault();
        e.stopPropagation();
        setMenu(null);
        return;
      }
    }
    if (e.key === 'Enter' && !e.shiftKey && !e.nativeEvent.isComposing) {
      e.preventDefault();
      void send();
      return;
    }
    if (e.key === 'Enter' && e.shiftKey) {
      e.preventDefault();
      document.execCommand('insertLineBreak');
    }
  };

  const addFiles = async (list: File[]) => {
    const imgs = list.filter((f) => f.type.startsWith('image/'));
    if (imgs.length === 0) return;
    const read = await Promise.all(imgs.map(readImage));
    setFiles((prev) => [...prev, ...read]);
  };

  const onPaste = (e: React.ClipboardEvent) => {
    const imgs = [...e.clipboardData.files].filter((f) => f.type.startsWith('image/'));
    e.preventDefault();
    if (imgs.length) {
      void addFiles(imgs);
      return;
    }
    // Plain text only: no foreign markup in the message.
    document.execCommand('insertText', false, e.clipboardData.getData('text/plain'));
  };

  const startMention = () => {
    const root = editor.current;
    if (!root) return;
    root.focus();
    const sel = window.getSelection();
    if (sel && !root.contains(sel.anchorNode)) {
      const r = document.createRange();
      r.selectNodeContents(root);
      r.collapse(false);
      sel.removeAllRanges();
      sel.addRange(r);
    }
    const prev = sel?.anchorNode?.textContent?.slice(0, sel.anchorOffset) ?? '';
    document.execCommand('insertText', false, prev && !/\s$/.test(prev) ? ' @' : '@');
    refresh();
  };

  const states = targets.map((t) => runtime[t]?.activity ?? 'off');
  const notice = targetNotice(states, runtime, targets);
  const canSend = targets.length > 0 && !busy && (!empty || files.length > 0) && !notice?.blocking;

  return (
    <div className="composer" role="dialog" aria-label="Transmitir">
      {menu && targets.length > 0 && (
        <div className="mention-menu popover">
          <div className="mm-section">Na frequência de {targets.length === 1 ? nodeName(byId.get(targets[0])!) : 'todos os destinos'}</div>
          {matches.length === 0 && <div className="mm-empty">Nada na frequência com esse nome.</div>}
          {matches.map((n, i) => (
            <button
              key={n.id}
              className={`mm-item${i === menu.index ? ' on' : ''}`}
              onMouseDown={(e) => {
                e.preventDefault();
                insertMention(n);
              }}
            >
              <NodeGlyph n={n} />
              <Highlight text={nodeName(n)} query={menu.query} />
              <span className="tn-spacer" />
              {n.terminal && <MiniState state={runtime[n.id]?.activity ?? 'off'} />}
              {n.note && <span className="mono mm-meta">{n.note.file}</span>}
            </button>
          ))}
          {others.length > 0 && (
            <>
              <div className="mm-section">Fora da frequência</div>
              {others.slice(0, 5).map((n) => (
                <div key={n.id} className="mm-item disabled" title="Ponha este nó na frequência do destino para mencioná-lo">
                  <NodeGlyph n={n} />
                  {nodeName(n)}
                  <span className="tn-spacer" />
                  <Icon name="link_off" size={15} />
                  <span className="mm-meta">ligue para mencionar</span>
                </div>
              ))}
            </>
          )}
        </div>
      )}
      <div className="cmp-to">
        <span className="cmp-label">Para</span>
        {targets.map((id) => {
          const n = byId.get(id);
          if (!n?.terminal) return null;
          const role = roleOf(n.terminal.role);
          const r = roleColor(role);
          return (
            <span key={id} className="cmp-target" style={{ background: `var(--rb-${r})`, color: `var(--r-${r})` }}>
              <Icon name={role ? roleIcon(role) : 'terminal'} size={14} />
              <span className="cmp-target-name">{n.terminal.name}</span>
              <span className="mono cmp-target-agent">{n.terminal.agent}</span>
              <button className="cmp-x" onClick={() => setTargets(targets.filter((x) => x !== id))} aria-label={`Remover ${n.terminal.name}`}>
                <Icon name="close" size={14} />
              </button>
            </span>
          );
        })}
        <div style={{ position: 'relative' }}>
          <button className="cmp-add" onClick={() => setPicker(!picker)}>
            <Icon name="add" size={15} />
            destino
          </button>
          {picker && (
            <div className="popover cmp-picker" onMouseLeave={() => setPicker(false)}>
              {terminals.filter((n) => !targets.includes(n.id)).length === 0 && <div className="mm-empty">Todos os voos já são destino.</div>}
              {terminals
                .filter((n) => !targets.includes(n.id))
                .map((n) => (
                  <button
                    key={n.id}
                    className="mm-item"
                    onClick={() => {
                      setTargets([...targets, n.id]);
                      setPicker(false);
                      editor.current?.focus();
                    }}
                  >
                    <NodeGlyph n={n} />
                    {n.terminal!.name}
                    <span className="tn-spacer" />
                    <MiniState state={runtime[n.id]?.activity ?? 'off'} />
                  </button>
                ))}
            </div>
          )}
        </div>
        {notice && (
          <span className="cmp-notice" style={{ color: `var(--stt-${notice.state})` }}>
            {notice.text}
          </span>
        )}
        <span className="tn-spacer" />
        <button className="icon-btn" onClick={close} aria-label="Fechar (Esc)">
          <Icon name="close" size={18} />
        </button>
      </div>
      <div
        ref={editor}
        className={`cmp-body${empty ? ' empty' : ''}`}
        contentEditable
        suppressContentEditableWarning
        role="textbox"
        aria-multiline="true"
        data-placeholder={targets.length ? 'Escreva para o voo… use @ para mencionar o que está na frequência' : 'Escolha um destino em "+ destino"'}
        onInput={refresh}
        onKeyDown={onKeyDown}
        onKeyUp={(e) => (e.key.startsWith('Arrow') && !menu ? refresh() : undefined)}
        onClick={refresh}
        onPaste={onPaste}
        onDrop={(e) => {
          if (e.dataTransfer.files.length) {
            e.preventDefault();
            void addFiles([...e.dataTransfer.files]);
          }
        }}
      />
      {files.length > 0 && (
        <div className="cmp-files">
          {files.map((f, i) => (
            <div key={i} className="cmp-file">
              <img src={f.data} alt="" />
              <div className="cmp-file-meta">
                <span className="cmp-file-name">{f.name}</span>
                <span className="mono cmp-file-sub">{f.w ? `${f.w}×${f.h} · colada` : 'colada'}</span>
              </div>
              <button className="cmp-x" onClick={() => setFiles(files.filter((_, j) => j !== i))} aria-label="Remover imagem">
                <Icon name="close" size={14} />
              </button>
            </div>
          ))}
        </div>
      )}
      <div className="cmp-foot">
        <button className="icon-btn" onClick={() => fileInput.current?.click()} title="Anexar imagem (ou cole com ⌘V)">
          <Icon name="image" size={19} />
        </button>
        <button className="icon-btn" onClick={startMention} title="Mencionar o que está na frequência (@)" disabled={targets.length === 0}>
          <Icon name="alternate_email" size={19} />
        </button>
        <input
          ref={fileInput}
          type="file"
          accept="image/*"
          multiple
          hidden
          onChange={(e) => {
            void addFiles([...(e.target.files ?? [])]);
            e.target.value = '';
          }}
        />
        <span className="cmp-hint mono">⏎ enviar · ⇧⏎ nova linha · Esc fecha</span>
        <span className="tn-spacer" />
        <button className="cmp-send" onClick={() => void send()} disabled={!canSend}>
          {targets.length > 1 ? `Transmitir a ${targets.length}` : 'Transmitir'}
          <Icon name="arrow_upward" size={17} />
        </button>
      </div>
    </div>
  );
}

function targetNotice(states: UIState[], runtime: Record<string, { status?: string }>, targets: string[]): { text: string; state: UIState; blocking?: boolean } | null {
  if (targets.some((t) => runtime[t]?.status !== 'running')) return { text: 'hangar · inicie o voo', state: 'err', blocking: true };
  if (states.includes('work')) return { text: 'no ar · entra na fila', state: 'work' };
  if (states.includes('wait')) return { text: 'aguarda · entra na fila', state: 'wait' };
  if (states.includes('boot')) return { text: 'taxiando', state: 'boot' };
  return null;
}

function NodeGlyph({ n }: { n: WNode }) {
  const role = useRole(n.terminal?.role);
  if (n.note) return <Icon name="description" size={16} />;
  if (n.portal) return <Icon name="public" size={16} />;
  return <i className="mention-square" style={{ background: `var(--r-${roleColor(role)})` }} />;
}

function MiniState({ state }: { state: UIState }) {
  return (
    <span className="mm-state" style={{ color: `var(--stt-${state})` }}>
      <Icon name={STATE_META[state].icon} size={14} />
      {STATE_META[state].label}
    </span>
  );
}

function Highlight({ text, query }: { text: string; query: string }) {
  const i = query ? text.toLowerCase().indexOf(query.toLowerCase()) : -1;
  if (i < 0) return <span>{text}</span>;
  return (
    <span>
      {text.slice(0, i)}
      <u>{text.slice(i, i + query.length)}</u>
      {text.slice(i + query.length)}
    </span>
  );
}
