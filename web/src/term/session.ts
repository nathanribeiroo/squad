// One live terminal per node: an xterm.js instance fed by /ws/term/{id}.
// Sessions live outside React so zoom-level changes (which swap the node's
// markup) never drop the buffer or the socket; the node only lends a DOM slot.
//
// Renderer: xterm's DOM renderer. The WebGL addon sizes its canvas from a
// device-pixel-content-box ResizeObserver, which Chrome misreports under a
// scaled ancestor (the canvas zoom): at DPR 2 the canvas came out at half
// resolution and the grid was drawn at twice its size and cropped.
import { Terminal } from '@xterm/xterm';
import { Unicode11Addon } from '@xterm/addon-unicode11';
import { SerializeAddon } from '@xterm/addon-serialize';
import '@xterm/xterm/css/xterm.css';
import { wsPath, wsURL } from '../api/client';
import { useSquad } from '../state/store';
import { noite, papel } from '../theme/xterm-themes';

/** Terminal type at zoom 1 (handoff: Geist Mono 11/15). */
export const BASE_FONT = 11;
const LINE_PX = 15;
const FONT_FAMILY = "'Geist Mono Variable', 'Geist Mono', ui-monospace, monospace";

const themeOf = () => (useSquad.getState().termTheme === 'papel' ? papel : noite);

/** Keys the canvas owns even while a terminal has the keyboard. */
function canvasKey(e: KeyboardEvent): boolean {
  if (!e.altKey || e.ctrlKey || e.metaKey) return false;
  if (e.shiftKey && (e.code === 'ArrowLeft' || e.code === 'ArrowRight')) return true; // dock
  return e.code === 'KeyA' || e.code === 'KeyP' || e.code === 'KeyK' || e.code === 'KeyF' || e.code === 'KeyM' || e.key === 'Escape' || /^Digit[1-9]$/.test(e.code);
}

let fontsReady: Promise<unknown> | null = null;
function loadFonts(): Promise<unknown> {
  fontsReady ??= Promise.all([
    document.fonts.load(`${BASE_FONT}px 'Geist Mono Variable'`),
    document.fonts.load(`bold ${BASE_FONT}px 'Geist Mono Variable'`),
  ]).catch(() => undefined);
  return fontsReady;
}

/**
 * A slot laid out zoom times larger than its node's body and scaled back by
 * 1/zoom: at that zoom xterm sees no transform at all (text at screen
 * resolution, mouse cells right; xterm doesn't support scaled ancestors).
 */
function styleSlot(slot: HTMLElement, zoom: number) {
  slot.style.width = `calc((100% - 20px) * ${zoom})`;
  slot.style.height = `calc((100% - 16px) * ${zoom})`;
  slot.style.transform = `scale(${1 / zoom})`;
}

let cell: { w: number; h: number; natural: number } | null = null;

/**
 * Exact advance and line height of the terminal font at the base size. The
 * rendered grid rounds cells to device pixels (more so at small zoom), so it
 * is a poor ruler for sizing a node's terminal.
 */
function baseCell(): { w: number; h: number; natural: number } {
  if (cell) return cell;
  const ctx = document.createElement('canvas').getContext('2d')!;
  ctx.font = `${BASE_FONT}px ${FONT_FAMILY}`;
  const w = ctx.measureText('W').width;
  const probe = document.createElement('span');
  probe.textContent = 'W';
  probe.style.cssText = `position:absolute;visibility:hidden;white-space:pre;line-height:normal;font:${BASE_FONT}px ${FONT_FAMILY}`;
  document.body.appendChild(probe);
  const natural = probe.offsetHeight || LINE_PX;
  probe.remove();
  cell = { w, h: LINE_PX, natural };
  return cell;
}

