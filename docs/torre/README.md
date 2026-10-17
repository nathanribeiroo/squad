# Handoff: Squad → Torre

## Visão geral

O Squad passa a se chamar **Torre**. Esta pasta reúne tudo para o Claude Code fazer a troca: a marca nova, o vocabulário novo da interface, o redesign de todas as telas, as funcionalidades novas e o que elas exigem do backend.

- **O produto não muda de natureza:** continua sendo o binário Go local com a interface React embutida, os terminais reais no canvas e os fios que dão permissão para os agentes conversarem.
- **O que muda:** nome, identidade visual, vocabulário, a organização da tela (duas vistas: Sinóptico e Espaço) e alguns recursos novos (o principal é o estado **PRIORIDADE**, "o voo pede você").
- **Ponto de partida do design:** a exploração "Torre", opção **9d Sinóptico · a síntese** (`design/origem/Torre.dc.html`, último quadro). Os quatro arquivos `design/Torre 0X *.dc.html` destrincham essa tela no produto inteiro.

## Como ler os arquivos de design

- São **referências de design feitas em HTML**: mostram aparência e comportamento, **não são código de produção**. A tarefa é recriá-los no código que já existe (React + TypeScript + CSS com tokens, React Flow, xterm.js, zustand), seguindo os padrões do repositório.
- Abra os `.dc.html` direto no navegador. Precisam do `support.js` e do `TorreBar.dc.html` na mesma pasta (a barra superior é um componente compartilhado).
- Cada arquivo é uma página de apresentação (fundo bege) com quadros de **1440×900**. Cada quadro tem um número (01 a 22), um título, uma descrição e, embaixo, três notas: o que muda, como se comporta e o que precisa para implementar.
- Marcas **NOVO** (laranja) indicam o que não existe hoje.
- Os mockups usam dados de exemplo e, em alguns textos, ainda o nome `squad` nos comandos e nas pastas. **Vale o `RENOMEACAO.md`.**

## Fidelidade

**Alta fidelidade.** Cores, tipografia, medidas, estados e textos são finais. Recrie com precisão usando os tokens de `tokens/torre-tokens.css`. Os quadros são fixos em 1440×900 só para apresentação; o app é fluido a partir de 1280 px.

## Documentos desta pasta

| Arquivo | O que é |
|---|---|
| `PROMPT-INICIAL.md` | O prompt para colar no Claude Code. |
| `README.md` | Este arquivo: visão geral, tokens, fases. |
| `RENOMEACAO.md` | Tudo o que muda de nome, dentro e fora da interface, com compatibilidade e migração. |
| `NOVIDADES-E-BACKEND.md` | Funcionalidades novas, o que é só front e o que exige backend, com o contrato proposto. |
| `TELAS.md` | As 22 telas, uma a uma, ligadas aos componentes de hoje. |
| `tokens/torre-tokens.css` | Tokens novos (escuro e claro), no mesmo formato de `src/theme/tokens.css`. |
| `tokens/xterm-themes-torre.ts` | Temas Noite e Papel dos terminais, recalibrados. |
| `design/` | Os mockups (`Torre 01 Nucleo`, `02 Comunicacao`, `03 Estrutura`, `04 Sistema`, `TorreBar`, `support.js`). |
| `design/origem/Torre.dc.html` | A exploração original da Torre (9a a 9d). |
| `referencia-squad-atual/` | O pacote do Squad de hoje: `FUNCIONALIDADES.md`, `REFERENCIA-TECNICA.md`, `capturas/`, `html-renderizado/`, `codigo-web/`, `design-original/`, `contexto/` (com o README do projeto e o brief original). |

## Mapa dos mockups

| Página | Quadros |
|---|---|
| `Torre 01 Nucleo.dc.html` | Sistema visual (vocabulário, estados, papéis, tipos) · 01 Sinóptico · 02 Espaço (canvas) · 03 Conversa na frequência · 04 Níveis de zoom e Foco · 05 Prioridade |
| `Torre 02 Comunicacao.dc.html` | 06 Transmitir (Composer) · 07 Rádio (Atividade) · 08 Boletim e Briefing · 09 NOTAM (nota) · 10 Portal |
| `Torre 03 Estrutura.dc.html` | 11 Workspaces · 12 Sessões · 13 Níveis de voo · 14 Autorização de pouso · 15 Confirmações na interface |
| `Torre 04 Sistema.dc.html` | 16 Novo voo e menu do voo · 17 Papéis · 18 Configurações · 19 Primeira abertura · 20 Espaço vazio e Planos de voo · 21 Arremetida, espera geral e falhas · 22 Tema claro |

