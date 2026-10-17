# As 22 telas da Torre

Cada tela diz onde está o mockup, o que faz, o que a compõe e qual componente de hoje ela substitui. Os caminhos de código são relativos ao `src/` da interface web (no pacote de referência: `referencia-squad-atual/codigo-web/src/`). Medidas em px, cores em hex; os tokens estão em `tokens/torre-tokens.css`.

---

## Sistema visual (topo de `Torre 01 Nucleo.dc.html`)

- **Vocabulário:** a tabela de `RENOMEACAO.md`, item 2.
- **Estados:** faixa à esquerda + ícone + PALAVRA, sempre os três juntos. A faixa tem 6 px no voo vivo, 8 no retrato e 9 na ficha.
- **Papéis:** a cor do papel aparece só num quadradinho de 8×8 antes do indicativo, nos nomes do Rádio e nos chips. Ela nunca pinta o voo inteiro (a faixa é sempre o estado).
- **Indicativo:** 3 letras em B612 Mono 700.

---

## 01 · Sinóptico (vista nova) · `Torre 01`, quadro 01

**Para quê:** deixar aberto o dia inteiro e saber, de relance, quem pede você, quem espera quem e o que está pronto para pousar.

**Layout (1440×900):** barra superior (48) · coluna do radar (0–620) · coluna central de 330 (fichas, fila de pouso, frequências) a partir de x=630 · coluna direita de 432 (prioridade e terminal) a partir de x=984 · rodapé de 100 (rádio e Transmitir).

**Radar:**

- **Desenho:** círculo r=290, fundo radial `#0a1f12`→`#040b07`, anéis `#163020` (um por nível: PISTA no centro, FL1, FL2…), cruz `#122818`.
- **Varredura:** setor com gradiente `#3cff8a` a 0→30%, girando em 6 s.
- **Voos como símbolos:**

  | Símbolo | Estado |
  |---|---|
  | ▲ azul `#a9c8ff` | no ar (circula devagar) |
  | ● âmbar com anel pulsando | pede você |
  | □ verde `#5fe39a` | em solo |
  | ○ cinza | hangar |
  | △ âmbar | aguarda |

- **Bloco de dados ao lado de cada voo** (mono 10,5): indicativo, nível e estado.
- **Linhas entre os símbolos:** pedido pendente (âmbar tracejado) e resposta (verde).
- **Clique** num símbolo: seleciona o voo. Duplo clique: abre o Espaço centrado nele.
- **Em cima:** os contadores (1 NO AR · 1 PRIORIDADE · 1 AGUARDA · 1 EM SOLO · 1 HANGAR). **Embaixo:** a legenda.

**Fichas:**

- **Linha de 44 px:** grade `7px | 50px | 1fr | 82px` sobre papel `#efe8cf`, com faixa de estado, indicativo (13 mono 700), o que está fazendo + "agente · nível", e o estado com o tempo.
- **Ordem:** prioridade > aguarda > no ar > em solo > hangar.
- **A ficha em prioridade** tem anel âmbar de 2 px e a faixa piscando.

**Fila de pouso:**

- Uma caixa por nível com commits, com borda de 1,5 px `#cfeedd`.
- **Conteúdo:** "FL1 · feat/export-csv → main · 3 commits", as checagens (item 6 de `NOVIDADES-E-BACKEND.md`) e o botão "AUTORIZAR POUSO…", que abre o diálogo 14.

**Frequências:** os fios com pedido aberto (âmbar), com resposta recente (verde) e os outros (cinza), mais a taxa e os saltos.

**Prioridade:**

- **Cartão de 432×256:** borda 2 px `#e0a526`, fundo `#1a1407`.
- **Cabeçalho:** ponto piscando, "PRIORIDADE · PEDE VOCÊ", "1 de 1" e o cronômetro.
- **Corpo:** indicativo em 24 + nome + "agente · nível" + VER NO ESPAÇO; a pergunta num bloco `#0c0a04`; AUTORIZAR (preenchido `#ffd27a`) · SEMPRE · NEGAR, com as teclas.
- **Sem prioridade,** o cartão some e o terminal sobe.

