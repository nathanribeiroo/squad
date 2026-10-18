// What a flight asks the user, in words (no store here: the store uses it).
import type { Attention } from '../api/types';

/** "Quer executar um comando", from the tool the agent wants to use. */
export function askPhrase(a?: Attention): string {
  if (!a) return 'Pede você';
  if (a.kind === 'question') return 'Fez uma pergunta';
  if (a.kind === 'plan') return 'Pede para aprovar o plano';
  const tool = a.tool ?? '';
  // Claude Code's names, then Devin's.
  if (['Bash', 'BashOutput', 'exec'].includes(tool)) return 'Quer executar um comando';
  if (['Edit', 'MultiEdit', 'Write', 'NotebookEdit', 'edit', 'write'].includes(tool)) return 'Quer editar um arquivo';
  if (tool === 'Read' || tool === 'read') return 'Quer ler um arquivo';
  if (tool === 'WebFetch' || tool === 'webfetch') return 'Quer abrir uma página';
  if (tool === 'WebSearch' || tool === 'websearch') return 'Quer pesquisar na web';
  if (tool === 'ExitPlanMode') return 'Quer seguir o plano';
  const mcp = /^mcp__(.+?)__(.+)$/.exec(tool);
  if (mcp) return `Quer usar ${mcp[2]} (${mcp[1]})`;
  if (tool) return `Quer usar ${tool}`;
  return 'Pede a sua permissão';
}