## Estrutura da tela

- **Barra superior (48 px)**, lida como um caminho: logo TORRE · workspace (cor, nome, ⌥1, ponto de atividade em outro workspace) › sessão › abas PISTA / FL1 / FL2… com `+`. À direita: a troca de vista SINÓPTICO | ESPAÇO, ESPERA GERAL (⌥P), RÁDIO com contador (⌥A), ENLACE (estado do servidor), ⚙ e o relógio. O medidor de pedidos por minuto e o caminho da pasta **saíram da barra**: foram para o Rádio, o Sinóptico, o rodapé dos voos e o menu do nível.
- **Vista Espaço:** o canvas de hoje (React Flow), com a barra de ferramentas à esquerda, o minimapa em forma de radar embaixo à direita, a pílula Transmitir embaixo no centro e a aba do Rádio na borda direita.
- **Vista Sinóptico (nova):** um painel para deixar aberto. Radar com todos os voos da sessão (pista e níveis em anéis), fichas, fila de pouso, frequências, o voo que pede você em destaque, o terminal selecionado e o rádio correndo embaixo.

## Tokens (resumo; o completo está em `tokens/torre-tokens.css`)

**Escuro (referência principal)**

- Fundo do app `#030705`, canvas `#050b08` com pontos `#15301f` a cada 20 px, superfícies `#07100b` / `#0b1710` / `#10241a` (selecionado), fundo do terminal `#040806`.
- Linhas `#16301f` / `#1f3a29` / `#2c5a3d`, fio em repouso `#3f6b52`.
- Texto `#cfeedd` / `#9fc4ae` / `#7fa892` / `#5a8a6c` (este último só em dado mono de 10,5 px para cima).
- Botão principal: fundo `#cfeedd`, texto `#030705`. Anel de foco: 2 px `#cfeedd`.
- Acento da marca `#e05a2b` (farol do logo, rótulo RÁDIO, etiqueta NOVO). Verde do logo `#7fe0a0`.

**Estados (faixa · texto · fundo)**

| Estado | Palavra | Ícone | Faixa | Texto | Fundo |
|---|---|---|---|---|---|
| `boot` | TAXIANDO | `progress_activity` | `#5b7a8c` | `#a8c2d2` | — |
| `idle` | PÁTIO | `radio_button_unchecked` | `#3a4a40` | `#9fb5a8` | — |
| `work` | NO AR | `flight` | `#3b74d0` | `#a9c8ff` | `#0b1730` |
| `wait` | AGUARDA | `hourglass_top` | listrada `#e0a526` | `#ffd27a` | `#1a1407` |
| `ask` (novo) | PRIORIDADE / PEDE VOCÊ | `front_hand` | `#e0a526` cheia, piscando 1 Hz | `#ffd27a` | `#1a1407` |
| `done` | EM SOLO | `flight_land` | `#2e9b5b` | `#8fe8b4` | `#07130c` |
| `err` | PANE | `error` | `#d9443c` | `#ff9a8f` | `#1a0908` |
| `off` | HANGAR | `bedtime` | `#26332b` | `#7fa892` | — |

**Papéis** (só no quadradinho de 8 px e nos nomes): plan `#b49cff` · indigo/arq `#8fa2ff` · impl `#6fd3d8` · rev `#f29ac0` · test `#c6e06a` · teal `#5fd6b4` · magenta `#e08ae8` · orange `#f0a66a`.

**Papel (fichas e NOTAM):** papel `#efe8cf`, tinta `#1b1a14`, tinta 2 `#6b6450`, linha `#cfc6a6`.

**Claro:** canvas `#e9e3cf`, superfície `#f6f2e4`, terminal `#fbf8ee`, linha `#cfc6a6`, texto `#1b1a14` / `#5b5747`. As faixas de estado são as mesmas; muda a tinta do texto (NO AR `#1f4f9f`, AGUARDA `#7a4f00`, EM SOLO `#1f6b3a`, PANE `#a3231b`).

**Tipografia:** B612 (interface) e B612 Mono (dados, indicativos, terminais), as duas com licença OFL. Embuta os arquivos no binário (o app roda sem internet). Escala principal:

| Uso | Tamanho / peso |
|---|---|
| Wordmark TORRE | 15 / 700, espaçamento .1em |
| Rótulos de seção, MAIÚSCULAS | 10,5–11,5 / 700, espaçamento .12em |
| Botões, MAIÚSCULAS | 11–12 / 700, espaçamento .06–.08em |
| Corpo | 12,5–13 |
| Título do nó | 12,5 / 700 |
| Indicativo | 13 mono / 700 (vivo) · 16 (retrato) · 22 (ficha) |
| Dados | mono 10,5–11,5 |
| Terminal | mono 11/15,5 no canvas; 11,5/16,5 no Sinóptico |
| Título de diálogo | 14–15 / 700, espaçamento .1em |

