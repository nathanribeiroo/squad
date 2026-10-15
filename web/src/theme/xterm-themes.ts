import type { ITheme } from '@xterm/xterm';

// Fonte do terminal: 'Geist Mono', 11px, lineHeight 15px (≈1.36). Tamanho mínimo útil: 80×24.
export const noite: ITheme = {
  background: '#0d0e10', foreground: '#d7d9de',
  cursor: '#f4f5f7', cursorAccent: '#0d0e10', selectionBackground: '#2c3a52',
  black: '#1c1e22', red: '#f0767a', green: '#7fcf8e', yellow: '#e6c26a',
  blue: '#7aa7f0', magenta: '#c79af0', cyan: '#6fcfd6', white: '#c9ccd2',
  brightBlack: '#5d626b', brightRed: '#ff9a9c', brightGreen: '#a2e3ae', brightYellow: '#f4d68e',
  brightBlue: '#a0c2ff', brightMagenta: '#ddbcff', brightCyan: '#96e3e8', brightWhite: '#f4f5f7',
};

export const papel: ITheme = {
  background: '#fbfbf9', foreground: '#2a2c30',
  cursor: '#17181a', cursorAccent: '#fbfbf9', selectionBackground: '#d6e2f7',
  black: '#2a2c30', red: '#c4373d', green: '#2f7d43', yellow: '#8a6a00',
  blue: '#2f5fc4', magenta: '#8b44b8', cyan: '#1b7a84', white: '#6b6e74',
  brightBlack: '#80838a', brightRed: '#d9474d', brightGreen: '#3a9152', brightYellow: '#a07c00',
  brightBlue: '#3d70d9', brightMagenta: '#9f55cf', brightCyan: '#238f9a', brightWhite: '#111214',
};
