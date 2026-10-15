# Handoff: Squad, canvas de agentes (F0 + F1)

## Visão geral
O Squad é uma ferramenta local (servidor Node em `127.0.0.1`, interface React no navegador) em que agentes de código e shells viram nós num canvas infinito. Os nós só conversam pelas conexões que o usuário desenha. Este pacote cobre o sistema visual e as telas de F0/F1: canvas principal, nó de terminal (3 zooms × 7 estados), conversa entre agentes e Composer. Contexto de produto completo em `brief.md`.

Para começar no Claude Code, use **`PROMPT.md`**.

## Sobre os arquivos de design
Os arquivos em `design/` são **referências de design feitas em HTML**: protótipos que mostram aparência e comportamento, não código de produção. A tarefa é **recriá-los em React + Vite + TypeScript**, com `@xyflow/react` (canvas) e `@xterm/xterm` + WebGL (terminais). Para visualizar: `npx serve design` e abra `Squad.dc.html`. Os arquivos `.dc.html` dependem de `support.js`, que está na mesma pasta.

## Fidelidade
**Alta fidelidade.** Cores, tipografia, espaçamentos, raios e estados são finais. Recrie pixel a pixel usando `tokens/tokens.css`. Os textos dos terminais são simulados; na implementação, quem desenha a tela é o xterm.

## Decisões fechadas
- Cabeçalho do nó: variante **1a** (faixa de papel no topo + pílula de estado à direita).
- Pedido pendente: variante **1d** (fio tracejado âmbar + pílula com cronômetro no ponto baixo; pontos de curva visíveis quando o fio está selecionado).
- Tema padrão: segue o sistema; escuro é a referência principal. Tema do terminal escolhido à parte (Noite / Papel).
- Conexões: fio com curva física (Bézier cúbica com "barriga" para baixo), sem setas.
- Painel de atividade: gaveta recolhível à direita.
- Vocabulário mantido: workspace, andar, papel, portal, nota.

---

## Telas e componentes

### 1. Canvas principal (`SquadCanvas.dc.html`, seção 01)
Referência de 1760×1040, zoom 80%. Fundo `--bg` com grade de pontos: `radial-gradient(var(--grid) 1px, transparent 1.2px)`, 20×20 px (a grade escala com o zoom no React Flow: use `<Background variant="dots" gap={20} size={1} color="var(--grid)">`).

**TopBar**: altura 44, `--surface`, borda inferior 1px `--line`, padding 0 12, gap 10, itens centralizados verticalmente. Da esquerda para a direita:
1. Logo: grade 2×2 de quadrados 6×6 (gap 2), três cheios em `--text` e o último só com contorno de 1.5px; wordmark "squad" em Geist Mono 700 14px, letter-spacing −0.02em.
2. Divisor vertical de 1×20 `--line`.
3. Seletor de workspace: altura 30, padding 0 8, raio 6, borda 1px `--line`. Quadrado de 10×10, raio 3, na cor do workspace · "squad-api" 13/600 · "3 agentes ativos" 11.5 `--text-2` · kbd "⌥1" (Geist Mono 10, borda 1px `--line-2`, raio 3, padding 0 4) · ícone `expand_more`.
4. Ícone `chevron_right` em `--text-3`.
5. Seletor de andares (segmentado): altura 30, padding 2, raio 6, `--surface-2`, borda `--line`. Aba ativa: altura 24, raio 4, `--surface` com anel de 1px `--line-2`; ícone `layers` 15 + "main" 12.5/600 + "limpo" em mono 11 `--text-3`. Aba inativa: "exp-streaming" + "● 4 alterações" em `--st-wait`. Botão `add` de 24×24.
6. "⎇ main · ~/code/squad-api": mono 11 `--text-3`.
7. Espaçador flexível.
8. Medidor de taxa: ícone `swap_horiz` + "12/30" em mono + "pedidos por min", 11.5 `--text-2`.
9. Botão "Pausar conversas": altura 30, raio 6, borda 1px `--line-2`, 12.5/600, ícone `pause` + kbd ⌥P. **Sempre visível.**
10. Botão "Atividade": mesmo estilo, com contador de pendentes (círculo de 18 px, `--st-wait`, texto `--on-signal` mono 700 11). Quando a gaveta está aberta, o botão fica preenchido (fundo `--text`, texto `--surface`).
11. Estado do servidor: ícone `lan` em `--st-done` + "servidor". Ao reconectar, troque para `--st-wait` + "reconectando…".
12. Ícone `settings`.

