# Investigação do Maestri (macOS): Observado, Inferido e Desconhecido

- **Alvo:** Maestri 0.53.1 (build 213), `com.evercraftlabs.Maestro`.
- **Binário principal:** `Contents/MacOS/Maestri`, universal, 64 MB, sha256 `a432f258…5dea`. Analisei o slice arm64.
- **CLI:** `Contents/Resources/maestri`, universal, 230 KB, sha256 `d60a8522…0d80`.
- **Período:** 14 e 15 de outubro de 2026.

## Como ler este documento

- **Observado:** visto diretamente, com a evidência ao lado (endereço de função no slice arm64, string curta, arquivo em disco ou medição em execução).
- **Inferido:** conclusão tirada da evidência, com nível de confiança (alta, média ou baixa).
- **Desconhecido:** ponto que não consegui determinar.
- Os endereços seguem a base do Hopper (`0x100000000`). Funções do Maestri não têm símbolos, então aparecem como endereço (`sub_…`).
- Não analisei nem documentei validação de licença, trial ou pagamento. Este documento não reproduz código, pseudocódigo extenso, prompts internos nem assets do Maestri; todas as descrições são minhas.

## Método

1. **Inventário com o REA:**
   - `inspect_artifact`, para o grafo do bundle;
   - `inspect_plist`;
   - `inspect_signature`;
   - `inspect_macho`, para dependências e imports.

   O `trace_dylib_resolution` não existe nesta versão do REA, então as dependências vieram do `inspect_macho`. O bundle do Mac não tem camada JS nem Electron, logo `analyze_javascript_application` não se aplica.
2. **Análise nativa da CLI:** feita no Hopper pelo REA, com `trace_feature`, `batch_decompile` e `procedure_callers`.
3. **Análise nativa do binário principal:** feita no Hopper, mas não pelo REA, por dois motivos. O REA desiste de iniciar o Hopper depois de 120 s, e a análise inicial do binário leva cerca de 3 minutos. Além disso, o Hopper da máquina é demo: não salva `.hop` e fecha a cada 30 minutos. A solução foi uma ponte própria, um script Python executado dentro do Hopper (`-Y`) que expõe a API por um socket Unix. A ponte foi validada contra o REA na CLI e deu as mesmas referências cruzadas. Em janelas de 30 minutos, exportei a lista de 78 mil funções, 23,7 mil strings, o mapa string → funções e cerca de 60 pseudocódigos. Completei com uma desmontagem `objdump` do slice arm64, usada offline.
4. **Execução:** num workspace de teste "sandbox", com um terminal Claude Code, um terminal shell e uma nota ligados entre si, fiz três coisas:
   - chamei a CLI com a identidade desses terminais, passando por um proxy que grava o tráfego do socket;
   - rodei no PTY de destino um registrador de bytes com horário;
   - li os transcripts JSONL que o próprio Claude Code grava.

## Inventário

| Item | O que é | Evidência |
|---|---|---|
| `MacOS/Maestri` | App Swift/SwiftUI/AppKit. Linka Network, WebKit, FoundationModels, Accessibility, Metal/MetalKit, XPC, UserNotifications e CoreSpotlight. | `inspect_macho` |
| `Resources/maestri` | CLI fina: Foundation e sockets, nada mais | imports `socket/connect/send/recv/setsockopt/isatty/signal` |
| `SwiftTerm_SwiftTerm.bundle` | Recursos do SwiftTerm (fork), com `default.metallib` (shaders) | arquivo no bundle |
| `Sparkle.framework` | Atualização automática | feed `updates.maestri.dev` |
| Assinatura | Developer ID, hardened runtime, **sem sandbox**; entitlements só de microfone, câmera e arquivos escolhidos | `inspect_signature` |
| Info.plist | Tipos `.maestri` (workspace), `.maestripartitura`, `.maestripartituras` (biblioteca), UTIs `partitura-ref` e `remote-file`; exceção ATS para HTTP em `localhost` | `inspect_plist` |
| Imports C do app | `forkpty`, `posix_spawn*`, `socket/bind/listen/accept/poll`, `proc_pidinfo`, `sysctl`, `kill/waitpid`, `shm_open`, `renamex_np`, `statfs` (não há `clonefile`) | `inspect_macho` |
| Dados | `~/.maestri/` (manifest, app-state, preferences, workspaces) e `~/.maestri-local/` | `find` |
| Skills | O app instala 7 skills em `~/.claude/skills/maestri*`, que ensinam os agentes a usar a CLI | arquivos `SKILL.md` |

