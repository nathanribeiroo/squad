// Torre — temas do xterm.js (ponto de partida; conferir contraste com as TUIs reais).
// Substitui os temas "Noite" e "Papel" de src/theme/xterm-themes.ts.
import type { ITheme } from "@xterm/xterm";

export const noite: ITheme = {
  background: "#040806",
  foreground: "#d6dfe9",
  cursor: "#d6dfe9",
  cursorAccent: "#040806",
  selectionBackground: "#1f3a29",
  selectionForeground: "#ffffff",
  black: "#0b1710",
  red: "#f08a84",
  green: "#8fd19e",
  yellow: "#e9c46a",
  blue: "#8fb4f0",
  magenta: "#c9a0f0",
  cyan: "#7fd0d6",
  white: "#d6dfe9",
  brightBlack: "#7f8a7a",
  brightRed: "#ffaaa5",
  brightGreen: "#b3e6bf",
  brightYellow: "#ffd27a",
  brightBlue: "#a9c8ff",
  brightMagenta: "#dcbcff",
  brightCyan: "#9be6ea",
  brightWhite: "#f3f8f5",
};

export const papel: ITheme = {
  background: "#fbf8ee",
  foreground: "#2b2a22",
  cursor: "#1b1a14",
  cursorAccent: "#fbf8ee",
  selectionBackground: "#e3dabb",
  selectionForeground: "#1b1a14",
  black: "#1b1a14",
  red: "#b3261e",
  green: "#1f6b3a",
  yellow: "#8a5a00",
  blue: "#1f4f9f",
  magenta: "#7b3fb0",
  cyan: "#11727a",
  white: "#d9d0b2",
  brightBlack: "#8a836c",
  brightRed: "#d43c33",
  brightGreen: "#2e8a50",
  brightYellow: "#a86f00",
  brightBlue: "#3466c0",
  brightMagenta: "#9455cc",
  brightCyan: "#178a92",
  brightWhite: "#fffbea",
};

// Fonte dos terminais: "B612 Mono", 11px, line-height ~1.41 (15.5px).
// Se a grade ficar larga demais para 80 colunas no tamanho padrão do nó (560px),
// alternativa: manter Geist Mono só no terminal e B612 Mono no resto.