**Toolbar** (vertical, flutuante): left 12, top 60, padding 4, gap 2, `--surface`, borda `--line`, raio 8, `--shadow`. Botões de 36×36, raio 6, ícones de 19 px em `--text-2`. Ativo: fundo `--text`, ícone `--surface`. Ordem: `near_me` Selecionar (V), `pan_tool` Mover (H), divisor, `terminal` Novo terminal (T), `sticky_note_2` Nota (N), `public` Portal (P), `gesture` Desenho (D), `select` Grupo (G). Tooltip: altura 24, raio 5, fundo `--text`, texto `--surface` 11.5/600, com o atalho em mono a 75% de opacidade, posicionado 8 px à direita.

**Minimapa**: bottom 18, right 18 (398 quando a gaveta está aberta). 220×132, `--surface`, borda `--line-2`, raio 8. Nós de terminal como retângulos com fundo `--stb-<estado>` e borda de 1.5px `--st-<estado>`; nota e portal em `--surface-3`. Viewport: borda de 1.5px `--text`, raio 3. Abaixo, os controles de zoom: altura 30, raio 7; `remove` · "80%" (mono 600 11.5) · `add` · divisor · `fit_screen`.

**ComposerPill** (recolhido): centralizado, bottom 18, 460×40, raio 10, `--surface`, borda `--line-2`, `--shadow`. Ícone `edit_square` · "Escrever para Implementador…" 13 `--text-3` · kbd ⌥K · botão de 30×30, raio 7, `--surface-3`, ícone `arrow_upward`.

**ActivityDrawer** (aberto): right 0, top 44, bottom 0, largura 380, `--surface`, borda esquerda `--line-2`, `--shadow`.
- Cabeçalho de 46 px: "Atividade" 14/650 + "squad-api · main" 12 `--text-2` + `close`.
- Filtros (chips): altura 24, raio 12. Ativo: fundo `--text`. Inativo: borda `--line-2` + quadrado de 7×7 na cor do papel.
- Item: grid de `52px 1fr`, gap 10, padding 12 14, borda inferior `--line`. Hora em mono 11 `--text-3`. Linha "Origem → Destino id": 12.5/600, nomes na cor do papel, id em mono `--text-3`. Mensagem: 12.5/1.45 `--text-2`. Pílula de desfecho: altura 20, raio 10, `--stb-*`/`--st-*`, 11/600 com ícone.
- Rodapé: "máx. 6 saltos · 30 pedidos/min" em mono 11 + link "Limites".

**Gaveta fechada:** aba vertical presa à borda direita (top 120, largura 28, raio `8 0 0 8`), com `chevron_left` + "Atividade" em `writing-mode: vertical-rl` + contador.

**Disposição dos nós (coordenadas do mundo, zoom 1):** Planejador (120,90) 560×440 · Implementador (860,90) 560×440 · Revisor (860,690) 560×400 · Nota (120,690) 400×350 · Portal (1600,260) 560×420.

### 2. TerminalNode (`SquadNode.dc.html`, seção 02)
Props: `label`, `agent` (claude|codex|gemini|shell), `role` (plan|impl|rev|test|none), `state` (boot|idle|work|wait|done|err|off), `elapsed`, `focused`, `inbox`, `strip` ({kind: from|wait|reply, text, fromRole}), `andar`, `cols×rows`, `cwd`.

**Nível vivo (zoom ≥ 60%)**: 560×440 por padrão (≈80×24). `--surface`, borda 1px `--line-2`, raio 8, `--shadow`, overflow hidden, coluna flex.
- Faixa de papel: 3 px, `--r-<role>`.
- Cabeçalho (variante 1a): altura 32, padding 0 8 0 10, gap 8, borda inferior `--line`.
  - Nome: 12.5/600 `--text`.
  - Chip do agente: Geist Mono 500 10.5/16, `--text-2`, borda 1px `--line-2`, raio 4, padding 0 5.
  - Chip do papel: altura 20, padding 0 7 0 5, raio 4, fundo `--rb-<role>`, cor `--r-<role>`, 11/600, ícone de 14.
  - Espaçador.
  - (se focado) ícone `keyboard` 15 + "teclado aqui" 11/600 `--text`.
  - Pílula de estado: altura 22, padding 0 8 0 6, raio 11, fundo `--stb-<state>`, cor `--st-<state>`, 11.5/600, ícone de 15, tempo em mono 10.5.
  - `more_horiz` 18 `--text-3` (menu: parar, reiniciar, trocar papel, abrir em foco, encaixar na borda, ocultar conteúdo).