---

## Feature 1: conexão entre agentes

### Observado

**Transporte e CLI**
- **A CLI só repassa o comando.** Ela envia `POST /cli HTTP/1.0` com `Host: maestri`, `X-Terminal-ID: <uuid>`, `Content-Type: application/json` e corpo `{"workingDirectory", "args"}`. Status ≥ 300 faz a CLI escrever o corpo no stderr e sair com código 1; caso contrário, ela imprime o corpo. Evidência: CLI `0x100002048` → cliente HTTP `0x1000062fc` (constantes decodificadas `POST`, `/cli`, `X-Terminal-ID`) e tráfego capturado byte a byte pelo proxy.
- **O transporte é um socket Unix.** O app escuta em `$TMPDIR/maestri-<8 hex>/maestri.sock` (`srwx------`), com um `.owner-pid` ao lado. O app copia a CLI para `$TMPDIR/maestri-cli/maestri`; o hash é idêntico ao do bundle. Evidência: `lsof` do app, `shasum`.
- **O servidor usa sockets BSD.** É o `InterAgentServer`, com filas de `accept` dedicadas (`com.maestri.interagent.accept`, `…tcp-accept`; `0x100156ecc`) e uma requisição por conexão. Responde `HTTP/1.1 <código>` com `text/plain; charset=utf-8` e `Connection: close` (`0x1001bc0f8`; confirmado na captura). Um `list` leva cerca de 1 ms.
- **Cada PTY recebe três variáveis:** `MAESTRI_SOCKET`, `MAESTRI_TERMINAL_ID` (o UUID do nó) e `MAESTRI_CLI` (caminho absoluto da CLI). O PATH começa pelo diretório da CLI. Evidência: `ps eww` dos shells de teste.
- **Dentro do tmux,** a CLI recupera o socket com `tmux show-environment -g MAESTRI_SOCKET_<id>`, e o binário do tmux pode ser trocado por `MAESTRI_TMUX` (`0x100008318`).
- **O timeout de socket depende do comando** (`0x100001de4`):

  | Comando | Timeout |
  |---|---|
  | `ask` com prompt | nenhum |
  | `ask --raw` | 10 s |
  | `say` | 60 s |
  | `recall` | 30 s |
  | `recall attachment` | 130 s |
  | demais | 10 s |

- **O `say` lê a mensagem do stdin** quando ele não é um TTY; é assim que o heredoc funciona (`0x100001bcc`).

**Roteamento e endereçamento**
- **Quem interpreta os comandos é o app.** O roteador fica em `0x10016aae0` e dá as mensagens "Unknown command '…'" e "Usage: maestri …". Handlers:

  | Comando | Handler |
  |---|---|
  | `list` | `0x10016c040` |
  | `ask` | `0x100185e68` |
  | `ask --raw` | `0x100171e4c` |
  | `ask --batch` | `0x100171734` |
  | `check` | `0x100173114` |
  | `note` | `0x100175270` |
  | `portal` | `0x100175b94` |
  | `connect` | `0x1001fa860` |
  | `role` | `0x100178f74` |
  | `floor` | `0x10017d570` |

- **O remetente é identificado pelo `X-Terminal-ID`.** Um UUID inválido dá 400.
- **O destino é endereçado pelo nome do nó, e só entre nós conectados.**
  - Destino inexistente dá 404.
  - Sem conexão dá 403 (`No connection to '…'`).
  - Nome duplicado dá 409.
  - Verificação em `0x10143ec58` e `0x101389950`.
  - Em execução, `list` mostra, para cada lado, apenas os nós ligados a ele.
- **A conexão não tem direção.** É gravada como `{id, createdAt, terminalIdA, terminalIdB, ropePoints}`, e os dois lados se enxergam.

**Entrega no PTY de destino** (medida com o registrador de bytes e conferida no estático em `0x101292be8` e `0x10129ed90`)
- **Com colagem delimitada** (o destino ativou DECSET 2004):
  1. Uma única escrita `ESC[200~<texto>ESC[201~`.
  2. Um Enter (CR) depois de cerca de 200 ms (917 ms para 6 KB).
  3. Um segundo CR de reforço, 250 a 300 ms depois.
