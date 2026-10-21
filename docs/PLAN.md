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
| Torre F3 PRIORIDADE | `feat/torre-f3-prioridade` → `feat/torre` | concluído |
| Torre F4a Configurações | `feat/torre-f4a-configuracoes` → `feat/torre` | concluído |
| Torre F4b Sinóptico | `feat/torre-f4b-sinoptico` → `feat/torre` | concluído |
| Torre F4c Primeira abertura | `feat/torre-f4c-primeira-abertura` → `feat/torre` | concluído |
| Torre F5a Extras (nível, pedidos, NOTAM, pouso) | `feat/torre-f5a-extras` → `feat/torre` | concluído |
| Torre F5b Foco, encaixe, ocultar, Tab | `feat/torre-f5b-foco` → `feat/torre` | concluído |
| Torre F5c @ELEMENTOS no portal | `feat/torre-f5c-elementos` → `feat/torre` | concluído |
| Torre F5d Planos de voo | `feat/torre-f5d-planos` → `feat/torre` | concluído |
| Pilha de NOTAMs | `feat/pilha-notams` | concluído |
| Responder ao Devin pela Torre, e o som dos avisos | `feat/devin-prioridade` | concluído |
| Histórico do Rádio | `feat/radio-historico` | concluído |
| Permissões nos níveis | `feat/permissoes-niveis` | concluído |
| Fila de pedidos por voo | `feat/fila-pedidos` | concluído |
| Testes e revisão no pouso, e ocupação no `torre list` | `feat/checagens-pouso` | concluído |
| Cartão do laço | `feat/laco` | concluído |
| Perguntas e planos pela Torre, e SUA VEZ | `feat/perguntas-e-sua-vez` | concluído |
| Desenho e Grupo | `feat/desenho-grupo` | concluído |
| Consumo | `feat/consumo` | concluído |
| Consumo em contas de empresa | — | decidido em parte; esperando o relatório do trabalho |
| Ajustes da interface (telas largas, remover, nova sessão) | `feat/ajustes-interface` | concluído |
| NOTAM: cabeçalho com nome longo e tamanho | `feat/notam-cabecalho` | concluído |
| Fios como corrente | `feat/fios-corrente` | concluído |
| Fluidez da interface (e fios com 15% de folga) | `feat/fluidez` | concluído |
| Fios com 8% de folga | `feat/folga-fio` | concluído |
| Fio que não se enrola, e 5% de folga | `feat/fio-sem-enrolar` | concluído |
| Fios com 3% de folga | `feat/folga-3` | concluído |
| Fios do nó selecionado em destaque | `feat/fios-do-selecionado` | concluído |
| Destaque dos fios mais visível | `feat/fios-do-selecionado-forte` | concluído |
| Workspaces inativos fora da tela | `feat/workspaces-inativos` | concluído |
| Novos papéis e o fluxo de especificação | `feat/novos-papeis` | concluído |
| Relatório do consumo (`torre usage-report`) | `feat/relatorio-consumo` | concluído (PR) |

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

**Status:** todas as fases concluídas e mescladas na `feat/torre` em 2026-10-17, e na `main` pelo PR #26. O que ficou de fora está logo abaixo.

**Para depois (fora destas fases), com a referência no handoff:**

- **Consumo em contas de empresa:** dólares e ACUs no lugar das janelas, totais que não somem com o `/clear` e o gateway do Claude. Veja a seção "Consumo em contas de empresa", mais abaixo.
- **Grupo com sentido para os agentes** (depois de Desenho e Grupo, que são só visuais). Mexe nas regras de conversa e merece uma etapa própria. Roteiro para quando vier:
  1. **Decidir a regra:** o grupo só informa (o `torre list` diz "você está no grupo Front-end, com IMP e REV") ou também autoriza (falar com quem está no grupo sem fio)? Recomendo só informar: os fios continuam sendo a única autorização.
  2. **Quem está no grupo:** `store.Inside` (em `internal/store/visual.go`) já diz o que está dentro de uma moldura, aberta ou recolhida. Usar a mesma conta no servidor, sem lista fixa.
  3. **`torre list` e MCP `list`** (`internal/agentbus/cli.go`): uma linha com o grupo e os indicativos de dentro.
  4. **Pedir ao grupo:** `torre ask --group "Front-end" "…"` mandaria um pedido a cada voo do grupo ligado a quem pede (respeitando os fios e os limites de `internal/agentbus`); o MCP ganharia o mesmo parâmetro.
  5. **O líder agrupa:** `torre new --group "Front-end"` poria o voo novo dentro da moldura (o lugar livre teria de procurar dentro dela, em `store.FreeSpot`).
  6. **Textos:** a skill (`skillMD`, em `internal/cli/hook.go`, e `make skill-zip`), o `--help` e as instruções do MCP.
  7. **Testes:** com agentes falsos em `internal/app`, como `TestGroupsAndDrawings` (que hoje confere que os agentes **não** veem o grupo).