- Faixa de remetente (opcional, **acima** do terminal, nunca por cima): altura 24, padding 0 10, 11/600, borda inferior `--line`.
  - `from`: fundo `--rb-<papel do remetente>`, cor `--r-<papel do remetente>`, ícone `call_received`, texto "Pedido 7f3a de Planejador · a resposta volta para ele".
  - `wait`: `--stb-wait`/`--st-wait`, ícone `hourglass_top`, "Aguardando Implementador · pedido 7f3a · enviado há 42 s".
  - `reply`: `--stb-done`/`--st-done`, ícone `reply`, "Resposta de Implementador · 7f3a · 2m14s".
- Área do xterm: flex 1, fundo `--term-bg`, padding 8 10, Geist Mono 11/15. Tema em `tokens/xterm-themes.ts`.
- Rodapé: altura 22, padding 0 10, gap 12, borda superior `--line`, mono 500 10 `--text-3`: "⎇ main" · "80×24" · "caixa N" (em `--st-wait`, se houver) · espaçador · cwd.
- Foco: `outline: 2px solid var(--focus); outline-offset: 3px`.

**Nível miniatura (35–60%)**: 280×210 na tela. Faixa de 3 px, cabeçalho de 28 (nome 12/600, agente mono 10, pílula de estado de 20 de altura). Corpo: snapshot de texto em Geist Mono 5.5/7.5 (fazer `serialize` do buffer xterm a cada ~3 s; não manter o WebGL vivo). Rodapé de 20: `photo_camera` + "miniatura · há 3 s" + caixa.

**Nível cartão (< 35%)**: 240 de largura. Faixa de 5 px. Corpo com padding 10 12: nome 18/650 (letter-spacing −0.01em) + contador da caixa (círculo de 22, `--st-wait`). Linha "codex · Implementador" 12.5 (papel em `--r-*` 600). Banda de estado: altura 36, fundo `--stb-*`, cor `--st-*`, ícone de 20 + rótulo 14/650 + tempo em mono 600 12, borda superior `--line`.

Durante o gesto de zoom, congele o conteúdo (último frame) e redesenhe no `onMoveEnd`.

**Estados** (ícones Material Symbols):
| estado | rótulo | ícone |
|---|---|---|
| boot | Iniciando | progress_activity |
| idle | Ocioso | radio_button_unchecked |
| work | Trabalhando | autorenew |
| wait | Esperando resposta | hourglass_top |
| done | Terminou | check_circle |
| err | Parado · erro (ou "Parado · saída N") | error |
| off | Descarregado | bedtime |

### 3. NoteNode
400×350, fundo `--note-bg`, borda `--line-2`, raio 8. Cabeçalho de 36: `description` + título 13/600 ("Plano: exportar CSV") + nome do arquivo em mono 11 `--text-3` ("PLANO.md", sempre separado do título) + `lock_open`/`lock` + toggle "Prévia/Editar". Corpo em Markdown com padding 12 16, 13/1.5, h1 16/650, checkboxes `check_box` em `--st-done` ou `check_box_outline_blank` em `--text-3`. Trecho editado por um agente: bloco com raio 6, fundo `--rb-<papel>`, rótulo "Revisor escreveu há 12 s" 11/600 na cor do papel com ícone `edit`. O destaque some depois de ~30 s.

### 4. PortalNode
560×420. Faixa de 3 px `--line-2`. Cabeçalho de 36: `public` + "Portal" + `refresh` + barra de URL (altura 24, raio 5, `--surface-2`, borda `--line`, mono 11.5) + "● ao vivo · 120 ms" (`--st-done`; acima de 400 ms, `--st-wait`). Corpo: imagem transmitida (`<img>` ou `<canvas>`). Quando um agente clica: cursor `arrow_selector_tool` de 22 px na cor do papel + etiqueta com o nome do agente (fundo `--r-*`, texto `--on-signal`, 11/600, raio 4). No fio, a pílula "clicando" com o ícone `ads_click`.