**Terminal selecionado:** voo completo (cabeçalho, faixa de remetente, tela, rodapé) com alfinete para fixar. Rodapé: "segue a seleção · clique para digitar".

**Rodapé:** etiqueta RÁDIO (`#e05a2b`) + letreiro passando com as últimas mensagens (40 s por volta; parado com reduced-motion) + barra TRANSMITIR de largura total.

---

## 02 · Espaço (canvas) · `Torre 01`, quadro 02 → `canvas/Canvas.tsx`, `canvas/nodes/*`

- **Canvas:** `#050b08` com pontos `#15301f` a cada 20 px.
- **Barra de ferramentas** (`Toolbar.tsx`):
  - 40 de largura, botões de 32. O ativo fica preenchido (`#cfeedd` com ícone `#030705`).
  - Grupos: Selecionar, Mover | Novo voo, NOTAM, Portal | Desenho, Grupo, Planos de voo.
- **Voo vivo** (`TerminalNode.tsx`):
  - **Cabeçalho de 34** (`#07100b`): faixa de estado de 6 · quadradinho do papel de 8 · indicativo 13 mono 700 · nome 12,5/700 · chip do agente (mono 10,5, borda `#2c5a3d`) · chip "líder" (ícone `groups`) · chip "⌨ TECLADO" (preenchido `#cfeedd`) · espaço · estado (ícone 14 + PALAVRA + tempo, mono 11/700 na cor do estado) · ⋯.
  - **Faixa de remetente de 24**, entre o cabeçalho e a tela:

    | Caso | Fundo | Texto |
    |---|---|---|
    | Aguardando | `#1a1407` | `#ffd27a` |
    | Pedido chegando | tinta do papel de quem pediu (PLN: `#0f0c1c`) | clara (PLN: `#cbbdff`) |
    | Resposta recebida | `#07190f` | `#8fe8b4` |

  - **Tela:** xterm sobre `#040806`.
  - **Rodapé de 22:** "NÍVEL · branch", grade, "caixa N" em âmbar, "● criado por PLN", a pasta.
  - **Voo com o teclado:** borda de 2 px `#cfeedd`.
- **Boletim** (`TurnCard.tsx`): barra de 30 logo abaixo do nó (borda `#1f4a30`, fundo `#07130c`) com ícone, "BOLETIM · 31s", a primeira linha, RESUMIR e ×.
- **NOTAM** (`NoteNode.tsx`): quadro 09.
- **Portal** (`PortalNode.tsx`): quadro 10.
- **Frequências** (`WireEdge.tsx`): curva com barriga, como hoje.
  - Em repouso: `#3f6b52`, 2,5 px, com bolinhas nas pontas.
  - Os outros estados estão no quadro 03.
- **Entre níveis** (`CrossFloorLinks.tsx`): etiqueta "⇄ TST · FL1" embaixo do nó (borda tracejada `#3f6b52`); em âmbar enquanto passa um pedido.
- **Minimapa** (`ZoomControls.tsx`):
  - **Scope de 230×118:** retângulos na cor do estado (contorno) e cinza para NOTAM e portal; a janela de visão com borda `#cfeedd`; uma linha de varredura lenta.
  - **Embaixo:** − 70% + ⛶.
- **Aba do Rádio** na borda direita: 24 de largura, "RÁDIO" na vertical e o contador âmbar.
- **Pílula Transmitir** (`ComposerPill.tsx`): 500×40, borda 1,5 `#cfeedd`, "TRANSMITIR → ■ IMP · escreva para o voo… ⌥K ↑".
- **Faixa de prioridade:** 500×30 no topo central, quando um voo pede você e não está visível.

---

## 03 · Conversa na frequência · `Torre 01`, quadro 03 → `WireEdge.tsx`, `TerminalNode.tsx`

A sequência de hoje (`FUNCIONALIDADES.md` §11), no estilo novo:

1. **O pedido sai:** a frequência acende em âmbar `#e0a526`; o envelope âmbar cheio "✉ 7f3a →" corre (~600 ms). Quem pediu passa a AGUARDA.
2. **Pendente:** tracejado âmbar `7 6` deslizando; ponta cheia em quem pediu e anel em quem atende; pílula "⧗ 0:42 · 7f3a" (fundo `#2a1f07`, borda `#e0a526`) no ponto mais baixo. Quem atende mostra a faixa "↙ Pedido 7f3a de PLN…" e "caixa 1".
3. **A resposta volta:** a frequência acende em verde `#2e9b5b`; o envelope verde "← resposta 7f3a ↩" volta. Quem pediu mostra "↩ Resposta de IMP · 7f3a · 1:58" (60 s); quem atendeu passa a EM SOLO e ganha o boletim.

**Estados da frequência:**

| Estado | Como aparece |
|---|---|
| Em silêncio | Repouso, como descrito acima |
| Selecionada | `#cfeedd`, pontos de curva quadrados e o botão "DESLIGAR" no ponto mais baixo |
| Cancelada | Vermelho a 60%; pílula "⊘ 7f3a cancelado" (`#2a0d0b` / `#d9443c` / `#ff9a8f`) |
| Retida (espera geral) | Pontilhado `2 5`; pílula tracejada âmbar "∞ retido" |
| Respondida | Verde; "✓ c8d0 · 0:31" por ~4 s |
| Ação no portal | Na cor do papel, com a pílula da ação ("clicando"…) por 2,5 s |
| Entre níveis | Etiqueta |
| Laço | "∞ laço · 6 saltos · espera geral" em âmbar cheio |

---

## 04 · Níveis de zoom e Foco · `Torre 01`, quadro 04 → `TerminalNode.tsx` (vivo/miniatura/cartão)

| Zoom | Nome | Como fica |
|---|---|---|
| ≥ 60% | **Vivo** | Como no quadro 02. |
| 35–60% | **Retrato** | Cabeçalho de 38 (faixa de 8, indicativo 16, estado com tempo); tela como texto em mono 9 `#6f8a7b`, atualizada a cada ~3 s; rodapé de 26 "retrato · há 3 s · caixa N". |
| < 35% | **Ficha** | Ficha de voo em papel de 260×72: faixa de 9, indicativo 22, "agente · papel", caixa ou líder; à direita, separada por tracejado, ícone 18 + PALAVRA + tempo, na tinta do estado sobre papel (`#1f4f9f` / `#7a4f00` / `#1f6b3a`). |

- Histerese de ±3% nos limites; congela durante o gesto; o portal pausa abaixo de 60%. Igual a hoje.
- **Foco (novo):**
  - ⌥F põe o voo em tela cheia.
  - ⌥⇧←/→ encaixa o voo numa borda, separado do canvas por um divisor de 2 px `#cfeedd`; o resto do canvas continua ao lado, em fichas.
  - Tab / ⇧Tab passam a seleção; Enter dá o teclado.
  - ⋯ → Ocultar troca a tela por um padrão listrado opaco.

---

## 05 · Prioridade · `Torre 01`, quadro 05 → estado novo

Todo o comportamento está em `NOVIDADES-E-BACKEND.md`, item 1. Medidas:

- **Contorno do voo:** 2 px `#e0a526` + halo `0 0 0 6px rgba(224,165,38,.16)`.
- **Cabeçalho:** fundo `#1a1407`.
- **Faixa de 26:** âmbar cheio `#e0a526` com texto `#1a1407` em mono 11,5/700.
- **Barra RESPONDER** (fora do nó, 4 px abaixo): 44 de altura, borda 1,5 `#e0a526`.
  - AUTORIZAR: preenchido `#ffd27a`, com a tecla "1" numa moldura.
  - SEMPRE (2) e NEGAR (3): borda `#8a6a26`.
  - ABRIR TELA: só texto.
- **À direita do quadro**, o que aparece quando o voo está longe: faixa no canvas, ponto na aba do nível, título da janela, seletor de workspace.

---

## 06 · Transmitir · `Torre 02`, quadro 06 → `Composer.tsx`, `ComposerPill.tsx`