- **Sem colagem delimitada:** o texto vai cru (um `\n` interno é mantido). O CR vem cerca de 50 ms depois e o segundo CR, mais uns 100 ms.
- **Nenhum cabeçalho de remetente é acrescentado.** O transcript do Claude Code mostra o prompt exatamente como foi enviado. O app também remove `ESC[200~`/`ESC[201~` embutidos no texto (`0x10018670c`).
- **`--raw`:** os escapes viram bytes e o texto é escrito uma vez, sem CR extra. A resposta é a tela do destino 0,3 s depois; medi 0,33 s.
- **Fila de escrita por terminal:** envios concorrentes ao mesmo destino são serializados (`0x101292454`).

**Espera e retorno**
- **O `ask` normal mantém a conexão HTTP aberta** até responder 200 com a **tela renderizada do destino**, não um diff.
- **O `ask` é liberado cerca de 5 a 6 s depois que a tela para de mudar.** Medições:
  - Pausas de 4,5 s e de 5,0 s na saída não liberaram.
  - A liberação veio 5,1 s depois da última saída, dentro de uma pausa de 7 s.
  - Com saída pesada (≥1,5 KB/s por 6 s), veio 5,7 s depois.
  - Num `ask` real ao Claude Code, a resposta "pong" apareceu em ~2 s e o retorno veio com 8,3 s.
- **Ask back observado:**
  1. O Shell perguntou ao Claude, e esse `ask` ficou pendente.
  2. O Claude rodou `maestri ask "Shell" "feito"`.
  3. O `ask` pendente do Shell foi resolvido **na hora**, com uma nota padrão: o destino respondeu mandando uma mensagem direta, que chega como novo prompt.
  4. "feito" chegou ao PTY do Shell como colagem + CR + CR.
  5. O `ask` do Claude seguiu esperando o Shell ficar ocioso.
- **`--batch`:** cria uma tarefa por destino, em paralelo, e devolve um array JSON de `{name, output}` ou `{name, error}` (`0x100171734`, `0x1001c9d84`).

### Inferido

- **Regra de "fim de turno"** (média-alta), lida no estático:
  - O `TerminalActivityMonitor` amostra cada terminal a cada 1 s (`0x1011ae270`, `sub_1011b1354`).
  - Uma amostra é "ativa" com ≥500 bytes de saída **e** mudança no hash da tela visível. O hash pula as 3 primeiras linhas quando a tela tem mais de 11 e troca glifos braille de spinner por espaço (`sub_10119b488`, `sub_10119c240`).
  - Três amostras ativas seguidas põem o terminal em "trabalhando". Mais de 3 s sem mudança encerram o turno e disparam os callbacks de quem espera.
- **Fallback e timeout** (alta):
  - O fallback começa 5 s depois do envio, checa a cada 1 s e libera com ≥5 s de tela estável.
  - O timeout é de 120 s. Se houve saída nos últimos 10 s, ele é prorrogado por mais 120 s, sem limite. Se a tela está estável há ≥10 s, responde 200.
  - Em execução, só o comportamento de ~5 s ficou visível. A regra de 3 s não se mostrou separada, e o motivo está nos Desconhecidos.
- **Ciclos de espera** (alta): um ciclo maior que A↔B (por exemplo A→B→C→A) é achado por busca em profundidade sobre os `ask` pendentes (`sub_100190250`). O app libera o pendente de quem está bloqueando, com uma nota de "liberado para evitar deadlock" (`0x100190cb0`), e nunca o pedido novo. Não testei em execução, porque exigiria três agentes.
- **Terceiro CR** (média): o estático prevê três CRs no modo com colagem, mas medi dois. Provavelmente o terceiro só sai se o destino ainda não reagiu.
- **Área de transferência** (média): se ela contém imagem, o app a guarda e esvazia antes de colar e a restaura cerca de 1,5 s depois (`0x10129f4f4`). O objetivo provável é evitar que o agente anexe a imagem por engano.
- **Destino descarregado** (média): um destino num andar ou workspace descarregado é acordado. O app espera 10 s e tenta uma vez; se não der, responde 503 "still starting".
- **Uso remoto** (alta para as strings, não testado):
  - Há um listener TCP em `127.0.0.1` (porta 1024–65535) para terminais remotos, alcançado por túnel SSH reverso ou `host.docker.internal`.
  - O remoto usa um shim `curl` com `X-Maestri-Token`, e o token é republicado no tmux como `MAESTRI_TOKEN_<id>`.

