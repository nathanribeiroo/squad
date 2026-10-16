import { useState } from 'react';
import { api } from '../../api/client';
import type { TurnCard as Card } from '../../api/types';
import { Icon } from '../../components/Icon';
import { Markdown } from '../../lib/markdown';
import { toast } from '../../lib/toast';
import { useSquad } from '../../state/store';

/** "2m10s" between two times, "" when the start isn't known. */
function took(c: Card): string {
  if (!c.start) return '';
  const s = Math.max(0, Math.round((Date.parse(c.at) - Date.parse(c.start)) / 1000));
  return s < 60 ? `${s}s` : `${Math.floor(s / 60)}m${String(s % 60).padStart(2, '0')}s`;
}

/** Markdown marks out of one line of text. */
function plain(s: string): string {
  return s.replace(/^\s*(#+|[-*]|\d+\.)\s+/, '').replace(/\*\*|__|`/g, '');
}

/** The card's one-line text: the summary, else the message's first lines. */
function gist(c: Card): string {
  if (c.summary?.text) return plain(c.summary.text);
  const lines = c.message.split('\n').filter((l) => l.trim());
  return (c.source === 'screen' ? lines.slice(-2) : lines.slice(0, 3)).map(plain).join(' ').trim() || '(sem mensagem)';
}

/** Whether a terminal's card is on screen: cards on, not closed, and the
 * agent not already working on its next turn. */
export function useCardShown(id: string, working: boolean): boolean {
  return useSquad((s) => {
    const c = s.cards[id];
    return !!c && s.summarySettings?.cards !== false && s.hiddenCards[id] !== c.at && (!working || !!c.summarizing);
  });
}

/** Whether a summary engine is configured and installed. */
export function useCanSummarize(): boolean {
  return useSquad((s) => !!s.summarySettings && s.summarySettings.engine !== 'off' && s.summarySettings.engines.some((e) => e.available));
}

/**
 * The agent's last turn, at the bottom of its terminal (R6): its final
 * message, how long the turn took and, on demand, a summary with next steps
 * that open the Composer. It goes away when the next turn starts or with ×.
 */
export function TurnCard({ id, working }: { id: string; working: boolean }) {
  const card = useSquad((s) => s.cards[id]);
  const shown = useCardShown(id, working);
  const canSummarize = useCanSummarize();
  const [open, setOpen] = useState(false);
  if (!card || !shown) return null;

  const summarize = () =>
    api.summarize(id).catch((e: Error) => {
      toast(e.message);
    });
  const send = (step: string) => {
    const st = useSquad.getState();
    setOpen(false);
    st.setComposer(false);
    window.setTimeout(() => useSquad.getState().setComposer(true, id, step), 0);
  };
  const dur = took(card);
  const steps = card.summary?.steps ?? [];

  return (
    <div className="tc nodrag nopan">
      {open && (
        <div className="tc-panel nowheel" role="dialog" aria-label="Turno">
          {card.summary && (
            <section>
              <div className="tc-label">
                <Icon name="psychology" size={14} />
                Resumo <span className="tc-by">· {card.summary.by}</span>
              </div>
              <div className="tc-summary">
                <Markdown text={card.summary.text} />
              </div>
              {steps.length > 0 && (
                <div className="tc-steps">
                  {steps.map((s) => (
                    <button key={s} className="tc-step" onClick={() => send(s)} title="Abrir no Composer para este agente">
                      <Icon name="arrow_forward" size={14} />
                      {plain(s)}
                    </button>
                  ))}
                </div>
              )}
            </section>
          )}
          {card.prompt && (
            <section>
              <div className="tc-label">Pedido</div>
              <p className="tc-text">{card.prompt}</p>
            </section>
          )}
          <section>
            <div className="tc-label">{card.source === 'screen' ? 'Fim da tela' : 'Mensagem final'}</div>
            {card.source === 'screen' || !card.message ? (
              <pre className="tc-text mono">{card.message || '(vazia)'}</pre>
            ) : (
              <div className="tc-md">
                <Markdown text={card.message} />
              </div>
            )}
          </section>
        </div>
      )}
      <Icon name={card.summary ? 'psychology' : 'check_circle'} size={15} className="tc-icon" />
      <span className="tc-head">Turno{dur && ` · ${dur}`}</span>
      <button className="tc-gist" onClick={() => setOpen(!open)} title={open ? 'Fechar' : 'Ver o turno inteiro'}>
        {gist(card)}
        {steps.length > 0 && <span className="tc-count"> · {steps.length === 1 ? '1 passo' : `${steps.length} passos`}</span>}
      </button>
      {card.summarizing ? (
        <span className="tc-busy">
          <Icon name="progress_activity" size={14} className="spin" />
          resumindo…
        </span>
      ) : (
        canSummarize &&
        !card.summary && (
          <button className="tc-btn" onClick={summarize} title={card.error ? `O último resumo falhou: ${card.error}` : 'Resumir com um agente (gasta do plano dele)'}>
            {card.error && <Icon name="error" size={14} style={{ color: 'var(--st-err)' }} />}
            Resumir
          </button>
        )
      )}
      <button
        className="tc-x"
        onClick={() => {
          setOpen(false);
          useSquad.getState().hideCard(id, card.at);
        }}
        aria-label="Fechar o cartão"
      >
        <Icon name="close" size={14} />
      </button>
    </div>
  );
}