### 5. WireEdge (seção 04, variante 1d)
- Caminho: `M sx sy C sx+a sy+sag, tx-a ty+sag, tx ty`, com sag ≈ 0.35 × distância, limitado a 120. Pontos de curva do usuário (`ropePoints`) viram nós de controle extras.
- Repouso: stroke `--wire` 2.5 px (no mundo), round cap. Alças: círculo de r 6, fill `--surface`, stroke `--line-2` 2 px.
- **Pendente**: stroke `--st-wait` 3 px, `stroke-dasharray: 8 6`. A alça de origem fica preenchida em `--st-wait`; a de destino, com anel `--st-wait`. Pílula no ponto mais baixo da curva: altura 30 (mundo), raio total, fundo `--surface`, borda de 2 px `--st-wait`, `hourglass_top` + tempo "0:42" (Geist Mono) + id "7f3a" em `--text-2`.
- Selecionado: mostrar os pontos de curva (r 4, fill `--surface`, stroke `--text` 1.5).
- Use `EdgeLabelRenderer` para as pílulas.

### 6. Conversa em andamento (seção 05)
Quatro momentos do pedido 7f3a (Planejador → Implementador):
1. **Viajando**: o fio acende em `--st-wait` da origem até o envelope (`pathLength=100`, `stroke-dasharray: p 200`, p animado de 0 a 100 em ~600 ms, `ease-out`). Envelope: altura 24, raio 5, fundo `--st-wait`, texto `--on-signal` 700 12, `mail` + "7f3a" + `arrow_forward`. A origem entra em **Esperando resposta**.
2. **Pendente**: fio tracejado 1d + cronômetro. No destino aparece a faixa `from`; o terminal recebe o envelope `[de: Planejador · pedido 7f3a]` colado antes do prompt.
3. **Resposta chegando**: o fio acende em `--st-done` do destino para a origem; envelope verde com `arrow_back` + "resposta 7f3a" + `reply`. O destino passa a **Terminou**. Disparo pelo sinal nativo de fim de turno (hooks do Claude Code, `notify` do Codex).
4. **Recebida**: o fio volta ao repouso, com um marcador "✓ 7f3a · 2:14" que some em ~4 s. A origem mostra a faixa `reply` e passa a **Trabalhando**.

Só esses envelopes animam; nada de animação contínua nos nós. Respeite `prefers-reduced-motion` (pule direto para o estado final).

### 7. Composer (seção 06)
Flutuante, 680 de largura, raio 12, `--surface`, borda `--line-2`, `--shadow`. Abre com ⌥K, fecha com Esc.
- Linha "Para": "Para" 12 `--text-3` + chips de destino (altura 24, raio 5, `--rb-*`/`--r-*`, ícone do papel, nome 12.5/600, agente em mono, `close`) + "+ destino" + aviso do estado do destino (ex.: "trabalhando · entra na fila" em `--st-work`).
- Corpo: 14/30. Menção vira chip inline (altura 22, raio 4, `--surface-3`, 13/600, ícone por tipo: `description` nota, `public` portal, quadrado de papel para terminal).
- Menu @: aparece **acima** do Composer, 310 de largura, raio 8. Seção "Ligados a <destino>" com itens de 34 de altura (destaque `--surface-3`, trecho digitado sublinhado, estado do agente à direita). Seção "Não ligados" desabilitada, com `link_off` + "ligue para mencionar". Com vários destinos, só aparecem nós ligados a todos eles.
- Anexos: cartão de 44 de altura com miniatura de 52×36, nome e "1440×900 · colada".
- Rodapé: `image` · `alternate_email` · dica "⏎ enviar · ⇧⏎ nova linha · Esc fecha" · botão "Enviar" (altura 32, raio 7, fundo `--text`, texto `--surface`, 13/600, `arrow_upward`). Com vários destinos, o botão diz "Enviar a 2".

---