- **Painel de 680:** borda 1,5 `#cfeedd`, fundo `#07100b`.
  - **Linha PARA (44):** chips dos destinos (28 de altura: quadradinho do papel + indicativo + agente + ×), "+ DESTINO" tracejado, o aviso do destino à direita e ×.
  - **Avisos do destino:** "no ar · entra na fila" (azul), "aguarda · entra na fila" (âmbar), "pede você · responda antes" (âmbar), "hangar · inicie o voo" ou "pane" (vermelho, **bloqueia**).
  - **Corpo:** 14/1,7. Menções em chip: NOTAM em papel (`▤ arquivo.md`), portal com contorno, voo com o quadradinho.
  - **Imagens:** cartão de 220×54 com miniatura, nome e "1440×900 · colada".
  - **Rodapé (46):** botões imagem e @, "⏎ transmitir · ⇧⏎ nova linha · Esc fecha" e "TRANSMITIR A 2".
- **Menu @ acima do painel:** "NA FREQUÊNCIA DE IMP E REV" (NOTAMs, portais e voos ligados a **todos** os destinos) e "FORA DA FREQUÊNCIA" desligado, com `link_off` + "ligue para mencionar".
- **Menu + DESTINO:** os voos com o estado de cada um; no fim, "Todos os voos do FL1" (novo).
- **Toast:** "Transmitido a IMP e REV".

---

## 07 · Rádio · `Torre 02`, quadro 07 → `ActivityDrawer.tsx`

- **Gaveta de 440** à direita, de altura total.
  - **Cabeçalho (52):** ícone `cell_tower`, "RÁDIO", "workspace · sessão", BRIEFING e ×.
  - **Filtros:** TODOS · PEDEM VOCÊ · PENDENTES · um chip por voo (com o quadradinho do papel).
  - **Rodapé (44):** "⇄ 3/30 por min · máx. 6 saltos" e o botão LIMITES, que abre as Configurações → Limites.
- **Item:**
  - hora · origem → destino id (indicativos nas cores claras do papel; "VOCÊ" para a pessoa) · pílula de desfecho;
  - embaixo, a mensagem (12,5/1,45);
  - clicar expande o pedido e a resposta inteiros em Markdown.
- **Itens pregados no topo:** o que pede você (fundo `#1a1407` + barra âmbar de 3 à esquerda) e os pendentes (`#0b1710`), com VER NO ESPAÇO e CANCELAR PEDIDO.
- **Pílulas de desfecho:** Pede você (âmbar cheio) · Pendente (tracejada âmbar) · Respondido (verde) · Entregue (verde) · NOTAM editado (cinza) · Retido · espera geral (tracejada âmbar) · Em solo (verde) · Cancelado (vermelho).
- **Vazio:** "Rádio em silêncio. Ponha dois voos na mesma frequência e use torre ask."

---

## 08 · Boletim e briefing · `Torre 02`, quadro 08 → `TurnCard.tsx`, `SummaryDialogs.tsx`

- **Boletim aberto:** 520 de largura, abre para cima a partir da barra.
  - **RESUMO:** ícone do papel de quem resumiu (`psychology`), "por claude -p · haiku · 14:31", texto 13/1,55.
  - **PRÓXIMOS PASSOS:** até 3 botões de 30, cada um com "→ IMP", que abrem o Transmitir preenchido.
  - **PEDIDO** "c8d0 de IMP" e **MENSAGEM FINAL** em Markdown.
- **Estados da barra:** sem resumo (RESUMIR) · resumindo… (spinner) · resumido · falhou (motivo na linha + DE NOVO).
- **Briefing:** diálogo de 680, cabeçalho "BRIEFING · workspace · por claude · 14:32" com ATUALIZAR.
  - Conteúdo: o parágrafo; "PEDE ATENÇÃO" (borda 1,5 âmbar, até 3 itens com ícone de estado); "POR VOO" (quadradinho + indicativo sublinhado, que leva ao voo, + o texto).
  - Rodapé com o aviso de custo.

