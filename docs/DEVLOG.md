# Diário de desenvolvimento

O que foi feito em cada etapa, as decisões tomadas sem consulta e o que precisa de você. Entradas mais novas no topo.

## 2026-10-15, B3 conversa entre agentes

- **`internal/activity`, o monitor:**
  - amostra periódica (1 s): uma amostra é ativa com ≥500 B de saída **e** mudança no hash da tela, ou com título de spinner;
  - 3 amostras ativas seguidas = "trabalhando"; mais de 3 s sem mudança = fim de turno;
  - sinais explícitos (hooks) encerram o turno na hora;
  - estados da interface: boot, idle, work, wait, done, err, off.
- **`internal/agentbus`, a conversa:**
  - `list`, `ask` (bloqueante), `ask --raw`, `ask --batch`, `check`, `reply <id>` e `help`, todos via `POST /cli`;
  - envelope `[de: Nome · pedido id]` (sem envelope para shells);
  - pendentes com fim de turno, fallback de tela estável (5 s), timeout de 120 s prorrogável e teto de 30 min;
  - cancelamento quando o cliente desiste ou a conexão ou o nó somem;
  - limite de saltos (6, com pausa automática), limite de taxa (30/min, pedidos retidos) e pausar/retomar;
  - eventos `request.*` e `reply.*`.
- **`internal/app`:** monta servidor, monitor e barramento; rotas `POST /api/conversations/pause|resume`; `terminal.state` passa a levar `activity`.
- **CLI `squad`:** cliente fino (`SQUAD_URL`, `SQUAD_TERMINAL_ID`, `SQUAD_TOKEN`); mensagem pelo stdin (heredoc); sem timeout no `ask` bloqueante.
- **Testes de integração com agentes falsos conversando pela CLI real:**
  - resposta no fim do turno e pelo fallback;
  - erros 400/403/404/503;
  - resposta direta, `reply` por id e ciclo A→B→C→A liberado;
  - `--raw`, `check` e `--batch`;
  - limite de saltos, pausa e cancelamento por remoção de conexão.

  Passam com `-race`, repetidos.

**Decisões (melhorias sobre o Maestri):**
- **Resposta direta:** se B manda `ask A` enquanto A espera por B, a mensagem **é a resposta** do pedido de A. Ela chega completa ao comando bloqueado de A, e B recebe "OK" na hora. O Maestri colava como prompt novo em A e deixava B esperando o turno de A.
- **`squad reply <id>`:** responde explicitamente pelo id do envelope.
- **Turno com hook:** quando o fim de turno vem com a mensagem final (hooks, na B4), a resposta é essa mensagem, e não a tela.
- **Fallback desligável:** o fallback de tela estável pode ser desligado por tipo de agente (os que têm hooks), para um agente que pensa em silêncio não liberar a espera antes da hora.
- **Teto na espera:** o timeout tem teto (30 min), e o pedido é cancelado se o cliente desconecta.
- **Saída da CLI em inglês:** é lida principalmente pelos agentes. A interface fica em português.

**Observação:** a detecção de "trabalhando" depende de saída contínua entre as amostras. Um agente que imprime em rajadas espaçadas mais que o intervalo de amostragem pode não ser marcado como trabalhando; aí vale o fallback, ou o hook.

## 2026-10-15, B2 servidor

- **`internal/store`:** workspace com nós (terminal, nota, portal) e arestas não direcionadas (`ropePoints`), em JSON atômico no envelope `{type, schemaVersion, payload}`. Validações: nome único de terminal, sem aresta para si mesmo nem duplicada. Consultas de vizinhos e conexão.
- **`internal/events`:** pub/sub que descarta assinantes lentos.
- **`internal/server`:**
  - **segurança:** só escuta em 127.0.0.1; verifica `Host` (contra DNS rebinding) e `Origin`; sessão por cookie `HttpOnly`/`SameSite=Strict`, criado a partir de `/?token=…`, ou por `Authorization: Bearer`; token por terminal (`X-Terminal-ID` + `X-Squad-Token`) para a CLI e os hooks;
  - **REST:** `/api/state`, CRUD de nós e arestas, `start`/`stop`/`restart`;
  - **WebSocket:** `/ws/events` (começa com `hello` e o estado completo) e `/ws/term/{id}` (`init`, render inicial em binário, saída em binário; entrada em binário; `resize` em texto);
  - cada terminal recebe `SQUAD_URL`, `SQUAD_TERMINAL_ID`, `SQUAD_TERMINAL_NAME`, `SQUAD_TOKEN` e `SQUAD_CLI`.
