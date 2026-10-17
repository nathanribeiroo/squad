# Squad: todas as funcionalidades (com foco na interface web)

Documento para o redesign da interface. Ele descreve o que o app faz hoje, tela por tela, com cada botão, estado, mensagem e comportamento. Tudo foi tirado do código (`codigo-web/src`) e conferido rodando o app de verdade, com três Claude Code reais conversando. As capturas citadas estão em `capturas/`.

- **Nome:** "Squad" é provisório e deve mudar depois. A seção 26 lista onde o nome aparece.
- **Vocabulário:** termos como "andar", "papel", "portal" e "gerente" são os de hoje. O redesign pode trocá-los.

---

## Sumário

1. O que é o Squad
2. Vocabulário e hierarquia
3. Mapa da tela
4. Barra superior
5. Barra de ferramentas lateral
6. Canvas
7. Nó de terminal
8. Nó de nota
9. Nó de portal
10. Fios (conexões)
11. Conversa entre agentes: o que a pessoa vê
12. Composer
13. Gaveta de Atividade
14. Cartões de turno e resumos
15. Workspaces
16. Sessões
17. Sessão numa cópia isolada
18. Andares
19. Papéis
20. Agente gerente e o que os agentes criam
21. Conversas que continuam e trabalho interrompido
22. Configurações
23. Login, janela de app e conexão
24. Atalhos de teclado
25. Mensagens, avisos e confirmações
26. Onde aparece o nome "Squad"
27. Sistema visual atual
28. Observações para o redesign
29. Índice das capturas
30. Mapa do código da interface

---

## 1. O que é o Squad

O Squad é uma ferramenta local para orquestrar agentes de código (Claude Code, Codex, Devin, Gemini) e shells num **canvas infinito**.

- **Terminais reais:** cada agente roda num terminal de verdade, que vira um nó no canvas.
- **Os fios são a permissão:** os agentes só conversam com quem estiver ligado a eles por um fio que a pessoa desenha.
- **Pedido e resposta:** um agente pede algo a outro (`squad ask`). O pedido viaja pelo fio, o outro agente trabalha, e a resposta (a mensagem final do turno dele) volta pelo mesmo fio.

| Aspecto | Hoje |
|---|---|
| Para quem | Quem programa com dois ou mais agentes ao mesmo tempo (por exemplo, planejador, implementador e revisor) e hoje alterna entre abas de terminal. |
| Problema que resolve | Os agentes não se falam, não dá para ver quem trabalha ou espera, e experimentos paralelos se atropelam no mesmo repositório. |
| Forma | Um binário Go (`squad serve`) que escuta só em `127.0.0.1` (porta 7420) e serve uma interface React, aberta numa janela própria (modo app do Chrome, Edge ou Brave), sem abas nem barra de endereço. |
| Plataformas | macOS, Linux e Windows (Git Bash). Só desktop: celular está fora do escopo. |
| Idioma da interface | Português do Brasil. |
| Inspiração | O Maestri, um app de desktop parecido. O design original pedia para não copiar a aparência dele (veja `design-original/brief.md`). |

---

## 2. Vocabulário e hierarquia

```
Servidor local (um só)
└── Workspace (uma pasta de projeto; vários, todos rodando ao mesmo tempo)
    └── Sessão (um canvas inteiro; várias por workspace, só uma aberta por vez)
        ├── Térreo (o canvas da pasta do projeto, ou da cópia da sessão)
        └── Andares (cópias isoladas do repositório, cada uma com o seu canvas)
            └── Nós (terminal, nota, portal) e fios entre eles
```

| Termo | O que é |
|---|---|
| **Workspace** | Uma pasta de projeto, com nome, cor e sessões. Trocar de workspace na tela não para os outros: os agentes deles continuam trabalhando. |
| **Sessão** | Um canvas inteiro de um workspace: terminais, notas, portais, fios, posições, andares e cartões. Só a sessão aberta tem processos rodando. Pode trabalhar na pasta do projeto ou numa **cópia isolada** (um branch próprio). |
| **Térreo** | O canvas principal da sessão. No seletor, aparece com o nome do branch atual (ex.: `main`). |
| **Andar** | Uma cópia isolada do repositório (`git worktree`) num branch próprio, com um canvas próprio, para frentes de trabalho em paralelo. **Aterrissar** = trazer de volta (mesclar o branch e fechar o andar). **Descartar** = apagar. |
| **Nó** | Um item do canvas. Há três tipos: **terminal**, **nota** e **portal**. |
| **Terminal** | Um terminal real (xterm.js) com um agente (Claude Code, Codex, Devin, Gemini), um shell ou um comando próprio. |
| **Nota** | Um arquivo Markdown que a pessoa e os agentes leem e escrevem. |
| **Portal** | Um navegador dentro do canvas: um Chromium sem janela no servidor, transmitido como imagem ao vivo. Os agentes ligados a ele também o usam. |
| **Fio** | A linha entre dois nós, sem direção e sem seta. Só nós ligados se enxergam. |
| **Pedido** | Uma mensagem de um agente para outro, com um id curto de 4 caracteres (ex.: `2d3e`). |
| **Turno** | Um ciclo de trabalho de um agente: recebe a mensagem, trabalha e termina com uma mensagem final. |
| **Cartão de turno** | O resumo curto que aparece embaixo do terminal quando o turno acaba. |
| **Papel** | Instruções reutilizáveis que o agente recebe ao abrir (Planejador, Arquiteto, Implementador, Revisor, Testador, ou um papel criado pela pessoa), com cor e ícone. |
| **Gerente** | Um agente que pode criar e ligar outros terminais (`squad new`) e criar andares. |

---

## 3. Mapa da tela

Capturas: `01-canvas-visao-geral.png`, `06-gaveta-de-atividade.png`.

```
┌──────────────────────────────────────────────────────────────────────────────┐
│ BARRA SUPERIOR (44 px): logo · workspace › sessão › andares · caminho ·      │
│   taxa · Pausar conversas · Atividade · servidor · ⚙                         │
├──┬───────────────────────────────────────────────────────────┬───────────────┤
│T │ [faixa "X foi interrompido" quando há agentes              │ Aba vertical  │
│O │  interrompidos, centralizada no topo]                      │ "Atividade"   │
│O │                                                            │ (fechada), ou │
│L │                 CANVAS INFINITO                            │ GAVETA DE     │
│B │   (grade de pontos, nós, fios)                             │ ATIVIDADE     │
│A │                                                            │ (380 px)      │
│R │                                                            │               │
│  │                 [Composer: pílula centralizada embaixo]    │ Minimapa      │
│  │                                                            │ + zoom        │
└──┴───────────────────────────────────────────────────────────┴───────────────┘
```

Elementos sempre presentes:

- **Barra superior:** seção 4.
- **Barra de ferramentas vertical**, à esquerda: seção 5.
- **Canvas:** seção 6.
- **Minimapa e controles de zoom**, no canto inferior direito.
- **Pílula do Composer**, embaixo, no centro: seção 12.
- **Aba da Atividade**, na borda direita, que abre a gaveta: seção 13.

Elementos que aparecem conforme o estado:

- **Faixa de interrompidos:** seção 21.
- **Toasts:** avisos curtos embaixo, no centro.
- **Diálogos modais:** papéis, andares, resumos.
- **Popovers:** menus e formulários presos aos botões.

---

## 4. Barra superior

Altura de 44 px, fundo `--surface`, borda inferior. Da esquerda para a direita:

| # | Elemento | O que mostra | O que faz |
|---|---|---|---|
| 1 | **Logo** | Grade 2×2 de quadrados (três cheios, um vazado) e o texto "squad" em mono negrito. | Nada (só marca). |
| 2 | **Seletor de workspace** | Quadrado na cor do workspace · nome · "N agentes ativos" · ponto azul se houver atividade em **outro** workspace · atalho `⌥1`…`⌥9` · seta. | Abre o seletor de workspaces (seção 15). |
| 3 | `›` | Separador. | — |
| 4 | **Sessão** | Ícone de histórico · nome da sessão aberta · seta. Uma sessão sem nome aparece como "Nova sessão · 17/10 14:32". | Abre o painel de sessões (seção 16). |
| 5 | `›` | Separador. | — |
| 6 | **Seletor de andares** | Abas segmentadas: o térreo (nome do branch, ex.: `main`) e uma aba por andar. Cada aba mostra o estado git ("limpo", "● 3 alterações", "2 commits", "preparando", "preparo falhou", "pasta sumiu") e um ponto azul se houver agentes trabalhando nela. A aba ativa tem o ícone `layers` e uma seta. Termina com o botão `+`. | Clicar numa aba mostra aquele andar. Clicar na aba ativa abre o menu do andar (seção 18). `+` cria um andar; fica desligado, com dica, se a pasta não for um repositório git. |
| 7 | **Caminho** | `⎇ main · ~/projetos/loja-demo`, em mono, discreto. Mostra o branch e a pasta reais do andar aberto ou da cópia da sessão. | Nada (o texto completo aparece ao passar o mouse). |
| 8 | (espaço flexível) | | |
| 9 | **Medidor de taxa** | Ícone de troca · "1/30" · "pedidos por min". | Nada: mostra quantos pedidos entre agentes houve no último minuto, sobre o limite. |
| 10 | **Pausar conversas** | Botão com ícone de pausa e `⌥P`. Pausado, vira "Retomar conversas", com ícone de play. | Pausa e retoma todas as conversas entre agentes do workspace. Pausado, os pedidos novos ficam retidos. Fica sempre visível. |
| 11 | **Atividade** | Botão com ícone e contador âmbar de pedidos pendentes. Fica preenchido quando a gaveta está aberta. | Abre e fecha a gaveta de Atividade (`⌥A`). |
| 12 | **Servidor** | Ícone `lan` + "servidor", em verde. Sem conexão: "reconectando…", em âmbar. | Nada (indicador). |
| 13 | **⚙ Configurações** | Ícone de engrenagem. | Abre o menu de configurações (seção 22). |