- **Trocar os atalhos** (`TELAS.md`, quadro 18, aba Atalhos: "os de ⌥ podem ser trocados"): a aba só lista os atalhos.
- **Modelo no boletim** ("por claude -p · haiku", quadro 08): o resumo só sabe a CLI que usou, não o modelo.
- **Autor dos commits por indicativo** (quadro 14): no git o autor é a pessoa; seria preciso marcar os commits de cada voo.
- **Responder pela Torre no Codex e no Gemini** (F3, `internal/attention`, `verified`; o Devin foi feito em `feat/devin-prioridade`): hoje eles entram em PRIORIDADE pela tela e mostram só "Abrir tela". Para liberar AUTORIZAR/SEMPRE/NEGAR em cada um, conferir com a CLI real que tecla escolhe a opção e se confirma sozinha, guardar as telas como casos de teste e pôr o agente em `verified`.
- **Mais erros com `code`** (F2, `internal/apierr`): os da interface que mais aparecem já têm (pasta, nomes, siglas, níveis, sessões, cópia, git, resumos). Os que só os agentes veem (CLI e MCP) continuam em inglês de propósito; os raros da interface que faltam (portal, NOTAM travada, cores) mostram a mensagem em inglês.
- **Trocar a sigla de um papel renomeia os voos dele?** (F1, DEVLOG): hoje o indicativo é dado quando o voo é criado ou troca de papel; mudar a sigla em Papéis vale para os próximos. Renomear os voos abertos mexe no que os outros agentes já conhecem.
- **Fim da compatibilidade com o nome antigo** (`RENOMEACAO.md`, 1.2): tirar o atalho `squad`, a leitura de `SQUAD_*`, o cookie e o cabeçalho antigos e a busca por `Squad.app`, quando o usuário quiser.
- **Renomear o repositório e o módulo Go** para `torre`: muda o caminho do `go install`.
- **Ocultar a tela guardado no nó** (F5b; `NOVIDADES-E-BACKEND.md`, item 17, "Guardar `redacted` no nó é opcional"): hoje fica no navegador. No nó, valeria em todas as janelas e no Sinóptico de outro computador.
- **Planos de voo pelos agentes** (F5d): hoje só a interface salva e decola planos (decolar inicia processos, então ficou fora do `/cli` e do MCP, como a resposta da PRIORIDADE). Um líder poderia pedir "decole o plano Trio" com aprovação, se fizer falta.
- **Galeria com planos prontos** (F5d; o mockup do quadro 20 mostra "Trio clássico", "Front-end com teste visual", "Duas frentes em paralelo"): a galeria começa vazia. Planos prontos que venham com a Torre precisariam escolher agentes e papéis que existam na máquina.
- **Terminais em WebGL** (item 4 da análise de fluidez, DEVLOG de 2026-10-19): o desenho do xterm em HTML ainda é a maior parte do trabalho com muitos agentes escrevendo. O WebGL foi descartado na W2 porque mede o próprio canvas errado sob a escala do canvas da Torre; o caminho seria uma camada sem `transform`, por cima dos nós, com a posição de cada terminal. Só se as correções de `feat/fluidez` não bastarem.
- **Dois voos encaixados** (F5b): hoje um por vez, numa borda. As duas bordas ao mesmo tempo pediriam um desenho para o canvas espremido no meio.
- **Modelos de NOTAM pela interface** (Novos papéis): o `--template` existe só para os agentes (`torre note create "PRD: X" --template prd` e o parâmetro `template` da ferramenta MCP `note`). Quem cria uma NOTAM pelo canvas começa com ela vazia.
  - **A ideia:** o formulário de criar NOTAM ofereceria os mesmos modelos (PRD, Spec, ADR, Tarefas, Tarefa, Design, Memória) e preencheria o conteúdo com as seções.
  - **Onde mexer:** os modelos ficam em `internal/notes/templates.go`. Faltariam uma rota que os liste e um seletor no criar NOTAM.
- **Retomar a conversa de um voo dispensado** (Novos papéis): hoje o `torre close` remove o nó, e com ele vai o id da conversa (`Terminal.Conversation`).
  - **Como é hoje:** o arquivo da conversa continua no disco do agente (no Claude, em `~/.claude/projects`; no Devin, no `sessions.db`), mas nada aponta para ele. Chamar de volta cria um voo novo, e o contexto vem das NOTAMs: a passagem de serviço na Memória e o que o voo escreveu.
  - **A alternativa:** ao dispensar, guardar o nome, o papel e o id da conversa. Um `torre new --resume "Nome"` subiria o agente continuando a conversa; a Torre já retoma conversas ao reabrir (`internal/agents/conversation.go`).
  - **O custo:** dá mais contexto, mas a conversa inteira volta, o que é mais caro, e ela pode estar desatualizada. Só vale se as NOTAMs não bastarem.
- **Papéis que não escrevem código, de verdade** (Novos papéis, e já citado quando o Arquiteto entrou): Produto, Especificador, Arquiteto, Designer, Revisor e Segurança dizem nas instruções que não editam o código, mas nada impede. O Depurador só corrige se pedirem.
  - **Como obrigar:** uma opção por agente na linha de lançamento (`internal/agents`):
    - no Claude, `--disallowedTools` com as ferramentas de edição. O Bash ainda escreve arquivos, então ali a barreira é parcial;
    - no Codex, `--sandbox read-only`;
    - no Devin e no Gemini, conferir com o binário real o que cada um oferece.
  - **Na interface:** o papel ganharia um campo "somente leitura", com uma chave no editor de papéis.
  - **Cuidado:** esses papéis escrevem NOTAMs pela CLI e pelo MCP, o que tem de continuar liberado. O Depurador escreve testes.
- **Modelo de IA por papel** (Novos papéis): hoje cada voo usa o modelo padrão do agente, ou o comando padrão das Configurações.
  - **A ideia:** um papel escolheria o modelo. Por exemplo, o Arquiteto e o Especificador num modelo mais forte, e o Documentador num mais barato.
  - **Onde mexer:** um campo no papel, por agente, porque o nome do modelo e a opção mudam de um para outro (`--model` no Claude, `-m` no Codex e no Gemini; no Devin, conferir). Ele seria aplicado na linha de lançamento, em `internal/agents`.
  - **Para medir:** o Consumo (`internal/usage`) já mostra o gasto de cada voo.
