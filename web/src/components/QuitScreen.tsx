import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './QuitScreen.css';

/** Sair: over everything while the server saves and stops, and after it, when
 * the browser doesn't let the page close its window (a tab). */
export function QuitScreen() {
  const quitting = useSquad((s) => s.quitting);
  if (!quitting) return null;
  const done = quitting === 'closed';
  return (
    <div className="quit" role="alert" aria-live="assertive">
      <div className="quit-card">
        <Icon name={done ? 'power_settings_new' : 'progress_activity'} size={26} className={done ? '' : 'spin'} />
        <strong>{done ? 'A Torre foi encerrada' : 'Encerrando a Torre…'}</strong>
        <span>
          {quitting === 'saving'
            ? 'Salvando o que está aberto.'
            : quitting === 'closing'
              ? 'Salvando a tela de cada voo e parando os agentes.'
              : 'Tudo ficou salvo. Pode fechar esta janela.'}
        </span>
        {done && (
          <span className="quit-again">
            Para abrir de novo: <span className="mono">torre serve</span>
          </span>
        )}
      </div>
    </div>
  );
}
