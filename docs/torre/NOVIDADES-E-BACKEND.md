# Novidades e o que elas pedem do backend

Tudo o que não existe hoje no Squad, com o quadro do mockup onde aparece. Cada item diz se é **só front** ou se **pede backend**, e traz um contrato proposto, para ser ajustado ao código real. A base atual está em `referencia-squad-atual/REFERENCIA-TECNICA.md`.

Resumo:

| # | Novidade | Quadro | Backend? |
|---|---|---|---|
| 1 | **Estado PRIORIDADE (o voo pede você)** | 05, 01, 02, 07 | **sim, o principal** |
| 2 | Indicativo do voo e sigla do papel | todos, 17 | sim (dados) |
| 3 | Níveis numerados (FL1…) e renomear nível | 13 | sim (dados + rota) |
| 4 | Limites configuráveis | 18, 07 | sim |
| 5 | Cancelar um pedido sem desligar a frequência | 07 | sim (rota) |
| 6 | Checagem prévia de conflito no pouso | 14, 01 | sim (rota) |
| 7 | Agentes detectados (versão, comando padrão, caminho) | 16, 18, 19 | sim (rota) |
| 8 | Primeira abertura (enlace, pasta, agentes) | 19 | sim (pouco) |
| 9 | Planos de voo (salvar e restaurar uma equipe) | 20 | sim |
| 10 | @ELEMENTOS no portal | 10 | sim (expor os refs) |
| 11 | NOTAM redimensionável | 09 | sim (pouco) |
| 12 | Rádio: pedido inteiro, fila de prioridade, filtros | 07 | depende |
| 13 | Erros do servidor em português | 21 | sim (mensagens) |
| 14 | Atividade em outro workspace com prioridade | 11, 05 | sim (resumo) |
| 15 | Vista Sinóptico | 01 | só front |
| 16 | Confirmações na interface (no lugar de `window.confirm`) | 15 | só front |
| 17 | Foco, encaixe, ocultar, Tab entre nós | 04, 16 | só front (+ persistência opcional) |
| 18 | Transmitir a todos de um nível | 06 | só front |
| 19 | Avisos (título da janela, notificação, som) | 18, 05 | só front |
| 20 | Espaço vazio com primeiros passos | 20 | só front |
| 21 | Configurações num painel (fecha com Esc e clique fora) | 18 | só front |
| — | Pilha de NOTAMs, Desenho, Grupo | 09, 02 | depois |

---

## 1. Estado PRIORIDADE: "o voo pede você"

**O problema** (`FUNCIONALIDADES.md` §7.2 e §28.7): um agente parado num pedido de permissão aparece como "Trabalhando". A pessoa só descobre olhando a tela dele. É o ponto fraco mais citado, e a Torre gira em torno de resolver isso.

**O que a pessoa vê** (quadro 05; também 01, 02 e 07):

- **No voo:** contorno âmbar de 2 px com halo fixo; a faixa de estado âmbar cheia **piscando a 1 Hz**; o estado "PEDE VOCÊ 0:24"; a faixa de 26 px com a frase do pedido ("Quer executar um comando"). Embaixo do nó, **fora dele**, a barra RESPONDER com AUTORIZAR (1), SEMPRE (2), NEGAR (3) e ABRIR TELA.
- **Quando o voo está fora da tela:** uma faixa no topo do canvas, "PRIORIDADE · TST em FL1 quer rodar um comando · IR ATÉ ELE ⌥⏎"; um ponto âmbar na aba do nível; o contador no título da janela; uma notificação do sistema (opcional, item 19).
- **No Sinóptico:** o cartão "PRIORIDADE · PEDE VOCÊ" com a pergunta e os três botões. Várias prioridades viram fila ("1 de 3"), da mais antiga para a mais nova.
- **No Rádio:** um item "TST → VOCÊ · Pede você · 0:24", pregado no topo, e o filtro "PEDEM VOCÊ".
- **Em outro workspace:** o ponto do seletor fica âmbar (azul é só "voos no ar").
- **Âmbar é a cor da espera.** AGUARDA (faixa listrada) espera outro voo; PRIORIDADE (faixa cheia, piscando) espera você. Só a prioridade pisca.

**Backend:**

1. **Valor novo em `TerminalRuntime.activity`: `ask`.**
   - A ordem passa a ser `err`/`off` > **`ask`** > `wait` > `work` > `boot` > `done` > `idle`.
   - O cronômetro usa `activitySince`.
