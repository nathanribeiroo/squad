# Plano de desenvolvimento da Torre (antes Squad)

O Squad passou a se chamar Torre em 2026-10-17 (veja "Torre (F0–F5)", no fim). As seções antigas mantêm os nomes da época (`squad`, `~/.squad`, andar, térreo).

Referências: `05-proposta-arquitetura-go.md` (arquitetura), `03-spec-agentes-conversam.md` (comportamento da conversa) e `design_handoff_squad/` (interface).

## Regras do fluxo

- Uma branch por etapa, criada a partir da `main` (`feat/<etapa>-<nome>`), com commits pequenos.
- Ao concluir: `go vet`, testes locais, compilação cruzada para Windows (`GOOS=windows go build ./...`), merge (squash) na `main` e `git push` antes da próxima etapa.
- Sem CI no GitHub por enquanto; tudo é testado localmente no macOS. O Windows é coberto por compilação cruzada e por código específico de plataforma (ConPTY, Job Objects); a execução no Windows fica para validação manual.
- O Codex não pode ser testado com conta real; a integração segue a documentação oficial (`notify`) e é testada com agentes falsos.
- O Devin CLI também não foi testado com conta real; a integração segue a documentação oficial (hooks `Stop`/`UserPromptSubmit` via `--config`) e é testada com agentes falsos.
- Decisões que a documentação não cobre: escolher a opção reversível, registrar em `DEVLOG.md` e seguir.

## Etapas e status

| Etapa | Branch | Status |
|---|---|---|
| B0 Esqueleto | `feat/b0-bootstrap` | concluído |
| B1 Núcleo de terminal | `feat/b1-terminal-core` | concluído |
| B2 Servidor (HTTP, WebSocket, persistência) | `feat/b2-server` | concluído |
| B3 Conversa entre agentes | `feat/b3-agent-bus` | concluído |
| B4 Adaptadores de agentes (hooks) | `feat/b4-agent-adapters` | concluído |
| B5 Notas | `feat/b5-notes` | concluído |
| W1 Web: shell e tokens | `feat/w1-web-shell` | concluído |
| W2 Web: nó de terminal | `feat/w2-terminal-node` | concluído |
| W3 Web: fios e conversa | `feat/w3-wires` | concluído |
| W4 Web: Composer, notas e portal | `feat/w4-composer` | concluído |
| A1 Adaptador do Devin CLI | `feat/devin-agent` | implementado, falta teste com conta real |
| Pasta e shell por terminal, fonte de ícones | `feat/terminal-folder-shell` | concluído |
| Vários workspaces | `feat/workspaces` | concluído |
| P1 Portal ao vivo (CDP) | `feat/portal-live` | concluído |
| P2 Agentes no portal (`squad portal`) | `feat/portal-agents` | concluído |
| R1 Papéis com instruções | `feat/roles` | concluído |
| R2 Agente gerente e portais criados por agentes | `feat/manager` | concluído |
| R3 MCP (`squad mcp`) | `feat/mcp` | concluído |
| R4 Andares: servidor | `feat/floors` | concluído |
| R5 Andares: interface | `feat/floors-ui` | concluído |
| R6 Resumos de turno | `feat/summaries` | concluído (PR) |
| R7 Portal Android | — | cortado (decisão do usuário) |
| R8 Terminais remotos | — | cortado (decisão do usuário) |
| R9 Playwright opcional | — | cortado (decisão do usuário) |
| Janela de app | `feat/app-window` | concluído |
| Paridade do Devin | `feat/devin-parity` | concluído |
| Nomes de nota | `feat/note-names` | concluído |
| Papel Arquiteto | `feat/role-architect` | concluído |
| Planejador espera o resultado (`squad wait`) | `feat/planner-waits` | concluído |
| S1 Conversas que continuam | `feat/resume-conversations` | concluído |
| S2 Sessões: servidor | `feat/sessions` | concluído |
| S3 Sessões: interface | `feat/sessions-ui` | concluído |
| S4 Sessão numa cópia isolada | `feat/session-copy` | concluído |
| Torre F0a Renomeação | `feat/torre-f0a-renomeacao` → `feat/torre` | concluído |
| Torre F0b Base visual e vocabulário | `feat/torre-f0b-visual` → `feat/torre` | concluído |
| Torre F1 Redesenho do que existe | `feat/torre-f1-redesenho` → `feat/torre` | concluído |
| Torre F2 Confirmações, vazios e erros | `feat/torre-f2-confirmacoes` → `feat/torre` | concluído |
| Torre F3 PRIORIDADE | `feat/torre-f3-prioridade` → `feat/torre` | pendente |
| Torre F4a Configurações | `feat/torre-f4a-configuracoes` → `feat/torre` | pendente |
| Torre F4b Sinóptico | `feat/torre-f4b-sinoptico` → `feat/torre` | pendente |
| Torre F4c Primeira abertura | `feat/torre-f4c-primeira-abertura` → `feat/torre` | pendente |
| Torre F5a Extras (nível, pedidos, NOTAM, pouso) | `feat/torre-f5a-extras` → `feat/torre` | pendente |
| Torre F5b Foco, encaixe, ocultar, Tab | `feat/torre-f5b-foco` → `feat/torre` | pendente |
| Torre F5c @ELEMENTOS no portal | `feat/torre-f5c-elementos` → `feat/torre` | pendente |
| Torre F5d Planos de voo | `feat/torre-f5d-planos` → `feat/torre` | pendente |

