// A wire hangs from its ends and bend points with a small sag, a few px
// whatever its length. At rest it takes the shortest way: straight, or round
// the corners of the nodes in its way, on the short side (so it can't be
// wound round a node). That pose is worked out, not simulated. When
// something moves, the wire is a chain of Verlet particles under gravity:
// the chain left over when its ends come closer falls and swings while it
// is reeled in, and then the chain springs back to its rest pose and stops
// there, so a wire never grows and never freezes halfway.
import type { Pt, Rect } from './geometry';

/** A node the chain can't pass through; own: one of the wire's ends. */
export type Solid = Rect & { own?: boolean };

/** Sag at the middle of a level span (canvas px); less as it gets steeper. */
const SAG = 8;
/** Target length of a link (canvas px); a span keeps it between half and double. */
const LINK = 18;
const GRAVITY = 0.45;
const DAMPING = 0.985;
const ITERATIONS = 10;
/** Gap kept between a wire and a node's border. */
export const MARGIN = 6;
/** Average motion (px per step) under which the chain counts as still. */
export const STILL = 0.03;
/** The simulation's step: the same on any screen, whatever its frame rate. */
export const STEP_MS = 1000 / 60;
/** Pull toward the rest pose, per step, once nothing is left over. */
const SPRING = 0.12;
/** Damping added while it springs back: a small overshoot, then still. */
const SPRING_DAMPING = 0.12;
/** Share of the chain left over that is reeled in per step, and at least this (px). */
const REEL = 0.06;
const REEL_MIN = 0.3;
/** Most chain left over when the ends come closer (px, plus a share of the span). */
const LEFT_PX = 12;
const LEFT_SHARE = 0.1;
/** Chain left over (px, plus a share of the span) above which it hangs free. */
const FREE_PX = 6;
const FREE_SHARE = 0.04;
/** A step's slack under this (px) and the chain this close to its rest pose: settled. */
const SETTLED = 0.5;
/** Shrinks a node by this so that touching its border isn't running through it. */
const EPS = 0.5;

/** Constraint passes a step makes: long chains need more, or their links stay stretched. */
const passesFor = (count: number) => Math.max(ITERATIONS, Math.ceil(count / 4));

/** Points evenly spaced along a polyline, by length. */
function resample(pts: Pt[], count: number): Pt[] {
  const len: number[] = [0];
  for (let i = 1; i < pts.length; i++) len.push(len[i - 1] + Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y));
  const total = len[len.length - 1];
  const out: Pt[] = [];
  let j = 0;
  for (let k = 0; k <= count; k++) {
    const at = total === 0 ? 0 : (total * k) / count;
    while (j < pts.length - 2 && len[j + 1] < at) j++;
    const seg = len[j + 1] - len[j];
    const u = seg === 0 ? 0 : (at - len[j]) / seg;
    out.push({ x: pts[j].x + (pts[j + 1].x - pts[j].x) * u, y: pts[j].y + (pts[j + 1].y - pts[j].y) * u });
  }
  return out;
}

function pathLength(pts: Pt[]): number {
  let n = 0;
  for (let i = 1; i < pts.length; i++) n += Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y);
  return n;
}

/** Inside r, not on its border. */
function within(p: Pt, r: Rect): boolean {
  return p.x > r.x + EPS && p.x < r.x + r.w - EPS && p.y > r.y + EPS && p.y < r.y + r.h - EPS;
}

/** Whether the segment a→b runs through r (touching its border or a corner is fine). */
function crosses(a: Pt, b: Pt, r: Rect): boolean {
  const x0 = r.x + EPS;
  const x1 = r.x + r.w - EPS;
  const y0 = r.y + EPS;
  const y1 = r.y + r.h - EPS;
  if (x1 <= x0 || y1 <= y0) return false;
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  let t0 = 0;
  let t1 = 1;
  // Liang–Barsky: clip the segment to the rectangle, side by side.
  for (const [p, q] of [
    [-dx, a.x - x0],
    [dx, x1 - a.x],
    [-dy, a.y - y0],
    [dy, y1 - a.y],
  ]) {
    if (p === 0) {
      if (q < 0) return false;
      continue;
    }
    const t = q / p;
    if (p < 0) t0 = Math.max(t0, t);
    else t1 = Math.min(t1, t);
    if (t0 >= t1) return false;
  }
  return t1 - t0 > 1e-9;
}

