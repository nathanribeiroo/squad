import './lib/legacyStorage'; // before the store reads its settings
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@fontsource/b612/400.css';
import '@fontsource/b612/700.css';
import '@fontsource/b612-mono/400.css';
import '@fontsource/b612-mono/700.css';
import '@fontsource-variable/geist-mono'; // terminals (--font-term)
import '@xyflow/react/dist/style.css';
import './theme/tokens.css';
import './theme/base.css';
import { App } from './App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