2. **Campo novo em `TerminalRuntime`:** `attention?: { kind: "permission" | "question" | "idle", text: string, detail?: string, since: string, choices?: Choice[] }`.
   - `text` é a frase curta ("Quer executar um comando").
   - `detail` é o que está em jogo (ex.: `npm run e2e -- --grep export`).
   - Sai no evento `terminal.state` como já acontece com a atividade.
3. **Como detectar, por agente:**

   | Agente | Como |
   |---|---|
   | Claude Code | Hook **`Notification`**, já tratado pelo `squad hook` → `torre hook`. O payload traz a mensagem, por exemplo "Claude needs your permission to use Bash". Entra em `ask` na notificação de permissão e sai quando a tela muda e o agente volta a trabalhar (hook seguinte, `PreToolUse`/`PostToolUse`, ou mudança da tela). Para preencher `detail` com o comando, ler a tela ou usar o hook `PreToolUse`. |
   | Devin | Hooks equivalentes (verificar quais existem). |
   | Codex, Gemini | Detecção pela tela, como reserva: procurar os padrões de pergunta de permissão de cada CLI ("Do you want to proceed?", "Allow command?"…) numa tela parada. Marcar `source: "screen"`. |
   | Shell, comando próprio | Não se aplica. |

4. **Rota nova:** `POST /w/<id>/api/nodes/{id}/attention/respond { choice: "allow" | "always" | "deny" }`.
   - O servidor traduz a escolha nas teclas daquele agente, a partir de uma tabela por agente e versão. No Claude Code de hoje: `1`, `2`, `3` (ou Esc), mais Enter se precisar.
   - Responder digitando direto no terminal funciona igual: o estado sai de `ask` quando a tela muda.
   - **Cuidado:** a tabela de teclas tem que ser conferida em cada CLI; se o padrão não bater, não mande teclas e mostre só ABRIR TELA.
5. **`WorkspaceSummary` ganha `attention`** (quantos voos em `ask`), para o seletor de workspaces e o ponto da barra.
6. **`StateExtra` ou `hello` com a fila:** `attention[]` = `{ terminal, name, callsign, floor, kind, text, detail, since }`, da mais antiga para a mais nova, para a fila do Sinóptico e o ⌥⏎.
7. **Item novo no Rádio:** evento `attention.raised` / `attention.cleared` (ou usar `terminal.state`), para registrar "Pede você" e quanto tempo levou.
8. **Testes:** a transição `work → ask → work`, a prioridade sobre `wait`, a resposta por rota e a limpeza quando o processo sai.

**Front:**

- A pílula e as faixas do estado `ask`.
- A barra RESPONDER e a faixa de prioridade no canvas.
- O atalho **⌥⏎** (vai para a próxima prioridade, centra o voo e dá a ele o teclado).
- O contador no `<title>` e o ponto âmbar na aba do nível e no seletor.

---

## 2. Indicativo do voo e sigla do papel

- **`RoleDef.abbr`:** 3 letras, editável no painel de papéis (quadro 17, campo SIGLA). Padrões: PLN, ARQ, IMP, REV, TST.
- **Nó de terminal `callsign`:** 3 caracteres, único no workspace.
  - Padrão: a sigla do papel; com repetição, numera (IMP, IM2…). Sem papel: as 3 primeiras consoantes do nome (DEV, GMN).
  - Editável no Novo voo e no Configurar (quadro 16).
- **CLI e MCP (opcional, recomendado):** aceitar o indicativo onde hoje vai o nome (`torre ask IMP "…"`) e usá-lo no envelope (`[de: PLN · pedido 7f3a]`).

## 3. Níveis numerados e renomear

- **`Floor.number`:** inteiro estável por workspace, na ordem de criação (FL1, FL2…). Não reaproveitar números de níveis descartados na mesma sessão.
- **Rota nova:** `PATCH /api/floors/{id} { name }` (o menu do nível ganha "Renomear"). O branch não muda.

## 4. Limites configuráveis

- **Hoje fixos:** 6 saltos, 30 pedidos por minuto, 6 voos por líder, 3 portais por agente.
- **Rotas novas:** `GET/PUT /w/<id>/api/limits { maxHops, perMinute, maxTeam, portalsPerAgent }`, por workspace, guardado no workspace. `StateExtra.limits` passa a refletir o valor salvo.
- O botão LIMITES do Rádio, que hoje é só um rótulo, abre esse painel.