- **`torre new --connect`** (Novos papéis): hoje, para criar um voo com contexto, o Planejador roda o `torre new` e depois um `torre connect` para cada NOTAM (Memória, Spec, a tarefa).
  - **A ideia:** um `--connect "Nome"` repetível no `torre new`, e um campo `connect` na ferramenta MCP `new`, ligaria tudo na criação, antes do primeiro pedido.
  - **O ganho:** menos chamadas e menos chance de o voo começar sem as NOTAMs.
  - **Onde mexer:** `parseNew` e `create`, em `internal/team/team.go`.

## Pilha de NOTAMs

Pedido do usuário em 2026-10-18, depois da Torre (estava em "Para depois"; quadro 09 em `docs/torre/TELAS.md`). As três decisões foram dele:

1. **Empilhar dá acesso:** um fio ligado à pilha vale para todas as NOTAMs dela. Empilhar pede confirmação, com a lista de quem ganha acesso.
2. **Tirar da pilha mantém os fios:** a NOTAM que sai continua ligada a quem a pilha estava ligada.
3. **O cadeado continua por NOTAM.**

- **Servidor:** `stack` e `stackPos` nas notas. A base guarda os fios e a caixa da pilha, e o acesso segue a pilha como segue as notas encadeadas (`notes.accessible`). Rotas só da interface para empilhar e tirar; `torre list`, `torre note list`, as @menções, os planos de voo e o pouso dos níveis conhecem as pilhas.
- **Interface:** arrastar uma NOTAM sobre outra, a confirmação, a faixa com as abas, o aviso de edição de um agente, "Tirar da pilha" e "Desfazer a pilha".
- **Fora:** agentes empilhando (pela CLI ou pelo MCP), reordenar as abas e trocar a base.

**Status:** mesclado na `main` pelo PR #27.

## Responder ao Devin pela Torre, e o som dos avisos

Pedido do usuário em 2026-10-18 (item "Responder pela Torre no Devin…" do "Para depois"), com ênfase no Devin; o Codex e o Gemini ficam para depois. Junto, o som quando um voo pede você.

- **Devin:** AUTORIZAR, SEMPRE e NEGAR pela Torre, com as teclas conferidas no Devin real (conta gratuita do usuário, modelo SWE-1.6 Slow), a pergunta com opções do Devin entrando em PRIORIDADE, e o texto do agente em cada botão.
- **Som:** ligado por padrão, também para outro workspace, liberado desde o começo na janela de app, e a opção de repetir enquanto o pedido espera.

**Status:** mesclado na `main` pelo PR #28.

## Histórico do Rádio

Pedido do usuário em 2026-10-18 (item 12 de `docs/torre/NOVIDADES-E-BACKEND.md`, que estava em "Para depois"). O plano foi aprovado com os padrões propostos: 500 linhas por sessão, textos até 16 KB, "Limpar o Rádio" com confirmação e as edições de NOTAM agrupadas.

- **Servidor (`internal/radio`):** monta as linhas do Rádio a partir dos eventos do workspace e as guarda em `radio.jsonl`, na pasta da sessão. Guarda também o texto inteiro de cada pedido e da resposta. Rotas `GET/DELETE /api/radio` e o evento `radio.item`.
- **Interface:** o Rádio carrega o histórico do servidor, com "Mais antigos", "Ver a resposta", "Limpar o Rádio" e a data nas linhas de outro dia.
- **Fora:** os agentes lerem o histórico (`torre radio`), busca no Rádio, exportar.

**Status:** mesclado na `main` pelo PR #29.

## Permissões nos níveis

Pedido do usuário em 2026-10-18 (item 18 da lista de pendências): o que ele libera com SEMPRE num nível de voo não pode se perder no pouso. Os agentes guardam essas regras fora do git, num arquivo da pasta do projeto: o Claude em `.claude/settings.local.json`, o Devin em `.devin/config.local.json`. O plano foi aprovado com a caixa de levar as regras desmarcada.

- **Criar um nível (ou a cópia da sessão):** os dois arquivos vêm da pista, como o `.env`, e entram em `copied`.
- **Um Claude ou Devin decola no nível:** ganha as regras que a pista ganhou desde então. Só soma, nunca tira.
- **Pousar (e "Pedir pouso" da sessão em cópia):** o diálogo lista as regras novas do nível e a caixa "Levar essas permissões para a pista" (para a cópia, "para a pasta do projeto"). Ela vem desmarcada, porque um voo também pode escrever esses arquivos. Descartar o nível joga as regras fora; o pouso pela CLI e pelo MCP nunca as leva.
- **Esses arquivos nunca são alteração:** não bloqueiam o pouso. Um branch que os commita continua recusado, como o `.env`.
- **SEMPRE do Claude:** algumas telas do Claude escrevem "don’t ask again" com apóstrofo tipográfico, e o SEMPRE não aparecia nelas. Corrigido.
- **Achado no Claude real (2.1.302):** num worktree, ele lê os dois arquivos (o da pasta principal e o do worktree) e grava o SEMPRE no da pasta principal. Então o SEMPRE do Claude num nível já vale na pista na hora, e o pouso não tem o que levar dele. A cópia e a soma ficam para versões que gravem no worktree e para regras escritas à mão.
- **Fora:** o Codex e o Gemini; levar para os níveis as regras apagadas na pista.

**Status:** mesclado na `main` pelo PR #30.

## Fila de pedidos por voo

Pedido do usuário em 2026-10-18 (item 19 da lista de pendências, que apareceu nos testes do item 18): uma pergunta entregue a um voo no meio do turno ficava com o fim desse turno, e quem perguntou recebia a resposta de outro pedido. Confirmado com o Claude real. O plano foi aprovado com os padrões propostos: as mensagens do usuário fora da fila, folga de 1,5 s e nenhuma nota na resposta.

