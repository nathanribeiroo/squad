import { useEffect, useRef, useState } from 'react';
import { api } from '../api/client';
import { sepOf } from '../lib/paths';
import './DirInput.css';

/**
 * Folder input with suggestions from the server (GET /api/dirs): ↑/↓ pick,
 * Enter or Tab accepts (Tab alone completes a single match), Esc closes.
 */
export function DirInput({
  value,
  onChange,
  placeholder,
  suggest = api.dirs,
}: {
  value: string;
  onChange: (v: string) => void;
  placeholder?: string;
  /** Source of suggestions: the workspace's folder (default) or the hub's. */
  suggest?: (path: string) => Promise<{ suggestions: string[] }>;
}) {
  const [items, setItems] = useState<string[]>([]);
  const [open, setOpen] = useState(false);
  const [active, setActive] = useState(-1);
  const seq = useRef(0);
  const listRef = useRef<HTMLUListElement>(null);

  useEffect(() => {
    if (!open) return;
    const n = ++seq.current;
    const timer = window.setTimeout(() => {
      suggest(value)
        .then((r) => {
          if (n !== seq.current) return;
          setItems(r.suggestions);
          setActive(-1);
        })
        .catch(() => setItems([]));
    }, 120);
    return () => window.clearTimeout(timer);
  }, [value, open, suggest]);

  useEffect(() => {
    listRef.current?.children[active]?.scrollIntoView({ block: 'nearest' });
  }, [active]);

  // Accepting adds a separator so the next level is suggested right away.
  const accept = (s: string) => onChange(s + sepOf(value || s));
  const shown = open && items.length > 0;

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown' || e.key === 'ArrowUp') {
      e.preventDefault();
      setOpen(true);
      setActive((a) => (e.key === 'ArrowDown' ? Math.min(a + 1, items.length - 1) : Math.max(a - 1, -1)));
    } else if (e.key === 'Enter' && shown && active >= 0) {
      e.preventDefault();
      accept(items[active]);
    } else if (e.key === 'Tab' && shown && (active >= 0 || items.length === 1)) {
      e.preventDefault();
      accept(items[Math.max(active, 0)]);
    } else if (e.key === 'Escape' && shown) {
      // Close the list, not the form around it.
      e.nativeEvent.stopPropagation();
      setOpen(false);
    }
  };

  return (
    <div className="dir-input">
      <input
        className="mono"
        value={value}
        placeholder={placeholder}
        spellCheck={false}
        autoComplete="off"
        role="combobox"
        aria-expanded={shown}
        aria-autocomplete="list"
        onChange={(e) => {
          onChange(e.target.value);
          setOpen(true);
        }}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onKeyDown={onKeyDown}
      />
      {shown && (
        <ul className="popover dir-input-list mono" role="listbox" ref={listRef}>
          {items.map((s, i) => (
            <li
              key={s}
              role="option"
              aria-selected={i === active}
              className={i === active ? 'on' : ''}
              // mousedown, not click: keep the focus in the input.
              onMouseDown={(e) => {
                e.preventDefault();
                accept(s);
              }}
            >
              <bdi>{s}</bdi>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