---

## 09 · NOTAM · `Torre 02`, quadro 09 → `NoteNode.tsx`

- **Papel:** `#efe8cf`, tinta `#1b1a14`, linha `#cfc6a6`. No tema claro, `#fffbea`.
- **Cabeçalho (34):** etiqueta "NOTAM" (preto sobre papel, 10/700) · título 13/700 · "● por ARQ" · arquivo em mono 10,5 `#6b6450` (corta com reticências) · cadeado · PRÉVIA | EDITAR (segmentado, o ativo preenchido) · ⋯.
- **Prévia:** Markdown (títulos 17/14, caixas de tarefa de 13 px clicáveis).
- **Trecho de agente:** fundo claro do papel dele (REV: `#f7d9e6`), barra de 3 à esquerda (`#c4507f`), rótulo "REV escreveu há 12 s" (`#8a2f57`), por ~30 s.
- **Edição:** mono 12/18; aviso de conflito de 30 entre o cabeçalho e o texto ("REV alterou a NOTAM enquanto você editava. RECARREGAR"); rodapé "grava sozinha 0,8 s depois · Esc volta à prévia".
- **Outros estados:**
  - **Nova:** campo de nome selecionado, com o arquivo ao lado.
  - **Travada:** fundo `#e4dec6`, `lock`, EDITAR desligado.
  - **Vazia:** ícone e o texto.
- **Redimensiona** (novo): alças de 10×10 `#cfeedd` nos cantos quando selecionada; mínimo 280×200.
- **Pilha:** depois.

---

## 10 · Portal · `Torre 02`, quadro 10 → `PortalNode.tsx`

- **Cabeçalho (36):** `public` · nome · "● por TST" · ← → ⟳ · URL em mono 11,5 (fundo `#030705`, corta com reticências) · "@ ELEMENTOS" (novo) · indicador de transmissão · `open_in_new` · ⋯.
- **Indicador:** "● ao vivo · 70 ms" verde (âmbar acima de 400 ms) · "◌ carregando…" azul · "◌ abrindo…" · "❙❙ pausado · zoom" · "○ sem transmissão" · "✕ sem navegador" vermelho · "⌨ TECLADO" preenchido.
- **Agente na página:** cursor na cor do papel com ondulação e "TST · clicando @e4"; ações sem posição viram chip no canto ("TST · teclando Enter"); a frequência acende na cor do papel com "clicando".
- **@ELEMENTOS:** contornos tracejados (na cor do papel, escurecida) com a etiqueta "@e4 botão".
- **Vazios e erros:**
  - "Defina um endereço na barra acima para abrir a página.";
  - "Abrindo a página…";
  - "Nenhum Chrome, Edge ou Chromium no servidor. Instale um deles ou defina TORRE_BROWSER." (borda `#6a2a26`).

---

## 11 · Workspaces · `Torre 03`, quadro 11 → `WorkspaceMenu.tsx`

- **Seletor de 440**, embaixo do botão do workspace.
  - **Linha:** cor de 9 · nome 13,5/700 · ⌥N; embaixo, a pasta em mono; à direita, lápis, número de sessões e ›.
  - **Atividade:** "1 pede você · 1 no ar · 1 aguarda", com as cores dos estados.
  - A linha ativa tem fundo `#10241a` e barra de 2 à esquerda.
  - No fim, "NOVO WORKSPACE".
- **Sessões ao lado:** painel de 330, "SESSÕES DE LOJA-DEMO".
- **Formulário de 460:**
  - Campos: PASTA DO PROJETO, com sugestões e "git · main" / "sem git"; NOME (opcional); NOME DA SESSÃO (opcional); COR (8 quadrados de 26, o escolhido com anel duplo).
  - Botões: CANCELAR · CRIAR E ABRIR.
  - **Editar:** SALVAR e REMOVER WORKSPACE.

---

## 12 · Sessões · `Torre 03`, quadro 12 → `SessionsMenu.tsx`

