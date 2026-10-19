// A wire hangs like a chain: Verlet particles under gravity, held at its
// ends and at the user's bend points, with 5% more length than the straight
// line between them. The nodes in the way are solid: their tops are a floor
// and their sides walls, so a wire drapes over a node instead of passing
// behind it. It can't be wound round a node: going round the long way, it
// jumps to the short side.
import type { Pt, Rect } from './geometry';

/** A node the chain can't pass through; own: one of the wire's ends. */
export type Solid = Rect & { own?: boolean };

/** Length of each span over the straight distance between its pins. */
const SLACK = 1.05;
/** Target length of a link (canvas px); a span keeps it between half and double. */
const LINK = 18;
const GRAVITY = 0.45;
const DAMPING = 0.985;
/** Settling at once nobody sees the swing: it can stop much sooner. */
const SETTLE_DAMPING = 0.85;
const ITERATIONS = 10;
/** Gap kept between a wire and a node's border. */
export const MARGIN = 6;
/** Steps run at once for a pose without animation. */
const SETTLE_STEPS = 260;
/** Average motion (px per step) under which the chain counts as still. */
export const STILL = 0.03;
/** Longest a span pays out when wrapped around nodes, over its slack length. */
const MAX_EXTRA = 3;
/** Steps in a row a span must stay taut around a node before it pays out. */
const TAUT_STEPS = 12;
/**
 * Under its own weight a chain hangs longer than its links say: by about
 * STRETCH × links / passes (the solver can't fully undo gravity each step).
 * Its links start that much shorter, so it hangs with the slack asked for.
 */
const STRETCH = 0.024;

/**
 * A span going round more than half of a node (its center inside the loop
 * the span makes with its chord) jumps to the other side; within this share
 * of the node's half-width across the chord, either side is as short.
 */
const UNWIND_MARGIN = 0.1;

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

function inside(p: Pt, r: Rect): boolean {
  return p.x > r.x && p.x < r.x + r.w && p.y > r.y && p.y < r.y + r.h;
}

function pathLength(pts: Pt[]): number {
  let n = 0;
  for (let i = 1; i < pts.length; i++) n += Math.hypot(pts[i].x - pts[i - 1].x, pts[i].y - pts[i - 1].y);
  return n;
}

/** How many times the closed polygon winds round a point (0: outside). */
function winding(px: number, py: number, xs: number[], ys: number[], from: number, to: number): number {
  let w = 0;
  for (let i = from; i <= to; i++) {
    const j = i === to ? from : i + 1;
    const cross = (xs[j] - xs[i]) * (py - ys[i]) - (px - xs[i]) * (ys[j] - ys[i]);
    if (ys[i] <= py) {
      if (ys[j] > py && cross > 0) w++;
    } else if (ys[j] <= py && cross < 0) w--;
  }
  return w;
}

/**
 * The way from p to q round r on the short side: when more than half of r
 * lies between a span and its chord, the span went round the long way. Null
 * when it didn't, or when it's too close to call.
 */
function shortWay(p: Pt, q: Pt, r: Rect, xs: number[], ys: number[], from: number, to: number): Pt[] | null {
  const ux = q.x - p.x;
  const uy = q.y - p.y;
  const d = Math.hypot(ux, uy);
  if (d < 1) return null;
  const nx = -uy / d;
  const ny = ux / d;
  const cx = r.x + r.w / 2;
  const cy = r.y + r.h / 2;
  const across = (cx - p.x) * nx + (cy - p.y) * ny;
  const half = (Math.abs(nx) * r.w + Math.abs(ny) * r.h) / 2;
  if (Math.abs(across) < UNWIND_MARGIN * half || winding(cx, cy, xs, ys, from, to) === 0) return null;
  // Round the corners on the other side of the chord, in order along it.
  const g = 4;
  const corners = [
    { x: r.x - g, y: r.y - g },
    { x: r.x + r.w + g, y: r.y - g },
    { x: r.x + r.w + g, y: r.y + r.h + g },
    { x: r.x - g, y: r.y + r.h + g },
  ].filter((c) => Math.sign((c.x - p.x) * nx + (c.y - p.y) * ny) === -Math.sign(across));
  corners.sort((a, b) => (a.x - p.x) * ux + (a.y - p.y) * uy - ((b.x - p.x) * ux + (b.y - p.y) * uy));
  return [p, ...corners, q];
}

const grown = (r: Solid): Solid => ({ x: r.x - MARGIN, y: r.y - MARGIN, w: r.w + 2 * MARGIN, h: r.h + 2 * MARGIN, own: r.own });

interface Span {
  first: number; // index of its first particle (a pin)
  n: number; // links
  extra: number; // paid-out length over the slack length (wrapped around nodes)
  taut: number; // steps in a row taut around a node
  free: boolean; // touching no node but the wire's own ends (last step)
}

export class Chain {
  x: number[] = [];
  y: number[] = [];
  private px: number[] = [];
  private py: number[] = [];
  private pinned: boolean[] = [];
  private rest: number[] = [];
  private spans: Span[] = [];
  private pins: Pt[] = [];
  /** Steps in a row with the chain still. */
  quiet = 0;