export class TermSession {
  readonly id: string;
  readonly term: Terminal;
  /** Stable element that moves between the node's slots. */
  readonly host: HTMLDivElement;
  private serializer = new SerializeAddon();
  private ws: WebSocket | null = null;
  private opened = false;
  private opening = false;
  private disposed = false;
  private retry = 0;
  private timer: number | undefined;
  /** The zoom the terminal is laid out for (its slot and its type). */
  private zoom = 1;
  /** The canvas zoom now. */
  private wantZoom = 1;
  private enc = new TextEncoder();
  private markReady!: () => void;
  /** Resolves once the terminal is open and laid out (cell size known). */
  readonly ready = new Promise<void>((r) => (this.markReady = r));

  constructor(id: string) {
    this.id = id;
    this.host = document.createElement('div');
    this.host.className = 'xterm-host';
    this.term = new Terminal({
      fontFamily: FONT_FAMILY,
      fontSize: BASE_FONT,
      lineHeight: 1,
      allowProposedApi: true,
      cursorBlink: false,
      scrollback: 5000,
      theme: themeOf(),
      macOptionIsMeta: false,
      rescaleOverlappingGlyphs: true,
    });
    this.term.loadAddon(new Unicode11Addon());
    this.term.unicode.activeVersion = '11';
    this.term.loadAddon(this.serializer);
    this.term.onData((d) => this.send(this.enc.encode(d)));
    this.term.onBinary((d) => this.send(Uint8Array.from(d, (c) => c.charCodeAt(0) & 255)));
    this.term.attachCustomKeyEventHandler((e) => {
      if (!canvasKey(e)) return true;
      // ⌥Esc hands the keyboard back to the canvas; ⌥A/⌥P/⌥K reach the app.
      if (e.key === 'Escape' && e.type === 'keydown') this.term.blur();
      return false;
    });
    this.connect();
  }

  // ---- socket ----

  private connect() {
    if (this.disposed) return;
    const ws = new WebSocket(wsURL(wsPath(`/ws/term/${this.id}`)));
    ws.binaryType = 'arraybuffer';
    this.ws = ws;
    ws.onmessage = (m) => {
      if (typeof m.data === 'string') {
        try {
          const msg = JSON.parse(m.data) as { type: string; cols?: number; rows?: number };
          if (msg.type === 'init') {
            this.retry = 0;
            // A fresh render of the screen follows; start from a clean slate.
            this.term.reset();
            this.resize(msg.cols ?? 0, msg.rows ?? 0);
          }
        } catch {
          /* ignore */
        }
        return;
      }
      this.term.write(new Uint8Array(m.data as ArrayBuffer));
    };
    ws.onclose = () => {
      if (this.ws !== ws) return;
      this.ws = null;
      if (this.disposed || useSquad.getState().quitting) return;
      this.retry++;
      this.timer = window.setTimeout(() => this.connect(), Math.min(5000, 250 * this.retry));
    };
  }

  /** Reconnect now (the terminal was just started or restarted). */
  kick() {
    if (this.disposed) return;
    window.clearTimeout(this.timer);
    if (this.ws && this.ws.readyState <= WebSocket.OPEN) {
      // A restart replaces the process; the old stream ends by itself.
      return;
    }
    this.retry = 0;
    this.connect();
  }

