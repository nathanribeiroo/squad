// A small Markdown renderer for notes: headings, lists, task lists, code
// fences, paragraphs and inline code/bold/italic/links. It builds React
// elements (no HTML injection) and keeps source line numbers so task boxes
// can be toggled and agent edits highlighted.
import type { ReactNode } from 'react';
import { Icon } from '../components/Icon';

type Block =
  | { kind: 'h'; level: number; text: string; line: number }
  | { kind: 'task'; done: boolean; text: string; line: number; indent: number }
  | { kind: 'li'; text: string; line: number; indent: number; n?: string }
  | { kind: 'code'; text: string; line: number; end: number }
  | { kind: 'p'; text: string; line: number; end: number }
  | { kind: 'gap'; line: number };

function parse(src: string): Block[] {
  const lines = src.replace(/\r\n/g, '\n').split('\n');
  const out: Block[] = [];
  for (let i = 0; i < lines.length; i++) {
    const l = lines[i];
    if (/^\s*```/.test(l)) {
      const start = i;
      const body: string[] = [];
      for (i++; i < lines.length && !/^\s*```/.test(lines[i]); i++) body.push(lines[i]);
      out.push({ kind: 'code', text: body.join('\n'), line: start, end: Math.min(i, lines.length - 1) });
      continue;
    }
    let m = l.match(/^(#{1,4})\s+(.*)$/);
    if (m) {
      out.push({ kind: 'h', level: m[1].length, text: m[2], line: i });
      continue;
    }
    m = l.match(/^(\s*)[-*+]\s+\[( |x|X)\]\s*(.*)$/);
    if (m) {
      out.push({ kind: 'task', done: m[2] !== ' ', text: m[3], line: i, indent: m[1].length });
      continue;
    }
    m = l.match(/^(\s*)[-*+]\s+(.*)$/);
    if (m) {
      out.push({ kind: 'li', text: m[2], line: i, indent: m[1].length });
      continue;
    }
    m = l.match(/^(\s*)(\d+)[.)]\s+(.*)$/);
    if (m) {
      out.push({ kind: 'li', text: m[3], line: i, indent: m[1].length, n: m[2] });
      continue;
    }
    if (l.trim() === '') {
      out.push({ kind: 'gap', line: i });
      continue;
    }
    const prev = out[out.length - 1];
    if (prev?.kind === 'p' && prev.end === i - 1) {
      prev.text += ' ' + l.trim();
      prev.end = i;
    } else out.push({ kind: 'p', text: l.trim(), line: i, end: i });
  }
  return out;
}

const INLINE = /(`[^`]+`)|(\*\*[^*]+\*\*)|(\*[^*\s][^*]*\*|_[^_\s][^_]*_)|(\[[^\]]+\]\([^)\s]+\))/g;

function inline(text: string): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  let k = 0;
  for (const m of text.matchAll(INLINE)) {
    if (m.index! > last) out.push(text.slice(last, m.index));
    const t = m[0];
    if (m[1]) out.push(<code key={k++}>{t.slice(1, -1)}</code>);
    else if (m[2]) out.push(<strong key={k++}>{t.slice(2, -2)}</strong>);
    else if (m[3]) out.push(<em key={k++}>{t.slice(1, -1)}</em>);
    else {
      const lm = t.match(/^\[([^\]]+)\]\(([^)\s]+)\)$/)!;
      const safe = /^(https?:|mailto:)/i.test(lm[2]);
      out.push(
        safe ? (
          <a key={k++} href={lm[2]} target="_blank" rel="noreferrer noopener">
            {lm[1]}
          </a>
        ) : (
          <span key={k++}>{lm[1]}</span>
        ),
      );
    }
    last = m.index! + t.length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

export interface MarkdownHighlight {
  from: number;
  to: number;
  label: ReactNode;
  role: string;
}

export function Markdown({ text, highlight, onToggleTask }: { text: string; highlight?: MarkdownHighlight | null; onToggleTask?: (line: number) => void }) {
  const blocks = parse(text);
  const render = (b: Block, key: number): ReactNode => {
    switch (b.kind) {
      case 'h':
        return (
          <div key={key} className={`md-h md-h${b.level}`}>
            {inline(b.text)}
          </div>
        );
      case 'task':
        return (
          <div key={key} className="md-task" style={{ paddingLeft: b.indent * 8 }}>
            <button className="md-box" onClick={() => onToggleTask?.(b.line)} disabled={!onToggleTask} aria-label={b.done ? 'Desmarcar' : 'Marcar'}>
              <Icon name={b.done ? 'check_box' : 'check_box_outline_blank'} size={18} fill={b.done} style={{ color: b.done ? 'var(--st-done)' : 'var(--text-3)' }} />
            </button>
            <span className={b.done ? 'md-done' : ''}>{inline(b.text)}</span>
          </div>
        );
      case 'li':
        return (
          <div key={key} className="md-li" style={{ paddingLeft: 4 + b.indent * 8 }}>
            <span className="md-bullet">{b.n ? `${b.n}.` : '•'}</span>
            <span>{inline(b.text)}</span>
          </div>
        );
      case 'code':
        return (
          <pre key={key} className="md-code">
            {b.text}
          </pre>
        );
      case 'p':
        return (
          <p key={key} className="md-p">
            {inline(b.text)}
          </p>
        );
      case 'gap':
        return <div key={key} className="md-gap" />;
    }
  };

  if (!highlight) return <div className="md">{blocks.map(render)}</div>;
  const end = (b: Block) => ('end' in b ? b.end : b.line);
  const before = blocks.filter((b) => end(b) < highlight.from);
  const inside = blocks.filter((b) => end(b) >= highlight.from && b.line <= highlight.to);
  const after = blocks.filter((b) => b.line > highlight.to);
  return (
    <div className="md">
      {before.map(render)}
      <div className="md-edited" style={{ background: `var(--rb-${highlight.role})` }}>
        <div className="md-edited-label" style={{ color: `var(--r-${highlight.role})` }}>
          <Icon name="edit" size={13} />
          {highlight.label}
        </div>
        {inside.map(render)}
      </div>
      {after.map(render)}
    </div>
  );
}

/** Lines changed between two versions (common prefix/suffix), or null. */
export function changedLines(prev: string, next: string): { from: number; to: number } | null {
  if (prev === next) return null;
  const a = prev.split('\n');
  const b = next.split('\n');
  let s = 0;
  while (s < a.length && s < b.length && a[s] === b[s]) s++;
  let ea = a.length - 1;
  let eb = b.length - 1;
  while (ea >= s && eb >= s && a[ea] === b[eb]) {
    ea--;
    eb--;
  }
  if (eb < s) return null; // only deletions
  return { from: s, to: eb };
}