/**
 * The shortest way from p to q round the rectangles, by their corners (a
 * visibility graph). Straight when nothing is in the way; a rectangle that
 * holds p or q doesn't count (the wire can't get out of it anyway).
 */
function shortest(p: Pt, q: Pt, rects: Rect[]): Pt[] {
  const live = rects.filter((r) => !within(p, r) && !within(q, r));
  if (!live.some((r) => crosses(p, q, r))) return [p, q];
  const pts: Pt[] = [p, q];
  for (const r of live) {
    for (const c of [
      { x: r.x - 1, y: r.y - 1 },
      { x: r.x + r.w + 1, y: r.y - 1 },
      { x: r.x + r.w + 1, y: r.y + r.h + 1 },
      { x: r.x - 1, y: r.y + r.h + 1 },
    ]) {
      if (!live.some((o) => within(c, o))) pts.push(c);
    }
  }
  const n = pts.length;
  const dist = new Array<number>(n).fill(Infinity);
  const prev = new Array<number>(n).fill(-1);
  const done = new Array<boolean>(n).fill(false);
  dist[0] = 0;
  for (;;) {
    let u = -1;
    for (let i = 0; i < n; i++) if (!done[i] && dist[i] < Infinity && (u < 0 || dist[i] < dist[u])) u = i;
    if (u < 0 || u === 1) break;
    done[u] = true;
    for (let v = 0; v < n; v++) {
      if (done[v]) continue;
      const d = dist[u] + Math.hypot(pts[v].x - pts[u].x, pts[v].y - pts[u].y);
      // Whether it can see v is asked only when it would be a shorter way.
      if (d < dist[v] && !live.some((r) => crosses(pts[u], pts[v], r))) {
        dist[v] = d;
        prev[v] = u;
      }
    }
  }
  if (dist[1] === Infinity) return [p, q];
  const way: Pt[] = [];
  for (let i = 1; i >= 0; i = prev[i]) way.unshift(pts[i]);
  return way;
}

/**
 * A way with its sag: each straight stretch hangs SAG px at the middle when
 * level, less as it gets steeper, and not at all where it would sink into a
 * node (a stretch lying along a node's top).
 */
function sagged(way: Pt[], rects: Rect[]): Pt[] {
  const out: Pt[] = [way[0]];
  for (let i = 0; i < way.length - 1; i++) {
    const a = way[i];
    const b = way[i + 1];
    const len = Math.hypot(b.x - a.x, b.y - a.y);
    const m = Math.max(2, Math.ceil(len / 9));
    const at = (k: number, h: number) => {
      const u = k / m;
      return { x: a.x + (b.x - a.x) * u, y: a.y + (b.y - a.y) * u + 4 * h * u * (1 - u) };
    };
    let h = len < 1 ? 0 : Math.min((SAG * Math.abs(b.x - a.x)) / len, len * 0.1);
    for (let k = 1; k < m && h > 0; k++) if (rects.some((r) => within(at(k, h), r))) h = 0;
    for (let k = 1; k <= m; k++) out.push(at(k, h));
  }
  return out;
}

const grown = (r: Solid): Solid => ({ x: r.x - MARGIN, y: r.y - MARGIN, w: r.w + 2 * MARGIN, h: r.h + 2 * MARGIN, own: r.own });

/** The rest pose of a span from p to q: the shortest way, with its sag. The
 * wire's own ends count without the margin (it starts on their border). */
function restWay(p: Pt, q: Pt, obstacles: Solid[]): Pt[] {
  const rects = obstacles.map((r) => (r.own ? r : grown(r)));
  return sagged(shortest(p, q, rects), rects);
}

const keyOf = (pins: Pt[], obstacles: Solid[]) =>
  pins.map((p) => `${p.x.toFixed(1)},${p.y.toFixed(1)}`).join(' ') +
  '|' +
  obstacles.map((r) => `${Math.round(r.x)},${Math.round(r.y)},${Math.round(r.w)},${Math.round(r.h)}${r.own ? 'o' : ''}`).join(' ');