  constructor(pins: Pt[], obstacles: Solid[]) {
    this.layout(pins, obstacles, null);
  }

  /** The span's target length for its pins' distance. */
  private static base(p: Pt, q: Pt): number {
    const d = Math.hypot(q.x - p.x, q.y - p.y);
    return Math.max(d * SLACK, d + 8);
  }

  /** Rest length of a link, for a span of this length in n links (see STRETCH). */
  private static link(len: number, n: number, passes: number): number {
    return len / n / (1 + (STRETCH * n) / passes);
  }

  /**
   * Places the particles: along the old chain when there is one, else along a
   * hanging curve, or along the straight line when that curve would run into
   * a node (the chain then falls onto it, as if let go from up there).
   */
  private layout(pins: Pt[], obstacles: Solid[], old: Chain | null, ways?: Map<number, Pt[]>) {
    const x: number[] = [];
    const y: number[] = [];
    const pinned: boolean[] = [];
    const rest: number[] = [];
    const spans: Span[] = [];
    const lens: number[] = [];
    for (let i = 0; i < pins.length - 1; i++) {
      const p = pins[i];
      const q = pins[i + 1];
      const way = ways?.get(i);
      // A span sent the short way round a node fits that way, with no chain paid out before.
      const extra = way ? Math.min(MAX_EXTRA, Math.max(1, pathLength(way) / Chain.base(p, q))) : (old?.spans[i]?.extra ?? 1);
      const len = Chain.base(p, q) * extra;
      const n = Math.min(80, Math.max(3, Math.round(len / LINK)));
      let pts: Pt[];
      const was = old?.spans[i];
      if (way) {
        pts = resample(way, n);
      } else if (old && was) {
        const prev: Pt[] = [];
        for (let k = was.first; k <= was.first + was.n; k++) prev.push({ x: old.x[k], y: old.y[k] });
        prev[0] = p; // the pins have moved
        prev[prev.length - 1] = q;
        pts = resample(prev, n);
      } else {
        const d = Math.hypot(q.x - p.x, q.y - p.y);
        // A parabola of the same length: L ≈ d + 8h²/3d.
        const curve = (h: number) =>
          Array.from({ length: n + 1 }, (_, k) => ({ x: p.x + ((q.x - p.x) * k) / n, y: p.y + ((q.y - p.y) * k) / n + (4 * h * k * (n - k)) / (n * n) }));
        pts = curve(Math.sqrt(Math.max(0, (3 * d * (len - d)) / 8)));
        const solid = obstacles.map(grown);
        if (pts.some((pt, k) => k > 0 && k < n && solid.some((r) => inside(pt, r)))) pts = curve(0);
      }
      spans.push({ first: x.length - (i > 0 ? 1 : 0), n, extra, taut: 0, free: true });
      pts.forEach((pt, k) => {
        if (i > 0 && k === 0) return; // shared with the previous span
        x.push(pt.x);
        y.push(pt.y);
        pinned.push(k === 0 || k === n);
      });
      lens.push(len);
    }
    const passes = passesFor(x.length);
    spans.forEach((s, i) => {
      for (let k = 0; k < s.n; k++) rest.push(Chain.link(lens[i], s.n, passes));
    });
    this.x = x;
    this.y = y;
    this.px = x.slice();
    this.py = y.slice();
    this.pinned = pinned;
    this.rest = rest;
    this.spans = spans;
    this.pins = pins.map((p) => ({ ...p }));
  }

  /** Moves the pins (the ends, the bend points); false when nothing moved. */
  setPins(pins: Pt[], obstacles: Solid[]): boolean {
    if (pins.length !== this.pins.length) {
      this.layout(pins, obstacles, null);
      return true;
    }
    if (pins.every((p, i) => Math.abs(p.x - this.pins[i].x) < 0.5 && Math.abs(p.y - this.pins[i].y) < 0.5)) return false;
    // A span whose links grew or shrank too far gets new particles, along its current shape.
    const relayout = this.spans.some((s, i) => {
      const link = (Chain.base(pins[i], pins[i + 1]) * s.extra) / s.n;
      return link < LINK / 2 || link > LINK * 2;
    });
    if (relayout) {
      this.layout(pins, obstacles, this);
      return true;
    }
    this.pins = pins.map((p) => ({ ...p }));
    const passes = passesFor(this.x.length);
    this.spans.forEach((s, i) => {
      const link = Chain.link(Chain.base(pins[i], pins[i + 1]) * s.extra, s.n, passes);
      for (let k = 0; k < s.n; k++) this.rest[s.first + k] = link;
      this.x[s.first] = pins[i].x;
      this.y[s.first] = pins[i].y;
      this.x[s.first + s.n] = pins[i + 1].x;
      this.y[s.first + s.n] = pins[i + 1].y;
    });
    return true;
  }

