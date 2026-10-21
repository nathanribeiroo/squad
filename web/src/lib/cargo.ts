// CARGA: what a cargo's explorer drags out, and how a path is written where
// it lands (a flight's prompt, Transmitir). The agents get the path as text.
import type { TerminalData } from '../api/types';

/** The drag of a cargo's files and folders: JSON CargoItem[]. */
export const CARGO_MIME = 'application/x-torre-paths';

export interface CargoItem {
  /** Absolute. */
  path: string;
  dir: boolean;
}

/** Whether a drag carries a cargo's items (readable during dragover). */
export function hasCargo(dt: DataTransfer | null): boolean {
  return !!dt && [...dt.types].includes(CARGO_MIME);
}

/** The items a drop carries (only on drop). */
export function cargoItems(dt: DataTransfer | null): CargoItem[] {
  try {
    const v = JSON.parse(dt?.getData(CARGO_MIME) || '[]') as CargoItem[];
    return Array.isArray(v) ? v.filter((x) => typeof x?.path === 'string' && x.path) : [];
  } catch {
    return [];
  }
}

/** Starts dragging items: the custom type, and the quoted paths as text for
 * anything else (an editor, another app). */
export function setCargoDrag(dt: DataTransfer, items: CargoItem[]) {
  dt.effectAllowed = 'copy';
  dt.setData(CARGO_MIME, JSON.stringify(items));
  dt.setData('text/plain', quotePaths(items.map((i) => i.path)));
}

export const isWindowsPath = (p: string) => /^[A-Za-z]:[\\/]/.test(p) || p.startsWith('\\\\');

/** posix: '…' with ' escaped (sh, bash, zsh, Git Bash, and the agents);
 * windows: "…" (cmd, PowerShell; a Windows path never holds a "). */
export type QuoteStyle = 'posix' | 'windows';

/** A path as a shell reads it: quoted only when it has to be. */
export function quotePath(p: string, style: QuoteStyle = isWindowsPath(p) ? 'windows' : 'posix'): string {
  if (style === 'windows') return /^[\w\\/:.\-+@~]+$/.test(p) ? p : `"${p}"`;
  return /^[\w@%+=:,./~-]+$/.test(p) ? p : `'${p.replace(/'/g, `'\\''`)}'`;
}

export function quotePaths(paths: string[], style?: QuoteStyle): string {
  return paths.map((p) => quotePath(p, style)).join(' ');
}

/** How a flight's terminal wants quotes: a Windows path in cmd or
 * PowerShell takes "…"; Git Bash and every POSIX shell, '…'. Agents read
 * either; on Windows "…" reads more naturally. */
export function quoteStyleFor(t: TerminalData | undefined, path: string): QuoteStyle {
  if (!isWindowsPath(path)) return 'posix';
  const shell = (t?.shell ?? '').split(/[\\/]/).pop()?.toLowerCase() ?? '';
  return /^(ba|z|fi|da|k)?sh(\.exe)?$/.test(shell) ? 'posix' : 'windows';
}

/** The last name of a path. */
export function baseName(p: string): string {
  const parts = p.split(/[\\/]+/).filter(Boolean);
  return parts[parts.length - 1] ?? p;
}

export function joinPath(dir: string, name: string, sep: string): string {
  return dir.endsWith(sep) || dir.endsWith('/') ? dir + name : dir + sep + name;
}

/** The path with the home folder as ~ (on screen only). */
export function tildePath(p: string, home?: string): string {
  if (!home || !p.startsWith(home)) return p;
  const rest = p.slice(home.length);
  return rest === '' || rest[0] === '/' || rest[0] === '\\' ? `~${rest}` : p;
}

/** 1,2 MB; 830 B. */
export function formatSize(n?: number): string {
  if (n === undefined) return '';
  if (n < 1024) return `${n} B`;
  const units = ['KB', 'MB', 'GB', 'TB'];
  let v = n / 1024;
  let u = 0;
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024;
    u++;
  }
  return `${v.toLocaleString('pt-BR', { maximumFractionDigits: v < 10 ? 1 : 0 })} ${units[u]}`;
}