---

## 5. Barra de ferramentas lateral

Flutuante, vertical, presa à esquerda (captura `01`). Botões de 36×36. Ao passar o mouse, uma dica mostra o nome e o atalho.

| Ícone | Nome | Atalho | O que faz |
|---|---|---|---|
| `near_me` | Selecionar | V | Modo padrão: arrastar no vazio desenha um retângulo de seleção; arrastar um nó o move. |
| `pan_tool` | Mover | H | Arrastar move o canvas; os nós ficam travados. |
| — | divisor | | |
| `terminal` | Novo terminal | T | Abre o formulário "Novo terminal" (seção 7.7). |
| `sticky_note_2` | Nota | N | Cria uma nota no centro da tela, já com o campo de nome selecionado. |
| `public` | Portal | P | Cria um portal no centro da tela, sem endereço. |
| `gesture` | Desenho (em breve) | D | **Desligado.** Previsto no design original, ainda não existe. |
| `select` | Grupo (em breve) | G | **Desligado.** Previsto no design original, ainda não existe. |

O botão ativo fica preenchido (fundo `--text`, ícone `--surface`).

---

## 6. Canvas

Feito com React Flow (`@xyflow/react`). Fundo `--bg` com uma grade de pontos a cada 20 px, que acompanha o zoom.

### 6.1 Navegação

- **Mover o canvas:**
  - rolar (roda ou trackpad);
  - arrastar com o botão do meio ou o direito;
  - no modo Mover (H), arrastar com o botão esquerdo.
- **Zoom:**
  - pinça, ou ⌘/Ctrl + roda (também quando o ponteiro está sobre um terminal ou portal);
  - os botões `−`/`+` embaixo à direita;
  - limites: de 15% a 200%. O duplo clique no canvas não dá zoom.
- **Enquadrar:** o botão `fit_screen` ajusta o zoom para mostrar todos os nós.
- **Minimapa** (220×132): terminais aparecem como retângulos na cor do estado deles; notas e portais, em cinza. Dá para arrastar e dar zoom por ele. Quando a gaveta abre, ele se desloca para a esquerda.
- **Posição e zoom ficam guardados** por workspace, sessão e andar, no navegador.

### 6.2 Seleção e movimento

- **Selecionar um nó:** clique nele. Um nó só selecionado vira o destino padrão do Composer.
- **Arrastar um nó:** só pelo **cabeçalho**. Nas miniaturas e nos cartões, pelo corpo todo.
- **Redimensionar:** terminal e portal mostram alças quando estão selecionados. Mínimos: terminal 320×200; portal 320×240. **Notas não redimensionam** (ficam em 400×350).
- **Delete/Backspace remove só fios selecionados.** Nós saem pelo menu ⋯, porque remover um terminal encerra o processo.

### 6.3 Níveis de zoom

Capturas: `14-zoom-miniatura.png` e `15-zoom-cartao.png`. Os níveis têm histerese de ±0,03 para não piscarem no limite.

| Zoom | Nível | Como o terminal aparece |
|---|---|---|
| ≥ 60% | **Vivo** | Terminal completo, legível e digitável. |
| 35%–60% | **Miniatura** | Faixa do papel; cabeçalho com nome, agente e pílula de estado; corpo com um retrato do texto da tela, atualizado a cada ~3 s; rodapé "miniatura · há 3 s" e "caixa N". |
| < 35% | **Cartão** | Cartão de 240 px: faixa do papel; nome grande; contador da caixa; "claude · Implementador"; faixa inferior com ícone, rótulo do estado e cronômetro. |

- **Durante o gesto de zoom,** o conteúdo fica congelado e é redesenhado quando o gesto para.
- **Portais** abaixo de 60% mostram o último quadro e param de transmitir ("pausado").

### 6.4 Conectar nós

- **Criar:** cada nó tem quatro bolinhas, uma em cada lado. Arraste de uma delas e solte sobre **o corpo de outro nó**. Durante o arraste:
  - os destinos válidos ganham um anel;
  - os já ligados ficam esmaecidos (inválidos).
- **Ao soltar,** aparece o toast "Planejador ↔ Implementador ligados".
- **Regras:** nada de ligar um nó a ele mesmo, nem de fio repetido. Um fio pode ligar terminais de andares diferentes (seção 18.6).

---

## 7. Nó de terminal

Capturas: `01`, `02`, `04`, `09`, `37`. Tamanho padrão de 560×440 (cerca de 80×24 caracteres).

### 7.1 Anatomia (nível vivo)

```
┌──────────────────────────────────────────────────────────────────┐ ← faixa de 3 px na cor do papel
│ Planejador [claude] [⌘ Planejador] [👥 gerente]   ⌨ teclado aqui  (⏳ Esperando resposta 0:42) ⋯ │ ← cabeçalho (32 px)
├──────────────────────────────────────────────────────────────────┤
│ ⏳ Aguardando Implementador · pedido 2d3e · enviado há 12 s        │ ← faixa de remetente (24 px, opcional)
├──────────────────────────────────────────────────────────────────┤
│                                                                  │
│   (tela real do terminal: xterm.js, Geist Mono 11/15)            │ ← corpo
│                                                                  │
├──────────────────────────────────────────────────────────────────┤
│ ⎇ main  81×24  caixa 1  ● criado por Plano         ~/projetos/app │ ← rodapé (22 px)
└──────────────────────────────────────────────────────────────────┘
 ✓ Turno · 18s  O Planejador pediu ao Implementador…  [Resumir] ×    ← cartão de turno (fora do nó, embaixo)
```

**Cabeçalho** (arrasta o nó):

- **Nome** do terminal, único no workspace.
- **Chip do agente:** `claude`, `codex`, `devin`, `gemini`, `shell` ou `custom`, em mono.
- **Chip do papel:** ícone e nome, nas cores do papel. Num comando próprio, a dica diz que o papel é "só a etiqueta".
- **Chip "gerente"** (ícone `groups`), quando o modo gerente está ligado.
- **"⌨ teclado aqui"**, quando este terminal recebe o teclado.
- **Pílula de estado** (7.2), com cronômetro nos estados com tempo.
- **Menu ⋯** (7.5).

**Faixa de remetente** (acima do terminal, nunca por cima dele). Há três variações:

| Variação | Quando | Texto e cor |
|---|---|---|
| Pedido chegando | Este terminal está atendendo um pedido. | ↙ "Pedido 2d3e de Planejador · a resposta volta para ele", nas cores do papel de quem pediu. |
| Esperando | Este terminal fez um pedido e espera. | ⏳ "Aguardando Implementador · pedido 2d3e · enviado há 12 s", em âmbar. |
| Resposta recebida | Chegou a resposta (fica 60 s). | ↩ "Resposta de Implementador · 2d3e · 9s", em verde. |

**Corpo:**

- **Tela real** do terminal (xterm.js), com o tema "Noite" (escuro) ou "Papel" (claro).
- **Teclado:** um clique no corpo dá o teclado ao terminal; `⌥Esc` devolve o teclado ao canvas. Com o teclado no terminal, os atalhos `⌥A`, `⌥P`, `⌥K` e `⌥1–9` continuam valendo.
- **Roda:** sem modificador, rola o terminal; com ⌘/Ctrl, dá zoom no canvas.
- **Processo parado:** com o processo encerrado ou com falha, uma tarja sobre o corpo diz "Processo encerrado (saída N). Use o menu ⋯ para iniciar." ou mostra o erro, em vermelho.

**Rodapé** (mono, pequeno):

- `⎇ branch` (o branch do andar ou da sessão);
- `colunas×linhas`;
- "caixa N" em âmbar (pedidos esperando resposta deste terminal);
- "● criado por Plano", quando outro agente criou o terminal;
- a pasta real, alinhada à direita.

**Cartão de turno:** embaixo do nó (seção 14).

**Alças de redimensionar:** visíveis com o nó selecionado e sem o teclado. A grade do terminal acompanha o tamanho.

### 7.2 Estados de atividade

Cada estado combina cor, ícone e texto: o estado nunca é mostrado só pela cor.

| Estado | Rótulo | Ícone (Material Symbols) | Cor (token) | Com cronômetro? | Quando |
|---|---|---|---|---|---|
| `boot` | Iniciando | `progress_activity` | `--st-boot` (azul acinzentado) | sim | Acabou de abrir, sem saída ainda. |
| `idle` | Ocioso | `radio_button_unchecked` | `--st-idle` (cinza) | não | Parado, sem nada acontecendo. |
| `work` | Trabalhando | `autorenew` | `--st-work` (azul) | sim | O agente está num turno. |
| `wait` | Esperando resposta | `hourglass_top` | `--st-wait` (âmbar) | sim | Fez um pedido a outro agente e espera. |
| `done` | Terminou | `check_circle` | `--st-done` (verde) | não | O turno acabou há pouco (até 10 min). |
| `err` | Parado · erro (ou "Parado · saída N") | `error` | `--st-err` (vermelho) | não | O processo falhou ou saiu com código diferente de 0. |
| `off` | Descarregado | `bedtime` | `--st-off` (cinza) | não | O processo foi parado, ou saiu com 0. |