- **`internal/webui`:** interface embutida no binário (por ora, página provisória).
- **`squad serve`:** abre o workspace padrão, escuta em 127.0.0.1:7420, grava `~/.squad/run/server.json`, mostra a URL com token, abre o navegador e sobe os terminais. No Ctrl-C, encerra tudo e limpa a descoberta.

**Decisões:**
- **Workspace único (`default`)** por enquanto, com a pasta do projeto definida na primeira execução (`-dir`). O modelo já comporta vários; o seletor de workspaces fica para depois.
- **Terminais morrem com o servidor.** Mantê-los vivos entre reinícios (pty-host ou tmux) fica para depois, como prevê a `05`.

## 2026-10-15, B1 núcleo de terminal

- **`internal/screen`:**
  - interface `Screen` sobre o `vt`, com filtro de OSC (título, cwd, progresso);
  - hash normalizado para detectar atividade;
  - modos rastreados (2004, tela alternativa) e `Render()` para pintar um cliente que conecta;
  - as respostas do emulador são drenadas internamente, para o `Write` nunca travar.
- **Golden tests** com as gravações reais (vim, htop, Codex, Claude): a tela bate com a referência do xterm.js. A única tolerância é para linhas com emoji + seletor de variação.
- **`internal/pty`:**
  - no Unix, `creack/pty` (nova sessão + terminal de controle);
  - no Windows, ConPTY do `xpty` dentro de um Job Object com `KILL_ON_JOB_CLOSE`;
  - parada da árvore: SIGHUP+SIGTERM no grupo e SIGKILL depois de 2 s;
  - código de saída 128+sinal.
- **`internal/terminal`:**
  - shell de login por plataforma;
  - linha de lançamento digitada (type-ahead);
  - ambiente sem os marcadores de sessão de agentes (`CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`…);
  - fila de escrita serializada;
  - `Paste` com as temporizações da spec;
  - assinantes com render inicial + stream, sem lacuna nem duplicação;
  - `Manager` com criar, parar, reiniciar e remover.
- **Agente falso** (`internal/testutil/fakeagent`) para os testes.
- `go test -race` limpo.

**Decisões:**
- **PTY no Unix:** `creack/pty` em vez do `xpty`. O `xpty` não define `Setsid`/`Setctty` nem fecha o lado escravo no pai; sem isso, o shell fica sem controle de jobs e a leitura não termina quando o processo sai.
- **Enter de reforço só quando o destino fica calado:** na colagem, ele só é enviado se nenhuma saída chegou depois do primeiro Enter. Isso evita duplo envio em shells e em agentes que reagiram, e mantém a rede de segurança das TUIs que engolem o primeiro Enter. É uma melhoria sobre o Maestri, que manda sempre.
- **Segundo patch no `third_party/vt`:** `closed` virou `atomic.Bool`, porque o detector de corrida apontou o problema. Registrado em `third_party/README.md` e `spikes/go/patches/vt-v0.1.0-squad.patch`.

**Limitação:** no Unix, a parada atinge o grupo de processos. Um descendente que crie a própria sessão (daemon) escapa. Se for um problema, varrer a tabela de processos como o Maestri faz.

## 2026-10-15, B0 esqueleto

- Repositório privado `nathanribeiroo/squad` criado, com tudo o que existia na `main` (docs, design, spikes, research). A pasta `sandbox/`, resto dos testes do Maestri, ficou fora do git.
- Módulo `github.com/nathanribeiroo/squad`, com Go 1.26.
- `third_party/vt`: cópia do `charmbracelet/x/vt` v0.1.0 com o patch do buffer de 64 KB, via `replace`.
- `cmd/squad` com o despacho de subcomandos, `internal/config` (`~/.squad`, porta 7420, arquivo de descoberta `run/server.json` com escrita atômica) e `internal/version`.
- `make check`: vet (incluindo Windows), testes e build para os dois sistemas.

**Decisão:** a porta padrão é 7420 e pode ser trocada por `SQUAD_PORT`. O diretório de dados é `~/.squad` e pode ser trocado por `SQUAD_HOME`.