- **Painel de 520:** "SESSÕES DE RELATORIOS-API".
  - **Linha:** nome (cinza enquanto provisório) + "ATUAL"; datas · onde trabalha ("na pasta do projeto" ou "⎇ branch · cópia isolada"); conteúdo e chips dos agentes.
  - **Ações:** lápis, lixeira e ABRIR.
- **Nova sessão (560):**
  - Aviso âmbar de trabalho pendente; NOME; "Começar com os mesmos voos, sem as conversas".
  - ONDE A SESSÃO TRABALHA: dois cartões de escolha, "Na pasta do projeto" e "Numa cópia isolada".
  - Na cópia: A PARTIR DE e BRANCH NOVO.
  - A nota fixa e CRIAR E ABRIR.
- **Sessão em cópia:** a aba PISTA mostra o branch dela, e o menu da pista tem "Pedir pouso…" (o antigo "Mesclar…"), "Log do preparo" e "Copiar o caminho".

---

## 13 · Níveis de voo · `Torre 03`, quadro 13 → `FloorBar.tsx`, `FloorDialogs.tsx`, `FloorEmpty.tsx`

- **Abas na barra:** "PISTA main" | "FL1 export-csv 4 alt. ▲" | +. A ativa tem fundo `#10241a` e sublinhado de 2 `#cfeedd`.
- **Estados da aba:** limpo · N alt. · N commits · ◌ preparando · ✕ preparo falhou · pasta sumiu · ▲ voos no ar · ● pede você.
- **Menu do nível (280):** "FL1 · nome · ⎇ branch · commits · alt."; Pedir pouso… · Log do preparo · Copiar o caminho · Renomear (novo) · Descartar… (vermelho).
- **Novo nível de voo (560):**
  - NÍVEL (FL2, automático) · NOME · BRANCH;
  - a frase sobre o commit de origem;
  - PREPARAÇÃO recolhível: copiar da pista, comando de preparo;
  - CRIAR NÍVEL.
- **Nível vazio:** cartão de 420.
  - "NÍVEL VAZIO", "FL2 · Página de cupom" em 22/700, branch e pasta.
  - Estado do preparo com "ver log"; o atalho T e o comando `torre new … --floor "…"`.
  - Botão "NOVO VOO NESTE NÍVEL".
  - O erro de preparo em vermelho.

---

## 14 · Autorização de pouso · `Torre 03`, quadro 14 → `FloorDialogs.tsx` (Aterrissar) e o diálogo de mesclar da cópia

- **Diálogo de 700:** borda 1,5 `#cfeedd`.
  - **Título:** `flight_land` + "AUTORIZAR POUSO · FL1" + "nome · branch → main".
  - **Quatro caixas:** sem conflito (verde) · tudo commitado (verde) · N voos no ar (âmbar tracejado: avisa e não bloqueia) · main +N.
  - **COMMITS DO NÍVEL:** hash em âmbar, assunto e indicativo do autor, se der para saber; "+188 −12 em 4 arquivos".
  - **COMO POUSAR:** "Mesclar em [main ▾] com commit de merge" ou "Só guardar o branch".
  - **O que acontece ao pousar**, em caixa tracejada.
  - **Rodapé:** "quem pode: você, ou o líder que criou o nível" · CANCELAR · MESCLAR E POUSAR (ou POUSAR).
- **POUSO NEGADO** (conflito): borda `#d9443c`, cabeçalho `#1a0908`, `flight_takeoff`, "nada mudou · o nível continua no ar"; lista dos arquivos; botão "Pedir ao IMP para resolver os conflitos", que abre o Transmitir preenchido.
- **AINDA NÃO DÁ PARA POUSAR** (alterações sem commit): borda âmbar, com a lista em mono âmbar.

---

## 15 · Confirmações na interface · `Torre 03`, quadro 15 → componente novo `ConfirmDialog`

- **Diálogo de 420–440.**
  - **Título:** a pergunta, com ícone ("DESCARTAR FL2?", "TROCAR DE SESSÃO?", "REMOVER O VOO PLN?").
  - **Corpo:** consequências em lista, ✕ vermelho para o que se perde e ✓ verde para o que fica, com os números reais.
  - **Botão:** o verbo ("DESCARTAR NÍVEL", "TROCAR MESMO ASSIM", "REMOVER VOO", "TROCAR E REINICIAR").