### Desconhecido

- Por que a regra de 3 s não apareceu nas medições. O monitor não tem regra de 5 s, então os ~5 a 6 s vêm do fallback do `ask`. A hipótese é que, com destino shell, o turno nunca chega a "trabalhando" (ver feature 2).
- As regras exatas de `Nome @ Workspace` e qual nó duplicado prevalece.
- Como o diretório `maestri-<8 hex>` é nomeado e como um socket órfão é limpo (só há a string "Cleaned up stale directory").
- O que acontece com o `ask` pendente se o cliente desconectar.

---

## Feature 2: terminais (PTY, ciclo de vida, captura e término)

### Observado (em execução)

- **O processo é um login shell.** O filho do app é o shell do usuário com `-l` (no caso, `fish -l`) e é líder de sessão.
  - O ambiente recebe `MAESTRI_SOCKET`, `MAESTRI_TERMINAL_ID`, `MAESTRI_CLI`, `TERM=xterm-256color`, `COLORTERM=truecolor`, `TERM_PROGRAM=Maestri` e `LANG`.
  - O PATH fica `<dir da CLI>:/usr/bin:/bin:/usr/sbin:/sbin`, e o HOME é o real.
- **O agente roda dentro do shell.** O `claude` aparece como filho do fish, **sem nenhuma flag**. O `config.fish` do usuário reescreve o PATH e o diretório da CLI se perde; por isso existe `MAESTRI_CLI` como alternativa, e as skills mandam usá-la.
- **O nó de terminal persiste:** `name`, `agentType` (`claude_code`, `generic_shell`), `command` (`claude`), `shellPath`, `workingDirectory`, `status`, `scrollbackFile` (`<uuid>.scrollback`), `scrollbackLineCount`, `monitorWithOmbro` e `chatThreads`.
- **Reinício:** atribuir um papel reinicia o terminal (novo shell e novo agente). Ativar o Modo Maestro **não** reinicia.
- **O `check`** devolve a tela renderizada atual (o mesmo texto que o `ask` devolve).

### Observado no estático (confiança alta, salvo indicação)

- **Criação:**
  - O provedor local `SwiftTermProvider` (`0x101194890`) chama o lançador do SwiftTerm (`0x1016cc710`), o único que usa `forkpty`. O filho faz `chdir(cwd)` e `execve`; se o exec falhar, sai com 127.
  - O argv padrão é `["-l"]`. O shell vem do `shellPath` do nó, senão `$SHELL`, senão `/bin/zsh`, senão `/bin/sh`.
- **Ambiente:**
  - Parte do ambiente do app, menos TERM, COLORTERM, LANG, TERM_PROGRAM, TERM_PROGRAM_VERSION e PATH.
  - Soma os overrides do nó e as variáveis `MAESTRI_*`.
  - Monta o PATH com o diretório da CLI na frente.
  - Fixa TERM, COLORTERM, LANG e TERM_PROGRAM.
- **Tamanho e leitura:** o tamanho inicial vai para o `forkpty`, e cada redimensionamento faz `ioctl(TIOCSWINSZ)`. A leitura usa DispatchIO no master do PTY com blocos de até 128 KiB. A saída do processo é detectada por um process-exit source.
- **Lançamento do agente:**
  - A linha de lançamento é **digitada no PTY logo depois do spawn, sem esperar o prompt**. Ela fica no buffer do TTY até o shell ler, sem `exec` e sem `clear`.
  - Para shells não POSIX (fish, nu…), a linha é montada na sintaxe deles.
  - Acima de 999 caracteres, o app grava um script `launch/<uuid>.sh` (modo 0600, apaga a si mesmo ao rodar) e digita `/bin/sh '<caminho>'`.
  - Para o Codex, o preset é `codex --no-daemon`, com um teste que remove a flag se a versão instalada não aceitar.
- **Tipo do agente:** é deduzido do nome do programa no comando, ignorando `VAR=valor` e caminhos e desembrulhando `sh -c "…"`. Os conhecidos são `claude`, `codex`, `gemini`, `agy` e `opencode`; o resto vira `custom` ou `generic_shell`.
- **Retomada:** quando o agente sai, o app **lê da tela** a linha de "resume" que ele imprime (prazo de 10 s, leitura a cada 0,15 s) e a guarda como `resumeCommand`, para relançar a sessão depois (preferência `resumeSessionsOnRelaunch`).
- **Modo tmux** (`localTmux`):
  - A sessão se chama `maestri-<uuid sem hífens>`.
  - A linha digitada cria a sessão ou reaproveita uma existente, publica `MAESTRI_SOCKET_<id>` com `set-environment -g`, desliga a barra de status e faz `exec tmux attach`.
  - Fechar o app mata só o cliente tmux. No relançamento ele reconecta e republica as variáveis.