## Interações e comportamento
- **Conectar**: arraste a partir da alça; durante o arraste, destinos válidos ganham anel de 2 px `--text` e os inválidos ficam com 40% de opacidade. Ao soltar, mostre um toast discreto "Planejador ↔ Implementador ligados".
- **Apagar conexão**: corta a conversa na hora. Pedidos pendentes nela viram "cancelado".
- **Zoom em níveis**: ≥ 0.6 vivo · 0.35–0.6 miniatura · < 0.35 cartão. Use `useStore(s => s.transform[2])` com histerese de ±0.03.
- **Foco de teclado**: um clique no terminal dá o foco ao xterm. Esc duas vezes (ou ⌥Esc) devolve o foco ao canvas. Tab/⇧Tab navegam entre nós quando o canvas tem o foco.
- **Atalhos globais (⌥)**: ⌥1–9 workspace · ⌥K Composer · ⌥P pausar conversas · ⌥A gaveta de atividade · ⌥F foco em tela cheia · ⌥T/N/P criar nó.
- **Pausar conversas**: novos pedidos ficam retidos e o fio mostra `pause`. O botão vira "Retomar".
- **Laço**: ao passar do limite de saltos, pause automaticamente e mostre um aviso na conexão e na gaveta.
- **Erros**: agente ausente → estado err com mensagem e ações (r reiniciar, e editar comando). WebSocket caindo → TopBar em "reconectando…" (`--st-wait`); os terminais continuam no estado anterior, porque o processo sobrevive.

## Estado (sugestão)
- `workspaces[]`, `activeWorkspaceId`, `floors[]` (id, branch, alterações, podeAterrissar), `activeFloorId`.
- Nós do React Flow com `data`: { kind, label, agent, role, state, stateSince, inbox[], strip }.
- Arestas com `data`: { ropePoints[], pending?: { id, from, startedAt, hop } , paused }.
- `activity[]` (eventos), `limits` { maxHops: 6, perMinute: 30 }, `paused`.
- Eventos do servidor (WS): `terminal.state`, `request.sent`, `request.delivered`, `reply.sent`, `reply.received`, `note.edited`, `portal.frame`, `server.status`.

## Design tokens
Todos em `tokens/tokens.css`. Resumo:
- **Neutros escuros**: bg #111214 · surface #1a1b1e · surface-2 #212327 · surface-3 #2b2d32 · line #2c2e33 · line-2 #3c3f46 · text #e8e9ec · text-2 #a9acb3 · text-3 #8a8e96 · wire #5a5e68.
- **Neutros claros**: bg #f2f2ef · surface #fff · surface-2 #f7f7f5 · surface-3 #ebebe8 · line #e3e3df · line-2 #cbcbc6 · text #17181a · text-2 #4f5258 · text-3 #666970 · wire #9c9d99.
- **Estados** (matiz oklch): boot 250 (croma baixo) · work 245 · wait 78 · done 152 · err 25; idle e off neutros. Escuro em L≈0.77–0.83; claro em L≈0.50–0.53. Os fundos tingidos (`--stb-*`) ficam em L 0.28 (escuro) e 0.95 (claro).
- **Papéis** (fora da faixa dos estados): plan 300 (violeta) · impl 200/210 (ciano) · rev 350 (rosa) · test 110 (lima).
- **Tipografia**: Geist (UI), Geist Mono (terminal e dados). Escala: 22/650 título · 18/650 cartão · 14 Composer · 13 corpo · 12.5/600 cabeçalho · 11/600 rótulo (uppercase, +0.06em) · 11/15 terminal · 10.5 mono dado.
- **Espaçamento**: base 4 (2, 4, 6, 8, 12, 16, 24, 32).
- **Raios**: 4 chip · 6 controle · 8 nó · 12 Composer · pílula.
- **Sombra**: escuro `0 1px 0 rgba(0,0,0,.35), 0 10px 28px rgba(0,0,0,.35)`; claro `0 1px 0 rgba(0,0,0,.04), 0 6px 18px rgba(20,20,15,.08)`.

## Assets
- Fontes: Geist e Geist Mono (Google Fonts ou o pacote npm `geist`).
- Ícones: Material Symbols Outlined (peso 400, opsz 20). Sugestão: `@material-symbols/font-400` ou um subset.
- Logo: geométrico, feito em CSS (grade 2×2, um quadrado vazio). Não há imagens.

## Arquivos
- `design/Squad.dc.html`: página de referência completa (seções 00 a 06).
- `design/SquadCanvas.dc.html`: canvas principal (prop `drawer`).
- `design/SquadNode.dc.html`: nó de terminal (props `zoom`, `state`, `role`, `variant` etc.; contém as telas simuladas).
- `design/support.js`: runtime que só serve para abrir os arquivos acima.
- `tokens/tokens.css`, `tokens/xterm-themes.ts`: use diretamente.
- `brief.md`: brief de produto original.
- `PROMPT.md`: prompt inicial para o Claude Code.
