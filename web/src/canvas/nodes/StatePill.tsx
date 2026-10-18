import type { UIState } from '../../api/types';
import { Icon } from '../../components/Icon';
import { STATE_META, elapsed, stateLabel } from '../../lib/model';
import { useNow } from '../../lib/useNow';

// Only the envelopes animate: the state is static, with a ticking clock
// (icon + WORD + time, in the state's text color; docs/torre/TELAS.md, 02).
const TIMED: UIState[] = ['boot', 'work', 'wait', 'ask', 'done'];

export function StatePill({
  state,
  since,
  exitCode,
  compact = false,
  yourTurn = false,
}: {
  state: UIState;
  since?: string;
  exitCode?: number;
  compact?: boolean;
  /** In solo after a turn of the user's: it waits for them (SUA VEZ). */
  yourTurn?: boolean;
}) {
  const timed = TIMED.includes(state);
  const now = useNow(1000, timed);
  return (
    <span className={`state-pill${compact ? ' compact' : ''}`} style={{ color: `var(--stt-${state})` }}>
      <Icon name={STATE_META[state].icon} size={14} />
      {stateLabel(state, exitCode)}
      {timed && since && <span className="mono state-time">{elapsed(since, now)}</span>}
      {yourTurn && state === 'done' && (
        <span className="your-turn" title="O turno era seu e terminou: o voo espera você">
          Sua vez
        </span>
      )}
    </span>
  );
}