- **Fim e parada:**
  - O fim do processo é tratado com `waitpid(WNOHANG)`; uma morte por sinal vira código 128+sinal.
  - "Parar" manda SIGTERM ao grupo de processos do shell e a cada descendente (lidos da tabela de processos) e SIGKILL 2 s depois. "Forçar" manda SIGKILL direto.
- **Scrollback:** fica em `terminals/<id>.scrollback`, como um array JSON de linhas com faixas de estilo (cores, negrito…), com limite de tamanho na leitura.

### Inferido

- **O término é detectado pela tela** (alta). O mecanismo é o mesmo da feature 1: o `TerminalActivityMonitor` amostra a cada 1 s, exige ≥500 bytes e mudança de hash, conta 3 amostras ativas para "trabalhando" e encerra o turno após mais de 3 s sem mudança. **O monitor não tem regra de 5 s.** Os ~5 a 6 s medidos vêm do fallback do próprio `ask`. Não encontrei hooks nos agentes; o `settings.json` do Claude Code não foi alterado.
- **Destino shell** (baixa). É plausível que o caminho de "fim de turno" valha só para tipos de agente. Há um conjunto `{claude, codex, gemini}` que habilita algum comportamento, e isso explicaria por que, com destino shell, só o fallback de 5 s apareceu.

### Desconhecido

- Qual regra distingue `exited` de `crashed`.
- Quando o scrollback é gravado.
- Que tecla ou comando faz um agente sair e imprimir a linha de "resume".

---

## Feature 3: papéis (instruções reutilizáveis)

### Observado

- **Biblioteca:** os papéis ficam em `~/.maestri/preferences.json`, em `rolePresets[] = {id, name, prompt, color, icon, workspaceId}`. Um papel com `workspaceId` vale só naquele workspace.
- **A atribuição reinicia o terminal.** O agente volta **sem flags** e com **cwd = `<projeto>/.maestri/roles/<uuid>/`**. Essa pasta contém:
  - `CLAUDE.md` e `AGENTS.md`, **idênticos** (303 B no teste);
  - `role.json` (`{id, name, prompt, color, icon, schemaVersion}`).
- **Conteúdo dos arquivos:** o texto do papel vem entre as tags `your_assigned_role`, seguido de um bloco `working_directory` com uma frase que aponta o caminho real do projeto.
- **O efeito foi confirmado.** Com o papel "responda em maiúsculas", o agente respondeu em maiúsculas e citou a pasta real.
- **Efeito colateral:** o Claude Code passou a gravar as sessões numa pasta de projeto separada (`…sandbox--maestri-roles-<id>`), porque a chave da sessão é o cwd.
- **Gemini:** só para ele o app grava um `.gemini/settings.json` adicional, que aponta o arquivo de contexto para `AGENTS.md` e inclui o diretório do projeto (`0x100260588`).
- **Escrita segura:** temporário + rename, limite de 1 MiB, recusa de symlink (regenera o arquivo) e validação UTF-8 (`0x10025f0f4`, `0x10025f43c`).

### Inferido

- **A técnica é universal** (alta). Cada CLI de agente carrega sozinha o `CLAUDE.md` ou o `AGENTS.md` do diretório onde foi aberta, então não são necessárias flags específicas. Não há `--append-system-prompt` nem `--add-dir` no binário.
- **Realinhamento** (alta). Se o projeto muda de lugar, o app troca só o bloco `working_directory` e preserva o resto, inclusive edições manuais (`0x10025fc50`).
- **Papéis viajam com o repositório** (alta). A importação varre `<projeto>/.maestri/roles/*/role.json`, e o id precisa bater com o nome da pasta (`0x100264664`).

### Desconhecido

- Se um papel global (sem `workspaceId`) muda algo na hora de materializar os arquivos.
- Como o Claude Code lida com permissões para editar fora da pasta do papel.

---

## Feature 4: notas conectadas

### Observado