interface Span {
  first: number; // index of its first particle (a pin)
  n: number; // links
  len: number; // the chain it has: its rest length, plus what is left over
  rest: number; // the length of its rest pose
}

export class Chain {
  x: number[] = [];
  y: number[] = [];
  private px: number[] = [];
  private py: number[] = [];
  /** Where each particle sits in the rest pose. */
  private tx: number[] = [];
  private ty: number[] = [];
  private pinned: boolean[] = [];
  private link: number[] = [];
  private spans: Span[] = [];
  private pins: Pt[] = [];
  /** The pins and nodes the rest pose was worked out for. */
  private aimed = '';
  /** Farthest a particle was from its rest pose, last step. */
  private off = 0;
  /** Steps in a row with the chain still. */
  quiet = 0;

  constructor(pins: Pt[], obstacles: Solid[]) {
    this.layout(pins, obstacles, null);
  }

  /** A span's chain for its rest length: never shorter (it doesn't stretch), and only a little left over. */
  private static length(rest: number, had: number): number {
    return Math.min(Math.max(rest, had), rest + LEFT_PX + LEFT_SHARE * rest);
  }

  /**
   * Places the particles: along the old chain when there is one (keeping the
   * chain it had left over), else at rest.
   */
  private layout(pins: Pt[], obstacles: Solid[], old: Chain | null) {
    const x: number[] = [];
    const y: number[] = [];
    const tx: number[] = [];
    const ty: number[] = [];
    const pinned: boolean[] = [];
    const link: number[] = [];
    const spans: Span[] = [];
    for (let i = 0; i < pins.length - 1; i++) {
      const p = pins[i];
      const q = pins[i + 1];
      const way = restWay(p, q, obstacles);
      const rest = pathLength(way);
      const was = old?.spans[i];
      const len = Chain.length(rest, was?.len ?? 0);
      const n = Math.min(80, Math.max(3, Math.round(len / LINK)));
      let pts: Pt[];
      if (old && was) {
        const prev: Pt[] = [];
        for (let k = was.first; k <= was.first + was.n; k++) prev.push({ x: old.x[k], y: old.y[k] });
        prev[0] = p; // the pins have moved
        prev[prev.length - 1] = q;
        pts = resample(prev, n);
      } else {
        pts = resample(way, n);
      }
      const aim = resample(way, n);
      spans.push({ first: x.length - (i > 0 ? 1 : 0), n, len, rest });
      for (let k = 0; k <= n; k++) {
        if (i > 0 && k === 0) continue; // shared with the previous span
        x.push(pts[k].x);
        y.push(pts[k].y);
        tx.push(aim[k].x);
        ty.push(aim[k].y);
        pinned.push(k === 0 || k === n);
      }
      for (let k = 0; k < n; k++) link.push(len / n);
    }
    this.x = x;
    this.y = y;
    this.px = x.slice();
    this.py = y.slice();
    this.tx = tx;
    this.ty = ty;
    this.pinned = pinned;
    this.link = link;
    this.spans = spans;
    this.pins = pins.map((p) => ({ ...p }));
    this.aimed = keyOf(pins, obstacles);
  }

  /**
   * Works out the rest pose again when the pins or the nodes around moved.
   * A span never gets shorter than its rest pose (it doesn't stretch); one
   * whose links grew or shrank too far gets new particles.
   */
  private aim(obstacles: Solid[]) {
    const key = keyOf(this.pins, obstacles);
    if (key === this.aimed) return;
    const ways = this.spans.map((_, i) => restWay(this.pins[i], this.pins[i + 1], obstacles));
    const relayout = this.spans.some((s, i) => {
      const link = Chain.length(pathLength(ways[i]), s.len) / s.n;
      return link < LINK / 2 || link > LINK * 2;
    });
    if (relayout) {
      this.layout(this.pins, obstacles, this);
      return;
    }
    this.spans.forEach((s, i) => {
      s.rest = pathLength(ways[i]);
      s.len = Chain.length(s.rest, s.len);
      const aim = resample(ways[i], s.n);
      for (let k = 0; k <= s.n; k++) {
        this.tx[s.first + k] = aim[k].x;
        this.ty[s.first + k] = aim[k].y;
      }
      for (let k = 0; k < s.n; k++) this.link[s.first + k] = s.len / s.n;
    });
    this.aimed = key;
  }

