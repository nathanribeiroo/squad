# Squad: referência técnica (o que existe por trás da interface)

Complemento de `FUNCIONALIDADES.md`. Aqui estão os dados, eventos, comandos e regras que a interface usa ou poderia mostrar. Tudo foi tirado do código Go (`cmd/squad`, `internal/*`). Serve para o redesign saber **o que está disponível** para cada tela.

---

## 1. Arquitetura em uma frase

- **Um binário só, o `squad`**, com quatro papéis:
  - servidor local: `squad serve`, só em `127.0.0.1`, porta 7420;
  - CLI que os agentes usam dentro dos terminais: `squad ask`…;
  - servidor MCP: `squad mcp`;
  - receptor dos hooks dos agentes: `squad hook`.
- **Hierarquia:** servidor → workspaces (todos rodando) → sessões (uma aberta por vez) → canvas (nós e fios) → andares.
- **Interface:** a React vem embutida no binário. Ela recebe o estado por WebSocket e manda as ações por REST.

---

## 2. Modelo de dados (o que a interface recebe)

### Workspace (`Workspace` / `WorkspaceSummary`)

- **Campos:** `id` (8 hex), `name`, `cwd` (pasta), `color` (teal, blue, purple, pink, red, amber, lime, green), `createdAt`.
- **Resumo do seletor:**
  - `terminals`;
  - `active` (agentes trabalhando ou esperando);
  - `pending` (pedidos esperando resposta);
  - `session` (a aberta);
  - `sessions` (quantas existem).
- **Canvas:** `nodes[]`, `edges[]`, `floors[]`, `floorConfig {copy[], setup}` e `copy?` (o andar especial "cópia da sessão").

### Sessão (`Session` / `SessionSummary`)

- **Campos:** `id`, `name`, `named` (falso enquanto o nome é provisório, "Nova sessão · 17/10 14:32"), `createdAt`, `openedAt`.
- **No painel:** `current`, `terminals`, `notes`, `portals`, `floors`, `agents[]` (tipos de agente), `branch?` e `copy?` (sessão numa cópia).

### Nó (`WNode`)

- **Campos comuns:** `id`, `kind` (`terminal` | `note` | `portal`), `x`, `y`, `w`, `h`, `createdBy?` (o terminal do agente que criou o nó) e `floor?` (vazio é o térreo).
- **`terminal`:**
  - `name` (único);
  - `agent` (`claude` | `codex` | `devin` | `gemini` | `shell` | `custom`);
  - `command?`, `shell?`, `cwd?`, `role?`;
  - `manager?` (nulo segue o papel);
  - `cols`, `rows`;
  - `conversation?` (id da conversa do agente);
  - `empty?` (nada foi dito ainda);
  - `busy?` (estava no meio de um turno).
- **`note`:** `title`, `file` (nome do arquivo `.md`; vazio enquanto rascunho), `locked?`, `draft?`, `aliases[]` (nomes antigos).
- **`portal`:** `url`, `name`.

### Fio (`WEdge`)

- **Campos:** `id`, `a`, `b`, `ropePoints [[x,y]…]` (pontos de curva), `createdAt`.
- **Sem direção nem tipo.**

### Estado de execução do terminal (`TerminalRuntime`)

- `status`: `starting` | `running` | `exited` | `stopped` | `failed`;
- `exitCode`, `error?`;
- `title?` (título que o programa define, com o spinner do agente);
- `activity`: `boot` | `idle` | `work` | `wait` | `done` | `err` | `off`;
- `activitySince` (para os cronômetros);
- `interrupted?`.

| Atividade | Regra |
|---|---|
| `boot` | Sem saída ainda (até 30 s). |
| `work` | Turno aberto pelo hook, ou tela mudando bastante, ou spinner no título. |
| `wait` | Tem um `ask` pendente ou está num `squad wait`. |
| `done` | Até 10 min depois do fim do turno. |
| `err` | Falha, ou saída com código diferente de 0. |
| `off` | Parado, ou saída com 0. |
| `idle` | O resto. |

- **Prioridade:** `err`/`off` > `wait` > `work` > `boot` > `done` > `idle`.
- **Não há um estado "esperando permissão do usuário":** um agente parado num pedido de permissão conta como `work`.

### Extras da conversa (`StateExtra`)

- `paused`;
- `limits {maxHops: 6, perMinute: 30}`;
- `rate` (pedidos no último minuto);
- `pending[]`: `{id, from, to, fromName, toName, preview, hop, sentAt}`.

### Andar (`Floor`)