  private send(data: Uint8Array) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(data);
  }

  // ---- layout ----

  /** Follows the PTY size (from init or a node.updated). */
  resize(cols: number, rows: number) {
    if (cols > 0 && rows > 0 && (cols !== this.term.cols || rows !== this.term.rows)) {
      this.term.resize(cols, rows);
      this.layout();
    }
  }

  /**
   * The canvas zoom (see XTermView). A terminal laid out for another zoom is
   * only scaled: its text is drawn by the page, so it stays sharp, and laying
   * it out again redraws every row. It is laid out for the new zoom when it
   * is used (fitZoom: xterm maps the mouse to cells as if unscaled), or when
   * the zoom is far from its own.
   */
  setZoom(z: number) {
    if (z === this.wantZoom) return;
    this.wantZoom = z;
    const ratio = z / this.zoom;
    if (ratio < 0.7 || ratio > 1.4) this.layout();
  }

  /** Laid out for the canvas zoom now, at once (before a click or the keyboard). */
  fitZoom() {
    if (this.opened && this.zoom !== this.wantZoom) {
      layoutQueue.delete(this);
      this.layoutNow();
    }
  }

  private layoutGen = 0;

  /** Re-lays the terminal out for its zoom and slot, in its turn (see runLayouts). */
  private layout() {
    if (this.opened) scheduleLayout(this);
  }

  /** On screen now: laid out before the others. */
  onScreen(): boolean {
    const r = this.host.getBoundingClientRect();
    return r.width > 0 && r.right > 0 && r.bottom > 0 && r.left < window.innerWidth && r.top < window.innerHeight;
  }

  /**
   * Type size = 11 × zoom, so text is drawn at screen resolution, and a row
   * 15 × zoom (handoff 11/15), both worked out from the font's metrics at the
   * base size and set at once: each change of type makes xterm measure every
   * glyph again and redraw every row. One check once it is drawn corrects
   * what the font rounds differently.
   */
  layoutNow() {
    const slot = this.host.parentElement;
    if (!this.opened || !slot) return;
    const gen = ++this.layoutGen;
    this.zoom = this.wantZoom;
    styleSlot(slot, this.zoom);
    const { w, natural } = baseCell();
    const rows = this.term.rows || 1;
    let size = BASE_FONT * this.zoom;
    // A node smaller than its grid: smaller type.
    const wide = ((this.term.cols || 80) * w * size) / BASE_FONT;
    if (wide > slot.clientWidth) size *= (slot.clientWidth / wide) * 0.995;
    const want = Math.min(LINE_PX * this.zoom, slot.clientHeight / rows);
    // xterm rounds the glyph's height up to device pixels, then the row down.
    const dpr = window.devicePixelRatio || 1;
    let lh = (want * dpr) / Math.max(1, Math.ceil(((natural * size) / BASE_FONT) * dpr));
    if (lh < 1) {
      // Rows can't get shorter than the type; use a smaller type instead.
      size *= lh;
      lh = 1;
    }
    this.setType(size, lh);
    requestAnimationFrame(() => this.check(gen, slot, 2));
  }

  private setType(size: number, lh: number) {
    const o = this.term.options;
    if (o.lineHeight !== lh) o.lineHeight = lh;
    if (o.fontSize !== size) o.fontSize = size;
  }

  /** Corrects the type from the drawn grid: it must fit the slot, rows 15 × zoom. */
  private check(gen: number, slot: HTMLElement, tries: number) {
    const screen = this.term.element?.querySelector<HTMLElement>('.xterm-screen');
    if (gen !== this.layoutGen || !screen || tries <= 0 || screen.offsetHeight === 0) return;
    const rows = this.term.rows || 1;
    let size = this.term.options.fontSize ?? BASE_FONT;
    let lh = this.term.options.lineHeight ?? 1;
    if (screen.offsetWidth > slot.clientWidth + 0.5) {
      size *= (slot.clientWidth / screen.offsetWidth) * 0.995;
    } else {
      const want = Math.min(LINE_PX * this.zoom, slot.clientHeight / rows);
      const got = screen.offsetHeight / rows;
      if (got <= want && got > want - 1) return;
      const next = (lh * want) / got;
      if (next < 1) {
        lh = 1;
        size *= want / got;
      } else {
        lh = next;
      }
    }
    this.setType(size, lh);
    requestAnimationFrame(() => this.check(gen, slot, tries - 1));
  }

  /** Cell size in world units at the base font (for node resizing). */
  cellSize(): { w: number; h: number } | null {
    return this.opened ? baseCell() : null;
  }

  // ---- DOM ----

  private slotObserver: ResizeObserver | null = null;

  /** Places the terminal in a slot (live zoom level), at the canvas zoom. */
  attach(slot: HTMLElement, zoom: number) {
    // An open terminal keeps the type it has until its turn to be laid out
    // again: its slot matches that type meanwhile.
    this.wantZoom = zoom;
    if (!this.opened) this.zoom = zoom;
    styleSlot(slot, this.zoom);
    slot.appendChild(this.host);
    // A click lands on the cell under the pointer only at the terminal's own zoom.
    slot.addEventListener('pointerdown', this.fitOnPointer, true);
    // The slot shrinks when a sender strip appears or the node is resized.
    this.slotObserver?.disconnect();
    let last = '';
    this.slotObserver = new ResizeObserver(() => {
      const size = `${slot.clientWidth}x${slot.clientHeight}`;
      if (size !== last) {
        last = size;
        this.layout();
      }
    });
    this.slotObserver.observe(slot);
    if (this.opened) {
      this.layout();
      return;
    }
    if (this.opening) return;
    this.opening = true;
    void loadFonts().then(() => {
      if (this.disposed) return;
      // Line height 15 at size 11 (the font's natural line is shorter).
      this.term.options.lineHeight = LINE_PX / baseCell().natural;
      this.term.open(this.host);
      this.opened = true;
      const ta = this.term.textarea;
      ta?.addEventListener('focus', () => useSquad.getState().setFocusedTerminal(this.id));
      ta?.addEventListener('blur', () => {
        if (useSquad.getState().focusedTerminal === this.id) useSquad.getState().setFocusedTerminal(null);
      });
      this.layout();
      requestAnimationFrame(() => requestAnimationFrame(() => this.markReady()));
    });
  }

  private fitOnPointer = () => this.fitZoom();

  detach(slot: HTMLElement) {
    slot.removeEventListener('pointerdown', this.fitOnPointer, true);
    this.slotObserver?.disconnect();
    this.slotObserver = null;
    if (this.host.parentElement === slot) slot.removeChild(this.host);
  }

  focus() {
    this.fitZoom();
    this.term.focus();
  }

  /** Types text as a paste (bracketed when the program asked for it), never
   * with Enter. false: not connected. */
  paste(text: string): boolean {
    if (this.ws?.readyState !== WebSocket.OPEN) return false;
    this.term.paste(text);
    return true;
  }

  setTheme() {
    this.term.options.theme = themeOf();
  }

  /** Visible screen as HTML (miniature level). */
  snapshotHTML(): string {
    return this.serializer.serializeAsHTML({ scrollback: 0, includeGlobalBackground: true });
  }

  dispose() {
    this.disposed = true;
    layoutQueue.delete(this);
    this.slotObserver?.disconnect();
    window.clearTimeout(this.timer);
    const ws = this.ws;
    this.ws = null;
    ws?.close();
    this.term.dispose();
    this.host.remove();
  }
}