- **Âmbar é só para espera:** o terminal "Esperando resposta" e o fio com pedido pendente usam a mesma cor.
- **Não existe um estado "esperando permissão":** um agente parado num pedido de permissão aparece como "Trabalhando", e a pessoa precisa olhar a tela dele.

**Estados do processo** (além da atividade): `starting`, `running`, `exited`, `stopped`, `failed`. Eles decidem a tarja de "processo encerrado" e o item Iniciar/Parar do menu.

### 7.3 Tipos de terminal

| Agente | Rótulo no formulário | Recebe papel? | Observação |
|---|---|---|---|
| `claude` | Claude Code | sim | Fim de turno pelo hook (resposta exata); retoma a conversa. |
| `codex` | Codex | sim | Fim de turno pelo `notify`. |
| `devin` | Devin | sim | Fim de turno pelos hooks. |
| `gemini` | Gemini | sim | Sem hooks: o fim de turno é detectado pela tela. |
| `shell` | Shell | **não** | Shell puro; não recebe papel nem texto automático. |
| `custom` | Comando próprio | só como etiqueta | Roda o comando digitado (ex.: `npm run dev`). |

### 7.4 Quem recebe o teclado

- **Dar o teclado:** um clique no corpo dá o teclado àquele terminal. O nó ganha um anel de 2 px (`--focus`) e o rótulo "teclado aqui".
- **Devolver ao canvas:** `⌥Esc`, ou um clique no vazio do canvas.

### 7.5 Menu ⋯ do terminal

Captura `09-menu-do-terminal.png`.

| Item | Para | O que faz |
|---|---|---|
| **Parar** / **Iniciar** | todos | Para ou inicia o processo. Mostra Iniciar quando o processo não está rodando. |
| **Reiniciar** | todos | Reinicia; o agente volta à mesma conversa. |
| **Nova conversa** | agentes | Pede confirmação e reinicia sem o contexto. A conversa antiga continua guardada no agente. |
| **Copiar comando de retomada** | agentes com conversa | Copia `claude --resume <id>` (ou o do agente), para abrir a mesma conversa fora do Squad. |
| **Configurar…** | todos | Abre o formulário do terminal (7.7) em modo de edição. |
| **Modo gerente** ✓ | agentes | Liga ou desliga o modo gerente deste terminal. |
| — | | |
| **(lista dos papéis)** ✓ | não shells | Cada papel da biblioteca; o atual tem ✓. Clicar troca o papel: com o agente aberto, pede confirmação, reinicia e começa uma conversa nova. Clicar no atual tira o papel. |
| **Editar papéis…** | não shells | Abre o painel de papéis já no papel do terminal. |
| — | | |
| **Remover** (vermelho) | todos | Pede confirmação ("O processo do terminal é encerrado.") e remove o nó e os fios dele. |

### 7.6 Miniatura e cartão

Veja 6.3. As duas versões são somente leitura e se arrastam pelo corpo inteiro.

### 7.7 Formulário "Novo terminal" e "Configurar…"

Captura `08-novo-terminal.png`. Popover de 300 px de largura, preso no canto superior esquerdo do canvas.

| Campo | Tipo | Detalhes |
|---|---|---|
| **Agente** | lista | Claude Code, Codex, Devin, Gemini, Shell, Comando próprio. |
| **Nome** | texto | O padrão é o nome do agente, numerado se já existir ("Claude", "Claude 2"…). |
| **Comando** (opcional; obrigatório em "Comando próprio") | texto mono | Linha de lançamento. Dicas: "só o shell" (Shell) e "ex.: npm run dev" (Comando próprio). |
| **Pasta** (opcional) | texto com sugestões | Aceita um caminho absoluto, `~/…` ou relativo à pasta do workspace (ou do andar). As sugestões aparecem enquanto se digita: ↑/↓ escolhem, Enter aceita, Tab completa, Esc fecha a lista. |
| **Shell** | lista | "Padrão (bash)" e os shells instalados (no Windows: Git Bash, PowerShell 7, Windows PowerShell, cmd). |
| **Papel** | lista | "Nenhum" e os papéis da biblioteca, com o link "Editar papéis…". Não aparece para Shell. Num comando próprio, avisa que o papel fica só como etiqueta. |

- **Botões:** o formulário de criação tem **Criar**. O de edição tem **Salvar**, ou **Salvar e reiniciar** quando muda o que inicia o terminal (agente, comando, pasta, shell ou papel de um agente), com o aviso "O terminal reinicia e a sessão atual é encerrada.".
- **Onde nasce:** o terminal novo aparece no centro da tela, no andar aberto.
- **Erros:** aparecem em vermelho dentro do formulário (ex.: "folder not found").

---

## 8. Nó de nota

Capturas: `01` e `13-nota-em-edicao.png`. Tamanho fixo de 400×350, fundo `--note-bg` (levemente diferente do terminal).

**Cabeçalho** (arrasta o nó):

- ícone `description` e o **título**;
- "● por Bia", se um agente criou a nota;
- o **nome do arquivo** em mono (ex.: `plano-carrinho.md`), sempre separado do título;
- **cadeado** (`lock_open`/`lock`): trava a nota para todos;
- o seletor **Prévia | Editar**;
- o menu ⋯: **Renomear** (só nas notas da pessoa) e **Remover do canvas** (o arquivo continua no disco).

**Prévia:**

- Markdown renderizado: títulos, listas, listas de tarefas, blocos de código, negrito, itálico, código e links;
- as caixas de tarefa marcam e desmarcam com um clique (exceto com a nota travada);
- duplo clique entra na edição.

**Edição:**

- textarea em mono, com gravação automática 800 ms depois da última tecla;
- sai com Esc, com um clique fora ou em "Prévia";
- se outra pessoa ou um agente alterar a nota enquanto ela está sendo editada, aparece a faixa "Bia alterou a nota. [Recarregar]".

**Nome:**

- **Nota nova** (N ou barra de ferramentas): nasce com o campo de nome já selecionado. Enter confirma; Esc fica com o nome provisório ("Nota", "Nota 2"…). O arquivo nasce com o nome dado ("Plano de testes" → `plano-de-testes.md`) e não muda mais.
- **Renomear:** duplo clique no título, ou ⋯ → Renomear. O nome antigo continua valendo para os agentes.
- **Notas de agente** (`squad note create`) ficam com o nome que o agente deu, sem "Renomear".
- **Nomes únicos** no workspace, sem diferenciar maiúsculas.

**Edição de um agente:**

- o trecho alterado fica destacado por ~30 s, num bloco nas cores do papel do agente, com o rótulo "Revisor escreveu há 12 s";
- a gaveta de Atividade ganha o item "Nota editada".

**Nota vazia:** mostra "Nota vazia. Clique em Editar, ou ligue um agente a ela para ele escrever com squad note.".

**Nota travada:** "Editar" fica desligado, e os agentes recebem um erro ao tentar escrever.

---

## 9. Nó de portal

Capturas: `01` e `33-portal-agente-clicando.png`. Tamanho padrão de 560×420.

**Cabeçalho** (arrasta o nó):

- ícone `public` e o **nome** ("Portal", "Portal 2"…). Duplo clique, ou ⋯ → Renomear, troca o nome; os agentes usam esse nome para escolher o portal;
- "● por Bia", se um agente abriu o portal;
- **← → ⟳** (voltar, avançar, recarregar);
- **barra de endereço** em mono: clicar edita, Enter navega e Esc desiste. Sem esquema, vira `http://` para localhost e `https://` para o resto. Sem endereço, mostra "definir endereço…";
- **indicador de transmissão** (abaixo);
- **abrir no navegador** (`open_in_new`);
- menu ⋯: **Renomear** e **Remover**.

Estados do indicador de transmissão:

| Texto | Cor | Quando |
|---|---|---|
| "● ao vivo · 70 ms" | verde (âmbar acima de 400 ms) | Transmitindo. |
| "carregando…" | azul | A página está carregando. |
| "abrindo…" | cinza | Conectado, esperando o primeiro quadro. |
| "pausado" | cinza | Zoom abaixo de 60%: mostra o último quadro. |
| "sem navegador" | vermelho | Não há Chrome, Edge nem Chromium no servidor. |
| "sem transmissão" | cinza | Sem endereço ou sem conexão. |
| "⌨ teclado aqui" | — | Substitui o indicador quando a página recebe o teclado. |

**Corpo** (a página ao vivo):

- **Mouse:** um clique dá o teclado à página até `⌥Esc`; o mouse (clique, arraste, menu de contexto) é repassado à página.
- **Roda:** sem modificador, rola a página; com ⌘/Ctrl ou pinça, dá zoom no canvas.
- **Colar:** digita o texto na página.
- **Tamanho:** a página acompanha o tamanho do nó.

Mensagens no corpo:

- "Defina um endereço na barra acima para abrir a página.";
- "Abrindo a página…" (com spinner);
- "Nenhum navegador encontrado…", com a dica de instalar o Chrome ou o Edge, ou de definir `SQUAD_BROWSER`;
- "Não foi possível abrir: <erro>".

**Agentes no portal:**

- quando um agente ligado usa a página (`squad portal click/fill/type/…`), o **cursor dele** aparece onde ele clica, na cor do papel, com uma ondulação e a etiqueta "Implementador · clicando";
- ações sem posição (teclas, navegação) aparecem como um chip no canto;
- o fio entre o agente e o portal acende na cor do papel e mostra a pílula "clicando", "preenchendo", "digitando", "teclando", "rolando", "navegando" ou "recarregando";
- tudo isso dura 2,5 s.

---

## 10. Fios (conexões)

Capturas: `02`, `03`, `12`, `16`. Os fios são curvas Bézier com "barriga" para baixo, como uma corda pendurada.

- **Ancoragem:** os fios ligam os lados que se encaram dos dois nós.
- **Sem seta:** os fios não têm direção.

| Estado | Aparência |
|---|---|
| **Repouso** | Traço cinza (`--wire`) de 2,5 px, com bolinhas nas pontas. |
| **Selecionado** (clique) | Aparecem os pontos de curva e o botão **"Desligar"** (`link_off`) no ponto mais baixo. Delete também desliga. |
| **Pedido viajando** (~600 ms) | O fio acende em âmbar da origem ao destino; um "envelope" âmbar (`mail` + id + `→`) corre pelo fio. |
| **Pedido pendente** | Fio **tracejado âmbar**. A ponta de origem fica preenchida e a de destino com anel. No ponto mais baixo, uma **pílula** com ampulheta, cronômetro (`0:42`) e o id (`2d3e`). |
| **Resposta voltando** (~600 ms) | O fio acende em verde do destino para a origem, com o envelope verde "← resposta 2d3e ↩". |
| **Resposta recebida** | A pílula verde "✓ 2d3e · 0:08" fica ~4 s e some. |
| **Cancelado** | A pílula vermelha "⊘ 2d3e cancelado" (por exemplo, quando o fio é desligado no meio do pedido). |
| **Conversas pausadas** | Um ícone de pausa no meio de cada fio parado. |
| **Agente agindo num portal** | O fio acende na cor do papel do agente e mostra a pílula "clicando" e similares. |

- **Pontos de curva:** duplo clique no fio cria um ponto de curva, que se arrasta com o fio selecionado. Duplo clique no ponto o remove.
- **Fio para outro andar:** não é desenhado. Vira uma **etiqueta embaixo do nó**, "⇄ Planejador · térreo" (capturas `34` e `35`). A etiqueta pulsa enquanto um pedido passa por ela, e um clique leva ao outro andar, centrado no nó.
- **Desligar:** a conversa é cortada na hora, e os pedidos pendentes viram "cancelado". Aparece o toast "Planejador ↔ Implementador desligados".

---

## 11. Conversa entre agentes: o que a pessoa vê

Sequência real capturada (`02` → `03` → `04`). O Planejador pede algo ao Implementador:

1. **O Planejador trabalha** e decide pedir. O estado dele vai a **Trabalhando**.
2. **O pedido sai:** o envelope âmbar corre pelo fio. O Planejador passa a **Esperando resposta**, com a faixa "Aguardando Implementador · pedido 2d3e · enviado há 1 s".
3. **Pendente:** fio tracejado âmbar, com a pílula "⏳ 0:01 2d3e". O Implementador mostra a faixa "Pedido 2d3e de Planejador · a resposta volta para ele", "caixa 1" no rodapé e o estado **Trabalhando**. O terminal dele recebe a mensagem com o envelope `[de: Planejador · pedido 2d3e]`, como se tivesse sido digitada.
4. **Resposta:** quando o turno do Implementador acaba (pelo hook do agente, quase na hora), o envelope verde volta pelo fio. O Implementador passa a **Terminou** e ganha o cartão de turno.
5. **Recebida:** "✓ 2d3e · 0:08" no fio. O Planejador mostra a faixa verde "Resposta de Implementador · 2d3e · 9s" e volta a **Trabalhando** para usar a resposta. Depois, **Terminou**, com o cartão de turno dele.
6. **Na barra superior,** o medidor de taxa vira "1/30". O botão Atividade mostra o contador de pendentes enquanto o pedido está aberto.

Outros comportamentos:

- **Pausar conversas:** os pedidos novos ficam retidos; na Atividade aparece "Retido: conversas pausadas".
- **Limite por minuto** (30): o excedente é retido ("Retido: limite de pedidos por minuto").
- **Laço:** mais de 6 pedidos encadeados (A pede a B, que pede a C…) **pausam tudo sozinhos**, e a Atividade mostra "Laço: limite de 6 saltos atingido… Conversas pausadas até você retomar.".
- **Ask-back:** se B responde perguntando algo direto a A, A é liberado na hora.

---

## 12. Composer

Capturas: `10-composer-com-mencao.png` e `11-composer-adicionar-destino.png`. É o jeito de a pessoa escrever para um ou mais agentes sem clicar no terminal.

**Pílula recolhida** (embaixo, no centro, 460×40):

- ícone de edição, "Escrever para Implementador…" (ou "…para um agente…"), `⌥K` e o botão ↑;
- o destino é o terminal com o teclado, ou o selecionado.

**Composer aberto** (`⌥K` ou clique na pílula; 680 px de largura, flutuante):

- **Linha "Para":**
  - chips dos destinos, nas cores do papel, com ícone, nome, agente em mono e `×`;
  - o botão **"+ destino"** abre a lista dos outros terminais, cada um com o estado;
  - à direita, um aviso de estado do destino: "trabalhando · entra na fila" (azul), "esperando resposta · entra na fila" (âmbar), "iniciando", ou **"parado · inicie o terminal"** (vermelho, que bloqueia o envio);
  - um botão `×` fecha o Composer.
- **Corpo:** texto livre. Dicas do campo: "Escreva a mensagem… use @ para mencionar um nó ligado", ou, sem destino, "Escolha um destino em "+ destino"".
- **@menções:**
  - digitar `@` abre um menu **acima** do Composer, com a seção "Ligados a Implementador" (nós ligados ao destino: terminais com estado, notas com o nome do arquivo, portais) e a seção desligada "Não ligados", com `link_off` e "ligue para mencionar";
  - com vários destinos, só aparecem os nós ligados a todos eles;
  - ↑/↓ escolhem, Enter ou Tab inserem;
  - a menção vira um **chip** dentro do texto (quadrado na cor do papel, ou o ícone de nota ou portal);
  - o agente recebe, junto da mensagem, uma linha explicando como usar cada menção (por exemplo, `squad note read …`).
- **Imagens:** colar com ⌘V, arrastar, ou o botão `image`. Cada imagem vira um cartão com miniatura, nome e "1440×900 · colada", com `×`.
- **Rodapé:**
  - os botões `image` e `@`;
  - a dica "⏎ enviar · ⇧⏎ nova linha · Esc fecha";
  - o botão **"Enviar"**, ou **"Enviar a 2"** com vários destinos.
- **Ao enviar:**
  - o texto é colado no terminal e seguido de Enter, sem envelope;
  - toast "Enviado a Implementador" (ou "Enviado a 2 terminais");
  - em caso de erro, "Não enviado · …";
  - a Atividade registra o envio como "Entregue".

---

## 13. Gaveta de Atividade

Capturas: `06-gaveta-de-atividade.png` e `17` (tema claro).

**Fechada:** uma aba vertical presa à borda direita, com `‹`, "Atividade" escrito na vertical e o contador de pendentes.

**Aberta:** painel à direita, com 380 px de largura e altura total.

**Cabeçalho:**

- "Atividade" e "loja-demo · main" (workspace e branch);
- o botão **"Resumo"**, que abre o resumo do workspace (só aparece se houver um motor de resumo instalado);
- `×` para fechar.

**Filtros:** chips "Todos" e um chip por terminal, com um quadradinho na cor do papel. Clicar filtra o que envolve aquele terminal.

**Lista** (do mais novo para o mais antigo, até 200 itens). Cada item tem:

- a hora (`04:25`);
- a linha "Origem → Destino id", com os nomes nas cores do papel;
- a mensagem;
- uma pílula de desfecho.

| Desfecho | Pílula | Ícone | Cor |
|---|---|---|---|
| Pedido aberto | "Pendente · 0:42" (contando) | `hourglass_top` | âmbar |
| Respondido | "Respondido em 0:09" | `reply` | verde |
| Mensagem da pessoa (Composer) | "Entregue" | `check` | verde |
| Cancelado | "Cancelado" | `block` | vermelho |
| Retido (pausa ou limite) e laço | "Retido" | `pause` | âmbar |
| Nota editada | "Nota editada" | `description` | cinza |
| Fim de turno de um agente | "Fim do turno" ("Planejador terminou o turno") | `check_circle` | verde |

- **Lista vazia:** "Nenhuma conversa ainda. Ligue dois terminais e use squad ask."
- **Rodapé:** ícone `tune` · "máx. 6 saltos · 30 pedidos/min" · "Limites". Hoje "Limites" é só um rótulo, que não abre nada.

---

## 14. Cartões de turno e resumos