## B0, esqueleto

- Módulo Go `github.com/nathanribeiroo/squad`, Go 1.26.
- `third_party/vt`: cópia do `charmbracelet/x/vt` v0.1.0 (MIT), com o patch do buffer de 64 KB, via `replace`.
- `cmd/squad`: o binário único, com os subcomandos `serve`, `version` e `help`. Os comandos de cliente (`list`, `ask`, `check`, `note`, `hook`) entram como esqueleto.
- `internal/config`: diretório de dados (`SQUAD_HOME`, padrão `~/.squad`), porta e arquivo de descoberta do servidor.
- `Makefile` (build, test, vet, build-windows) e o `README.md` raiz com o início rápido.

**Aceite:** `go build ./...` e `GOOS=windows go build ./...` passam; `squad version` imprime a versão.

## B1, núcleo de terminal

- `internal/screen`:
  - interface `Screen` com `Write`, `Snapshot`, `Render`, `Hash`, `BracketedPaste`, `AltScreen`, `Resize`, `Replies` e `Title`;
  - implementação com o `vt`;
  - filtro de OSC (adaptado de `spikes/go/oscfilter`), que gera eventos de título (OSC 0/2), cwd (OSC 7) e progresso (OSC 9;4);
  - hash normalizado: linhas visíveis, ignorando as 3 primeiras quando houver mais de 11, spinners braille trocados por espaço e espaços finais removidos.
- **Golden tests** com `spikes/recordings`: Codex, Claude, vim e htop comparados à referência xterm.js. Com o Claude, só a linha de emoji com seletor de variação pode divergir.
- `internal/pty`:
  - `xpty` (Unix ou ConPTY), início com ambiente e cwd, `WaitProcess`, redimensionamento;
  - parar a árvore inteira: grupo de processos + SIGTERM e depois SIGKILL no Unix, Job Object no Windows.
- `internal/terminal`:
  - `Terminal`: id, nome, tipo de agente, comando, shell e cwd;
  - laço de leitura: filtro OSC → Screen → broadcast; goroutine que devolve as respostas do emulador ao PTY;
  - fila de escrita serializada;
  - `Paste(texto)` com colagem delimitada e as temporizações da spec;
  - `Manager` para criar, parar e reiniciar terminais e listar o estado.