  /** Moves the pins (the ends, the bend points); false when nothing moved. */
  setPins(pins: Pt[], obstacles: Solid[]): boolean {
    if (pins.length !== this.pins.length) {
      this.layout(pins, obstacles, null);
      return true;
    }
    if (pins.every((p, i) => Math.abs(p.x - this.pins[i].x) < 0.5 && Math.abs(p.y - this.pins[i].y) < 0.5)) return false;
    this.pins = pins.map((p) => ({ ...p }));
    this.spans.forEach((s, i) => {
      this.x[s.first] = pins[i].x;
      this.y[s.first] = pins[i].y;
      this.x[s.first + s.n] = pins[i + 1].x;
      this.y[s.first + s.n] = pins[i + 1].y;
    });
    this.aim(obstacles);
    return true;
  }

  /** One step of the simulation (STEP_MS); returns the average motion of the particles. */
  step(obstacles: Solid[]): number {
    this.aim(obstacles);
    const { x, y, px, py, tx, ty, pinned, link } = this;
    const count = x.length;
    // How much each particle springs to its rest pose (0: it hangs free).
    const pull: number[] = new Array(count).fill(1);
    for (const s of this.spans) {
      const free = Math.min(1, Math.max(0, (s.len - s.rest) / (FREE_PX + FREE_SHARE * s.rest)));
      for (let k = s.first + 1; k < s.first + s.n; k++) pull[k] = 1 - free;
    }
    let motion = 0;
    let x0 = Infinity;
    let x1 = -Infinity;
    let y0 = Infinity;
    let y1 = -Infinity;
    for (let i = 0; i < count; i++) {
      if (!pinned[i]) {
        const w = pull[i];
        const damping = DAMPING - SPRING_DAMPING * w;
        const vx = (x[i] - px[i]) * damping;
        const vy = (y[i] - py[i]) * damping;
        px[i] = x[i];
        py[i] = y[i];
        x[i] += vx + SPRING * w * (tx[i] - x[i]);
        y[i] += vy + GRAVITY * (1 - w) + SPRING * w * (ty[i] - y[i]);
        motion += Math.abs(vx) + Math.abs(vy);
      }
      x0 = Math.min(x0, x[i]);
      x1 = Math.max(x1, x[i]);
      y0 = Math.min(y0, y[i]);
      y1 = Math.max(y1, y[i]);
    }
    // Only the nodes around the chain (a link reaches a little further).
    const pad = LINK * 2;
    const solid = obstacles.map(grown).filter((r) => r.x < x1 + pad && r.x + r.w > x0 - pad && r.y < y1 + pad && r.y + r.h > y0 - pad);
    const passes = passesFor(count);
    for (let it = 0; it < passes; it++) {
      // Links only pull: a chain doesn't push back when it bunches up.
      for (let j = 0; j < count - 1; j++) {
        const dx = x[j + 1] - x[j];
        const dy = y[j + 1] - y[j];
        const d = Math.hypot(dx, dy);
        if (d <= link[j]) continue;
        const k = (d - link[j]) / d;
        const wa = pinned[j] ? 0 : pinned[j + 1] ? 1 : 0.5;
        const wb = pinned[j + 1] ? 0 : pinned[j] ? 1 : 0.5;
        x[j] += dx * k * wa;
        y[j] += dy * k * wa;
        x[j + 1] -= dx * k * wb;
        y[j + 1] -= dy * k * wb;
      }
      // Hanging free, it can't go through the nodes: out by the nearest side,
      // the top is the floor and the sides the walls. Springing back to its
      // rest pose (which is clear of them) it may cross one, to get to the
      // short side.
      for (let i = 0; i < count; i++) {
        if (pinned[i] || pull[i] >= 0.5) continue;
        for (const r of solid) {
          if (x[i] <= r.x || x[i] >= r.x + r.w || y[i] <= r.y || y[i] >= r.y + r.h) continue;
          const left = x[i] - r.x;
          const right = r.x + r.w - x[i];
          const top = y[i] - r.y;
          const bottom = r.y + r.h - y[i];
          // The sides are walls near the edges only: deeper in, a link goes
          // over the top or under the bottom, so a chain doesn't split around
          // a node (half on each wall, a link across behind it).
          const side = Math.min(left, right);
          const vertical = Math.min(top, bottom);
          if (side < vertical && side < LINK * 2) x[i] = left < right ? r.x : r.x + r.w;
          else if (top <= bottom) y[i] = r.y;
          else y[i] = r.y + r.h;
          // Friction: it rests where it lands instead of sliding off.
          px[i] += (x[i] - px[i]) * 0.5;
          py[i] += (y[i] - py[i]) * 0.5;
        }
      }
    }
    // The chain left over is reeled in.
    let relayout = false;
    for (const s of this.spans) {
      if (s.len <= s.rest) continue;
      s.len = Math.max(s.rest, s.len - Math.max(REEL_MIN, (s.len - s.rest) * REEL));
      for (let k = s.first; k < s.first + s.n; k++) link[k] = s.len / s.n;
      relayout ||= s.len / s.n < LINK / 2;
    }
    let off = 0;
    for (let i = 0; i < count; i++) off = Math.max(off, Math.abs(x[i] - tx[i]), Math.abs(y[i] - ty[i]));
    this.off = off;
    if (relayout) this.layout(this.pins, obstacles, this);
    return count > 2 ? motion / (count - 2) : 0;
  }