- **Campos:** `id`, `name`, `branch`, `base` (branch de origem), `baseCommit`, `path`, `cwd`, `newBranch`, `copied[]`, `status` (`preparing` | `ready` | `failed`), `error?`, `createdBy?`, `createdAt`, `terminals?`, `missing?`.
- **Estado git** (consultado a cada 10 s): `{branch, changes, ahead?, missing?}`, para o térreo e para cada andar.
- **Alterações** (diálogos de aterrissar e mesclar): `{files[{path, status}], ahead, behind, commits[{hash, subject}], missing?}`.

### Cartão de turno (`TurnCard`)

- **Campos:** `terminal`, `start?`, `at`, `source` (`hook` | `screen`), `prompt?` (o pedido), `message` (a mensagem final), `summary? {text, steps[≤3], by, at}`, `summarizing?`, `error?`.
- **Persistência:** fica guardado por sessão (`cards.json`).

### Resumo do workspace

`{text, terminals[{name, status}], attention[≤3], by, at}`.

### Papel (`RoleDef`)

- **Campos:** `id`, `name` (≤ 40), `prompt` (≤ 4000), `color`, `icon`, `manager?`, `builtin?`, `edited?`.
- **Cores:** `plan, impl, rev, test, teal, indigo, magenta, orange`.
- **Ícones:** `account_tree, code, rate_review, science, bug_report, palette, description, shield, database, rocket_launch, psychology, build, architecture`.

---

## 3. Eventos em tempo real (WebSocket `/w/<id>/ws/events`)

A primeira mensagem é `hello`, com o estado completo. Depois vem um evento por mudança:

| Evento | Para que a interface usa |
|---|---|
| `terminal.state` | Estado e atividade de um terminal (pílula, cronômetro, minimapa). |
| `terminal.title` | Título do terminal (spinner do agente). |
| `terminal.notice` (`not-resumed`) | Toast "Não deu para retomar a conversa…". |
| `node.added` / `node.updated` / `node.removed` | Nós no canvas. |
| `edge.added` / `edge.updated` / `edge.removed` | Fios. |
| `request.sent` / `request.pending` | Pedido saindo e pendente (envelope, fio tracejado, faixas, Atividade). |
| `reply.sent` / `reply.received` | Resposta voltando e recebida (`elapsedMs`, `source`). |
| `request.cancelled` | Pedido cancelado. |
| `request.held` | Pedido retido (pausa ou limite por minuto). |
| `conversations.paused` | Pausa (`reason`: `user`, ou "hop limit 6 reached (A → B)" num laço). |
| `message.sent` | Mensagem da pessoa pelo Composer. |
| `note.edited` | Nota editada (`author`: `user`, o nome do agente, ou `external`). |
| `portal.action` | Agente agindo num portal (`kind`: click, fill, type, key, scroll, goto, back, forward, reload; `label`; `x`, `y`). |
| `turn.card` | Cartão de turno novo ou atualizado (resumo pronto). |
| `summary.settings` | Configuração de resumos. |
| `floors.updated` | Andares, configuração de preparo e cópia da sessão. |
| `workspace.updated` | Nome, pasta e cor. |

**Outros WebSockets:**

- **`/ws/workspaces`:** lista de workspaces, biblioteca de papéis e sessões de cada workspace, ao vivo.
- **`/w/<id>/ws/term/<id>`:** a tela do terminal (binário) e o teclado.
- **`/w/<id>/ws/portal/<id>`:**
  - do servidor: quadros JPEG da página e o estado `{url, title, loading, error, canBack, canForward, width, height}`;
  - do cliente: mouse, teclado, colagem e navegação.

---

## 4. API REST

A API de cada workspace fica em `/w/<id>/api/…`; a do hub fica na raiz.

**Hub:**

| Rota | O que faz |
|---|---|
| `GET/POST /api/workspaces` | Lista e cria workspaces. |
| `PATCH/DELETE /api/workspaces/{id}` | Edita e remove um workspace. |
| `GET/POST /api/workspaces/{id}/sessions` | Lista sessões; cria uma nova (`name`, `keepTerminals`, `confirm`, `copy {branch, from}`). |
| `POST …/sessions/{sid}/open` | Abre outra sessão (409 com `working[]` se houver agentes trabalhando, a menos que venha `confirm`). |
| `PATCH/DELETE …/sessions/{sid}` | Renomeia e remove uma sessão (409 com `files[]` se a cópia tiver alterações sem commit). |
| `GET /api/dirs?path=` | Sugestões de pasta. |
| `GET/POST/PUT/DELETE /api/roles…`, `POST /api/roles/{id}/reset` | Biblioteca de papéis. |

**Workspace — canvas e terminais:**