// Terminals waiting to be laid out again (after a zoom, a resize, a level
// change): one per frame, the ones on screen first. Each one measures its
// glyphs and redraws its rows; all of them in the same frame froze the canvas.
const layoutQueue = new Set<TermSession>();
let layoutFrame = 0;

function scheduleLayout(s: TermSession) {
  layoutQueue.add(s);
  layoutFrame ||= requestAnimationFrame(runLayouts);
}

function runLayouts() {
  layoutFrame = 0;
  let next: TermSession | undefined;
  for (const s of layoutQueue) {
    next ??= s;
    if (s.onScreen()) {
      next = s;
      break;
    }
  }
  if (!next) return;
  layoutQueue.delete(next);
  next.layoutNow();
  if (layoutQueue.size) layoutFrame = requestAnimationFrame(runLayouts);
}

const sessions = new Map<string, TermSession>();

export function termSession(id: string): TermSession {
  let s = sessions.get(id);
  if (!s) {
    s = new TermSession(id);
    sessions.set(id, s);
  }
  return s;
}

/** Drops sessions of terminals that left the workspace. */
export function pruneSessions(keep: Set<string>) {
  for (const [id, s] of sessions) {
    if (!keep.has(id)) {
      s.dispose();
      sessions.delete(id);
    }
  }
}

useSquad.subscribe((st, prev) => {
  if (st.termTheme !== prev.termTheme) for (const s of sessions.values()) s.setTheme();
  // Another workspace on screen: its terminals get new sessions.
  if (st.activeWs !== prev.activeWs) pruneSessions(new Set());
});
