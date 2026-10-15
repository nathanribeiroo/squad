// Path helpers for display. Paths come from the server's OS: POSIX or Windows.

const HOME = [/^\/(Users|home)\/[^/]+/, /^[A-Za-z]:\\Users\\[^\\]+/i];

/** Abbreviates the home folder: /Users/ana/x → ~/x, C:\Users\ana\x → ~\x. */
export function shortPath(p?: string): string {
  if (!p) return '';
  for (const re of HOME) {
    const m = p.match(re);
    if (m) return '~' + p.slice(m[0].length);
  }
  return p;
}

function isAbs(p: string): boolean {
  return p.startsWith('/') || /^[A-Za-z]:[\\/]/.test(p) || p.startsWith('\\\\');
}

/** Separator a path is written with. */
export function sepOf(p: string): string {
  return p.includes('\\') && !p.includes('/') ? '\\' : '/';
}

/**
 * Folder a terminal opens in, as the server resolves it (ResolveCwd in
 * internal/server): its own folder, relative ones under the workspace folder.
 */
export function terminalCwd(cwd: string | undefined, wsCwd: string | undefined): string {
  if (!cwd) return wsCwd ?? '';
  if (!wsCwd || cwd === '~' || /^~[\\/]/.test(cwd) || isAbs(cwd)) return cwd;
  return wsCwd.replace(/[\\/]+$/, '') + sepOf(wsCwd) + cwd;
}

/** Drops trailing separators, keeping roots like "/" and "C:\". */
export function trimDir(p: string): string {
  const t = p.trim();
  return /^([A-Za-z]:)?[\\/]$/.test(t) ? t : t.replace(/[\\/]+$/, '');
}