| Rota | O que faz |
|---|---|
| `GET /api/state` | Estado completo. |
| `POST/PATCH/DELETE /api/nodes…` | Criar, mover, redimensionar, editar e remover nós. |
| `POST /api/nodes/{id}/start\|stop\|restart\|new-conversation` | Ações do terminal. |
| `POST /api/nodes/{id}/send` | Composer: `{text, mentions[], images[]}`. |
| `POST /api/nodes/{id}/summarize` | Pede o resumo do turno. |
| `POST/PATCH/DELETE /api/edges…` | Fios e pontos de curva. |
| `GET/PATCH/PUT /api/notes/{id}` | Ler, renomear e gravar uma nota. |
| `POST /api/conversations/pause\|resume` | Pausar e retomar as conversas. |
| `POST /api/interrupted/continue\|dismiss` | Faixa de interrompidos. |
| `GET /api/shells`, `GET /api/dirs` | Shells instalados e sugestões de pasta. |

**Workspace — resumos:**

| Rota | O que faz |
|---|---|
| `GET /api/summary` | Os cartões de turno. |
| `POST /api/summary/workspace` | Resumo do workspace. |
| `GET/PUT /api/summary/settings` | Configuração de resumos. |

**Workspace — andares e cópia da sessão:**

| Rota | O que faz |
|---|---|
| `GET/POST /api/floors` | Lista e cria andares. |
| `PUT /api/floors/config` | Lista de cópia e comando de preparo. |
| `GET /api/floors/status` | Estado git do térreo e dos andares. |
| `GET /api/branches` | Branches locais. |
| `GET /api/floors/{id}/changes` | O que aterrissar levaria. |
| `GET /api/floors/{id}/log` | Log do preparo. |
| `POST /api/floors/{id}/land` | Aterrissar (`merge`, `into`). |
| `DELETE /api/floors/{id}` | Descartar. |
| `GET /api/copy/changes`, `POST /api/copy/merge`, `GET /api/copy/log` | A cópia da sessão aberta. |

**Recusas que a interface mostra** (409):

- `files[]`: alterações sem commit;
- `conflicts[]`: conflito de merge;
- `folder` e `branch`: a pasta de destino tem alterações;
- `working[]` e `pending`: agentes trabalhando.

---

## 5. Comandos `squad` (CLI)

### Da pessoa

| Comando | O que faz |
|---|---|
| `squad serve [-dir] [-port] [-open app\|browser\|none] [-max-team N]` | Sobe o servidor e abre a janela de app. Com o servidor já no ar, abre ou cria o workspace da pasta nele. |
| `squad install-skill [-agent claude\|devin\|all]` | Ensina o Squad ao Claude Code e ao Devin; no Devin, registra também o servidor MCP. |
| `squad version` | Mostra a versão. |

### Dos agentes (dentro de um terminal do Squad)

| Comando | Quem pode | O que faz |
|---|---|---|
| `squad list` | todos | Quem você é e o que está ligado a você (com o estado de cada terminal). |
| `squad ask "Nome" "msg"` | todos | Entrega com o envelope `[de: Você · pedido 7f3a]` e espera a resposta final do turno. |
| `squad ask "Nome" --raw "teclas"` | todos | Digita teclas e devolve a tela. |
| `squad ask --batch '{"A":"…","B":"…"}'` | todos | Vários pedidos em paralelo. |
| `squad check "Nome"` | todos | Lê a tela de um terminal ligado. |
| `squad wait "Nome"` | todos | Espera o turno do outro acabar. |
| `squad reply <id> "msg"` | quem recebeu o pedido | Responde um pedido. |
| `squad note list\|read\|write\|edit\|create` | todos | Notas ligadas. |
| `squad portal snapshot\|click\|fill\|type\|key\|scroll\|goto\|back\|forward\|reload\|screenshot\|console\|eval\|create\|close` | todos | Usar e abrir portais. |
| `squad roles` | todos | Os papéis disponíveis. |
| `squad new "Nome" --role impl [--agent] [--cwd] [--floor] [--command]` | gerentes | Cria um terminal ligado e pronto. |
| `squad connect "A" "B"` · `squad close "Nome"` | gerentes | Liga nós; fecha o que criou. |
| `squad floor list\|create\|land` | todos listam; gerentes criam e aterrissam os seus | Andares. |

---

## 6. Ferramentas MCP (as mesmas ações, com argumentos tipados)

| Ferramenta | Aprovação |
|---|---|
| `list`, `roles`, `ask`, `ask_batch`, `check`, `wait`, `reply`, `note`, `portal` (sem `eval`), `new` (só agentes de IA), `connect`, `close`, `floors` (listar, criar) | **Liberadas** no Claude Code e no Devin. O servidor recusa nelas tudo o que rodaria um comando. |
| `keys` (digitar num terminal), `new_shell` (terminal com comando), `portal_eval` (JavaScript na página), `floor_land` (mescla e fecha) | **Pedem aprovação** a cada uso, dentro do terminal do agente. |

