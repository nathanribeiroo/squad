# Prompt base para o Claude Code

Cole isto no Claude Code, rodando na raiz do repositório (com esta pasta `design_handoff_squad/` dentro dele):

---

Você vai construir a interface do **Squad**, uma ferramenta local aberta no navegador para orquestrar agentes de código (Claude Code, Codex, Gemini) num canvas infinito.

Antes de escrever código, leia nesta ordem:
1. `design_handoff_squad/README.md`: especificação completa da interface (telas, componentes, estados, tokens, interações).
2. `design_handoff_squad/brief.md`: o brief de produto (conceito, objetos, fases F0–F6, restrições técnicas).
3. `design_handoff_squad/tokens/tokens.css` e `tokens/xterm-themes.ts`: use estes arquivos como estão.
4. `design_handoff_squad/design/Squad.dc.html`: referência visual. Abra no navegador (`npx serve design_handoff_squad/design`) para ver. **Não copie o HTML**; recrie em componentes React.

Stack: React + Vite + TypeScript, `@xyflow/react` para o canvas, `@xterm/xterm` com o addon WebGL para os terminais, CSS Modules ou CSS puro consumindo as variáveis de `tokens.css`. Sem Tailwind, a não ser que o repositório já use.

Escopo desta etapa (F0 + F1, só frontend, com dados simulados):
1. Instale os tokens e as fontes; aplique `data-theme` e `data-term` no `<html>`, com alternância entre claro e escuro.
2. Monte o shell: `TopBar`, `Toolbar`, `Minimap` + controles de zoom, `ComposerPill`, `ActivityDrawer` (gaveta recolhível).
3. Crie os nós do React Flow: `TerminalNode` (com os 3 níveis de zoom e os 7 estados), `NoteNode`, `PortalNode`.
4. Crie a aresta `WireEdge` (fio com curva física, pontos de curva editáveis) com o estado de pedido pendente (variante **1d**: tracejado âmbar + pílula com cronômetro no ponto mais baixo).
5. Simule uma conversa: um mock de WebSocket que emite `request.sent → request.pending → reply.sent → reply.received` e anima o pedido 7f3a pelo fio, como na seção 05 da referência.
6. Composer flutuante com @menções em chips, oferecendo só nós ligados ao destino.

Regras inegociáveis:
- Estado nunca só por cor: sempre ícone + texto + cor.
- Nada translúcido ou desfocado sobre terminais.
- Atalhos globais com Alt/⌥; Ctrl pertence ao terminal em foco.
- O terminal com foco de teclado tem anel de 2 px `--focus` e o rótulo "teclado aqui".
- Âmbar (`--st-wait`) só para espera: o nó Esperando e o pedido Pendente usam o mesmo token.

Trabalhe em passos pequenos. Ao terminar cada item da lista, rode o app, compare com a referência e me mostre o resultado antes de seguir.

---