## 5. Cancelar um pedido

- **Rota nova:** `POST /w/<id>/api/requests/{id}/cancel`.
- Emite `request.cancelled`, como acontece hoje ao desligar o fio, mas sem desligar a frequência.
- Botão CANCELAR PEDIDO no item pendente do Rádio.

## 6. Checagem prévia de conflito no pouso

- O diálogo de pouso mostra quatro caixas antes do botão: **sem conflito**, **tudo commitado**, **voos no ar** e **o quanto main andou**.
  - Commitado, voos no ar e quanto main andou já existem (`changes`, `ahead`, `behind`, `terminals`).
  - O conflito hoje só aparece ao tentar.
- **Proposta:** `GET /api/floors/{id}/changes` passa a trazer `conflicts?: string[]`, calculado sem mexer em nada (`git merge-tree --write-tree` ou equivalente). O mesmo vale para `GET /api/copy/changes`.
- **A fila de pouso do Sinóptico** usa os mesmos dados. O mockup mostra "✓ testes · ✓ revisão": **esses dois não existem**. Para a v1, mostre só conflito, commits, alterações e voos no ar. Testes e revisão ficam para uma versão futura (por exemplo, um agente marcar com `torre floor ready --check testes`).

## 7. Agentes detectados

- **Rota nova:** `GET /api/agents` → `[{ agent, found, version?, path?, defaultCommand, installHint? }]`, mais `{ browser: { found, path } }` para os portais.
  - Pode reaproveitar a busca que os Resumos já fazem ("instalado" / "não encontrado").
- **`PUT /api/agents/{agent} { defaultCommand }`:** o comando padrão editável na aba Agentes das Configurações (guardado em `~/.torre`).
- **Onde aparece:** Novo voo ("✓ instalado" / "✕ não achado" em cada agente), Configurações → Agentes e a primeira abertura.

## 8. Primeira abertura

O quadro 19 mostra três passos: **1. Enlace** (o login de hoje, com a cara nova), **2. Pasta** e **3. Agentes**.

- **Passo 1:** a página de login do Go é redesenhada. Com o `?token=` certo, pula direto para o passo 2.
- **Passo 2:**
  - Precisa saber se a pasta é git, o branch, se está limpa e se há `.env*`. **Rota nova:** `GET /api/dirs/info?path=` → `{ exists, git, branch?, clean?, envFiles[] }`.
  - Também mostra as pastas recentes.
- **Passo 3:** usa a rota do item 7.
- **Quando mostrar:** na primeira vez que o servidor sobe sem nenhum workspace guardado. Hoje o `serve` já cria o workspace da pasta atual; nesse caso, o passo 2 vem preenchido com ela.

## 9. Planos de voo (os "modelos" do brief original)

- **O que guarda:**
  - voos: agente, nome, indicativo, papel, líder, comando, pasta relativa, posição, tamanho;
  - NOTAMs: título, e o conteúdo se a pessoa quiser;
  - portais: nome e URL;
  - frequências e níveis: nome e branch-modelo.
- **Rotas novas (hub):** `GET/POST/DELETE /api/plans`, guardados em `~/.torre/plans/*.json`; `POST /api/plans/import` (arquivo); `POST /w/<id>/api/plans/{id}/apply { floor? }`.
- **Antes de aplicar**, a interface lista os comandos que vão rodar (segurança). Comandos próprios vindos de um plano importado nunca rodam sem essa revisão.
- **Salvar:** pela seleção no Espaço (⌥⇧M ou o menu da seleção). **Abrir a galeria:** ⌥M, ou a barra de ferramentas.

## 10. @ELEMENTOS no portal

- O `torre portal snapshot` já gera os refs (`@e1`, `@e2`…).
- **Proposta:** no WebSocket do portal, uma mensagem nova `elements` com `[{ ref, role, name, x, y, w, h }]` do último snapshot, enviada quando a camada está ligada.
- O botão "@ ELEMENTOS" desenha os contornos tracejados com o ref.

## 11. NOTAM redimensionável

- Hoje fixa em 400×350. Passa a aceitar `w` e `h` (mínimo 280×200) pelo `PATCH /api/nodes` que já existe. Só conferir se o servidor ignora `w`/`h` em notas e liberar.

