// A wire hangs like a chain: Verlet particles under gravity, held at its
// ends and at the user's bend points, with 25% more length than the straight
// line between them. The nodes in the way are solid: their tops are a floor
// and their sides walls, so a wire drapes over a node instead of passing
// behind it.
import type { Pt, Rect } from './geometry';

/** Length of each span over the straight distance between its pins. */
const SLACK = 1.25;
/** Target length of a link (canvas px); a span keeps it between half and double. */
const LINK = 14;
const GRAVITY = 0.45;
const DAMPING = 0.985;
/** Settling at once nobody sees the swing: it can stop much sooner. */
const SETTLE_DAMPING = 0.85;
const ITERATIONS = 14;
/** Gap kept between a wire and a node's border. */
export const MARGIN = 6;
/** Steps run at once for a pose without animation. */
const SETTLE_STEPS = 260;
/** Average motion (px per step) under which the chain counts as still. */
export const STILL = 0.03;
/** Longest a span pays out when wrapped around nodes, over its slack length. */
const MAX_EXTRA = 3;

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


interface Span {
  first: number; // index of its first particle (a pin)
  n: number; // links
  extra: number; // paid-out length over the slack length (wrapped around nodes)
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

  constructor(pins: Pt[], obstacles: Rect[]) {
    this.layout(pins, obstacles, null);
  }

  /** The span's target length for its pins' distance. */
  private static base(p: Pt, q: Pt): number {
    const d = Math.hypot(q.x - p.x, q.y - p.y);
    return Math.max(d * SLACK, d + 8);
  }

  /**
   * Places the particles: along the old chain when there is one, else along a
   * hanging curve, or along the straight line when that curve would run into
   * a node (the chain then falls onto it, as if let go from up there).
   */
  private layout(pins: Pt[], obstacles: Rect[], old: Chain | null) {
    const x: number[] = [];
    const y: number[] = [];
    const pinned: boolean[] = [];
    const rest: number[] = [];
    const spans: Span[] = [];
    for (let i = 0; i < pins.length - 1; i++) {
      const p = pins[i];
      const q = pins[i + 1];
      const extra = old?.spans[i]?.extra ?? 1;
      const len = Chain.base(p, q) * extra;
      const n = Math.min(80, Math.max(3, Math.round(len / LINK)));
      let pts: Pt[];
      const was = old?.spans[i];
      if (old && was) {
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
        const solid = obstacles.map((r) => ({ x: r.x - MARGIN, y: r.y - MARGIN, w: r.w + 2 * MARGIN, h: r.h + 2 * MARGIN }));
        if (pts.some((pt, k) => k > 0 && k < n && solid.some((r) => inside(pt, r)))) pts = curve(0);
      }
      spans.push({ first: x.length - (i > 0 ? 1 : 0), n, extra });
      pts.forEach((pt, k) => {
        if (i > 0 && k === 0) return; // shared with the previous span
        x.push(pt.x);
        y.push(pt.y);
        pinned.push(k === 0 || k === n);
      });
      for (let k = 0; k < n; k++) rest.push(len / n);
    }
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
  setPins(pins: Pt[], obstacles: Rect[]): boolean {
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
    this.spans.forEach((s, i) => {
      const link = (Chain.base(pins[i], pins[i + 1]) * s.extra) / s.n;
      for (let k = 0; k < s.n; k++) this.rest[s.first + k] = link;
      this.x[s.first] = pins[i].x;
      this.y[s.first] = pins[i].y;
      this.x[s.first + s.n] = pins[i + 1].x;
      this.y[s.first + s.n] = pins[i + 1].y;
    });
    return true;
  }

  /** One step of the simulation; returns the average motion of the particles. */
  step(obstacles: Rect[], damping = DAMPING): number {
    const { x, y, px, py, pinned, rest } = this;
    const count = x.length;
    const touching: boolean[] = new Array(count).fill(false);
    let motion = 0;
    for (let i = 0; i < count; i++) {
      if (pinned[i]) continue;
      const vx = (x[i] - px[i]) * damping;
      const vy = (y[i] - py[i]) * damping;
      px[i] = x[i];
      py[i] = y[i];
      x[i] += vx;
      y[i] += vy + GRAVITY;
      motion += Math.abs(vx) + Math.abs(vy);
    }
    const solid = obstacles.map((r) => ({ x: r.x - MARGIN, y: r.y - MARGIN, w: r.w + 2 * MARGIN, h: r.h + 2 * MARGIN }));
    // Long chains need more passes, or their links stay stretched.
    const passes = Math.max(ITERATIONS, Math.ceil(count / 3));
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
          touching[i] = true;
          // Friction: it rests where it lands instead of sliding off.
          px[i] += (x[i] - px[i]) * 0.5;
          py[i] += (y[i] - py[i]) * 0.5;
        }
      }
    }
    // A span pulled taut around nodes pays out more chain; it takes it back
    // when it hangs free again.
    for (const s of this.spans) {
      let actual = 0;
      let target = 0;
      let touch = false;
      for (let k = s.first; k < s.first + s.n; k++) {
        actual += Math.hypot(x[k + 1] - x[k], y[k + 1] - y[k]);
        target += rest[k];
        touch ||= touching[k];
      }
      const grow = touch && actual / target > 1.06 ? 1.01 : !touch && s.extra > 1 ? 0.99 : 1;
      if (grow === 1) continue;
      const extra = Math.min(MAX_EXTRA, Math.max(1, s.extra * grow));
      if (extra === s.extra) continue;
      for (let k = s.first; k < s.first + s.n; k++) rest[k] *= extra / s.extra;
      s.extra = extra;
    }
    return count > 2 ? motion / (count - 2) : 0;
  }

  /** Lays the chain again from its pins, as when it first appears. */
  reset(obstacles: Rect[]) {
    this.layout(this.pins, obstacles, null);
  }

  /** Runs until the chain is still, at once (the first pose, reduced motion). */
  settle(obstacles: Rect[]) {
    for (let i = 0; i < SETTLE_STEPS; i++) {
      if (this.step(obstacles, SETTLE_DAMPING) < STILL && i > 20) break;
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
