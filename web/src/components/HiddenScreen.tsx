import { useSquad } from '../state/store';
import { Icon } from './Icon';

/** ⋯ → Ocultar a tela: an opaque striped cover (not a blur) over a flight's screen. */
export function HiddenScreen({ id, compact }: { id: string; compact?: boolean }) {
  return (
    <div className="hidden-screen nodrag" onMouseDown={(e) => e.stopPropagation()}>
      <span className="hidden-screen-label">
        <Icon name="visibility_off" size={compact ? 14 : 18} />
        Tela oculta
      </span>
      {!compact && (
        <button className="btn" onClick={() => useSquad.getState().toggleHiddenScreen(id)}>
          Mostrar a tela
        </button>
      )}
    </div>
  );
}
