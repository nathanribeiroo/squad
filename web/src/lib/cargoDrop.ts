import { useRef, useState } from 'react';
import { termSession } from '../term/session';
import { useSquad } from '../state/store';
import { cargoItems, hasCargo, quotePath, quoteStyleFor, type CargoItem } from './cargo';
import { callsign } from './model';
import { toast } from './toast';

/**
 * A place a cargo's files can be dropped on (a flight, Transmitir): over is
 * true while a drag of them is above it. Other drags pass through.
 */
export function useCargoDrop(onDrop: (items: CargoItem[], e: React.DragEvent) => void) {
  const [over, setOver] = useState(false);
  // dragenter and dragleave fire for every child crossed.
  const depth = useRef(0);
  const props = {
    onDragEnter: (e: React.DragEvent) => {
      if (!hasCargo(e.dataTransfer)) return;
      e.preventDefault();
      depth.current++;
      setOver(true);
    },
    onDragOver: (e: React.DragEvent) => {
      if (!hasCargo(e.dataTransfer)) return;
      e.preventDefault();
      e.stopPropagation();
      e.dataTransfer.dropEffect = 'copy';
    },
    onDragLeave: (e: React.DragEvent) => {
      if (!hasCargo(e.dataTransfer)) return;
      if (--depth.current <= 0) {
        depth.current = 0;
        setOver(false);
      }
    },
    onDrop: (e: React.DragEvent) => {
      if (!hasCargo(e.dataTransfer)) return;
      e.preventDefault();
      e.stopPropagation();
      depth.current = 0;
      setOver(false);
      useSquad.getState().setCargoDrag(false);
      const items = cargoItems(e.dataTransfer);
      if (items.length) onDrop(items, e);
    },
  };
  return { over, props };
}

/**
 * Pastes paths into a flight's prompt, quoted for its terminal, with a
 * space after and never Enter: the user finishes the message. The agent
 * reads the path as text (one outside its folder may ask permission).
 */
export function pasteIntoFlight(id: string, items: CargoItem[]) {
  const st = useSquad.getState();
  const t = st.workspace?.nodes.find((n) => n.id === id)?.terminal;
  if (!t || !items.length) return;
  if (st.runtime[id]?.status !== 'running') {
    toast(`${callsign(t)} está parado: ligue o voo para soltar o caminho nele.`, 'wait');
    return;
  }
  const text = `${items.map((i) => quotePath(i.path, quoteStyleFor(t, i.path))).join(' ')} `;
  if (!termSession(id).paste(text)) {
    toast(`${callsign(t)} ainda está conectando. Tente de novo.`, 'err');
    return;
  }
  if (st.zoomLevel === 'live' || st.focusPane === id || st.dock?.id === id) termSession(id).focus();
  toast(items.length === 1 ? `Caminho colado em ${callsign(t)}` : `${items.length} caminhos colados em ${callsign(t)}`, 'ok');
}
