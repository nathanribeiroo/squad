// Sair: saves what is pending in this window, stops torre serve (which saves
// and closes every workspace, as Ctrl-C does) and closes the window.
import { hub, settled } from '../api/client';
import { confirmDialog } from '../components/ConfirmDialog';
import { useSquad } from '../state/store';
import { plural } from './floors';
import { toast } from './toast';

export async function quitTorre() {
  const st = useSquad.getState();
  if (st.quitting) return;
  const working = st.workspaces.reduce((n, w) => n + (w.active ?? 0), 0);
  const pending = st.workspaces.reduce((n, w) => n + (w.pending ?? 0), 0);
  const many = st.workspaces.length > 1;
  const ok = await confirmDialog({
    title: 'Sair da Torre?',
    icon: 'power_settings_new',
    tone: working ? 'warn' : 'info',
    text: `Fecha a janela e para o torre serve${many ? `, com os ${st.workspaces.length} workspaces` : ''}. Para abrir de novo: torre serve.`,
    lose: [
      working > 0 &&
        `${plural(working, 'voo trabalhando para', 'voos trabalhando param')} no meio do turno; ao abrir de novo, ${working === 1 ? 'volta' : 'voltam'} em arremetida, com Continuar`,
      pending > 0 && `${plural(pending, 'pedido', 'pedidos')} entre os voos ainda sem resposta`,
    ].filter(Boolean),
    keep: [
      'Os canvas, as NOTAMs, os níveis e as sessões',
      'A conversa de cada voo, que continua de onde parou ao abrir de novo',
      'A última tela de cada voo',
    ],
    action: 'Sair',
  });
  if (!ok) return;
  st.setQuitting('saving');
  // What is still being typed saves when it loses the focus (a NOTAM being
  // edited, a name); then every change on its way gets its answer.
  (document.activeElement as HTMLElement | null)?.blur?.();
  await new Promise((r) => window.setTimeout(r, 80));
  await settled();
  try {
    await hub.quit();
  } catch (e) {
    st.setQuitting(null);
    toast(`Não deu para sair: ${(e as Error).message}`, 'err');
    return;
  }
  await followQuit();
}

/**
 * The server is stopping (Sair, here or in another window): it saves the
 * screens and stops the flights; once it is gone, the window closes, when the
 * browser lets a page close it (an app window does; a tab opened by hand
 * doesn't, so it says it is over).
 */
export async function followQuit() {
  const st = useSquad.getState();
  if (st.quitting === 'closing' || st.quitting === 'closed') return;
  st.setQuitting('closing');
  await serverGone();
  window.close();
  window.setTimeout(() => useSquad.getState().setQuitting('closed'), 400);
}

/** Waits until the server stops answering (at most 20 s). */
async function serverGone() {
  const until = Date.now() + 20_000;
  while (Date.now() < until) {
    try {
      await fetch('/favicon.svg', { cache: 'no-store' });
    } catch {
      return;
    }
    await new Promise((r) => window.setTimeout(r, 300));
  }
}