- Shell por plataforma: `$SHELL -l` no Unix; Git Bash, `pwsh`, `powershell` ou `cmd` no Windows (`SQUAD_SHELL` passa à frente; se um não abrir, tenta o próximo). A linha de lançamento do agente é digitada no shell.

**Aceite:** os golden tests passam. Um teste de integração abre um shell, digita `echo` e lê a tela. Outro roda um agente falso que liga o modo 2004 e confere que a colagem chega delimitada, seguida dos CRs.

## B2, servidor

- `internal/store`: um workspace padrão com `nodes[]` (terminal, nota, portal; posição e tamanho) e `edges[]` (não direcionadas, com `ropePoints`). JSON com escrita atômica, no envelope `{type, schemaVersion, payload}`.
- `internal/events`: pub/sub de eventos (`terminal.state`, `node.*`, `edge.*`, `request.*`, `reply.*`, `note.edited`, `server.status`).
- `internal/server` (`net/http` em `127.0.0.1`):
  - **segurança:** token de sessão do navegador (cookie emitido na abertura via URL com token, ou header); verificação de `Host` e `Origin`; tokens por terminal para a CLI;
  - **REST:** `GET /api/state`; `POST/PATCH/DELETE` de nós e arestas; ações de terminal (start, stop, restart, resize);
  - **WebSocket:** `/ws/events` (JSON) e `/ws/term/{id}` (bytes do PTY, binário; entrada do teclado; redimensionamento). Ao conectar, recebe primeiro o snapshot renderizado;
  - arquivo de descoberta `~/.squad/run/server.json` (porta e token) para a CLI;
  - ao subir, abre (opcionalmente) o navegador na URL com token.

**Aceite:** testes HTTP (autenticação, Origin, CRUD) e um teste de WebSocket que liga um terminal e lê a saída.

## B3, conversa entre agentes (feature 1)

- `internal/bus`, seguindo `03-spec-agentes-conversam.md`:
  - **comandos:** `list`, `ask` (bloqueante), `ask --raw`, `ask --batch`, `check` e `reply <id>` (resposta explícita, que resolve o pedido na hora);
  - **resolução:** por nome, exigindo conexão; erros 400/403/404/409/503;
  - **entrega:** envelope `[de: Nome · pedido 7f3a]` + mensagem, por colagem delimitada;
  - **pendentes:** ask back, ciclos por busca em profundidade, timeout de 120 s com prorrogação e um **teto** (30 min), fallback de tela estável ≥ 5 s;
  - **limites:** saltos (máx. 6, com pausa automática), taxa (30 pedidos/min) e **pausar/retomar** (pedidos retidos);
  - eventos `request.sent`, `request.delivered`, `reply.sent`, `reply.received` e `request.cancelled`.
- `internal/activity`: amostragem de 1 s (≥ 500 B + mudança de hash; 3 amostras ativas = trabalhando; > 3 s sem mudança = fim de turno). O título OSC com spinner conta como atividade. Sinais explícitos (hooks) encerram o turno na hora. Daí sai o estado do nó: boot, idle, work, wait, done, err, off.
- `POST /cli` com `X-Terminal-ID` + token; os comandos da CLI no binário `squad`.

**Aceite:** os critérios da seção 14 da spec, com agentes falsos (programa Go de teste): envelope e colagem; liberação por fim de turno; `--raw`; ask back; ciclo A→B→C→A; `--batch`; 403/404; limite de saltos; pausa.

## B4, adaptadores de agentes

- **Claude Code:** a linha de lançamento passa `--settings <arquivo>` com um hook `Stop` que chama `squad hook claude-stop`. O receptor lê o `transcript_path`, extrai a última mensagem do assistente, encerra o turno e responde o pedido pendente com ela.
- **Codex:** `-c notify=[...]` chamando `squad hook codex-notify`. O payload `agent-turn-complete` traz `last-assistant-message`. Não testado com conta real.
- **Devin CLI** (A1, depois do W4): `--config <arquivo>` com hooks no formato do Claude Code (`Stop` → `squad hook devin-stop`, `UserPromptSubmit` → `squad hook devin-prompt`). O `Stop` traz `last_assistant_message` desde a v3000.5.20. Não testado com conta real.
- **Shell genérico e outros:** só a heurística.
- Instruções aos agentes: `squad install-skill` grava uma skill do Squad em `~/.claude/skills/squad/` (comandos, envelope, `reply`).
- `squad mcp`: servidor MCP por stdio, com as ferramentas `list`, `ask`, `check` e `reply`, se der tempo.

