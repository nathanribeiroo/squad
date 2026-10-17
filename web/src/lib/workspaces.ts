// Workspace colors: keys stored by the server (internal/hub Colors), drawn
// with theme tokens so they follow light and dark.
export const WS_COLORS: Record<string, string> = {
  teal: 'var(--ws-teal)',
  blue: 'var(--ws-blue)',
  purple: 'var(--ws-purple)',
  pink: 'var(--ws-pink)',
  red: 'var(--ws-red)',
  amber: 'var(--ws-amber)',
  lime: 'var(--ws-lime)',
  green: 'var(--ws-green)',
};

export function wsColor(key?: string): string {
  return WS_COLORS[key ?? ''] ?? WS_COLORS.teal;
}