Como cada agente recebe o MCP:

- **Claude Code:** `--mcp-config`;
- **Codex:** `mcp_servers`, sem liberação automática;
- **Devin:** `mcp_config.json`, gravado pelo `install-skill`;
- **Gemini:** não recebe MCP; usa a CLI.

---

## 7. Integração com cada agente

| | Claude Code | Codex | Devin | Gemini | Shell | Comando próprio |
|---|---|---|---|---|---|---|
| Fim de turno | hook `Stop` (exato) | `notify` | hook `Stop` | pela tela (5 s sem mudar) | — | pela tela |
| Papel | `--append-system-prompt-file` | `developer_instructions` | hook `SessionStart` | primeira mensagem | nunca | só etiqueta |
| MCP | sim | sim (sem liberação) | sim | não | — | — |
| Retomar conversa | `claude --resume <id>` | `codex resume <id>` | `devin --resume <id>` | `gemini --resume <id>` | — | — |
| "Interrompido" e "Continuar" | sim | não | sim | não | — | — |
| Aviso "não retomou" | sim | não | sim | não | — | — |
| Cartão de turno | mensagem final (hook) | mensagem final | mensagem final | fim da tela | — | — |

- **O agente é digitado num shell:** o Squad abre um shell e digita a linha do agente nele.
- **Ordem dos shells:**
  - Unix: `$SHELL`, zsh, bash, sh;
  - Windows: Git Bash, pwsh, powershell, cmd;
  - `SQUAD_SHELL` passa à frente das duas listas.

---

## 8. Regras e limites

| Regra | Valor |
|---|---|
| Saltos encadeados (laço) | 6; acima disso, **pausa tudo** |
| Pedidos por minuto | 30; o excedente fica retido |
| Espera de um `ask` | 120 s sem atividade (até 30 min com o agente trabalhando) |
| Terminais criados por gerente | 6 (`-max-team`) |
| Portais por agente | 3 |
| Nome de nota e de sessão | 60 caracteres |
| Nome de papel e de andar | 40 caracteres |
| Instruções de papel | 4.000 caracteres |
| Destaque de edição de agente numa nota | ~30 s |
| Faixa "Resposta de…" | 60 s |
| Pílula "✓ id · tempo" no fio | ~4 s |
| Ação de agente no portal (cursor e pílula) | 2,5 s |
| Estado "Terminou" | até 10 min depois do fim do turno |

Outras regras:

- **Conectividade é permissão:** só nós ligados se enxergam (ask, check, wait, note, portal, @menções).
- **Desligar um fio** cancela os pedidos e esperas que passam por ele.
- **Só quem criou fecha:** um agente só fecha os terminais e portais que criou; um gerente só aterrissa os andares que criou.
- **Só a pessoa** descarta andares, remove workspaces e remove sessões.
- **Workspace com andares ou sessões em cópia** não pode ser removido nem mudar de pasta.
- **O último workspace** não pode ser removido.
- **Papel em uso** não pode ser removido.

---

## 9. Onde os dados ficam

**Dentro de `~/.squad`** (ou `SQUAD_HOME`):

- `session-token`;
- `roles.json` (papéis novos e prontos editados);
- `browser/` (perfil do navegador dos portais: logins e cookies);
- `window/` (perfil da janela de app);
- `trash/` (workspaces e sessões removidos);
- `workspaces/<ws>/sessions.json`;
- `workspaces/<ws>/sessions/<sessão>/`, com:
  - `workspace.json` (o canvas);
  - `notes/*.md`;
  - `attachments/` (imagens do Composer, capturas de portal);
  - `cards.json`;
  - os logs de preparo.

**Fora de `~/.squad`:**

- andares: `<pai do repositório>/.squad/floors/<projeto>--<andar>`;
- cópias de sessão: `<pai do repositório>/.squad/sessions/<projeto>--<branch>`;
- skill: `~/.claude/skills/squad`, e a pasta de skills do Devin;
- as conversas, que ficam nos próprios agentes.

---

## 10. Segurança que aparece na interface

- **Rede e acesso:**
  - só loopback (`127.0.0.1`), com conferência de Host e Origin;
  - a sessão do navegador é um cookie HttpOnly, criado a partir de `?token=`;
  - sem token, aparece a página de login.
- **Comandos `squad` no Claude Code** pedem aprovação. O Squad não libera `Bash(squad *)`, porque `squad new --agent shell --command …` rodaria qualquer coisa.
- **Texto automático em shells:** nunca. Shells e comandos próprios não recebem papel nem mensagens automáticas.
- **Notas travadas:** os agentes recebem erro (423) ao escrever.
