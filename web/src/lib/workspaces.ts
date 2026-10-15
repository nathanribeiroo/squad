// Workspace colors: keys stored by the server (internal/hub Colors), drawn
// with theme tokens so they follow light and dark.
export const WS_COLORS: Record<string, string> = {
  teal: 'var(--r-impl)',
  blue: 'var(--st-work)',
  purple: 'var(--r-plan)',
  pink: 'var(--r-rev)',
  red: 'var(--st-err)',
  amber: 'var(--st-wait)',
  lime: 'var(--r-test)',
  green: 'var(--st-done)',
};

export function wsColor(key?: string): string {
  return WS_COLORS[key ?? ''] ?? WS_COLORS.teal;
}