**Aceite:** um teste com a CLI do Claude real (um prompt mínimo) e testes unitários dos parsers de hook com payloads de exemplo.

## B5, notas

- Notas como `.md` em `~/.squad/workspaces/<id>/notes/`, com nome estável; watcher (`fsnotify`) gera `note.edited` com o autor quando a escrita vem de um agente.
- CLI: `note list`, `note read` (numerada), `note write`, `note edit` e `note create` (liga automaticamente ao terminal). Acesso só a notas ligadas; bloqueio (`locked`).

**Aceite:** testes de CRUD, de acesso por conexão e de edição externa refletida.

## W1–W4, web (React + Vite + TS, conforme `design_handoff_squad/README.md`)

- **W1:** tokens e fontes; tema; TopBar, Toolbar, minimapa e zoom, ComposerPill, ActivityDrawer. Build embutido no binário (`squad serve` serve a interface).
- **W2:** TerminalNode com xterm WebGL ligado a `/ws/term/{id}`; três níveis de zoom e sete estados; foco de teclado.
- **W3:** WireEdge (curva física, `ropePoints`) com pedido pendente (1d) e animação da conversa dirigida pelos eventos reais; criar e apagar conexões.
- **W4:** Composer com @menções (só nós ligados), NoteNode (prévia/edição) e PortalNode (casca; o driver CDP vem depois).

**Aceite:** comparação visual com a referência HTML via Chrome headless; conversa real entre dois terminais visível no canvas.

## Segunda rodada: funcionalidades (R1–R9)

Ordem combinada em 2026-10-16. Depois de R1 e R2, um Planejador monta a equipe sozinho: cria e liga um Implementador e um Testador, delega com `squad ask`, e o Testador abre um portal e testa a interface.

- **R1, papéis com instruções:**
  - biblioteca de papéis: os quatro atuais (Planejador, Implementador, Revisor, Testador) com instruções padrão editáveis, e papéis novos com nome, cor, ícone e instrução;
  - o agente recebe a instrução ao abrir:
    - Claude Code: `--append-system-prompt-file`;
    - Codex: `-c developer_instructions=…`;
    - Devin: hook `SessionStart` com `additionalContext`;
    - Gemini: como primeira mensagem (`-i`, a partir de uma variável de ambiente);
    - shells e comandos próprios: sem instruções (digitar texto num shell poderia executá-lo); nos comandos próprios o papel fica só como etiqueta;
  - trocar o papel reinicia o agente, com aviso;
  - a instrução padrão do Testador inclui testar interfaces web pelo portal.
- **R2, agente gerente e portais criados por agentes:**
  - `squad new "Nome" --agent claude --role impl` cria, liga e inicia um terminal perto do gerente;
  - `squad connect "A" "B"` e `squad close "Nome"`;
  - só terminais com o modo gerente (ligado por padrão no Planejador), com limite de terminais por gerente;
  - `squad portal create URL "Nome"`, para qualquer agente: o portal nasce ligado a quem o criou, com limite por agente; `squad portal close`;
  - no canvas, o que um agente criou fica marcado ("criado por Planejador");
  - a skill ensina o fluxo planejar → criar a equipe → delegar → testar → revisar.