  /** Nothing left over, and at its rest pose: it can stop. */
  get settled(): boolean {
    return this.off < SETTLED && this.spans.every((s) => s.len - s.rest < SETTLED);
  }

  /** Puts the chain at its rest pose, at once. */
  snap(obstacles: Solid[]) {
    this.aim(obstacles);
    this.x = this.tx.slice();
    this.y = this.ty.slice();
    this.px = this.x.slice();
    this.py = this.y.slice();
    for (const s of this.spans) {
      s.len = s.rest;
      for (let k = s.first; k < s.first + s.n; k++) this.link[k] = s.len / s.n;
    }
    this.off = 0;
    this.quiet = Infinity;
  }

  /** Lays the chain again from its pins, at rest, as when it first appears. */
  reset(obstacles: Solid[]) {
    this.layout(this.pins, obstacles, null);
    this.quiet = Infinity;
  }

  points(): Pt[] {
    return this.x.map((x, i) => ({ x, y: this.y[i] }));
  }
}

const r1 = (n: number) => Math.round(n * 10) / 10;

/** A smooth path through the points (Catmull-Rom as cubic Béziers). */
export function smoothPath(pts: Pt[]): string {
  if (pts.length < 2) return '';
  let d = `M ${r1(pts[0].x)} ${r1(pts[0].y)}`;
  for (let i = 0; i < pts.length - 1; i++) {
    const p0 = pts[i - 1] ?? pts[i];
    const p1 = pts[i];
    const p2 = pts[i + 1];
    const p3 = pts[i + 2] ?? p2;
    d += ` C ${r1(p1.x + (p2.x - p0.x) / 6)} ${r1(p1.y + (p2.y - p0.y) / 6)}, ${r1(p2.x - (p3.x - p1.x) / 6)} ${r1(p2.y - (p3.y - p1.y) / 6)}, ${r1(p2.x)} ${r1(p2.y)}`;
  }
  return d;
}

/** The point halfway along the wire, where the pills sit. */
export function middle(pts: Pt[]): Pt {
  const half = pathLength(pts) / 2;
  let run = 0;
  for (let i = 1; i < pts.length; i++) {
    const d = Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y);
    if (run + d >= half && d > 0) {
      const u = (half - run) / d;
      return { x: pts[i - 1].x + (pts[i].x - pts[i - 1].x) * u, y: pts[i - 1].y + (pts[i].y - pts[i - 1].y) * u };
    }
    run += d;
  }
  return pts[Math.floor(pts.length / 2)];
}