Capturas: `04`, `05-cartao-de-turno-aberto-com-resumo.png`, `21-resumos-configuracao.png`, `36-resumo-do-workspace.png`.

### 14.1 Cartão de turno

Quando um agente (Claude, Codex, Devin, Gemini) termina um turno, aparece uma barra fina **embaixo do terminal**, fora do nó:

- **Conteúdo:** ícone ✓ (ou 🧠, se já foi resumido) · "Turno · 18s" · a primeira linha da mensagem final do agente · o botão **"Resumir"** · `×`.
- **Origem do texto:** a mensagem final vem do hook do agente e não custa nada. No Gemini, sem hook, o texto é o fim da tela.
- **Clicar no texto** abre um painel acima da barra com:
  - **Resumo** (se houver), com quem resumiu e os **próximos passos** sugeridos (até 3). Cada passo é um botão que abre o Composer para aquele terminal, com o texto pronto;
  - **Pedido:** a mensagem que iniciou o turno;
  - **Mensagem final**, em Markdown (ou "Fim da tela", em mono).
- **"Resumir"** pede a um agente um resumo de verdade. Enquanto roda, mostra "resumindo…". Se falhar, aparece um ícone de erro, com o motivo na dica.
- **Quando some:** quando o próximo turno começa, ou com `×`. O turno também entra na Atividade como "Fim do turno".

### 14.2 Resumo do workspace

- **Onde:** o botão "Resumo" na gaveta de Atividade abre o diálogo "Resumo do workspace".
- **Conteúdo:**
  - um parágrafo sobre o que a equipe está fazendo;
  - o bloco âmbar **"Pede atenção"**, com até 3 itens;
  - a lista de terminais com o que cada um fez. O nome de cada terminal é um link que leva até ele;
  - o rodapé com "Atualizar", e "por claude · 04:32" no subtítulo.
- **Tempo:** leva alguns segundos ("Lendo o workspace…").

### 14.3 Configuração de resumos (⚙ → Resumos…)

Diálogo por workspace:

- caixa **"Cartão ao fim de cada turno"** (liga ou desliga os cartões);
- **"Quem escreve os resumos"**, com opções de rádio, cada uma com a etiqueta "instalado" ou "não encontrado":
  - **Automático:** o agente do próprio terminal resume; sem a CLI dele, a primeira instalada;
  - Claude Code (`claude -p`, modelo Haiku), Devin (`devin -p`), Codex (`codex exec`), Gemini (`gemini -p`);
  - **Desligado:** só os cartões;
- aviso de custo: cada resumo gasta um pouco do plano da ferramenta e só roda com um clique. O resumo roda numa pasta vazia, sem ferramentas.

Sem nenhuma CLI instalada, o botão "Resumir" não aparece.

---

## 15. Workspaces

Capturas: `22-seletor-de-workspaces.png`, `23-seletor-de-workspaces-sessoes-ao-lado.png`, `24-novo-workspace.png`, `44-workspace-vazio.png`.

**Seletor** (popover sob o botão do workspace):

- título "Workspaces";
- **cada linha:**
  - ponto na cor do workspace e nome;
  - pasta em mono;
  - atividade ("2 ativos · 1 pendente", destacada; ou "3 terminais" ou "sem terminais");
  - atalho `⌥1`…`⌥9`;
  - lápis (editar);
  - seta com o número de sessões;
- **passar o mouse numa linha** (ou clicar na seta) mostra, **ao lado**, as sessões daquele workspace. Clicar numa sessão abre a sessão e aquele workspace;
- **"+ Novo workspace"**, no fim.

**Formulário "Novo workspace" / "Editar workspace":**

| Campo | Detalhes |
|---|---|
| Pasta do projeto | Com sugestões de pasta (obrigatória). |
| Nome (opcional) | O padrão é o nome da pasta. |
| Nome da sessão (opcional) | Só no novo: "sem nome, o agente sugere um". |
| Cor | 8 amostras: teal, blue, purple, pink, red, amber, lime, green. |

- **Botões:** "Criar e abrir" / "Salvar"; "Remover workspace" (vermelho, só na edição e nunca no último workspace).
- **Mudar a pasta** mostra o aviso "Os terminais abertos continuam na pasta antiga até serem reiniciados.".
- **Remover** pede confirmação: os terminais são encerrados, e o canvas e as notas vão para a lixeira (`~/.squad/trash`).
- **Recusas:** não dá para remover nem mudar a pasta de um workspace com andares ou com sessões em cópia.

Outros comportamentos:

- **`⌥1`–`⌥9`** trocam de workspace. A URL guarda o workspace aberto (`?ws=<id>`), então dá para abrir dois em abas diferentes.
- **Ponto azul** no botão do topo: há agentes trabalhando ou pedidos pendentes em outro workspace.
- **Novo workspace pelo terminal:** `squad serve` rodado em outra pasta, com o servidor no ar, abre ou cria o workspace dela no mesmo servidor.
- **Workspace vazio:** só a grade de pontos, sem nenhuma mensagem de "comece aqui" (captura `44`).

---

## 16. Sessões

Capturas: `25-painel-de-sessoes.png`, `26-nova-sessao.png`, `38`–`42`.

Cada workspace tem sessões, e cada sessão é um canvas inteiro. Só a aberta tem processos. Trocar de sessão para os processos da atual e volta tudo da outra como estava: cada agente retoma a sua conversa.

**Painel de sessões** (clique na sessão, no topo):

- título "Sessões de loja-demo";
- **cada linha:**
  - nome (em cinza enquanto provisório) e a marca **"atual"**;
  - "criada 17/10 04:23 · aberta agora" ou "última abertura há 10 min";
  - `⎇ branch`, numa sessão em cópia;
  - conteúdo ("3 terminais · 1 nota · 1 portal · 1 andar", ou "vazia");
  - os chips dos agentes (`claude`, `devin`…);
- **ações por linha:**
  - **lápis** (renomear no lugar: Enter salva, Esc desiste);
  - **lixeira** (remover; não aparece na sessão aberta e fica desligada numa sessão com andares);
  - **"Abrir"**;
  - na sessão atual em cópia, **"Mesclar…"**;
- **"+ Nova sessão"**, no fim.

**Formulário "Nova sessão":**

| Campo | Detalhes |
|---|---|
| Nome (opcional) | "sem nome, o agente sugere um". |
| "Começar com os mesmos terminais, sem as conversas" | Caixa: recria os terminais do térreo, com papéis e fios, e cada agente começa uma conversa nova. |
| Onde a sessão trabalha | Rádio "**Na pasta do projeto**" (a mesma das outras sessões) ou "**Numa cópia isolada**" (um branch novo numa cópia do repositório; o que está sem commit fica de fora). Só aparece em repositórios git. |
| A partir de / Branch novo | Na cópia: o branch de origem (padrão: o atual) e o nome do branch novo, que sai do nome da sessão e pode ser editado. Um branch repetido mostra erro. |

- **Aviso de trabalho pendente:** se a sessão atual tem agentes trabalhando ou arquivos sem commit, aparece "Carrinho novo tem 1 agente trabalhando e 5 arquivos alterados sem commit. Numa cópia, a nova sessão não mexe nisso.", e a opção da cópia já vem marcada.
- **Nota fixa:** "A sessão atual fica guardada e volta como está quando você abri-la de novo."
- **Botão:** **"Criar e abrir"** ("Criando a cópia…" enquanto cria a cópia).

**Trocar com agentes trabalhando** (captura `39`):

- abrir ou criar uma sessão com agentes no meio de um turno mostra um alerta: "Revisor está trabalhando. Trocando agora, o trabalho para no meio. Quando você voltar para esta sessão, o agente aparece como interrompido, com "Continuar"." Botões: **Cancelar** e **Trocar mesmo assim**;
- com pedidos pendentes, eles são cancelados.

**Nome automático:** uma sessão sem nome recebe um título quando o primeiro turno do orquestrador acaba. Quem escreve é o mesmo tipo de agente (o Squad pede um título de 2 a 6 palavras). Sem agente disponível, o título vem do começo do primeiro pedido. Até lá, a sessão aparece em cinza como "Nova sessão · 17/10 14:32".

**Remover uma sessão:** pede confirmação. O canvas e as notas vão para a lixeira; as conversas continuam guardadas nos agentes.

**Atualização automática:** a tela se atualiza sozinha quando a troca vem de outra janela ou da API.

---

## 17. Sessão numa cópia isolada

Capturas: `27-nova-sessao-copia-isolada.png`, `40-sessao-nova-numa-copia-isolada.png`, `41-painel-de-sessoes-com-duas-sessoes.png`.

- **Para quê:** começar uma sessão sem mexer no que outra deixou pela metade na pasta do projeto.
- **Como é feita:** igual a um andar (`git worktree`), a partir do último commit do branch escolhido. Recebe os arquivos da lista de cópia (`.env*`) e roda o comando de preparo.
- **Onde fica:** fora do projeto, em `<pai>/.squad/sessions/<projeto>--<branch>`.
- **Na tela:**
  - o térreo do seletor de andares mostra o **branch da sessão** (ex.: `login-novo`);
  - o caminho no topo e o rodapé dos terminais mostram a pasta da cópia;
  - clicar no térreo ativo abre o menu da cópia: **Mesclar…**, **Log do preparo** e **Copiar o caminho**.
