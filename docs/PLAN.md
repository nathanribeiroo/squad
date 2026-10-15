# Plano de desenvolvimento do Squad

Referências: `05-proposta-arquitetura-go.md` (arquitetura), `03-spec-agentes-conversam.md` (comportamento da conversa) e `design_handoff_squad/` (interface).

## Regras do fluxo

- Uma branch por etapa, criada a partir da `main` (`feat/<etapa>-<nome>`), com commits pequenos.
- Ao concluir: `go vet`, testes locais, compilação cruzada para Windows (`GOOS=windows go build ./...`), merge (squash) na `main` e `git push` antes da próxima etapa.
- Sem CI no GitHub por enquanto; tudo é testado localmente no macOS. O Windows é coberto por compilação cruzada e por código específico de plataforma (ConPTY, Job Objects); a execução no Windows fica para validação manual.
- O Codex não pode ser testado com conta real; a integração segue a documentação oficial (`notify`) e é testada com agentes falsos.
- Decisões que a documentação não cobre: escolher a opção reversível, registrar em `DEVLOG.md` e seguir.

## Etapas e status

| Etapa | Branch | Status |
|---|---|---|
| B0 Esqueleto | `feat/b0-bootstrap` | concluído |
| B1 Núcleo de terminal | `feat/b1-terminal-core` | em andamento |
| B2 Servidor (HTTP, WebSocket, persistência) | `feat/b2-server` | pendente |
| B3 Conversa entre agentes | `feat/b3-agent-bus` | pendente |
| B4 Adaptadores de agentes (hooks) | `feat/b4-agent-adapters` | pendente |
| B5 Notas | `feat/b5-notes` | pendente |
| W1 Web: shell e tokens | `feat/w1-web-shell` | pendente |
| W2 Web: nó de terminal | `feat/w2-terminal-node` | pendente |
| W3 Web: fios e conversa | `feat/w3-wires` | pendente |
| W4 Web: Composer, notas e portal | `feat/w4-composer` | pendente |

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
- Shell por plataforma: `$SHELL -l` no Unix; `pwsh`, `powershell` ou `cmd` no Windows. A linha de lançamento do agente é digitada no shell.

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

## Depois (fora desta rodada)

Papéis, andares (git worktree + CoW), portal via CDP, Ombro (resumos) e validação no Windows real.
