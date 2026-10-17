import { useNow } from '../lib/useNow';
import { useSquad } from '../state/store';
import { Icon } from './Icon';
import './NoLink.css';

/**
 * The link to torre serve has been down for a while (docs/torre/TELAS.md,
 * quadro 19, "Sem enlace"). The flights keep their last state underneath;
 * a short drop only shows RECONECTANDO… in the top bar.
 */
export function NoLink() {
  const lost = useSquad((s) => (s.connection === 'reconnecting' ? s.linkLostAt : 0));
  const retryAt = useSquad((s) => s.retryAt);
  const now = useNow(500, lost > 0);
  if (!lost || now - lost < 4000) return null;
  const secs = Math.max(0, Math.ceil((retryAt - now) / 1000));
  return (
    <div className="nolink" role="alert">
      <div className="nolink-card">
        <Icon name="sync" size={20} className="spin" />
        <div>
          <strong>Sem enlace</strong>
          <span>
            O <span className="mono">torre serve</span> parou? {secs > 0 ? `Tentando de novo em ${secs} s…` : 'Tentando agora…'}
          </span>
          <span className="nolink-sub">Os voos continuam no último estado conhecido. Se o servidor caiu, rode <span className="mono">torre serve</span> de novo.</span>
        </div>
      </div>
    </div>
  );
}