- **A fila:** um pedido para um agente só entra quando o voo está livre: sem trabalhar, sem pedir o usuário, parado há 1,5 s e sem outro pedido entregue e sem resposta. Senão, espera na fila do voo, por ordem de chegada. A espera conta para o limite de 30 min.
- **Fora da fila:** shells e comandos próprios, a pergunta de volta, a mensagem para quem está em `torre wait` esperando quem mandou e as mensagens do usuário.
- **Laços:** um voo ocupado que pergunta (ou espera) e tem na fila um pedido da mesma cadeia recebe esse pedido na hora, como resultado do comando. Pedido na fila não conta como pergunta de volta nem para o limite de saltos.
- **Interface:** o fio mostra "NA FILA"; as faixas dos voos e o Rádio dizem quem espera quem.
- **Fora:** um botão para furar a fila; mostrar a fila no `torre list`.

**Status:** mesclado na `main` pelo PR #31.

## Testes e revisão no pouso, e ocupação no `torre list`

Pedido do usuário em 2026-10-18: o item 12 da lista de pendências ("✓ testes · ✓ revisão" na fila de pouso, que estava em "Para depois") e, junto, mostrar no `torre list` quem está ocupado e a fila de cada voo. O plano foi aprovado com os padrões propostos: a marca não bloqueia o pouso; marcam os voos do nível e o líder que o criou; só testes e revisão; a interface só mostra; a cópia da sessão fica de fora.

- **Marcar:** `torre floor mark tests|review pass|fail "nota"` (ou a ferramenta `floors` do MCP, com `action: mark`). Fica guardado no nível o resultado, a nota, quem marcou, quando e o commit do nível. Com um commit novo depois, a marca fica desatualizada.
- **Onde aparece:** a fila de pouso do Sinóptico e o "Autorizar pouso" (✓, ✕, ⟳ ou ○), uma linha do Rádio a cada marca, `torre floor list` e a resposta do `torre floor land`.
- **Papéis padrão:** o Testador e o Revisor marcam; o Planejador confere antes de pousar e olha o `torre list` antes de delegar.
- **`torre list`:** em cada voo, o pedido que atende, a fila e por quem espera; para quem roda, quantos esperam o turno dele; a legenda dos estados. Um pedido na fila não mostra o número, e o `torre reply` recusa responder a um pedido que ainda não chegou.
- **Fora:** marcar pela interface; "arquitetura" como terceira marca; a cópia da sessão; bloquear o pouso com marca reprovada.

**Status:** mesclado na `main` pelo PR #32.

## Cartão do laço

Pedido do usuário em 2026-10-18 (item 10 da lista de pendências, que estava em "Para depois"; quadro 21 de `docs/torre/TELAS.md`). O "A → B → A" citado no "Para depois" já se resolvia sozinho: a pergunta de volta responde a quem perguntou, e um ciclo maior é solto. Laço, aqui, é a cadeia de pedidos que passa do limite de saltos e põe tudo em espera geral. O plano foi aprovado com os padrões propostos: o cartão só nos laços, liberar não muda o limite e a cadeia aparece pelos indicativos (o nome, ao passar o mouse).

- **A cadeia:** quando um pedido passa do limite, o servidor volta pelos pedidos que cada voo está atendendo até quem começou.
- **O estado:** o evento da espera leva a cadeia, os saltos e o limite. O estado guarda o motivo da espera (você ou um laço), então recarregar a página mantém o cartão. Liberar apaga.
- **Quem perguntou** recebe a cadeia na recusa, com ele mesmo como "you".
- **Rádio:** a linha do laço traz a cadeia inteira.
- **Interface:**
  - o cartão "ESPERA GERAL · LAÇO" no alto do Espaço e no Sinóptico, com a cadeia (cada indicativo leva ao voo), LIBERAR TODOS ⌥P e LIMITES…;
  - os fios da cadeia em âmbar cheio com "∞ laço · 6 saltos · espera geral";
  - no Sinóptico, o radar e as frequências marcam a cadeia;
  - a espera geral ligada pelo botão não mostra cartão.
- **Fora:** perceber o laço antes do limite (o mesmo voo duas vezes na cadeia) e baixar o limite sozinho.

**Status:** mesclado na `main` pelo PR #33.

## Perguntas e planos pela Torre, e SUA VEZ

Pedido do usuário em 2026-10-18: dois itens do "Para depois" juntos, "Aprovar o plano e responder às perguntas com opções do Devin pela Torre" e o aviso do voo ocioso (o hook `Notification` do Claude), para todos os agentes. O plano foi aprovado com os padrões propostos:
- no plano, todas as opções do agente viram botão, menos as que desligam todas as permissões;
- o texto livre entra, porque o levantamento mostrou como digitar com segurança;
- o aviso de SUA VEZ vem ligado, depois de 1 min, só com a janela atrás.

- **Levantamento real:**
  - **Claude Code 2.1.302:** pergunta, várias perguntas numa chamada, múltipla escolha, texto livre e plano, com as telas, os hooks e as teclas;
  - **Devin 3000.11.92:** pergunta, texto livre e plano; logado pelo usuário, com o SWE-1.6 Slow.
- **Perguntas e planos:** as opções do agente viram botões (teclas 1–9), com um campo para a resposta escrita e "Ver o plano". Ficam na tela: a múltipla escolha, as opções que desligam todas as permissões e a confiança na pasta.
- **SUA VEZ:** o servidor marca o fim de um turno seu; o voo e a ficha mostram SUA VEZ, e Configurações → Avisos avisa com a janela atrás. Vale para todos os agentes, sem o hook `Notification` do Claude.
- **Fora:**
  - o Codex e o Gemini, porque as CLIs não estão instaladas aqui;
  - a múltipla escolha;
  - o aviso de SUA VEZ dos voos de outro workspace (vale o workspace aberto).

