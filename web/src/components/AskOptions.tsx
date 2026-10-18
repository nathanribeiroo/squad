import { useState } from 'react';
import { createPortal } from 'react-dom';
import type { Attention } from '../api/types';
import { pickOption } from '../lib/attention';
import { Dialog } from './Dialog';
import { Icon } from './Icon';
import './AskOptions.css';

/**
 * The agent's own options on a question or a plan (PRIORIDADE): a button
 * each, with its number (the key when the flight is selected) and its
 * description on hover; its text field takes the user's own answer; a plan
 * can be read whole first.
 */
export function AskOptions({ id, a, who }: { id: string; a: Attention; who: string }) {
  const [text, setText] = useState('');
  const [sending, setSending] = useState(false);
  const [plan, setPlan] = useState(false);
  const field = a.options?.find((o) => o.input);
  const send = () => {
    if (!field || !text.trim()) return;
    setSending(true);
    void pickOption(id, field.key, text)
      .then((ok) => ok && setText(''))
      .finally(() => setSending(false));
  };
  return (
    <>
      {a.kind === 'plan' && a.plan && (
        <button className="ao-plan" onClick={() => setPlan(true)} title="O plano inteiro, antes de escolher">
          <Icon name="description" size={15} />
          Ver o plano
        </button>
      )}
      {a.options
        ?.filter((o) => !o.input)
        .map((o) => (
          <button key={o.key} className="ao-opt" title={o.detail ? `${o.text}: ${o.detail}` : o.text} onClick={() => void pickOption(id, o.key)}>
            <span className="kbd">{o.key}</span>
            <span className="ao-opt-text">{o.text}</span>
          </button>
        ))}
      {field && (
        <form
          className="ao-field"
          onSubmit={(e) => {
            e.preventDefault();
            send();
          }}
        >
          <input
            value={text}
            maxLength={2000}
            placeholder={a.kind === 'plan' ? 'O que mudar no plano…' : 'Outra resposta…'}
            title={`${field.text}${field.detail ? ` (${field.detail})` : ''}`}
            onChange={(e) => setText(e.target.value)}
          />
          <button type="submit" disabled={!text.trim() || sending} title="Escreve a resposta no voo e envia">
            <Icon name="send" size={14} />
          </button>
        </form>
      )}
      {plan &&
        a.plan &&
        // Out of the canvas: its zoom would carry the dialog along.
        createPortal(
          <Dialog icon="description" title={`Plano de ${who}`} sub={a.detail} onClose={() => setPlan(false)} wide>
            <pre className="ao-plan-text">{a.plan}</pre>
          </Dialog>,
          document.body,
        )}
    </>
  );
}