- **R3, MCP (`squad mcp`):** as mesmas ferramentas da CLI (inclusive gerente e portal), configurado em cada terminal do Claude (`--mcp-config`), do Codex e do Devin. As ferramentas que não podem rodar comandos vêm liberadas (o servidor recusa nelas o que rodaria um); `keys`, `new_shell` e `portal_eval` pedem aprovação. A CLI continua.
- **R4, andares (servidor):**
  - git worktree com branch próprio e canvas próprio por andar; os terminais do andar abrem dentro da cópia;
  - sem cópia instantânea no Windows, o andar recebe os arquivos versionados, uma lista de não versionados para copiar (ex.: `.env`) e um comando de preparo opcional;
  - aterrissar recusa alteração não commitada; sem mesclar (remove a cópia e guarda o branch) ou mesclando no branch base, com a lista de conflitos; descartar apaga tudo;
  - `squad floor create/list/land` para o gerente.
- **R5, andares (interface):** o seletor no topo, criar e alternar, alterações pendentes e o diálogo de aterrissar. Depois, as instruções do Planejador ganharam a regra de um andar por frente paralela.
- **R6, resumos de turno:** cartão ao fim do turno com a mensagem final (dos hooks, ou da tela), de graça; "Resumir" (até 3 próximos passos) e resumo do workspace sob demanda, escritos pela CLI do próprio agente (`claude -p`, `devin -p`, `codex exec`, `gemini -p`). Decidido com o usuário: sem modelo pago fixo.
- **R7, portal Android:** cortado em 2026-10-16; o usuário não precisa dele. Seria o `adb` com os mesmos comandos `squad portal` e a imagem ao vivo.
- **R8, terminais remotos:** cortado em 2026-10-16. Seria um terminal por SSH (ou WSL/Docker) com túnel de volta para o Squad.
- **R9, Playwright opcional:** cortado em 2026-10-16. Seria um segundo motor do portal (Firefox e WebKit).

**A rodada termina no R6:** o usuário cortou o R7, o R8 e o R9 por não precisar deles.

**Depois da rodada:**
- **Janela de app:** o `squad serve` abre o canvas numa janela própria (modo app do Chrome ou do Edge; no Mac só com Safari, o app do Dock), com ícone, título com o workspace e login que sobrevive ao reinício.
- **Papel Arquiteto:** desenha a solução numa nota "Arquitetura: …"; o Planejador o chama em mudanças de estrutura e usa as partes para os andares. `squad roles` e a ferramenta MCP `roles` mostram os papéis aos agentes. O papel de product manager fica para depois.
- **Planejador espera o resultado:** `squad wait "Nome"` (e a ferramenta MCP `wait`) espera o turno de um agente acabar depois de um `ask` que voltou por tempo esgotado; o Planejador não responde mais com andamento.
- **Nomes de nota:** a nota nova abre com o campo de nome; o usuário renomeia as suas notas, e os nomes antigos continuam valendo para os agentes; as notas de agente ficam com o nome que ele deu.
- **Paridade do Devin:** o Devin recebe o MCP (registrado pelo `install-skill`), mantém a configuração do usuário dentro do Squad e não pergunta a confiança nos andares; o `install-skill` instala para todos os agentes encontrados.

## Sessões (S1–S4)

Pedido do usuário: fechar o `squad serve` e voltar exatamente onde estava, e ter várias sessões por workspace, cada uma com o seu canvas, para abrir uma nova e voltar a uma antiga.

