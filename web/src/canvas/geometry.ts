// Wire geometry (docs/design_handoff_squad §5, variant 1d): floating anchors on
// the facing sides of both nodes and a cubic that sags like a rope.

export interface Rect {
  x: number;
  y: number;
  w: number;
  h: number;
}
export interface Pt {
  x: number;
  y: number;
}

interface Anchor extends Pt {
  nx: number; // outward normal of the side
  ny: number;
}

export interface Wire {
  path: string;
  s: Anchor;
  t: Anchor;
  /** Where the pills sit (the middle of a chain wire). */
  low: Pt;
}

/** Anchor points on the sides of a and b that face each other. */
export function anchors(a: Rect, b: Rect): [Anchor, Anchor] {
  const ac = { x: a.x + a.w / 2, y: a.y + a.h / 2 };
  const bc = { x: b.x + b.w / 2, y: b.y + b.h / 2 };
  const dx = bc.x - ac.x;
  const dy = bc.y - ac.y;
  // The sides, so the wire hangs between them, unless one box is above the other.
  const horizontal = a.x + a.w < b.x || b.x + b.w < a.x || Math.abs(dx) / (a.w + b.w) >= Math.abs(dy) / (a.h + b.h);
  if (horizontal) {
    const d = dx >= 0 ? 1 : -1;
    return [
      { x: d > 0 ? a.x + a.w : a.x, y: ac.y, nx: d, ny: 0 },
      { x: d > 0 ? b.x : b.x + b.w, y: bc.y, nx: -d, ny: 0 },
    ];
  }
  const d = dy >= 0 ? 1 : -1;
  return [
    { x: ac.x, y: d > 0 ? a.y + a.h : a.y, nx: 0, ny: d },
    { x: bc.x, y: d > 0 ? b.y : b.y + b.h, nx: 0, ny: -d },
  ];
}

function cubicAt(p0: Pt, p1: Pt, p2: Pt, p3: Pt, t: number): Pt {
  const u = 1 - t;
  const a = u * u * u;
  const b = 3 * u * u * t;
  const c = 3 * u * t * t;
  const d = t * t * t;
  return { x: a * p0.x + b * p1.x + c * p2.x + d * p3.x, y: a * p0.y + b * p1.y + c * p2.y + d * p3.y };
}

const f = (n: number) => Math.round(n * 10) / 10;

/** Builds the wire between two node rects, passing through user rope points. */
export function wire(a: Rect, b: Rect, rope: [number, number][] = []): Wire {
  const [s, t] = anchors(a, b);
  const dist = Math.hypot(t.x - s.x, t.y - s.y);
  // Control points reach out from the sides; short wires stay short (no loop).
  const reach = Math.min(200, Math.max(Math.min(40, dist * 0.5), dist * 0.4));
  const segs: [Pt, Pt, Pt, Pt][] = [];

  if (rope.length === 0) {
    const sag = Math.min(120, dist * 0.35);
    segs.push([s, { x: s.x + s.nx * reach, y: s.y + s.ny * reach + sag }, { x: t.x + t.nx * reach, y: t.y + t.ny * reach + sag }, t]);
  } else {
    // Catmull-Rom through the rope points; the end tangents follow the side normals.
    const pts: Pt[] = [s, ...rope.map(([x, y]) => ({ x, y })), t];
    const tan = pts.map((_, i) => {
      if (i === 0) return { x: s.nx * reach * 3, y: s.ny * reach * 3 };
      if (i === pts.length - 1) return { x: -t.nx * reach * 3, y: -t.ny * reach * 3 };
      return { x: (pts[i + 1].x - pts[i - 1].x) / 2, y: (pts[i + 1].y - pts[i - 1].y) / 2 };
    });
    for (let i = 0; i < pts.length - 1; i++) {
      const p = pts[i];
      const q = pts[i + 1];
      segs.push([p, { x: p.x + tan[i].x / 3, y: p.y + tan[i].y / 3 }, { x: q.x - tan[i + 1].x / 3, y: q.y - tan[i + 1].y / 3 }, q]);
    }
  }

  let path = `M ${f(s.x)} ${f(s.y)}`;
  const samples: Pt[] = [];
  for (const [p0, p1, p2, p3] of segs) {
    path += ` C ${f(p1.x)} ${f(p1.y)}, ${f(p2.x)} ${f(p2.y)}, ${f(p3.x)} ${f(p3.y)}`;
    for (let i = 0; i < 24; i++) samples.push(cubicAt(p0, p1, p2, p3, i / 24));
  }
  samples.push(t);
  // Pills sit at the lowest point of a hanging wire; a wire between stacked
  // nodes has its lowest point on a node, so it uses the middle instead.
  let low = samples[Math.floor(samples.length / 2)];
  if (s.ny === 0) {
    const from = Math.floor(samples.length * 0.15);
    const to = Math.ceil(samples.length * 0.85);
    for (let i = from; i < to; i++) if (samples[i].y > low.y) low = samples[i];
  }
  return { path, s, t, low };
}

/** Inserts a bend point where it lengthens the wire the least. */
export function insertRopePoint(a: Rect, b: Rect, rope: [number, number][], p: Pt): [number, number][] {
  const { s, t } = wire(a, b, rope);
  const pts: Pt[] = [s, ...rope.map(([x, y]) => ({ x, y })), t];
  const d = (u: Pt, v: Pt) => Math.hypot(u.x - v.x, u.y - v.y);
  let best = 0;
  let cost = Infinity;
  for (let i = 0; i < pts.length - 1; i++) {
    const c = d(pts[i], p) + d(p, pts[i + 1]) - d(pts[i], pts[i + 1]);
    if (c < cost) {
      cost = c;
      best = i;
    }
  }
  const next = rope.slice();
  next.splice(best, 0, [Math.round(p.x), Math.round(p.y)]);
  return next;
}