**Forma e medidas:**

- **Cantos:** retos em tudo (raio 0), como as fichas de voo. Só os pontos de estado são círculos.
- **Sombras:** popover `0 14px 36px rgba(0,0,0,.6)`, diálogo `0 20px 60px rgba(0,0,0,.6)`, fundo de diálogo `rgba(1,3,2,.72–.85)`.
- **Alturas:** barra superior 48 · cabeçalho do voo 34 · faixa de remetente 24 (26 na prioridade) · rodapé do voo 22 · boletim 30 · botões da ferramenta 32 (barra de 40) · pílula Transmitir 40 (500 de largura; aberto, 680) · gaveta do Rádio 440 · minimapa 230×118 · botões 30–38 · campos 32–34.

**Movimento:**

- **No Sinóptico:** a varredura do radar gira em 6 s e o voo no ar circula em ~14 s.
- **Nas frequências:** o tracejado do pedido pendente desliza em 1,6 s; os envelopes duram ~600 ms.
- **O que pisca:** a prioridade (1 Hz, opacidade 1→0,3) e o cursor. O rádio passa embaixo em 40 s.
- **`prefers-reduced-motion`:** nada se move.

## Ícones

Material Symbols Outlined, como hoje. Entram ícones novos no subconjunto. É preciso rodar `scripts/subset-icons.py` e conferir com `scripts/check-icons.mjs`:

> `flight`, `flight_land`, `flight_takeoff`, `flight_class`, `front_hand`, `cell_tower`, `all_inclusive`, `keep`, `call_received`, `swap_horiz`, `ads_click`, `data_object`, `u_turn_left`, `summarize`, `select_all`, `edit_note`, `library_add`, `upload`, `contrast`, `smart_toy`, `notifications`, `visibility_off`, `open_in_full`, `close_fullscreen`, `add_comment`, `restart_alt`, `bedtime`, `layers`, `play_arrow`, `mail`, `send`, `keyboard`

Confira um por um contra `src/lib/icons.ts`.

## Logo

A torre de controle, em SVG (está no topo de cada mockup, `viewBox 0 0 56 64`): base `M20 62 L24 26 H32 L36 62 Z`, cabine `M10 26 H46 L40 12 H16 Z`, vidro verde `M18 14 H38 L35 22 H21 Z` (`#7fe0a0`) e o farol laranja `circle cx=28 cy=4 r=3` (`#e05a2b`). Ele substitui a grade 2×2 do Squad na barra, no favicon, nos ícones do PWA (192 e 512) e na página de login.

## Plano por fases (sugestão)

| Fase | O que entra | Backend? |
|---|---|---|
| **F0 · Renomeação e base** | Nome, logo, favicon, manifest, `<title>`, página de login; binário, pastas, variáveis, MCP e skill com compatibilidade (`RENOMEACAO.md`); tokens, fontes B612 e ícones novos; vocabulário da interface. Sem mudar comportamento. | sim (renomeação e migração) |
| **F1 · Redesign do que existe** | Barra superior, ferramentas, voo (os 3 níveis de zoom), frequências, Transmitir, Rádio, boletim, briefing, NOTAM, portal, workspaces, sessões, níveis, pouso, papéis, novo voo, menus, toasts, tema claro, terminais Noite e Papel. | não |
| **F2 · Confirmações e vazios** | As 10 `window.confirm` viram diálogos da interface (quadro 15); espaço vazio com primeiros passos (20); arremetida e espera geral no estilo novo (21); erros do servidor em português. | pouco (mensagens) |
| **F3 · PRIORIDADE** | O estado novo `ask` de ponta a ponta (quadro 05). | **sim** |
| **F4 · Sinóptico e configurações** | A vista Sinóptico (01); o painel de configurações com limites, agentes, avisos e atalhos (18); a primeira abertura (19). | sim (limites, agentes, onboarding) |
| **F5 · Extras** | Indicativo e sigla do papel, numeração FL e renomear nível, cancelar um pedido, NOTAM redimensionável, Foco/encaixe/ocultar/Tab, @ELEMENTOS no portal, Planos de voo, transmitir a um nível inteiro, checagem prévia de conflito no pouso. | sim, em parte |
| **Depois** | Pilha de NOTAMs, Desenho, Grupo (só aparecem na barra de ferramentas; não foram desenhados em detalhe). | sim |