- **S1, conversas que continuam:** cada terminal guarda o id da conversa do agente e a retoma ao reabrir o `serve` e em "Reiniciar" (Claude e Gemini com `--session-id`/`--resume`, Devin com `--resume` e o id dos hooks, Codex com `codex resume` e o `thread-id`). Se o agente não acha a conversa, a própria linha de comando começa uma nova. "Nova conversa" no menu do terminal. Agentes parados no meio de um turno voltam como interrompidos, com "Continuar".
- **S2, sessões no servidor:** uma sessão é um canvas completo do workspace (terminais, notas, portais, fios, posições, andares, cartões de turno), numa pasta própria em `~/.squad/workspaces/<id>/sessions/<sessão>/`. Só a sessão atual tem processos rodando. Nova sessão vazia, ou com os mesmos terminais sem as conversas; abrir (volta tudo, com cada agente na sua conversa); renomear; remover. Sem nome, o mesmo agente do orquestrador sugere um título (`claude -p`, `devin -p`…). Aviso ao trocar com agentes trabalhando. Migração dos workspaces que já existem para a primeira sessão.
- **S3, sessões na interface:** a sessão atual no topo, o painel de sessões (abrir, renomear, remover, nova sessão), o aviso com quem está trabalhando antes de trocar, as sessões no seletor de workspaces e o nome no formulário de novo workspace. Junto, uma correção: um agente que ainda não recebeu mensagem começa uma conversa nova em vez de tentar retomar.
- **S4, sessão numa cópia isolada:** "Onde a sessão trabalha": na pasta do projeto ou numa cópia isolada (um andar, num branch novo a partir de um branch escolhido). Vem marcada quando a sessão atual tem agentes trabalhando ou alterações sem commit. "Mesclar" leva o trabalho para o projeto sem fechar a sessão; os andares da sessão nascem da cópia dela. Remover a sessão apaga a cópia e mantém o branch.

**As sessões terminam no S4.**

**Fora da rodada:**
- infraestrutura: CI com Windows, binários no GitHub Releases;
- funcionalidades: Modelos (salvar um arranjo de agentes), chat por terminal, simulador de iPhone.

**Validação no Windows real** continua pendente (ver DEVLOG).

## Torre (F0–F5)

Pedido do usuário em 2026-10-17: trocar o nome para **Torre** e redesenhar a interface inteira, com funcionalidades novas, a partir do handoff em [`docs/torre/`](torre/) (`README.md`, `RENOMEACAO.md`, `NOVIDADES-E-BACKEND.md`, `TELAS.md`, tokens e mockups). O plano por fases foi aprovado com as recomendações da revisão (as diferenças entre o handoff e o código estão no DEVLOG, entrada F0a).

**Como as fases andam:** a branch `feat/torre` sai da `main` e recebe as fases. Cada fase tem a sua branch, a partir da `feat/torre`, e entra nela por merge quando está pronta (o usuário pediu para não esperar a aprovação de cada fase). No fim, um PR leva a `feat/torre` para a `main`, e o usuário faz esse merge.

- **F0a, renomeação:**
  - binário `torre` (`cmd/torre`) e o atalho `squad` (`cmd/squad`), os dois sobre `internal/cli`;
  - `~/.torre` com a migração de `~/.squad`;
  - `TORRE_*` com reserva em `SQUAD_*`; os terminais recebem as duas formas;
  - cookie novo com o antigo aceito;
  - MCP `torre`; skill `torre`, com a remoção da `squad` e a troca do MCP no Devin;
  - `.torre/` para os andares e as cópias novas;
  - `Torre.app` e depois `Squad.app` no Mac;
  - marca na interface: logo, favicon, ícones, manifest, título e chaves do navegador migradas.