- **Mesclar** (diálogo "Mesclar a sessão"):
  - mostra os commits da sessão e mescla o branch dela no branch escolhido (padrão: o de origem), com commit de merge;
  - **a cópia continua:** a sessão segue trabalhando e pode mesclar de novo;
  - é recusado com alterações sem commit (lista os arquivos), ou se a pasta onde o destino está aberto tiver alterações;
  - num conflito, lista os arquivos para um agente resolver.
- **Remover a sessão** apaga a cópia (o branch fica). Se houver alterações sem commit, o painel lista os arquivos e pede **"Remover mesmo assim"**.

---

## 18. Andares

Capturas: `28-novo-andar.png` a `32-descartar-andar.png`, `34`, `35`.

### 18.1 Seletor no topo

Veja a seção 4, item 6. O andar aberto fica na URL (`&floor=`), e cada andar guarda o próprio zoom. Um andar aterrissado por um agente gera o toast "O andar X foi aterrissado (branch Y)".

### 18.2 Criar (diálogo "Novo andar")

- subtítulo: "Uma cópia isolada do repositório, num branch próprio, com um canvas próprio.";
- **Nome** (até 40 caracteres) e **Branch** (sai do nome sem acentos, editável). Se o branch já existe: "O branch X já existe: o andar abre esse branch como está." Senão: "Parte do último commit de main. O que ainda não foi commitado fica no térreo.";
- seção recolhível **"Preparação"**:
  - "Copiar do térreo (arquivos fora do git)", padrão `.env*`;
  - "Comando de preparo", ex.: `npm install`;
  - os dois valem para os próximos andares do workspace;
- botões **Cancelar** e **Criar andar** ("Criando…");
- ao criar, abre o andar novo, com o toast "Andar X criado: branch Y".

### 18.3 Andar vazio

Um cartão no centro (captura `29`):

- "Andar Página de cupom", `⎇ branch · pasta`;
- o estado do preparo: "Preparando: npm install", com spinner e "ver log"; ou "O preparo falhou…", com "ver log";
- a explicação de como pôr agentes ali: o atalho `T`, ou pedir a um gerente com `squad new "Bia" --role impl --floor "…"`;
- o botão **"Novo terminal neste andar"**.

### 18.4 Menu do andar (clique na aba ativa)

Título "Andar X · ⎇ branch". Itens:

- **Aterrissar…**;
- **Log do preparo**;
- **Copiar o caminho**;
- **Descartar…** (vermelho).

### 18.5 Diálogos do andar

**Aterrissar** (captura `31`):

- commits do andar (ou "Nenhum commit novo no andar"), e os commits novos da origem;
- aviso de alterações não commitadas, com a lista de arquivos, que bloqueia;
- opções:
  - **"Mesclar em [branch ▾]"** (commit de merge);
  - **"Só guardar o branch"**;
- o que acontece ao aterrissar: fecha os terminais e portais, as notas descem para o térreo, a pasta é apagada e o branch continua;
- botão **"Mesclar e aterrissar"** ou **"Aterrissar"**;
- **recusas** (nada muda): conflito, com a lista dos arquivos e a orientação para um agente resolver; pasta de destino com alterações.

**Descartar** (captura `32`):

- lista as consequências: apaga a pasta (com N alterações não commitadas), fecha terminais, as notas descem e o branch é apagado (se foi criado pelo andar);
- quando há trabalho a perder, exige marcar **"Entendo que esse trabalho se perde."**;
- botão vermelho **"Descartar andar"**.

**Log do preparo:**

- a saída do comando, em mono, atualizada a cada 2 s enquanto roda;
- sem comando: "Não rodou comando de preparo. Ele se configura em Novo andar → Preparação."

### 18.6 Andares e o canvas

- Terminal, nota e portal criados na tela vão para o andar aberto.
- Os terminais do andar abrem na cópia; o rodapé de cada um mostra o branch e a pasta de verdade.
- Fios podem ligar andares diferentes: o gerente no térreo conversa com quem está no andar. Esses fios viram etiquetas "⇄ Nome · andar" embaixo do nó (seção 10).

---

## 19. Papéis

Capturas: `19-papeis.png` e `20-papel-novo.png`. Abre por ⚙ → Papéis…, pelo link "Editar papéis…" do formulário do terminal, ou pelo menu ⋯ do terminal.

**Diálogo "Papéis"** ("As instruções que o agente recebe ao abrir. Valem para todos os workspaces."):

- **lista à esquerda:** cada papel com o ícone na cor dele e a etiqueta "editado", se um papel pronto foi alterado. No fim, "+ Novo papel";
- **editor à direita:**
  - **Nome** (até 40 caracteres);
  - **Cor:** 8 opções (`plan` violeta, `impl` ciano, `rev` rosa, `test` lima, `teal`, `indigo`, `magenta`, `orange`);
  - **Ícone:** 13 opções (`account_tree`, `code`, `rate_review`, `science`, `bug_report`, `palette`, `description`, `shield`, `database`, `rocket_launch`, `psychology`, `build`, `architecture`);
  - caixa **"Gerente"**: o agente pode criar terminais ligados a ele e ligar nós; cada terminal pode desligar isso no menu ⋯;
  - **Instruções:** textarea mono, com contador "1831/4000";
- **rodapé:**
  - "Restaurar padrão" (só em papéis prontos editados);
  - "Remover" (só papéis criados pela pessoa; recusado se o papel estiver em uso);
  - "Neste workspace: Planejador" (quem usa o papel);
  - **Salvar** / **Criar papel**.
- **Depois de salvar** um papel em uso: "Salvo. Planejador recebe as instruções novas ao reiniciar. [Reiniciar agora]".
- **Sair com alterações** pede confirmação ("Descartar as alterações deste papel?").

Papéis prontos:

| Papel | Cor | Ícone | Gerente | Função |
|---|---|---|---|---|
| Planejador | violeta | `account_tree` | sim | Planeja e coordena a equipe; divide em frentes e cria andares quando há frentes em paralelo. |
| Arquiteto | índigo | `architecture` | não | Desenha a solução técnica numa nota "Arquitetura: …"; não escreve o código do produto. |
| Implementador | ciano | `code` | não | Implementa. |
| Revisor | rosa | `rate_review` | não | Revisa as mudanças. |
| Testador | lima | `science` | não | Testa; usa o portal para testar interfaces web. |

**Como o papel chega ao agente:** Claude por `--append-system-prompt-file`; Codex por `developer_instructions`; Devin pelo hook `SessionStart`; Gemini como primeira mensagem. Shells não têm papel. Trocar o papel de um agente aberto reinicia o terminal e começa uma conversa nova.

---

## 20. Agente gerente e o que os agentes criam

- **Gerente:** o Planejador já vem como gerente. Cada terminal liga ou desliga isso no menu ⋯ → **Modo gerente**, e o chip **"gerente"** aparece no cabeçalho.
- **O que um gerente pode fazer** (pelos comandos `squad`):
  - criar terminais ao lado dele, já ligados e com papel (`squad new "Bia" --role impl`), até 6 por gerente;
  - ligar nós (`squad connect`);
  - fechar o que ele criou (`squad close`);
  - criar e aterrissar andares (só os que ele criou).
- **Qualquer agente** pode abrir até 3 portais (`squad portal create URL "Nome"`) e criar notas.
- **O que um agente criou aparece marcado:**
  - "● criado por Plano" no rodapé do terminal;
  - "● por Bia" no cabeçalho da nota ou do portal;
  - notas de agente não podem ser renomeadas;
  - os nós nascem num espaço livre ao lado de quem os criou.
- **Aprovação:**
  - as ferramentas MCP seguras vêm liberadas no Claude Code e no Devin;
  - as que rodariam um comando (`keys`, `new_shell`, `portal_eval`, `floor_land`) pedem aprovação **dentro do terminal do agente**: a pessoa vê o pedido na tela do terminal e aprova clicando nele.

---

## 21. Conversas que continuam e trabalho interrompido

- **Conversa lembrada:** cada terminal de agente lembra a conversa em que está. Ao reabrir o `squad serve`, ou em "Reiniciar", o agente volta para a mesma conversa (`claude --resume <id>`, `devin --resume <id>`, `codex resume <id>`, `gemini --resume <id>`).
- **Conversa que não pôde ser retomada:** aparece o toast "Não deu para retomar a conversa de X: começou uma nova.".
- **Faixa de interrompidos** (captura `42`): centralizada no topo do canvas, com fundo âmbar.
  - Mensagem: "⟳ **Revisor foi interrompido** no meio do trabalho. A conversa voltou; falta só continuar."
  - Com vários agentes: "Planejador e Revisor foram interrompidos".
  - Botões: **Continuar** (preenchido; manda "continue de onde parou" a cada um) e **Dispensar**.
  - Quando aparece: o Squad fechou, ou a sessão foi trocada, com um agente no meio de um turno. Funciona no Claude e no Devin.

---

## 22. Configurações (⚙)

Captura `07-menu-configuracoes.png`. Popover que fecha quando o mouse sai.

| Seção | Itens |
|---|---|
| **Tema** | Escuro · Claro. O primeiro uso segue o tema do sistema; a escolha fica guardada. Trocar o tema troca também o tema do terminal. |
| **Agentes** | **Papéis…** (seção 19) · **Resumos…** (seção 14.3) |
| **Terminal** | Noite · Papel: o tema das telas dos terminais, que pode ser diferente do tema da interface. |