**Status:** mesclado na `main` pelo PR #34.

## Desenho e Grupo

Pedido do usuário em 2026-10-18 (estava em "Para depois"; quadro 02 de `docs/torre/TELAS.md` e o brief em `docs/design_handoff_squad/`). Os dois são só visuais, para o usuário organizar e explicar o canvas: os agentes não veem nada. O grupo com sentido para os agentes foi para o "Para depois", como o usuário pediu. O plano foi aprovado com os padrões propostos:
- quem está no grupo é o que está inteiro dentro da moldura na hora, sem lista fixa;
- grupo dentro de grupo só anda junto;
- pousar ou descartar um nível remove as molduras e os desenhos dele.

- **Servidor (`internal/store/visual.go`):**
  - dois tipos novos de nó: `group` (nome, cor, recolhido, o tamanho aberto e quem ele esconde) e `drawing` (forma, pontos de 0 a 1 na caixa, cor, texto);
  - o fio para eles é recusado (`edge.visual`), o lugar livre de um voo novo ignora molduras abertas e desenhos, e `torre connect`/`close` não os acham pelo nome;
  - `POST /api/nodes/move` move vários nós de uma vez (o grupo com o que está dentro);
  - planos de voo salvam e decolam grupos e desenhos; um grupo selecionado leva o que está dentro dele.
- **Interface:**
  - **Grupo (G):** moldura em volta da seleção, ou arrastada no canvas; renomear, cor, recolher num cartão com os estados de dentro (âmbar quando alguém pede você), expandir, remover a moldura; arrastar leva o que está dentro;
  - **recolhido:** os voos continuam no ar, os fios vão para o cartão com as pílulas de pedido, a PRIORIDADE diz "no grupo …" e IR ATÉ ELE abre o grupo;
  - **Desenho (D):** caneta, seta, retângulo, elipse e texto, nas cores do tema; Delete apaga, duplo clique edita o texto;
  - molduras e desenhos ficam atrás dos voos e só pegam o clique nas próprias linhas.
- **Fora:** borracha parcial, camadas, desenho no Sinóptico e o grupo com sentido para os agentes.

**Status:** mesclado na `main` pelo PR #35.

## Consumo

Pedido do usuário em 2026-10-18: ver quanto os agentes estão gastando, sem ocupar espaço fixo na tela. Para o Claude: tokens de entrada e de saída, o cache, a sessão de 5 h e a semana. Para o Devin: os tokens, o cache e os ACUs; a conta pessoal mostra os limites diário e semanal, e a de empresa só os ACUs. Com Claude e Devin no mesmo workspace, os dois aparecem. Tem de funcionar no Mac, no Windows e no Linux. O plano foi aprovado com os padrões propostos:
- a cota do Devin é lida da tela quando o usuário abre `/usage` no voo (ela só existe no servidor do Devin);
- o Codex e o Gemini entram agora, testados com arquivos de exemplo (as CLIs não estão instaladas aqui);
- nada vai para a rede e nenhuma credencial é lida (ficaram de fora a rota de uso da conta do Claude e a API do Devin).

- **Servidor (`internal/usage`):**
  - **Claude:** a status line nas configurações que a Torre passa ao Claude (`torre hook claude-statusline`) traz o contexto, o custo estimado e as janelas de 5 h e de 7 dias; os tokens vêm da transcrição (cada resposta contada uma vez). A status line do usuário continua: a Torre roda a dele e imprime o que ela imprimir;
  - **Devin:** o `sessions.db` do Devin, só para leitura, com um SQLite em Go puro (sem compilador C no Windows); o tamanho do contexto vem do rodapé da tela, e a cota do `/usage`;
  - **Codex** e **Gemini:** os arquivos de conversa deles;
  - as contas valem para todos os workspaces e ficam em `~/.torre/usage-accounts.json`; `GET /api/usage`, eventos `usage.flight` e `usage.account`.
- **Interface:** o botão CONSUMO no topo, entre RÁDIO e ENLACE, com a janela mais cheia; a caixa com as contas de cada agente e uma linha por voo.
- **Fora:**
  - a conta de empresa do Devin não foi vista (o formato dos ACUs no `/usage` veio do texto do binário);
  - o Codex e o Gemini reais;
  - o custo em dinheiro do Devin, do Codex e do Gemini (eles não informam);
  - histórico do consumo por dia.

**Status:** mesclado na `main` pelo PR #36.

## Consumo em contas de empresa

Pergunta do usuário em 2026-10-19, depois do merge do Consumo. Nas contas do trabalho, nem o Claude nem o Devin mostram a sessão de 5 h ou a semana: o Devin mostra só ACUs, e no Claude o gasto só aparece pelo valor em dólar. Nesse caso, a ideia é mostrar o valor gasto (dólar no Claude, ACU no Devin, ou dólar se ele mostrar), somado do que cada resposta devolve durante a sessão, e pôr o total de dólares e de ACUs na seção de cada agente, no topo da caixa do Consumo (e no botão). O usuário vai conferir como o Devin da empresa se comporta antes de decidirmos o plano.

