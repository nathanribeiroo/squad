import { Component, type ReactNode } from 'react';
import { Icon } from './Icon';
import './NoLink.css';

type Props = { reset: string; children: ReactNode };

/**
 * An error drawing the stage shows here, under the top bar, instead of an
 * empty page: what broke, and Recarregar. Another workspace or session
 * (reset) draws it again.
 */
export class CrashGuard extends Component<Props, { error: Error | null }> {
  state = { error: null as Error | null };

  static getDerivedStateFromError(error: Error) {
    return { error };
  }

  componentDidUpdate(prev: Props) {
    if (this.state.error && prev.reset !== this.props.reset) this.setState({ error: null });
  }

  render() {
    const { error } = this.state;
    if (!error) return this.props.children;
    return (
      <main className="stage">
        <div className="nolink" role="alert">
          <div className="nolink-card crash-card">
            <Icon name="error" size={20} />
            <div>
              <strong>A tela parou</strong>
              <span>Um erro interrompeu o desenho do canvas. Os voos continuam rodando no servidor.</span>
              <span className="nolink-sub mono">{error.message}</span>
              <button className="btn filled" onClick={() => location.reload()}>
                <Icon name="refresh" size={16} />
                Recarregar
              </button>
            </div>
          </div>
        </div>
      </main>
    );
  }
}