- **Gravidade pela borda:** vermelho `#d9443c` (perde trabalho), âmbar (interrompe), claro (só reinicia).
- **Caixa "Entendo que esse trabalho se perde."** só quando há alterações sem commit a perder.
- **Teclado:** Esc cancela; Enter não confirma as vermelhas.
- **Substitui:** remover workspace, sessão, voo e NOTAM do canvas; nova conversa; trocar papel; reiniciar quem usa um papel; descartar alterações de papel; restaurar e remover papel. Também cobre as que já existem na interface (trocar de sessão, remover sessão suja, descartar nível).

---

## 16 · Novo voo e menu do voo · `Torre 04`, quadro 16 → `TerminalForm.tsx`, `NodeMenu.tsx`

- **Novo voo (420):** "NOVO VOO · na pista · main".
  - **AGENTE:** 6 blocos de 44 (Claude Code, Codex, Devin, Gemini, Shell, Comando), cada um com "✓ instalado" / "✕ não achado" / bash / próprio.
  - **NOME + INDICATIVO** lado a lado; **PAPEL**, com "editar papéis…"; **PASTA** (opcional).
  - **AVANÇADO** recolhível: comando, shell, líder.
  - Botão **DECOLAR**.
- **Configurar:** mesmo formulário; quando muda o que inicia o voo, aparece o aviso âmbar "O voo reinicia…" e o botão vira SALVAR E REINICIAR.
- **Menu ⋯ (250):** Parar ("vai ao hangar") / Iniciar · Reiniciar · Nova conversa… · Copiar comando de retomada · Configurar… | Foco ⌥F · Ocultar a tela · Líder ☐ | PAPEL: lista com ✓ e Editar papéis… | Remover… (vermelho).
- **Tarjas de processo:** PANE · saída N (listrado vermelho opaco + INICIAR) · HANGAR ("A conversa está guardada." + INICIAR) · CLI ausente (com o comando de instalação).

---

## 17 · Papéis · `Torre 04`, quadro 17 → `RolesPanel.tsx`

- **Painel de 1200×780.**
  - **Lista de 280:** ícone de 26 sobre a tinta do papel · nome · sigla · "editado"; no fim, NOVO PAPEL.
  - **Editor:** NOME + **SIGLA** (novo, 3 letras) · COR (8) · ÍCONE (13) · "Líder" · INSTRUÇÕES (mono 12/18, contador 1831/4000) · faixa verde "Salvo. PLN recebe as instruções novas ao reiniciar. REINICIAR AGORA".
  - **Rodapé:** RESTAURAR PADRÃO · "neste workspace: PLN" · SALVAR.
- **Papéis prontos** e as regras: como hoje. O texto dos prontos passa a citar `torre`.

---

## 18 · Configurações · `Torre 04`, quadro 18 → `TopBar.tsx` (menu ⚙) + painel novo

- **Painel de 1200×780** com abas à esquerda: Aparência · Agentes · Limites · Resumos · Papéis… · Atalhos · Avisos.
- **Aparência:** tema da interface (Escuro · Claro · Sistema) e dos terminais (Noite · Papel).
- **Agentes:** tabela agente · versão · comando padrão · editar/procurar.
- **Limites:** sliders de saltos encadeados e pedidos por minuto; números de voos por líder e portais por voo.
- **Resumos:** o diálogo de hoje (§14.3) dentro do painel.
- **Atalhos:** lista; os de ⌥ podem ser trocados.
- **Avisos:** "Quando um voo pede você": título da janela, notificação do sistema, som curto.
- Fecha com Esc e clique fora.

---

## 19 · Primeira abertura · `Torre 04`, quadro 19 → página de login (Go) + fluxo novo