- **Cada nota é um arquivo Markdown** em `~/.maestri/workspaces/<id>/notes/`. O nó guarda só `{fileName, color, fontSize, isPreviewing, textModifiedAt}` e `isLocked`.
- **Nome automático:** o nome é um *slug* da primeira linha ("# Captura REA" vira `captura-rea.md`). A nota é **renomeada sozinha** quando a primeira linha muda, e a CLI avisa o novo nome.
- **Nome fixo:** `note create --name "X"` grava `X.md` com nome estável. A nota criada é **ligada automaticamente** ao terminal que a criou (entrada nova em `noteConnections`).
- **O arquivo é a fonte da verdade.** Uma edição feita direto no disco apareceu no `note read` em menos de 2 s. O app mantém a pasta `notes/` aberta (`lsof`).
- **Formato do `note read`:** devolve as linhas numeradas com o cabeçalho "[N lines total]".
- **Edição:** `note edit` troca uma substring e `note write` troca o conteúdo inteiro. Notas bloqueadas (`locked: true` no `list`) não aceitam escrita.
- **Acesso:** o agente só lê e escreve notas ligadas a ele, inclusive cadeias de notas (`noteToNoteConnections`), que o `list` mostra como árvore.

### Inferido

- O watcher é uma fonte de eventos de diretório (DispatchSource/kqueue) sobre `notes/` (média).
- As pilhas de notas ("fichário": `note stack/unstack`) são agrupamentos no modelo do canvas (alta, pela ajuda da CLI).

### Desconhecido

- Como o app resolve uma escrita da CLI que concorre com uma edição na interface.

---

## Feature 5: Prompt Composer e @menções

### Observado

- **Cada @menção vira uma tag autocontida no meio do texto**, com o nome e a skill a carregar. Transcript do Claude Code:
  - terminal: `<maestri-canvas-agent name="Shell" skill="maestri" />`
  - nota: `<maestri-canvas-note name="captura-rea" skill="maestri" />`
- **O catálogo de menções oferece só os nós ligados ao terminal.** O portal ligado ao Shell não apareceu para o Claude Code.
- **Entrega:** o texto vai pelo mesmo caminho de colagem do `ask`.

### Inferido

- **Serialização** (alta, `sub_10152b3b4`):
  - Portais usam `skill="maestri-portal"`, ou `"maestri-portal-devices"` para simulador e Android.
  - Andares usam `<maestri-floor … skill="maestri-manager" />`.
  - Há "chips" de ação fixos: Maestro, criar nota, portal, andar e rotinas.
  - Os nomes são escapados como XML, e menções a arquivo viram caminhos entre aspas simples.
- **Envio** (alta, `sub_1002429a8` → `sub_101291ff0` → fila de escrita):
  - O Composer não espera resposta, ao contrário do `ask`.
  - Um prompt que termina em `/>` recebe um espaço no fim.

### Desconhecido

- Como o agente reage a cada tag. A tag só nomeia a skill, e a skill instalada explica o que fazer.

---

## Feature 6: modelo de dados (workspace e Partitura)

### Observado

- **Envelope:** todo arquivo é `{ "type": "...", "schemaVersion": N, "payload": {...} }`. Exemplos:
  - `workspace` (v2);
  - `app-state` (`activeWorkspaceId`, `hasCompletedOnboarding`, …);
  - `manifest` (`app`, `appVersion`, `dataFormat`, `files`, `workspaces[{id, name, path}]`);
  - `preferences` (`rolePresets[]`, atalhos, …).
- **Estrutura do workspace** (`~/.maestri/workspaces/<UUID>/workspace.json`, mais as pastas `notes/`, `terminals/`, `snapshots/` e `files/`):