## 12. Rádio

- **Pregar no topo** o que pede você e os pendentes, e o filtro PEDEM VOCÊ: só front.
- **Expandir um item** com o pedido e a resposta inteiros: o evento traz `preview`. Se o texto inteiro não estiver no cliente, criar `GET /api/requests/{id}` → `{ text, reply?, from, to, sentAt, repliedAt? }`.
- **Histórico:** confirmar se fica só no cliente (até 200 itens). Se for só no cliente, avaliar guardá-lo por sessão, para não sumir ao recarregar.

## 13. Erros em português

- Erros que hoje chegam em inglês ("folder not found"…) passam a vir em português, ou com um `code` que o front traduz. Prefira `code` + `message` em português.

## 14. Prioridade em outro workspace

- `WorkspaceSummary.attention` (item 1.5) alimenta o ponto âmbar no botão do workspace e a linha "1 pede você" no seletor (quadro 11).

## 15 a 21. Só front

- **15 · Sinóptico:** tudo vem do estado que já existe.
  - O radar desenha as posições reais (`x`, `y`) escaladas, com um anel por nível (pista no centro).
  - As fichas usam `activity`, `activitySince` e `StateExtra`; as frequências, `edges` + `pending`.
  - O terminal da direita segue a seleção, com alfinete para fixar.
  - A troca de vista fica na barra; atalho sugerido **⌥S**.
- **16 · Confirmações:** um componente `ConfirmDialog` com título-pergunta, lista de consequências (✕ o que se perde, ✓ o que fica), cor da borda pela gravidade e o botão com o verbo. Substitui as 10 chamadas de `window.confirm` (`FUNCIONALIDADES.md` §25). Os números (alterações, frequências, pedidos) vêm do estado ou das respostas 409.
- **17 · Foco, encaixe, ocultar, Tab:**
  - ⌥F: tela cheia.
  - ⌥⇧← / ⌥⇧→: encaixa o voo numa borda; a borda fica guardada por workspace.
  - ⋯ → Ocultar a tela: um padrão listrado opaco, não desfoque.
  - Tab / ⇧Tab entre nós; Enter dá o teclado.
  - Guardar `redacted` no nó é opcional.
- **18 · Transmitir a um nível:** o "+ destino" ganha "Todos os voos do FL1"; manda pelo `send` que já existe, um por voo.
- **19 · Avisos:** contador no `<title>`, Notification API do navegador e som curto, com as opções em Configurações → Avisos.
- **20 · Espaço vazio:** três caminhos (plano de voo, novo voo, NOTAM). Some assim que entra o primeiro nó.
- **21 · Configurações:** vira um painel com abas (Aparência, Agentes, Limites, Resumos, Papéis, Atalhos, Avisos) e fecha com Esc e clique fora (hoje fecha quando o mouse sai).

## Depois (aparecem, mas não foram desenhados em detalhe)

- **Pilha de NOTAMs** (quadro 09): `stack` nas notas. A frequência ligada à pilha vale para todas, e isso mexe na regra de permissão; precisa de desenho próprio.
- **Desenho e Grupo:** os botões aparecem ligados na barra de ferramentas (quadro 02), mas os nós `drawing` e `group` ainda precisam de desenho. Até lá, mantê-los desligados com "em breve".

---

## Atalhos (consolidado)

| Atalho | Ação | Novo? |
|---|---|---|
| V, H, T, N, P | Selecionar, Mover, Novo voo, NOTAM, Portal | — |
| D, G | Desenho, Grupo | depois |
| ⌥A | Rádio | — |
| ⌥P | Espera geral / Liberar todos | — |
| ⌥K | Transmitir | — |
| ⌥1…⌥9 | Trocar de workspace | — |
| ⌥Esc | Devolver o teclado ao canvas | — |
| **⌥S** | Sinóptico ↔ Espaço | novo |
| **⌥⏎** | Ir à próxima prioridade | novo |
| **⌥F** | Foco (tela cheia) | novo |
| **⌥⇧← / ⌥⇧→** | Encaixar o voo numa borda | novo |
| **Tab / ⇧Tab** | Próximo / anterior nó | novo |
| **⌥M** | Planos de voo | novo |
| **⌥⇧M** | Salvar a seleção como plano | novo |
| 1 / 2 / 3 (com a barra RESPONDER em foco) | Autorizar / Sempre / Negar | novo |