  /** One step of the simulation; returns the average motion of the particles. */
  step(obstacles: Solid[], damping = DAMPING): number {
    const { x, y, px, py, pinned, rest } = this;
    const count = x.length;
    // Touching a node in the way (not one of the wire's own ends).
    const touching: boolean[] = new Array(count).fill(false);
    let motion = 0;
    let x0 = Infinity;
    let x1 = -Infinity;
    let y0 = Infinity;
    let y1 = -Infinity;
    for (let i = 0; i < count; i++) {
      if (!pinned[i]) {
        const vx = (x[i] - px[i]) * damping;
        const vy = (y[i] - py[i]) * damping;
        px[i] = x[i];
        py[i] = y[i];
        x[i] += vx;
        y[i] += vy + GRAVITY;
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
    // The node each particle last touched (an index in solid), to unwind.
    const hit: number[] = new Array(count).fill(-1);
    for (let it = 0; it < passes; it++) {
      // Links only pull: a chain doesn't push back when it bunches up.
      for (let j = 0; j < count - 1; j++) {
        const dx = x[j + 1] - x[j];
        const dy = y[j + 1] - y[j];
        const d = Math.hypot(dx, dy);
        if (d <= rest[j]) continue;
        const k = (d - rest[j]) / d;
        const wa = pinned[j] ? 0 : pinned[j + 1] ? 1 : 0.5;
        const wb = pinned[j + 1] ? 0 : pinned[j] ? 1 : 0.5;
        x[j] += dx * k * wa;
        y[j] += dy * k * wa;
        x[j + 1] -= dx * k * wb;
        y[j + 1] -= dy * k * wb;
      }
      // Out of the nodes, by the nearest side: the top is the floor, the sides the walls.
      for (let i = 0; i < count; i++) {
        if (pinned[i]) continue;
        for (let h = 0; h < solid.length; h++) {
          const r = solid[h];
          if (x[i] <= r.x || x[i] >= r.x + r.w || y[i] <= r.y || y[i] >= r.y + r.h) continue;
          hit[i] = h;
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
          if (!r.own) touching[i] = true;
          // Friction: it rests where it lands instead of sliding off.
          px[i] += (x[i] - px[i]) * 0.5;
          py[i] += (y[i] - py[i]) * 0.5;
        }
      }
    }
    // A span pulled taut around a node in its way pays out more chain; it
    // takes it back as soon as nothing but its own ends holds it. Its own
    // ends don't count: a wire always rests against the node it leaves, and
    // it is stretched for a moment whenever that node is dragged.
    for (const s of this.spans) {
      let actual = 0;
      let target = 0;
      let touch = false;
      for (let k = s.first; k < s.first + s.n; k++) {
        actual += Math.hypot(x[k + 1] - x[k], y[k + 1] - y[k]);
        target += rest[k];
        touch ||= touching[k];
      }
      s.free = !touch;
      // Against the length asked for, not the links' (they start shorter, see STRETCH).
      const asked = target * (1 + (STRETCH * s.n) / passes);
      s.taut = touch && actual / asked > 1.06 ? s.taut + 1 : 0;
      const grow = s.taut >= TAUT_STEPS ? 1.01 : !touch && s.extra > 1 ? 0.97 : 1;
      if (grow === 1) continue;
      const extra = Math.min(MAX_EXTRA, Math.max(1, s.extra * grow));
      if (extra === s.extra) continue;
      for (let k = s.first; k < s.first + s.n; k++) rest[k] *= extra / s.extra;
      s.extra = extra;
    }
    // A span wound round a node the long way jumps to the short side.
    let ways: Map<number, Pt[]> | undefined;
    this.spans.forEach((s, i) => {
      const seen = new Set<number>();
      for (let k = s.first + 1; k < s.first + s.n; k++) {
        const h = hit[k];
        if (h < 0 || seen.has(h)) continue;
        seen.add(h);
        const way = shortWay(this.pins[i], this.pins[i + 1], solid[h], x, y, s.first, s.first + s.n);
        if (way) {
          (ways ??= new Map()).set(i, way);
          break;
        }
      }
    });
    if (ways) {
      this.layout(this.pins, [], this, ways);
      return 1; // it moved: keep it awake
    }
    return count > 2 ? motion / (count - 2) : 0;
  }

  /** A span is still taking back chain it paid out: keep it moving. */
  get retracting(): boolean {
    return this.spans.some((s) => s.free && s.extra > 1);
  }

  /** Lays the chain again from its pins, as when it first appears. */
  reset(obstacles: Solid[]) {
    this.layout(this.pins, obstacles, null);
  }

  /** Runs until the chain is still, at once (the first pose, reduced motion). */
  settle(obstacles: Solid[]) {
    for (let i = 0; i < SETTLE_STEPS; i++) {
      if (this.step(obstacles, SETTLE_DAMPING) < STILL && i > 20 && !this.retracting) break;
    }
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

/** The lowest point away from the ends, where the pills sit. */
export function lowest(pts: Pt[]): Pt {
  const from = Math.floor(pts.length * 0.2);
  const to = Math.max(from + 1, Math.ceil(pts.length * 0.8));
  let low = pts[Math.floor(pts.length / 2)];
  for (let i = from; i < to && i < pts.length; i++) if (pts[i].y > low.y) low = pts[i];
  return low;
}