- **Como está hoje (PR #36):**
  - **Claude:** cada voo mostra o custo em US$ (o `cost.total_cost_usd` da status line, ou a linha `cost-state` da transcrição), e a linha TOTAL da tabela soma o workspace;
  - **Devin:** cada voo mostra os ACUs da sessão (`sessions.metadata.total_acu_cost` no `sessions.db`), mas só vimos esse campo em 0 (conta gratuita, SWE-1.6 Slow);
  - **sem limites:** a seção do agente no topo da caixa só diz "A conta não informa limites (chave de API ou plano de empresa)", e o botão mostra os tokens do workspace;
  - **o custo vale para a conversa atual do voo:** um `/clear` no Claude zera o valor (é outra sessão para ele), e fechar o voo tira o número da caixa. Não há total de hoje ou do mês.
- **O que a pesquisa achou:**
  - **Claude** (documentação da status line):
    - `rate_limits` só vem para os assinantes Pro e Max do claude.ai. Team, Enterprise, chave de API, Bedrock e Vertex não recebem as janelas;
    - `cost.total_cost_usd` vem sempre. É uma estimativa pelo preço de tabela, ou pela tabela `modelPricing`, se a empresa configurar uma, e volta a 0 num `/clear`;
    - atrás de um gateway da empresa com limite de gasto, vem `rate_limits.spend_limit`, com `used_percentage` e `resets_at`, e, a partir do Claude Code 2.1.284, `used_usd`, `limit_usd` e `period` (`daily`, `weekly` ou `monthly`). A Torre ainda ignora esse campo.
  - **Devin** (texto do binário 3000.11.92, sem login):
    - **por resposta:** a mensagem do modelo tem `committed_acu_cost` e `committed_credit_cost` no `metadata`. Na conta gratuita os dois não apareceram (devem ser gravados só quando há custo). Dá para somar resposta a resposta e conferir com o total da sessão;
    - **status do plano:** o Devin pede ao servidor (`GetUserStatus`) um status do plano com `acu_consumed`, `acu_limit`, `dollars_limit`, `overage_balance_micros`, `daily_quota_remaining_percent`, `weekly_quota_remaining_percent`, as horas de renovação e `hide_weekly_quota`;
    - **cópia local:** ele parece guardar uma cópia desse status num arquivo, `user_status.bin`. Se der para ler, a Torre teria os ACUs da conta sem o usuário abrir `/usage`. Ressalvas: o formato é binário e não documentado, e o arquivo tem também dados da conta (e-mail, time);
    - **na tela do `/usage`:** o binário tem os textos " ACUs consumed", "ACU limit ", "Extra usage balance", " of $" e "No ACUs consumed yet in this session.", o que sugere que a conta de empresa mostra ACUs e, às vezes, dólares.
- **O que falta fazer:**
  1. **Seção do agente e botão:** mostrar dólares e ACUs quando a conta não tem janelas, no lugar do aviso de "não informa limites" e dos tokens no botão.
  2. **Totais que não somem:** guardar o custo de cada conversa (por agente e id da conversa, o último valor visto), para o total sobreviver ao `/clear`, ao voo fechado e ao reinício, e somar por período em todos os workspaces (por exemplo, `~/.torre/usage-ledger.json`).
  3. **Claude atrás de gateway:** ler o `spend_limit` e mostrar "US$ 314 de US$ 500 · mês" como uma janela.
  4. **Devin de empresa:**
     - somar o `committed_acu_cost` de cada resposta (conferindo com o `total_acu_cost` da sessão);
     - ler os ACUs e os dólares do `/usage` com o formato real;
     - e, se valer o risco, o `user_status.bin`.
  5. **Testes:** arquivos de exemplo com as telas e os dados reais que o usuário trouxer; o Devin real só com a conta dele.
- **Para decidir antes do plano:**
  - **o período do total:** "hoje" e "este mês" (somando todas as conversas e todos os workspaces), ou só os voos abertos agora. A recomendação é hoje e mês;
  - **como o Claude do trabalho entra:** login do claude.ai (Team/Enterprise), chave de API, Bedrock/Vertex ou gateway da empresa. O `/status` dentro do Claude mostra isso;
  - **a tela do `/usage` do Devin da empresa:** o usuário vai trazer o texto, sem o e-mail, para confirmar o formato dos ACUs e dos dólares.

### O que o usuário trouxe (2026-10-20)

- **Claude do trabalho:** Claude Code 2.1.236 pelo Amazon Bedrock, com um endereço próprio da empresa, região us-east-1, proxy e certificado da empresa. Sem janelas e sem `spend_limit` (versão antiga e Bedrock). O limite é de US$ 180 por mês, controlado pela empresa.
- **Devin Enterprise:**
  - o `/status` diz "Cognition Platform (Enterprise)", "Devin Enterprise";
  - o `/usage` mostra "Billing cycle 599.41 ACUs consumed" e, de uma sessão, "ACUs 38.4671", os turnos, as ferramentas, os tokens e o modelo. Numa sessão nova: "No ACUs consumed yet in this session.". Nenhum limite aparece;
  - os ACUs do ciclo são só do usuário (não da equipe).
- **A caixa do Consumo hoje, com o Devin Enterprise:** os ACUs de cada voo e o contexto já funcionam (0,07 ACU, 2%). A seção do Devin fica em "A cota aparece quando você abre /usage num voo do Devin" mesmo depois do `/usage`: o leitor procura "Fetching account usage", que a tela da empresa não tem. O botão mostra os tokens (25,5K).

### O que ficou decidido

- **O botão:**
  - numa conta de empresa, o gasto **do workspace aberto**: ACUs no Devin, US$ no Claude. O geral (o mês contra o limite, os agentes e os voos) fica na caixa;
  - no Claude pessoal (Pro e Max), a porcentagem da **janela de 5 h** (hoje é a mais cheia, por isso aparecia a semanal);
  - uma opção nas Configurações para escolher o que o botão mostra: o gasto do workspace, a janela de 5 h, a semanal ou a mais cheia;
  - com Claude e Devin juntos no workspace, os dois lado a lado ("0,8 ACU · US$ 3,40"), a confirmar no plano.
- **A seção de cada agente, numa conta de empresa:** o consumo do mês contra o limite, com a barra e a porcentagem. Os voos continuam como hoje.
- **O limite:** configurado nas Configurações, uma vez para todos os workspaces (a conta é a mesma em todos):
  - **Devin:** em ACUs por mês, ou em US$ por mês com o preço de um ACU (a Torre converte);
  - **Claude:** em US$ por mês.
- **O ciclo** renova todo dia 1.
- **Qual vale:** o limite configurado vale quando existe. Sem ele, as janelas do plano, quando a conta tem.
- **A Torre nunca digita `/usage` num voo** (a regra de nunca digitar num terminal sozinha). O `/usage` que o usuário abrir continua sendo lido.

### De onde vêm os números (sem `/usage`)

- **Devin, o cache da conta:**
  - **onde:** o Devin 3000.11.92 guarda a resposta do `GetUserStatus` em `~/.cache/devin/cli/user_status.<digest>.bin` (no Windows, provavelmente em `%LOCALAPPDATA%\devin`). Há um arquivo por identidade (`identity_digest`); vale o buscado por último;
  - **o formato:** JSON com `version`, `identity_digest`, `fetched_at_secs` e `payload`, o protobuf da resposta em base64;
  - **mapeado com a conta gratuita do usuário** (o login de 2026-10-18 no container), conferido com o `/usage` daquele dia ("Daily 4% used", "Weekly 2% used · resets Oct 25, 8:00 AM (UTC)"). O campo 13 é o status do plano: `13.14` é o quanto resta da cota diária (96), `13.15` da semanal (98), e `13.17` e `13.18` são as renovações (Unix). Isso já daria as janelas do Devin pessoal sem `/usage`;
  - **a conferir:** pela ordem dos nomes no binário (`overage_balance_micros`, as renovações, `acu_consumed`, `acu_limit`, `dollars_limit`), os ACUs devem vir logo depois, no mesmo campo 13. Falta ver o arquivo de uma conta de empresa, e também quando o Devin atualiza o arquivo (ao abrir, em segundo plano, pelo texto do binário);
  - **cuidados:** o formato não é documentado e pode mudar numa versão nova. Se a leitura falhar, vale o próximo caminho. O arquivo tem o e-mail e a equipe, e a Torre só lê os números.
- **Devin, o `sessions.db`:** cada resposta deve guardar o próprio custo (`committed_acu_cost`) e a hora. Somar as respostas do mês dá o que este computador gastou, dentro e fora da Torre. Não vê outros computadores nem o Devin na nuvem. Serve de reserva, ou para somar ao último número lido do cache ou do `/usage`.
- **Claude, o custo do mês:**
  - os voos da Torre: a status line, guardada por conversa num registro, para o total sobreviver ao `/clear`, ao voo fechado e ao reinício;
  - fora da Torre, neste computador: as linhas `cost-state` das conversas (`~/.claude/projects`). Vistas nas versões 2.1.300 a 2.1.304; falta saber se a 2.1.236 grava;
  - é a estimativa do Claude a preço de tabela, e a conta do Bedrock pode diferir (desconto da empresa, preço por região);
  - quando o Claude do trabalho passar da 2.1.284 e o gateway informar o `spend_limit`, ele ganha (é o número da empresa).

### O relatório (`torre usage-report`)

Feito para mapear o que falta sem o usuário mandar arquivos com dados pessoais. Ele mostra só números: os campos do cache da conta do Devin (um por linha, o texto só pelo tamanho, as datas por extenso), as somas do `sessions.db`, as conversas do Claude com `cost-state`, as versões e o provedor (Bedrock, Vertex ou gateway, sem os endereços).

**O que falta do usuário:** no trabalho, abrir `/usage` num Devin, rodar `torre usage-report > relatorio.txt` logo depois e mandar o arquivo com a captura do `/usage`. Com o relatório, o plano detalhado:
1. ler o cache da conta do Devin (as janelas, e os ACUs na empresa) e consertar a leitura do `/usage` da empresa;
2. o limite e o ciclo nas Configurações, e a seção de cada agente com o mês contra o limite;
3. o registro do custo do Claude por conversa e o total do mês (status line e `cost-state`);
4. o botão, com a opção nas Configurações;
5. o `spend_limit` do Claude atrás de um gateway.

**Status:** decidido em parte; esperando o relatório do trabalho.

## Workspaces inativos fora da tela

Pedido do usuário em 2026-10-20: só o workspace que está na tela deveria ter os agentes carregados. Antes, todos os workspaces subiam os terminais quando a Torre abria e continuavam rodando, mesmo sem ninguém olhando. Os cenários foram combinados antes do plano:
- **ao abrir a Torre:** só o workspace da janela sobe os agentes; os outros carregam a estrutura, com os agentes parados;
- **ao trocar de workspace:** o novo sobe os agentes com um "carregando" por cima do canvas, no estilo do "Sem enlace", e o que saiu inativa depois de um tempo configurável (5 min por padrão);
- **trabalhando, não inativa:** um voo no meio de um turno, pedindo você ou esperando outro, um pedido na fila e um nível em preparo seguram o workspace. O tempo conta a partir de quando tudo termina;
- **shells e comandos próprios** continuam vivos: nada traria de volta o que eles tinham;
- **interrompidos** (a Torre fechou com um voo trabalhando) aparecem no seletor, sem subir nada;
- **janela fechada** com o `torre serve` rodando conta como ter saído; duas janelas em workspaces diferentes mantêm os dois;
- **configuração:** "Manter todos os workspaces ativos" (ligar não sobe ninguém na hora; desligar começa a contar), o tempo, e por workspace "Manter sempre ativo" e "Inativar agora";
- **sessões** continuam como antes.

Medido antes do plano, no container (4 núcleos): subir 8 Claudes de uma vez não é mais rápido e trava a CPU; em levas do tamanho dos núcleos, o tempo é parecido e sobra CPU para a janela.

- **Servidor:**
  - **quem está na tela:** o hub conta os sockets de eventos de cada workspace (`/w/<id>/ws/events`); o primeiro acorda o workspace, o último começa a contar (`internal/hub/sleep.go`);
  - **a regra:** checada a cada 5 s; inativa um workspace fora da tela, sem nada acontecendo (`app.Work`), há o tempo das Configurações;
  - **inativar** (`app.Sleep`): para só os agentes, salva a última tela (`screens/<id>.ans`, na pasta da sessão) e fecha as abas dos portais. O terminal fica, parado, mostrando a tela;
  - **acordar:** os shells que nunca rodaram sobem juntos; os agentes entram numa fila (`internal/hub/launch.go`) com uma vaga por núcleo menos um (de 2 a 6). Um agente libera a vaga quando desenhou a tela e ela ficou parada 0,5 s, quando sai ou depois de 15 s. A janela manda a ordem (foco, encaixe, os visíveis); sair no meio deixa os que faltam esperando a volta;
  - **ao abrir a Torre:** todos os workspaces abrem com os terminais parados mostrando a última tela, e só sobem os que uma janela mostra ou que ficam sempre ativos;
  - **configurações** em `~/.torre/settings.json`; o "sempre ativo" fica no workspace (`keepActive`, em todas as sessões).
- **Encerramento:** parar um terminal agora também encerra o job em primeiro plano (o agente, num grupo de processos próprio), que antes às vezes ficava órfão e parado.
- **Interface:** o cartão "Carregando o workspace · 3 de 8 voos de volta"; no seletor, "inativo", "carregando 3/8", "1 interrompido", o alfinete e a lua (inativar agora, com confirmação quando há voos no ar); a aba Configurações → Workspaces.
- **Fora:**
  - decolar um plano de voo grande e o `torre new` continuam subindo cada agente na hora, fora da fila;
  - as sessões continuam como antes (trocar de sessão para os agentes da que sai).

**Status:** concluído, mesclado na `main` pelo PR #47.

## Novos papéis e o fluxo de especificação

Pedido do usuário em 2026-10-20. Ele escolheu os papéis Produto, Depurador, Segurança, Documentador, Designer e Front-end, e pediu um papel para o desenvolvimento guiado por especificação (PRD, spec técnica, ADRs, tarefas), com a skill compozy como referência. O que dá para aproveitar dela foi analisado antes do plano:
- as perguntas uma por vez, com opções;
- as seções do PRD e da spec;
- as regras das tarefas;
- "sem evidência, não terminou";
- os critérios de revisão;
- a memória compartilhada.

Ficou de fora o que é próprio do programa deles: a pasta `.compozy/`, os campos YAML e os validadores. As decisões do usuário:
- **o Designer** cuida de UX e UI, antes (uma NOTAM "Design: …") e depois (a revisão pelo portal);
- **o Front-end** é o especialista que constrói a interface com qualidade;
- **a divisão** dos papéis de especificação ficou comigo, e ele aprovou;
- **o Planejador** pode dispensar quem terminou e chamar de volta com contexto, sempre com no máximo 6 voos ao mesmo tempo.

- **Divisão:**
  - **Produto:** o quê e o porquê; escreve o PRD e faz a aceitação.
  - **Especificador:** como fazer esta entrega; escreve a spec, as ADRs da entrega e a pilha de tarefas.
  - **Arquiteto:** o projeto inteiro; mapa do sistema, ADRs que valem para tudo, revisão da spec e do diff.
- **Papéis:** 7 novos (Produto, Especificador, Designer, Front-end, Depurador, Segurança, Documentador). Os textos do Planejador, do Arquiteto, do Implementador, do Revisor e do Testador mudam.
- **"Quando chamar":** cada papel ganha essa linha. O `torre roles` a mostra, e é por ela que o Planejador escolhe quem chamar. O texto dele fica só com as regras gerais e cabe nos 4.000 caracteres.
- **Modelos de NOTAM:** `torre note create "PRD: X" --template prd`, com os modelos `prd`, `spec`, `adr`, `tasks`, `task`, `design` e `memory`.
- **Pilha criada por agente:** `--stack "Tarefas: X"` põe a NOTAM nova em cima da pilha de uma NOTAM ligada a quem cria.
- **Dispensar:** o `torre close` passa ao Planejador os fios das NOTAMs que o voo dispensado criou. Antes, elas ficavam soltas.
- **Marcas no nível:** `security` e `design`, ao lado de testes e revisão. Aparecem só quando alguém marca.
- **Interface:**
  - 4 cores novas (céu, ardósia, areia, ouro) e 3 ícones (`flag`, `checklist`, `web`);
  - o campo "Quando chamar" no editor de papéis;
  - as marcas novas na fila de pouso, no Sinóptico e no Rádio.
- **Fora:** os itens "Modelos de NOTAM pela interface", "Retomar a conversa de um voo dispensado", "Papéis que não escrevem código, de verdade", "Modelo de IA por papel" e "`torre new --connect`", no "Para depois" da seção Torre, com o contexto de cada um.
- **O teste real com o Claude mudou três coisas** (DEVLOG):
  - o Planejador não pede ao Produto que decida sem perguntar;
  - uma pergunta longa, que o Claude mostra num quadro, vira PRIORIDADE;
  - o cabeçalho dos papéis lembra das ferramentas MCP.

  Junto, a prévia das NOTAMs passou a desenhar tabelas.

**Status:** PR aberto, esperando o merge.