- **F0b, base visual e vocabulário:** tokens da Torre, fontes B612 e B612 Mono, os ícones novos e o vocabulário em todos os textos (voo, frequência, nível FLn, pista, pousar, espera geral, Rádio, Transmitir, NOTAM, boletim, briefing, líder, arremetida, ENLACE, estados TAXIANDO…HANGAR), sem mudar o layout.
- **F1, redesenho do que existe:** sigla do papel (`RoleDef.abbr`), indicativo do voo (`callsign`, único por sessão) e número do nível (`Floor.number`, por sessão, nunca reaproveitado), e depois cada tela dos quadros 02–04, 06–17 e 22.
- **F2, confirmações, vazios e erros:** `ConfirmDialog` no lugar das 10 `window.confirm` e dos diálogos de troca e descarte; espaço vazio com primeiros passos; arremetida e espera geral no estilo novo; erros da API com `code`, traduzidos pela interface (os agentes e a CLI continuam em inglês).
- **F3, PRIORIDADE:** o estado `ask` ("o voo pede você"): hooks `PermissionRequest`/`Notification` do Claude e do Devin, detecção pela tela no Codex e no Gemini, resposta pela interface (só pela interface, conferindo a tela), fila, avisos no título, no nível e nos outros workspaces.
- **F4a, configurações:** painel com abas (Aparência com o tema Sistema, Agentes, Limites por workspace, Resumos, Papéis, Atalhos, Avisos); `GET /api/agents` e o comando padrão de cada agente; `GET/PUT /w/<id>/api/limits`.
- **F4b, Sinóptico:** a vista nova (⌥S), com radar, fichas, fila de pouso, frequências, prioridade, terminal e rádio.
- **F4c, primeira abertura:** login redesenhado, pasta (`GET /api/dirs/info`) e agentes.
- **F5a:** renomear nível, cancelar pedido, pedido inteiro no Rádio, NOTAM redimensionável, transmitir a um nível inteiro, conflito checado antes do pouso, indicativo na CLI e no MCP.
- **F5b:** foco ⌥F, encaixe ⌥⇧←/→, ocultar a tela, Tab entre os nós.
- **F5c:** @ELEMENTOS no portal.
- **F5d:** planos de voo (salvar, importar, aplicar com revisão dos comandos).

**Para depois (fora destas fases), com a referência no handoff:**

- **Pilha de NOTAMs** (`docs/torre/NOVIDADES-E-BACKEND.md`, "Depois"; quadro 09 em `TELAS.md`): `stack` nas notas. A frequência ligada à pilha valeria para todas, o que mexe na regra de permissão; precisa de desenho próprio.
- **Desenho e Grupo** (mesmo item; quadro 02): os botões ficam na barra de ferramentas, desligados com "em breve", até os nós `drawing` e `group` terem desenho.
- **Trocar os atalhos** (`TELAS.md`, quadro 18, aba Atalhos: "os de ⌥ podem ser trocados"): a aba só lista os atalhos.
- **Histórico do Rádio guardado** (`NOVIDADES-E-BACKEND.md`, item 12): hoje fica só no navegador (até 200 itens) e some ao recarregar; guardar por sessão.
- **"✓ testes · ✓ revisão" na fila de pouso** (item 6): não existem; poderiam vir de um agente marcando o nível (`torre floor ready --check testes`).
- **Modelo no boletim** ("por claude -p · haiku", quadro 08): o resumo só sabe a CLI que usou, não o modelo.
- **Autor dos commits por indicativo** (quadro 14): no git o autor é a pessoa; seria preciso marcar os commits de cada voo.
- **Cartão "ESPERA GERAL · LAÇO"** (`TELAS.md`, quadro 21): mostraria a cadeia de um laço de pedidos (A → B → A). O barramento já solta a espera de um ciclo (`agentbus/ask.go`), mas não guarda nem publica a cadeia; precisa de um evento novo.
- **Mais erros com `code`** (F2, `internal/apierr`): os da interface que mais aparecem já têm (pasta, nomes, siglas, níveis, sessões, cópia, git, resumos). Os que só os agentes veem (CLI e MCP) continuam em inglês de propósito; os raros da interface que faltam (portal, NOTAM travada, cores) mostram a mensagem em inglês.
- **Trocar a sigla de um papel renomeia os voos dele?** (F1, DEVLOG): hoje o indicativo é dado quando o voo é criado ou troca de papel; mudar a sigla em Papéis vale para os próximos. Renomear os voos abertos mexe no que os outros agentes já conhecem.
- **Fim da compatibilidade com o nome antigo** (`RENOMEACAO.md`, 1.2): tirar o atalho `squad`, a leitura de `SQUAD_*`, o cookie e o cabeçalho antigos e a busca por `Squad.app`, quando o usuário quiser.
- **Renomear o repositório e o módulo Go** para `torre`: muda o caminho do `go install`.
