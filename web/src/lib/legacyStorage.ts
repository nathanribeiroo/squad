// Squad became Torre: the settings kept in this browser under the old
// prefix (workspace on screen, floor, canvas position and zoom, themes)
// move to the new one, once, before anything reads them. Imported first in
// main.tsx.
const OLD = 'squad.';
const NEW = 'torre.';

try {
  for (const key of Object.keys(localStorage)) {
    if (!key.startsWith(OLD)) continue;
    const value = localStorage.getItem(key);
    const next = NEW + key.slice(OLD.length);
    if (value !== null && localStorage.getItem(next) === null) localStorage.setItem(next, value);
    localStorage.removeItem(key);
  }
} catch {
  /* storage may be unavailable */
}

export {};