```text
payload:
  id, name, icon, color, isPinned, createdAt, lastOpenedAt
  workingDirectory, locationType ("local" | ssh | docker…), sshConfig?, dockerConfig?
  canvasOrigin [x,y], canvasZoom, dockEdge, dockedNodeId?, liftedNodeId?
  nodes[]:
    id, createdAt, lastModifiedAt, zIndex, frame [[x,y],[w,h]], isLocked, isRedacted
    content: { terminal: { _0: TerminalNode } } | { stickyNote: { _0: NoteNode } } | (portal …)
  connections[]:          { id, createdAt, terminalIdA, terminalIdB, ropePoints[[x,y]…] }
  noteConnections[]:      { id, createdAt, noteNodeId, terminalId, ropePoints }
  noteToNoteConnections[], portalConnections[], portalToPortalConnections[]
  crossFloorConnections[], crossWorkspaceConnections[]
  floors[]:               { id, name, cloneKind ("worktree"|…), clonePath, branchName, seeding,
                            nodes[], connections[], noteConnections[], portalConnections[], … }
  groups[], drawings[], cableTies[]
  (estático: activeFloorId, groundFloorBranch, floorCloneExclusions, hooks, preferredIDE, shortcutIndex)

TerminalNode: id, name, agentType, command, shellPath, workingDirectory, status, scrollbackFile,
  scrollbackLineCount, monitorWithOmbro, shortcutMode{kind}, color, icon, lastActiveAt,
  chatThreads[7], activeChatThread, showsShellWindowDuringTurns
  (estático: environmentOverrides, exitCode, assignedRoleId, localTmux, isManager, isUnloaded,
   resumeCommand, prefersChatMode, sandboxConfig, restartsKeptAliveSession)
NoteNode: fileName, color, fontSize, isPreviewing, textModifiedAt
```

- **Codificação:** `content.terminal._0` é o formato que o Codable do Swift gera para um enum com valor associado, ou seja, o nó é um enum.
- **Andares:** cada andar é um **sub-canvas completo** dentro do workspace, com seus próprios `nodes` e `connections`.

### Inferido

- **A Partitura é um modelo criado a partir de uma seleção do canvas** (alta, pelas strings: "New Partitura from Selection" e "Each terminal is saved with the command it launches").
  - Ela guarda terminais com os comandos de lançamento, os papéis, as notas e o layout.
  - Ao importar, o app avisa quais comandos vão rodar.
  - `.maestripartituras` é uma biblioteca com várias Partituras.
- O mais provável é que use o mesmo envelope `{type, schemaVersion, payload}` com um subconjunto de `nodes` e `connections`, mais os papéis (média).

### Desconhecido

- As chaves exatas da Partitura. Nenhuma foi exportada nos testes, e as strings dão só os tipos (`Partitura`, `PartituraBundle`, `PartituraRoleResolution`).

---

## Feature 7: andares (cópia isolada e "aterrissar")

### Observado

- **Criação:** `floor create "teste"` levou menos de 1 s. Ela criou o branch `teste` e um **git worktree em `<pasta-pai do projeto>/.maestri/floors/<projeto>--<slug>`**, fora do projeto.
- **Conteúdo copiado:** o andar recebeu **toda** a árvore de trabalho do térreo, inclusive arquivo não versionado, `node_modules` ignorado (20 MB) e `.maestri/`. Os arquivos versionados ficaram limpos no git.
- **Registro:** o andar aparece em `workspace.json` como `floors[]`, com `cloneKind: "worktree"`, `clonePath` e `branchName`.
- **Recusa do land:** `floor land` recusa se o andar tiver alteração não commitada **ou** arquivo não versionado.
- **Land padrão:** **não mescla**. Remove o worktree e o andar e mantém o branch no repositório.
- **Land com `--into main`:** gera um **commit de merge sem fast-forward** ("Merge teste2 into main", com a identidade git do usuário), remove o andar e mantém o branch.

### Inferido

- **Sequência de criação** (alta, `0x100624034`, `0x1006414d4`):
  1. `git worktree add --no-checkout`.
  2. Para cada item do topo do térreo, `cp -c -R` (clone APFS copy-on-write).
  3. `git reset` para alinhar o índice.
  4. Sem copy-on-write, só os arquivos versionados (`reset --hard`).
- **Exclusões:** 36 pastas de build e cache (`.next`, `dist`, `target`, `DerivedData`, `.venv`…), mais `floorCloneExclusions`. O `node_modules` não é excluído. Os submódulos recebem tratamento próprio.
- **Ambiente remoto (SSH/Docker):** um script `sh` faz o mesmo, testando `cp --reflink=always` ou `cp -c` (`0x10064718c`).
- **Mescla sem checkout** (alta): se o branch alvo não está aberto em nenhum worktree, o app mescla com `merge-tree --write-tree` + `commit-tree` + `update-ref`, sem tocar em nenhuma pasta. Em conflito, aborta e lista os arquivos.
- **Squash e rebase:** só existem via PR (`gh`/`glab`).
- **Remoção:** `rm -rf` do andar + `git worktree prune` + `git branch -D` (exceto com `--keep-branch`).

### Desconhecido