- **Fundo:** radar estático em anéis `#0e2015`. Passos no topo: "1 ENLACE ✓ · 2 PASTA · 3 AGENTES".
- **1 · Enlace:** o texto do login de hoje com `torre serve`, o campo de URL e "✓ CONECTADO"; "só 127.0.0.1 · o token fica num cookie".
- **2 · Pasta:**
  - campo com sugestões e as checagens ("✓ repositório git · main · limpo", "✓ níveis de voo disponíveis", "○ .env encontrado");
  - RECENTES e CONTINUAR.
- **3 · Agentes:** a lista com versão ou "não achado", mais o navegador para os portais; depois, começar vazio ou com um plano de voo.
- **Sem enlace:** cartão "o torre serve parou? tentando de novo em 3 s…".

---

## 20 · Espaço vazio e planos de voo · `Torre 04`, quadro 20

- **Vazio:** "ESPAÇO AÉREO VAZIO · workspace", título "Ponha a primeira equipe no ar." (28/700) e três botões de 52 com o atalho (Usar um plano de voo ⌥M · Novo voo T · Escrever o plano numa NOTAM N), mais a frase sobre frequências.
- **Galeria (740):** "PLANOS DE VOO" + IMPORTAR.
  - **Cartões:** miniatura com fichas e frequências, nome e composição.
  - **Cartão tracejado:** "Salvar a seleção como plano" (⌥⇧M).
  - **"ANTES DE DECOLAR":** os comandos em mono e o aviso sobre planos importados.
  - **Rodapé:** "3 voos novos, conversas novas" · DECOLAR A EQUIPE.

---

## 21 · Arremetida, espera geral e falhas · `Torre 04`, quadro 21 → `InterruptedBar.tsx`, toasts, `TopBar.tsx`

- **Arremetida:** faixa de 860×44 no topo do canvas (fundo `#1a1407`, borda 1,5 âmbar).
  - `u_turn_left` · "ARREMETIDA" · "PLN e REV foram interrompidos no meio do turno. A conversa voltou; falta só continuar." · CONTINUAR (preenchido) · DISPENSAR.
  - O voo também mostra "ARREMETIDA" no cabeçalho.
- **Espera geral:**
  - O botão da barra vira "▶ LIBERAR TODOS" (âmbar cheio).
  - Cada frequência ganha um quadrado de 26 com ∞ no meio.
  - Num laço: o cartão "ESPERA GERAL · LAÇO" com a cadeia e LIBERAR TODOS ⌥P.
- **Reconectando:** na barra, "↻ RECONECTANDO…" tracejado âmbar. Os voos ficam no último estado.
- **Avisos curtos** (embaixo, no centro, 34 de altura, ~4 s):
  - Confirmação: fundo `#cfeedd`, texto `#030705`.
  - Espera: `#2a1f07` / `#e0a526` / `#ffd27a`.
  - Erro: `#2a0d0b` / `#d9443c` / `#ffc9c2`.
  - Sempre com ícone.

---

## 22 · Tema claro · `Torre 04`, quadro 22 → `theme/tokens.css` (`data-theme="light"`)

- **Canvas** `#e9e3cf` com pontos `#cfc6a6`; **barra** `#f6f2e4`; **voo** `#fbf8ee` com cabeçalho `#f6f2e4`; **linhas** `#d9d0b2` / `#cfc6a6` / `#b8ae8a`; **NOTAM** `#fffbea`.
- **Aba ativa:** `#fffbea` com sublinhado `#1b1a14`. **Vista ativa:** preenchida `#1b1a14`.
- **Faixas de remetente:**

  | Caso | Fundo | Texto |
  |---|---|---|
  | Aguardando | `#fff1c9` | `#6b4500` |
  | Pedido chegando (PLN) | `#ece6fb` | `#4a3596` |
  | Resposta recebida | `#e3f3e8` | `#1f6b3a` |

- **Frequência pendente:** `#c98a00`; pílula `#fff1c9`.
- **Papéis no claro:** plan `#7b5ce0`, impl `#178a92`, rev `#c4507f` (os outros escurecidos na mesma medida).
- **Terminal no tema Papel:** veja `tokens/xterm-themes-torre.ts`.