Não existem hoje configurações de atalhos, de limites de conversa (fixos em 6 saltos e 30 pedidos/min) nem de agentes e seus comandos.

---

## 23. Login, janela de app e conexão

- **Login** (captura `43-tela-de-login.png`):
  - página simples, servida pelo Go (não pelo React): "squad", o texto "Para entrar, rode `squad serve` no terminal: ele abre o Squad já com o login. Ou cole aqui a URL que ele mostrou." e um campo com o botão "Entrar";
  - o token fica num cookie e não muda entre reinícios.
- **Janela de app:** o `squad serve` abre o Squad numa janela própria (modo app do Chrome, Edge ou Brave), com o nome do workspace no título ("loja-demo · Squad"). A cor da barra de título segue o tema. Fechar a janela não para os agentes. No Mac com Safari, dá para usar "Adicionar ao Dock".
- **Reconexão:** se o servidor cair, a barra superior mostra "reconectando…", em âmbar, e tenta de novo sozinha. Os terminais continuam no estado anterior.
- **PWA:** há um manifest com o nome "Squad" e ícones de 192 e 512 px.

---

## 24. Atalhos de teclado

| Atalho | Onde | Ação |
|---|---|---|
| `V` | canvas | Ferramenta Selecionar |
| `H` | canvas | Ferramenta Mover |
| `T` | canvas | Abrir e fechar "Novo terminal" |
| `N` | canvas | Nova nota |
| `P` | canvas | Novo portal |
| `⌥A` | global | Abrir e fechar a gaveta de Atividade |
| `⌥P` | global | Pausar e retomar conversas |
| `⌥K` | global | Abrir e fechar o Composer |
| `⌥1`…`⌥9` | global | Trocar de workspace |
| `⌥Esc` | terminal e portal | Devolver o teclado ao canvas |
| `Esc` | global | Fechar o formulário de terminal, o Composer, diálogos e menus |
| `Delete` / `Backspace` | canvas | Desligar o fio selecionado |
| `⏎` / `⇧⏎` | Composer | Enviar / nova linha |
| `@` | Composer | Mencionar um nó ligado |
| `⌘V` | Composer, portal | Colar uma imagem (Composer) ou texto (portal) |
| ⌘/Ctrl + roda, pinça | em qualquer lugar | Zoom do canvas |

- **Teclas simples** (V, H, T, N, P) só valem fora de campos de texto e com o painel de papéis fechado.
- **Por que ⌥ nos atalhos globais:** o Ctrl pertence ao terminal em foco.

---

## 25. Mensagens, avisos e confirmações

**Toasts** (aviso escuro curto, embaixo, no centro):

- "Planejador ↔ Implementador ligados" / "desligados";
- "Enviado a Implementador" / "Enviado a 2 terminais" / "Não enviado · …";
- "Não deu para retomar a conversa de X: começou uma nova.";
- "Andar X criado: branch Y"; "X aterrissado: Y mesclado em main (abc1234)"; "Andar X descartado"; "O andar X foi aterrissado (branch Y)";
- "Caminho copiado"; "Copiado: claude --resume …";
- "Nota travada: destrave para editar."; "Já existe uma nota chamada "X" (ou que já teve esse nome)."; "Dê um nome à nota.";
- "Já existe um portal chamado "X"";
- mensagens de erro do servidor (em inglês, por exemplo "folder not found").

**Confirmações nativas do navegador** (`window.confirm`):

- remover workspace; remover sessão; remover terminal; remover nota do canvas;
- nova conversa; trocar o papel de um agente aberto (reinicia);
- reiniciar quem usa um papel; descartar alterações de um papel; restaurar papel padrão; remover papel.

**Confirmações dentro da interface:**

- trocar de sessão com agentes trabalhando ("Trocar mesmo assim");
- remover sessão com cópia suja ("Remover mesmo assim");
- descartar andar ("Entendo que esse trabalho se perde.").

---

## 26. Onde aparece o nome "Squad"

Para a troca de nome, que vem num segundo passo:

- **Interface:**
  - o wordmark "squad" e o logo na barra superior;
  - o `<title>` ("loja-demo · Squad");
  - o `manifest.webmanifest` ("Squad", "Agentes de código em terminais reais, num canvas.");
  - `favicon.svg` e os ícones PNG;
  - a página de login ("squad").
- **Textos com "Squad":**
  - "Restaurar o papel X como o Squad o traz?";
  - "lixeira do Squad (~/.squad/trash)";
  - "Volta às instruções que o Squad traz";
  - "Nenhuma CLI de agente foi encontrada no PATH do Squad" (Resumos);
  - dicas de comando (`squad new`, `squad note`, `squad portal`, `squad ask`).
- **Fora da interface** (afeta o produto inteiro):
  - o comando `squad` e os subcomandos;
  - a pasta `~/.squad`;
  - as variáveis `SQUAD_*`;
  - o servidor MCP `squad` (ferramentas `mcp__squad__*`);
  - a skill `squad`;
  - o envelope `[de: …]`;
  - as pastas `.squad/floors` e `.squad/sessions` ao lado do repositório.

---

## 27. Sistema visual atual

Fonte: `codigo-web/src/theme/tokens.css` (cópia de `design-original/tokens/tokens.css`) e `xterm-themes.ts`.

- **Temas:** escuro (referência principal) e claro, via `data-theme` no `<html>`. O tema do terminal é separado (`data-term`: noite/papel).
- **Neutros:**

  | Tema | Valores |
  |---|---|
  | Escuro | `bg #111214` · `surface #1a1b1e` · `surface-2 #212327` · `surface-3 #2b2d32` · `line #2c2e33` · `line-2 #3c3f46` · `text #e8e9ec` · `text-2 #a9acb3` · `text-3 #8a8e96` · `wire #5a5e68` · `note-bg #1d1d1b` · `term-bg #0d0e10` |
  | Claro | `bg #f2f2ef` · `surface #fff` · `surface-2 #f7f7f5` · `surface-3 #ebebe8` · `line #e3e3df` · `line-2 #cbcbc6` · `text #17181a` · `text-2 #4f5258` · `text-3 #666970` · `wire #9c9d99` · `note-bg #fffef8` · `term-bg #fbfbf9` |

- **Estados** (oklch; `--st-*` é a cor do texto e do ícone, `--stb-*` é o fundo tingido):
  - boot: matiz 250, croma baixo;
  - work: matiz 245 (azul);
  - wait: matiz 78 (âmbar);
  - done: matiz 152 (verde);
  - err: matiz 25 (vermelho);
  - idle e off: neutros.
- **Papéis** (`--r-*` e `--rb-*`):
  - plan: matiz 300 (violeta);
  - impl: 200/210 (ciano);
  - rev: 350 (rosa);
  - test: 110 (lima);
  - teal: 168; indigo: 272; magenta: 325; orange: 52;
  - none: neutro.
- **Workspaces:** 8 cores, que reaproveitam os tokens: teal = `--r-impl`, blue = `--st-work`, purple = `--r-plan`, pink = `--r-rev`, red = `--st-err`, amber = `--st-wait`, lime = `--r-test`, green = `--st-done`.
- **Tipografia:**
  - famílias: Geist (interface) e Geist Mono (terminal, dados, chips de agente, caminhos);
  - escala: 22/650 título · 18/650 cartão · 14 Composer · 13 corpo · 12.5/600 cabeçalho · 11/600 rótulo · 11/15 terminal · 10.5 dado em mono.
- **Medidas:**
  - espaçamento: base 4 (2, 4, 6, 8, 12, 16, 24, 32);
  - raios: 4 chip · 6 controle · 8 nó · 12 flutuante (Composer) · pílula;
  - sombra (escuro): `0 1px 0 rgba(0,0,0,.35), 0 10px 28px rgba(0,0,0,.35)`;
  - alturas fixas: barra superior 44 · cabeçalho do nó 32 · faixa do papel 3 · faixa de remetente 24 · rodapé do nó 22.
- **Ícones:** Material Symbols Outlined (peso 400, opsz 20), num subconjunto com só os 83 ícones usados (`codigo-web/src/lib/icons.ts`).
- **Logo:** feito em CSS (grade 2×2, um quadrado vazado); o favicon segue a mesma ideia.
- **Regras do design original** que a interface segue:
  - estado nunca só por cor;
  - nada translúcido ou desfocado sobre terminais;
  - atalhos globais com ⌥;
  - âmbar só para espera;
  - só os envelopes animam;
  - respeita `prefers-reduced-motion`.

---

## 28. Observações para o redesign

Pontos tirados do código e do uso real. Alguns são lacunas, outros são oportunidades:

1. **Confirmações nativas:** 10 ações usam `window.confirm` do navegador, o que destoa do resto da interface (seção 25).
2. **Workspace vazio** não tem estado vazio nem primeiros passos (captura `44`). Também não existe a "primeira abertura" que o design original previa: escolher a pasta e mostrar quais agentes estão instalados.
3. **"Desenho" e "Grupo"** estão na barra de ferramentas, mas desligados ("em breve").
4. **"Limites"** no rodapé da Atividade parece um link, mas não abre nada; os limites não são configuráveis.
5. **O menu ⚙ fecha quando o mouse sai**, sem clique fora nem Esc.
6. **Notas não redimensionam:** ficam fixas em 400×350.
7. **Não há um estado "precisa de você":** um agente parado esperando permissão aparece como "Trabalhando". A pessoa só descobre olhando a tela.
8. **Mensagens de erro do servidor** às vezes chegam em inglês (ex.: "folder not found").
9. **O design original previa e ainda não existe:**
   - abrir um terminal em foco ou tela cheia (⌥F);
   - encaixar um terminal na borda;
   - ocultar conteúdo sensível;
   - Tab entre nós;
   - camada com os elementos que o agente enxerga no portal (`@e1`…);
   - galeria de **Modelos** (salvar um arranjo de agentes);
   - **chat por terminal**;
   - configurações de atalhos, agentes e limites.