- Se alterações não commitadas do térreo são levadas para o andar recém-criado. Pelo `cp -c` da árvore inteira, devem ser.

---

## Feature 8: portais (árvore de elementos em vez de pixels)

### Observado (portal web)

- **Ligação:** `portal create URL "Nome"` cria o portal e o liga ao terminal que o criou.
- **Formato do `snapshot`:**
  - um cabeçalho com viewport, URL e título;
  - uma linha por elemento: `@eN <papel|tag> "<nome acessível>" [atributos] [x,y largura×altura]`;
  - só entram elementos interativos e marcos (`label`, `input`, `button`, `a`, `status`, `nav`); `h1` e `li` ficaram de fora.
- **Referências:** os elementos são marcados no próprio DOM com `data-mref`, e as referências `@eN` resolvem por esse atributo. Elas expiram quando a página muda.
- **Clique:** por ref ou por coordenada, gera eventos `pointer`/`mouse`/`click` com **`isTrusted: true`**, ou seja, eventos nativos injetados no WebView.
- **Digitação:** `type`/`fill` inserem o texto **sem** `keydown`.

### Inferido

- **Web** (alta, `0x100db0f74`, `0x100d715ec`):
  - O script de snapshot é injetado só quando um comando pede e percorre também os iframes da mesma origem.
  - O clique rola o elemento para a vista, calcula o centro e envia mouseDown e mouseUp nativos.
  - A digitação usa o setter nativo do valor e dispara os eventos `input`/`change`.
  - O `evaluate` tem timeout de 10 s, e há captura de console com os últimos 200 eventos.
- **Simulador iOS** (alta):
  - O app traz embutido o fonte Objective-C de um agente que percorre a árvore `UIAccessibility`. Ele o compila em runtime com `xcrun -sdk iphonesimulator clang`, assina e guarda em cache pelo hash.
  - O agente é injetado no app com `SIMCTL_CHILD_DYLD_INSERT_LIBRARIES` (`0x100c86f7c`).
  - Ele abre uma porta TCP local e anota o número num arquivo dentro do container do app. O Maestri conecta e recebe JSON com label, value, id, frame e traits.
  - Toques e teclas vão por HID privado do SimulatorKit, e a captura de tela vem do framebuffer do simulador.
- **Android** (alta): usa `adb exec-out uiautomator dump` (XML convertido em papéis), `input tap/swipe/text/keyevent`, `monkey` para abrir apps e `screenrecord` para a imagem ao vivo.

### Desconhecido

- O mapeamento de modificadores de teclado no portal web.
- O critério exato de "UI estável" antes do snapshot no simulador.

---

## Feature 9: renderização no canvas e Ombro

### Inferido

- **Renderização** (média):
  - O app usa um fork do SwiftTerm com renderizador Metal: atlas de glifos via Core Text e shaders no `default.metallib` do bundle.
  - Há um fallback em Core Graphics, controlado pela opção "Use Metal".
  - Durante o zoom do canvas, o terminal fica congelado como imagem e é re-renderizado na nova escala ao final (`ZoomFreezeHelper`).
- **Ombro** (alta para as APIs):
  - Usa o FoundationModels da Apple, no dispositivo, com sessões que têm instruções e ferramentas: listar workspaces e terminais, ler o buffer de um terminal, ler notas e anexar a uma nota "Ombro Notes".
  - As saídas estruturadas são schemas `Generable`.
  - Tarefas: resumir um terminal (2 a 3 frases, com ANSI removido); visão geral do workspace; ao fim de um turno, uma mensagem curta, 3 sugestões e um comando.
  - Não há outro modelo como fallback: sem Apple Intelligence, o app só mostra um aviso.

### Desconhecido

- O gatilho exato do Ombro ao fim de um turno. A ligação com o `TerminalActivityMonitor` é provável, mas não a confirmei.

---

## Limitações gerais

- O Hopper demo fecha a cada 30 minutos e não salva. Parte da análise estática foi feita sobre a desmontagem `objdump`, e algumas partes assíncronas do Swift foram reconstruídas por análise de continuações, com confiança média.
- O mapa string → funções pode atribuir uma string Swift longa ao vizinho errado, porque essas strings são referenciadas como endereço − 0x20. Os casos relevantes foram conferidos no pseudocódigo.
- Não testei terminais remotos (SSH/Docker), o simulador iOS, o Android nem ciclos de 3 ou mais agentes.