10. **Muita informação em pouco espaço:**
    - a barra superior acumula workspace, sessão, andares, caminho, taxa, pausa, atividade, servidor e ⚙. Em telas estreitas, as abas de andares rolam;
    - o cabeçalho do terminal junta nome, agente, papel, gerente, "teclado aqui", estado e menu.
11. **Andares e sessões em cópia** são parecidos (os dois são `git worktree`), mas aparecem em lugares diferentes: os andares no seletor do topo, a cópia da sessão no painel de sessões e no térreo.
12. **As capturas de terminal ficam pequenas** com zoom abaixo de ~70%. Os níveis "miniatura" e "cartão" existem para isso.

---

## 29. Índice das capturas (`capturas/`)

Todas foram feitas no app real (1600×1000, escala 1,5), com três Claude Code de verdade num projeto de exemplo, "loja-demo". As capturas 02–04 estão em escala 1.

| Arquivo | O que mostra |
|---|---|
| `01-canvas-visao-geral.png` | Canvas completo: Planejador, Implementador e Revisor (Claude reais), nota, portal, fios e cartões de turno. |
| `02-conversa-pedido-pendente.png` | Pedido em andamento: fio tracejado âmbar com cronômetro, faixas "Aguardando…" e "Pedido … de …", estados "Esperando resposta" e "Trabalhando", "caixa 1". |
| `03-conversa-resposta-chegando.png` | Resposta recebida: marcador "✓ 2d3e · 0:08" no fio, faixa verde "Resposta de…". |
| `04-conversa-fim-de-turno.png` | Os dois agentes em "Terminou", com os cartões de turno. |
| `05-cartao-de-turno-aberto-com-resumo.png` | Cartão de turno aberto: resumo, próximos passos, pedido e mensagem final. |
| `06-gaveta-de-atividade.png` | Gaveta de Atividade aberta, com filtros e itens "Fim do turno". |
| `07-menu-configuracoes.png` | Menu ⚙: tema, papéis, resumos, tema do terminal. |
| `08-novo-terminal.png` | Formulário "Novo terminal". |
| `09-menu-do-terminal.png` | Menu ⋯ de um terminal. |
| `10-composer-com-mencao.png` | Composer com destino, texto e o menu de @menção ("Ligados a…" e "Não ligados"). |
| `11-composer-adicionar-destino.png` | Composer com a lista "+ destino". |
| `12-fio-selecionado.png` | Fio selecionado, com o botão "Desligar". |
| `13-nota-em-edicao.png` | Nota em modo de edição (Markdown cru). |
| `14-zoom-miniatura.png` | Nível miniatura (57%). |
| `15-zoom-cartao.png` | Nível cartão (33%). |
| `16-conversas-pausadas.png` | Conversas pausadas: botão "Retomar conversas" e ícone de pausa nos fios. |
| `17-tema-claro.png` | Tema claro. |
| `18-tema-claro-com-atividade.png` | Tema claro com a gaveta de Atividade. |
| `19-papeis.png` | Painel de papéis com o Planejador aberto. |
| `20-papel-novo.png` | Painel de papéis: novo papel. |
| `21-resumos-configuracao.png` | Diálogo "Resumos". |
| `22-seletor-de-workspaces.png` | Seletor de workspaces. |
| `23-seletor-de-workspaces-sessoes-ao-lado.png` | Seletor com as sessões de um workspace ao lado. |
| `24-novo-workspace.png` | Formulário "Novo workspace". |
| `25-painel-de-sessoes.png` | Painel de sessões. |
| `26-nova-sessao.png` | Formulário "Nova sessão". |
| `27-nova-sessao-copia-isolada.png` | Nova sessão com "Numa cópia isolada" marcada (a partir de / branch novo). |
| `28-novo-andar.png` | Diálogo "Novo andar", com a Preparação aberta. |
| `29-andar-vazio.png` | Andar recém-criado, vazio, com o cartão explicativo. |
| `30-menu-do-andar.png` | Menu do andar (aterrissar, log, copiar caminho, descartar). |
| `31-aterrissar-andar.png` | Diálogo "Aterrissar". |
| `32-descartar-andar.png` | Diálogo "Descartar". |
| `33-portal-agente-clicando.png` | Agente clicando no portal: cursor com nome e pílula "clicando" no fio. |
| `34-andar-com-terminal-e-fio-para-o-terreo.png` | Terminal num andar, com a etiqueta "⇄ Planejador · térreo". |
| `35-terreo-etiqueta-de-fio-entre-andares.png` | Térreo: etiqueta "⇄ Testes · Página de cupom" embaixo do Planejador. |
| `36-resumo-do-workspace.png` | Diálogo "Resumo do workspace" (feito por um Claude real). |
| `37-agente-trabalhando.png` | Revisor trabalhando (estado "Trabalhando" com cronômetro). |
| `38-nova-sessao-com-trabalho-pendente.png` | Nova sessão com o aviso de agente trabalhando e a cópia já marcada. |
| `39-trocar-de-sessao-com-agente-trabalhando.png` | Alerta "Revisor está trabalhando…" com "Trocar mesmo assim". |
| `40-sessao-nova-numa-copia-isolada.png` | Sessão nova numa cópia: térreo `login-novo`, canvas vazio. |
| `41-painel-de-sessoes-com-duas-sessoes.png` | Painel com duas sessões (uma em cópia, com "Mesclar…"). |
| `42-agente-interrompido-continuar.png` | Faixa "Revisor foi interrompido… Continuar / Dispensar". |
| `43-tela-de-login.png` | Tela de login. |
| `44-workspace-vazio.png` | Workspace sem nada (pasta que não é git: `+` de andar desligado). |

---

## 30. Mapa do código da interface (`codigo-web/`)

React 19 + Vite + TypeScript, com `@xyflow/react` 12 (canvas), `@xterm/xterm` 6 (terminais) e `zustand` (estado). CSS puro com os tokens; sem Tailwind.

| Arquivo | O que é |
|---|---|
| `index.html` | O HTML da página: só `<div id="root">`. Todo o resto é renderizado pelo React. |
| `src/main.tsx`, `src/App.tsx` | Entrada, tema, título da janela, atalhos globais e a montagem da tela. |
| `src/state/store.ts` | Todo o estado do cliente e o tratamento dos eventos do servidor. |
| `src/api/types.ts` | Os tipos de dados (nós, fios, andares, sessões, cartões, papéis…). |
| `src/api/client.ts`, `events.ts`, `workspaces.ts` | Chamadas REST e WebSockets. |
| `src/components/TopBar.tsx` | Barra superior e menu ⚙. |
| `src/components/Toolbar.tsx` | Barra de ferramentas. |
| `src/components/WorkspaceMenu.tsx` | Seletor de workspaces e formulário. |
| `src/components/SessionsMenu.tsx` | Painel de sessões, nova sessão, confirmação de troca. |
| `src/components/FloorBar.tsx`, `FloorDialogs.tsx`, `FloorEmpty.tsx` | Andares e cópia da sessão. |
| `src/components/Composer.tsx`, `ComposerPill.tsx` | Composer. |
| `src/components/ActivityDrawer.tsx` | Gaveta de Atividade. |
| `src/components/RolesPanel.tsx` | Papéis. |
| `src/components/SummaryDialogs.tsx` | Resumo do workspace e configuração de resumos. |
| `src/components/TerminalForm.tsx` | Novo terminal / Configurar. |
| `src/components/InterruptedBar.tsx` | Faixa de interrompidos. |
| `src/components/ZoomControls.tsx` | Minimapa e zoom. |
| `src/components/Dialog.tsx`, `DirInput.tsx`, `Icon.tsx` | Peças comuns. |
| `src/canvas/Canvas.tsx` | O canvas (React Flow). |
| `src/canvas/nodes/TerminalNode.tsx` | Nó de terminal (vivo, miniatura, cartão), faixas, rodapé, menu. |
| `src/canvas/nodes/NoteNode.tsx`, `PortalNode.tsx`, `TurnCard.tsx`, `CrossFloorLinks.tsx`, `StatePill.tsx`, `NodeMenu.tsx`, `NodeHandles.tsx` | Os outros nós e peças. |
| `src/canvas/edges/WireEdge.tsx`, `geometry.ts` | Os fios e a geometria da curva. |
| `src/term/session.ts`, `XTermView.tsx` | Terminal xterm.js e a escala no zoom. |
| `src/theme/tokens.css`, `base.css`, `xterm-themes.ts` | Tokens, estilos base, temas do terminal. |
| `src/lib/*` | Textos de estado, papéis, andares, sessões, Markdown, ícones, caminhos. |
| `src/**/*.css` | Um CSS por componente. |
