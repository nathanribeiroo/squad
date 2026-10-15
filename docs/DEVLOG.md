# Diário de desenvolvimento

O que foi feito em cada etapa, as decisões tomadas sem consulta e o que precisa de você. Entradas mais novas no topo.

## 2026-10-15, portal: agentes (P2)

- **`squad portal`** (extensão do `agentbus`, como `squad note`): `snapshot`, `click`, `fill`, `type`, `key`, `scroll`, `goto`, `back`, `forward`, `reload`, `screenshot`, `eval`, `console` e `list`. O agente só enxerga portais ligados a ele.
  - **`snapshot`:** a árvore de acessibilidade do Chrome vira uma lista de elementos com referência (`@e3 button "Enviar" [282,218 81x38]`), ordenados pela posição na tela, com valor, marcado/desmarcado e "offscreen". No máximo 300 elementos. As referências valem até a página mudar de documento; uma referência velha pede um snapshot novo.
  - **Ações:** `click` rola o elemento para a tela e clica no centro; `fill` limpa o campo e digita; `key` aceita nomes e combinações (`Enter`, `Control+a`, `Shift+Tab`…). Depois de cada ação, a resposta diz se a página mudou de endereço ("The page is now … Take a new snapshot").
  - **`screenshot`** salva um PNG em `attachments/` do workspace e imprime o caminho; `console` mostra as últimas 200 mensagens e erros da página; `eval` roda JavaScript com limite de 10 s.
  - **Qual portal:** com um só ligado, é ele; com vários, o primeiro argumento é o nome (`squad portal "Docs" click @e2`, sem diferenciar maiúsculas). `squad list` e as @menções do Composer citam os portais pelo nome.
- **Nome do portal:** "Portal", "Portal 2"… dados na criação (os antigos ganham nome ao abrir). Duplo clique no nome ou ⋯ → Renomear; nomes repetidos são recusados. Trocar o endereço mantém o nome.
- **Na tela:** cada ação publica `portal.action` (quem, o quê, onde). O portal mostra o cursor do agente na página, com o nome e a cor do papel, deslizando de um ponto ao outro; teclas, digitação e navegação aparecem num selo no canto. O fio entre o agente e o portal acende na cor do papel, com a pílula "clicando", "preenchendo", "digitando"… por 2,5 s.
- **Correção do P1 (imagem distorcida):** quando a primeira carga da página corria junto com o início do screencast, o Chromium passava a capturar a janela headless (780×441) em vez da página (por exemplo, 638×479). A imagem ficava esticada, e os cliques no canvas caíam fora do lugar. Agora o quadro com tamanho errado é descartado, o tamanho da página é aplicado de novo e o screencast recomeça. Teste novo: `TestFramesHaveViewportSize`, que falha sem a correção.
- **Correção:** o binário `squad` não repassava o comando `portal` ao servidor ("unknown command").
- **Skill:** a seção "Portals" explica o ciclo snapshot → ação → snapshot e os comandos (`squad install-skill` atualiza).
- **Testes:** `internal/browser` (ações dos agentes e tamanho dos quadros) e `internal/app` (`TestPortalCommands`: comandos, nomes, ambiguidade, evento, screenshot, console e endereço gravado), com o Chromium de verdade. Na interface, com o Chrome headless: renomear o portal e um agente "Ana" (Testador) preenchendo, clicando e teclando, com o cursor e a pílula na cor do papel.

## 2026-10-15, portal ao vivo (P1)

- **Teste prévio:** três abas de um Chromium sem janela transmitiram ao mesmo tempo, cerca de 20 quadros por segundo cada. O navegador abriu em 1,3 s e cada aba em 0,2 s. Daí a escolha de um navegador para o servidor inteiro, com uma aba por portal.
- **`internal/browser`:**
  - procura Chrome, Edge, Chromium ou Brave (`SQUAD_BROWSER` força um executável) e abre o navegador só quando um portal precisa, com o perfil em `~/.squad/browser`;
  - fecha 30 s depois da última aba e reabre sob demanda;
  - `Page`: navegar, voltar e avançar (a página em branco inicial não conta), recarregar, tamanho da página, estado (URL, título, carregando, erro, histórico), mouse e teclado com eventos confiáveis, colar e `Evaluate`;
  - o screencast (JPEG) só roda enquanto alguém assiste, e um espectador lento pula quadros em vez de acumular atraso.
- **`internal/portal`** (um por workspace):
  - `GET /w/<id>/ws/portal/{nó}` envia o estado em JSON e os quadros em binário (8 bytes com a hora da pintura, em ms, e depois o JPEG); recebe mouse, teclado, colar e voltar/avançar/recarregar;
  - editar o endereço navega; navegar dentro da página grava o endereço novo no canvas, sem navegar de novo (a última URL aplicada fica guardada por aba, para que mover o nó não recarregue a página);
  - redimensionar o nó redimensiona a página;
  - remover o portal ou o workspace fecha a aba.
- **Web:**
  - transmite só no zoom "ao vivo"; abaixo disso, mostra o último quadro ("pausado");
  - "● ao vivo · N ms" fica âmbar acima de 400 ms. O primeiro quadro de cada conexão é o último guardado e não entra na conta, porque o Chrome só pinta quando a página muda;
  - as posições do mouse são convertidas da imagem na tela para a página em qualquer zoom; o teclado segue as regras dos terminais (`⌥Esc`);
  - o nó do portal ganhou alças de redimensionar;
  - sem navegador, aparece a mensagem com a solução.
- **Testes:**
  - `internal/browser` e `internal/portal` rodam contra um navegador de verdade quando há um (`SQUAD_BROWSER`; sem navegador, são pulados). Cobrem quadros, cliques e teclas, colar, histórico, erro de conexão, tamanho, URL gravada, mover sem recarregar, fechar a aba e o navegador ocioso. Também com `-race`;
  - na interface, com o Chrome headless: contador clicado três vezes, nome digitado, "Enviar", rolagem, link para outra página com a URL salva, voltar, sem navegador e zoom reduzido.
- **Binário:** cerca de 2,6 MB a mais por causa do `chromedp`.

**Precisa de você (Windows):** abrir um portal com um site e ver se o Edge (ou o Chrome) sobe sem janela. Se a política da empresa bloquear o `msedge.exe` nesse modo, o portal mostra o erro; aí vale testar com `SQUAD_BROWSER` apontando para o Chrome.

## 2026-10-15, vários workspaces

- **Uma instância por workspace:** cada workspace é um `app.App` completo (canvas, terminais, `agentbus`, notas, monitor de atividade), como antes. O pacote novo `internal/hub` carrega todos de `~/.squad/workspaces/*` e monta cada um em `/w/<id>/`. Por isso, nada dentro do workspace mudou: pausa, limite por minuto, notas e `squad ask` já ficam separados.
- **`/cli` e `/hooks`:** o hub descobre o workspace pelo `X-Terminal-ID`, e o token do terminal continua sendo conferido pelo workspace.
- **API do hub:**
  - `GET/POST /api/workspaces`, `POST /api/workspaces/open` (acha ou cria pela pasta), `PATCH/DELETE /api/workspaces/{id}`;
  - `GET /api/dirs` relativo à pasta do usuário;
  - `/ws/workspaces` com o resumo (terminais, agentes ativos, pedidos pendentes), agrupado a cada 300 ms.
  - `/api/…` sem `/w/<id>` responde 404 explicando o caminho novo.
- **Remover:** encerra os terminais e move a pasta do workspace para `~/.squad/trash/<id>-<data>`, com novas tentativas no Windows enquanto o watcher de notas solta a pasta. O último workspace não pode ser removido.
- **Cores:** chaves (`teal`, `blue`, …) desenhadas com tokens do tema. Um workspace novo recebe a menos usada; os sem cor (o `default` antigo) ganham uma ao carregar.
- **`server`:** `Handler` foi dividido em `APIHandler` e `TerminalHandler`, com `Guard`, `Session` e `UI` como funções. O redirecionamento do `?token=` mantém os outros parâmetros (`?ws=`). A interface agora é registrada em `/` (um `GET /` conflitava com `/api/` no `ServeMux`) e recusa métodos que não sejam GET e HEAD.
- **`squad serve -dir`:** abre ou cria o workspace da pasta (`.` por padrão). Com um servidor já rodando, pede o workspace a ele e abre o navegador, em vez de falhar com "already running".
- **Web:**
  - prefixo `/w/<id>` em toda a API do workspace;
  - a lista vem de `/ws/workspaces`, e o workspace na tela é o do `?ws=`, depois o último usado, depois o primeiro;
  - a troca limpa o estado do workspace anterior, reabre `/ws/events`, fecha os sockets dos terminais (o `term/session` observa `activeWs`, para evitar um ciclo de imports com o store) e restaura o enquadramento salvo daquele canvas;
  - seletor no topo, `⌥1`–`⌥9` (também com o teclado num terminal), criar, editar e remover.
- **Testes:**
  - `internal/hub`: criar, abrir, editar e remover, lixeira, recarga, cor do workspace antigo, roteamento por workspace, `check` de um terminal com o mesmo nome em dois workspaces, credenciais erradas, stream da lista e redirecionamento do token;
  - todos com `-race`;
  - interface conferida com o Chrome headless: um terminal trabalhando no p1 enquanto o p2 está na tela, o ponto de atividade, `⌥2`, a volta com o terminal ainda trabalhando, criar, renomear (ao vivo na barra) e remover.

**Decisões (combinadas):** todos os workspaces sobem com o servidor; `serve -dir` com servidor rodando abre o workspace nele; remover manda para a lixeira. A gaveta de atividade é por workspace e recomeça ao trocar (o servidor só guarda os pedidos pendentes).

## 2026-10-15, pasta e shell por terminal, fonte de ícones

- **Pasta do terminal:** absoluta, `~/…` ou relativa ao workspace (`ResolveCwd` em `internal/server`). A relativa segue o workspace se a pasta dele mudar. Criar ou editar com uma pasta inexistente dá 400 (`folder not found: …`), em vez de um terminal que não abre.
- **`GET /api/dirs?path=…`:** subpastas do trecho digitado, escritas do jeito que foram digitadas (relativas continuam relativas). Pastas ocultas só aparecem com `.`; até 50 sugestões.
- **`GET /api/shells`:** shells instalados com nome amigável (Git Bash, PowerShell 7, Windows PowerShell, cmd; no Unix, os candidatos mais o `/etc/shells`, sem `tmux`/`screen`), o padrão primeiro.
- **Iniciar e reiniciar** agora remontam o terminal a partir do nó (`StartTerminal`), para valer o que foi editado. O `Manager.Restart` reaproveitava a spec antiga.
- **Web:**
  - `TerminalForm` substitui o `NewTerminalForm` e também edita ("Configurar…" no menu do terminal), com "Salvar e reiniciar" quando muda agente, comando, pasta ou shell;
  - `DirInput` faz as sugestões de pasta;
  - o rodapé do terminal mostra a pasta resolvida, cortando o começo do caminho e não o fim;
  - `~` também abrevia `C:\Users\<nome>`.
- **Ícones:** `web/src/lib/icons.ts` lista os 63 ícones usados (tipo `IconName`). `scripts/subset-icons.py` (fontTools) mantém só as ligaduras deles, fixa peso 400, grade 0 e opsz 20 e mantém o eixo FILL: 4 MB viram 8,8 KB. O binário cai de 13,5 MB para 9,5 MB. `npm run build` falha se a lista e a fonte divergirem (`make icons`).
- **Testes:** `TestResolveCwd`, `TestTerminalFolder` (pasta relativa, inexistente, edição + reinício), `TestDirSuggestions`, `TestDirsAndShellsEndpoints`, `TestShellOptionsLabels`, `TestEtcShells`. A interface foi conferida com o Chrome headless: sugestões de pasta, Tab, criação, "Configurar…" com reinício e os ícones (incluindo o preenchido).

**Decisão:** a pasta é guardada do jeito que foi digitada (sem a barra final), e não resolvida, para que as relativas acompanhem o workspace.

## 2026-10-15, Windows: Git Bash como shell padrão

No primeiro teste num Windows real, nenhum terminal abria: `failed to create process: This program is blocked by group policy`. A máquina bloqueia o PowerShell, que era o primeiro da lista.

- **Ordem no Windows:** Git Bash, `pwsh`, `powershell`, `cmd`. O Git Bash (`<Git>\bin\bash.exe`) é procurado ao lado do `git.exe` no PATH, em `%ProgramFiles%\Git` e em `%LOCALAPPDATA%\Programs\Git`. Um `bash` sem caminho nunca passa pelo PATH, porque cairia no `System32\bash.exe` do WSL, onde o `squad.exe` e os agentes do Windows não funcionam.
- **Git Bash** abre com `--login -i`, `CHERE_INVOKING=1` (o profile não troca para `$HOME`) e `MSYS2_PATH_TYPE=inherit` (mantém o PATH do Windows). As duas variáveis só entram se o usuário não as tiver definido.
- **`SQUAD_SHELL`** (todas as plataformas) passa à frente do padrão. O shell escolhido no nó continua tendo prioridade.
- **Fallback:** se o processo do shell não puder ser criado, o terminal tenta o próximo candidato. A linha do agente é montada com as aspas do primeiro candidato; o fallback só a quebra se cair até o `cmd` com um caminho entre aspas.
- **Testes:** `shell_test.go` simula máquinas Windows (Git em `cmd\`, em `mingw64\bin`, instalação por usuário, sem Git, `SQUAD_SHELL`, WSL no PATH); `TestFallsBackWhenShellCannotStart` usa um "shell" que não executa.

**Precisa de você (Windows):** confirmar que o Git Bash abre na pasta do workspace, que `squad list` funciona dentro dele e que o `claude` abre com os hooks (`/hooks`).

## 2026-10-15, A1 adaptador do Devin CLI

Feito como o Codex: segue a documentação oficial do Devin CLI (docs.devin.ai/cli) e foi testado só com o agente falso.

- **Tipo `devin`** em `internal/agents` (`Hooks: true`, comando padrão `devin`).
- **Hooks:** ao subir, o servidor grava `~/.squad/run/devin-hooks.json` com `{"hooks": {"Stop": …, "UserPromptSubmit": …}}` no formato do Claude Code, chamando `squad hook devin-stop` e `squad hook devin-prompt`. A linha de lançamento vira `devin --config <arquivo>`. O gerador do arquivo do Claude virou `writeHooks`, usado pelos dois.
- **Parser:** `devin-prompt` abre o turno; `devin-stop` usa o mesmo payload do `claude-stop` (`last_assistant_message`, enviado pelo Devin desde a v3000.5.20). O Devin não tem `StopFailure`.
- **Skill:** `squad install-skill -agent devin` grava em `~/.config/devin/skills/squad/` (`$XDG_CONFIG_HOME` se definido; `%APPDATA%\devin\skills` no Windows). O Devin não lê `~/.claude/skills`, só o `.claude/skills` do projeto.
- **Web:** "Devin" no seletor de agente, com nome padrão "Devin".
- **Testes:** parser e linha de lançamento em `agents_test.go`, `TestDevinHookRepliesWithFinalMessage` (agente falso com `FAKEAGENT_HOOK=devin`) e `TestInstallSkillDevin`.

**Decisão:** hooks injetados por `--config`, porque é o único jeito por terminal sem tocar na config do usuário nem no repositório dele. A documentação só diz "Configuration file path"; não diz se o arquivo **soma** com `~/.config/devin/config.json` ou **substitui** esse arquivo.

**Precisa de você (máquina com Devin):**
1. Abrir um terminal "Devin" no canvas e rodar `/hooks`: devem aparecer `Stop` e `UserPromptSubmit` vindos de `devin-hooks.json`.
2. Conferir se as suas configurações (modelo, permissões, tema) continuam valendo dentro do Squad. Se o `--config` **substituir** a config do usuário, o plano B é um plugin local do Devin (`hooks/hooks.json`) instalado uma vez com `devin plugins install`. Fora de um terminal do Squad, o `squad hook` não faz nada.
3. Ligar um Claude ao Devin e pedir `squad ask "Devin" "diga pong"`: a resposta deve vir como `[reply from Devin · request …]` com a mensagem final, não a tela. Se vier a tela, teste o encaminhamento à mão num terminal do Squad: `SQUAD_DEBUG=1 squad hook devin-stop '{"last_assistant_message":"x"}'` mostra o erro, se houver.
4. Mensagem com várias linhas: o envelope `[de: … · pedido …]` deve chegar inteiro na caixa do Devin e ser enviado com o Enter.
5. Provocar um erro (sem rede, por exemplo) e ver se o `Stop` dispara. Se não disparar, o pedido só termina no timeout; a correção seria aceitar o "idle" da tela para o Devin.
6. Na primeira execução numa pasta, o Devin pergunta se confia no workspace; responder no próprio terminal.
7. Com `--sandbox`, ver se o `squad ask` alcança `127.0.0.1` (o sandbox filtra rede).

## 2026-10-15, skill empacotada

- **`squad-skill.zip` na raiz:** a pasta `squad/` com o `SKILL.md`, gerada por `make skill-zip` a partir do próprio binário (`squad install-skill -dir bin/skill`). Assim o zip nunca diverge do texto que o `install-skill` grava. Se o texto da skill mudar em `cmd/squad/hook.go`, rode `make skill-zip` de novo.
- **Limpeza:** o binário `fakeagent` tinha sido commitado por engano na raiz no B3. Saiu do repositório e entrou no `.gitignore`. Os testes continuam compilando o agente falso a partir de `internal/testutil/fakeagent`.

## 2026-10-15, W4 web: Composer, notas e portal

- **Servidor: `POST /api/nodes/{id}/send`** (`internal/app/compose.go`). Entrega uma mensagem do Composer a um terminal como colagem + Enter, igual a digitar.
  - **Menções:** cada `@Nome` precisa estar ligado ao destino; senão, 400. A mensagem ganha uma linha final que diz ao agente o que é cada menção e como alcançá-la, por exemplo `[menções: @B = terminal ligado (squad ask "B" "…") · @Plano = nota ligada plano.md (squad note read "plano.md")]`.
  - **Imagens coladas** (data URL PNG, JPEG, GIF ou WebP): são salvas em `~/.squad/workspaces/<id>/attachments/` e o caminho vai no texto. O Claude Code anexa imagens referenciadas por caminho.
  - **Evento `message.sent`:** a gaveta mostra "Você → Implementador".
  - **Teste:** `TestComposerSend` confere o texto entregue pelo log do agente falso, os anexos, o evento e as recusas (menção não ligada, mensagem vazia, imagem inválida).
- **Composer** (⌥K ou clique na pílula; Esc fecha):
  - **Destinos:** chips "Para" com papel e agente, "+ destino" para enviar a vários ("Enviar a 2"), e aviso do estado do destino ("trabalhando · entra na fila"; "parado · inicie o terminal" bloqueia o envio).
  - **Texto:** `contentEditable` em que as menções viram chips inline (ícone de nota ou portal, quadrado da cor do papel para terminal).
  - **Menu @:** acima do Composer, com "Ligados a X" (só nós ligados a todos os destinos), filtro com o trecho sublinhado, nomes que começam com o texto digitado primeiro, ↑↓ e ⏎/Tab. "Não ligados" aparecem desabilitados, com "ligue para mencionar".
  - **Imagens:** colar (⌘V), soltar ou escolher arquivo; o cartão mostra miniatura e "1440×900 · colada".
  - **Envio:** ⏎ envia, ⇧⏎ quebra a linha; a colagem só aceita texto puro.
- **NoteNode:**
  - prévia em Markdown com renderer próprio, sem HTML injetado: títulos, listas, tarefas clicáveis, código, negrito, itálico e links http/mailto;
  - modo de edição (Prévia/Editar ou duplo clique), salvo 800 ms depois de parar de digitar e ao sair;
  - cadeado para travar e destravar (travada, ninguém escreve);
  - edição de um agente: o trecho alterado fica destacado na cor do papel do autor, com "Implementador escreveu há 4 s", por 30 s. Se o arquivo muda enquanto você edita, aparece "X alterou a nota · Recarregar" em vez de sobrescrever o rascunho.
- **PortalNode:** endereço editável (`PATCH portal.url`), botão para abrir no navegador e "sem transmissão". A imagem ao vivo via CDP fica para depois.
- **ComposerPill:** mira só em terminais (foco, seleção ou último destino).
- **Teste com eventos reais via CDP:**
  - menu @ com "Ligados a Planejador", chip, envio entregue ao agente falso e toast "Enviado a Planejador";
  - `squad note edit` vindo de um agente, que destacou a linha;
  - edição pela UI salva no `.md` e tarefa marcada com clique.

**Decisão:** como o agente recebe as menções. No Maestri elas viram tags próprias; no Squad o texto mantém o `@Nome` legível e ganha uma linha final de contexto com o comando que alcança cada nó. Não há formato novo para o agente aprender, e a skill do Squad já ensina `squad ask` e `squad note`.

## 2026-10-15, W3 web: fios e conversa

- **A conversa no fio, dirigida pelos eventos reais** (`request.sent`, `reply.sent`, `reply.received`, `request.cancelled`):
  1. **Viajando:** o fio acende em âmbar da origem ao destino (`pathLength` = 100, traço animado em 600 ms) e o envelope "✉ id →" percorre a curva (CSS `offset-path`).
  2. **Pendente:**
     - fio tracejado em âmbar; a alça da origem fica cheia e a do destino ganha um anel;
     - pílula com ampulheta, cronômetro e id no ponto mais baixo da curva.
  3. **Resposta:** o fio acende em verde do destino à origem, com o envelope "← resposta id".
     - Uma resposta que vem direto de um hook ou da tela estável chega só como `reply.received` (sem `reply.sent`); a animação de volta dispara pelos dois.
  4. **Recebida:** o marcador "✓ id · 0:07" some em cerca de 4 s.
  - Um pedido cancelado mostra "⊘ id cancelado".
  - Com `prefers-reduced-motion`, as animações vão direto ao estado final.
- **Faixas de remetente**, acima do terminal e nunca por cima:
  - "Pedido id de X · a resposta volta para ele", na cor do papel de X;
  - "Aguardando Y · pedido id · enviado há N s";
  - "Resposta de Y · id · 7s", que fica 60 s ou até o próximo pedido.
  - Quando a faixa aparece, a grade do terminal se reajusta (`ResizeObserver` no slot).
- **Pausa e laço:**
  - com as conversas pausadas, cada fio mostra um selo ⏸;
  - ao atingir o limite de saltos, a gaveta registra "Laço: limite de N saltos atingido… Conversas pausadas até você retomar".
- **Pontos de curva:**
  - duplo clique no fio adiciona um ponto onde ele menos alonga a curva;
  - o ponto pode ser arrastado (fio selecionado); duplo clique no ponto o remove;
  - a curva passa pelos pontos (`PATCH /api/edges/{id}`).
- **Taxa de pedidos:** "N/30 pedidos por min" agora é uma janela deslizante de 60 s no cliente (antes só subia).
- **Rótulos no fio:** num fio entre nós empilhados, o ponto mais baixo cai sobre o nó, então as pílulas vão para o meio da curva. O "Desligar" se afasta de pontos de curva próximos.
- **`tools/shot`:** ganhou os passos `dblclick` e `shot FILE` (capturas intermediárias na mesma página).
- **Teste:** uma conversa real entre dois agentes falsos (`fakeagent` com o hook do Claude):
  - o Planejador roda `squad ask Implementador …`, e o Implementador trabalha por 7 s e responde pelo hook `claude-stop`;
  - as quatro fases, as faixas, os estados (Esperando resposta → Trabalhando/Terminou), a caixa e a gaveta ("Respondido em 0:07") foram capturados quadro a quadro.

## 2026-10-15, W2 web: nó de terminal

- **Sessão de terminal (`web/src/term/session.ts`):**
  - cada terminal tem uma instância do xterm.js e um WebSocket `/ws/term/{id}`, guardados num registro fora do React; trocar o nível de zoom não derruba o buffer nem a conexão;
  - na mensagem `init`, a tela é limpa e redimensionada para o tamanho do PTY; a tela atual chega em seguida;
  - a reconexão é automática e fica imediata quando o terminal volta a rodar (iniciar ou reiniciar);
  - a entrada vai em binário;
  - a sessão é descartada quando o nó sai do workspace.
- **Texto nítido e mouse correto em qualquer zoom (counter-scale):**
  - o xterm não aceita ancestrais com `transform: scale` (o mouse cai na célula errada), e o React Flow aplica o zoom com um `scale`;
  - por isso, o slot do terminal é dimensionado `zoom` vezes maior e reduzido com `scale(1/zoom)`, com fonte 11 × zoom. A escala efetiva fica 1;
  - a métrica é ajustada pela grade medida: linha de 15 × zoom (handoff 11/15), sem estourar o slot;
  - o layout é refeito quando o gesto de zoom termina; durante o gesto, o conteúdo fica congelado, como pede o handoff.
- **Três níveis de zoom, com histerese de ±0,03:**
  - vivo (≥ 0,6), com o xterm;
  - miniatura (0,35–0,6), com um snapshot em HTML colorido (`serializeAsHTML`) a cada 3 s e as legendas em pixels de tela;
  - cartão (< 0,35), com nome, agente, papel, caixa e a banda de estado, em 240 px de design reduzidos ao espaço do nó.
- **Foco:**
  - um clique dá o teclado ao terminal, com anel e "teclado aqui";
  - ⌥Esc devolve o teclado ao canvas;
  - ⌥A, ⌥P e ⌥K continuam globais mesmo com o terminal focado.
- **Roda e pinça:**
  - a roda simples rola o terminal sem mover o canvas;
  - pinça ou ⌘/ctrl + roda sobre o terminal dão zoom no canvas em torno do ponteiro (antes, o navegador dava zoom na página).
- **Tamanho:**
  - o nó tem alças de redimensionar quando está selecionado;
  - ao soltar, o cliente calcula cols×rows pela métrica exata da fonte e envia `PATCH` com `w`, `h` e `terminal.cols/rows`;
  - um terminal sem tamanho salvo recebe a grade que cabe no nó: 81×24 em 560×440.
- **Servidor:** um `PATCH` que muda `terminal.cols/rows` redimensiona o PTY em execução. Todos os clientes seguem pelo `node.updated` (teste `TestPatchTerminalSizeResizes`).
- **Também no nó:**
  - caixa de pedidos pendentes no rodapé e no cartão;
  - aviso de processo encerrado ou com erro sobre o terminal;
  - menu com iniciar/parar, reiniciar, papel e remover.
- **`tools/shot`:** ganhou `-steps`, com eventos reais via CDP (clique, arraste, texto, teclas, roda com modificadores, eval). Os testes usaram esses eventos:
  - digitação no shell, com acentos, CJK e cores;
  - seleção com o mouse a 80% em DPR 1 e 2;
  - zoom por pinça sobre o terminal;
  - miniatura e cartão;
  - redimensionamento até 104×31, confirmado com `tput cols/lines` dentro do PTY.

**Decisões:**
- **Renderer DOM do xterm, não WebGL.** O addon WebGL mede o próprio canvas com um `ResizeObserver` (`device-pixel-content-box`), e o Chrome informa esse tamanho errado quando há um ancestral com `scale`. Em DPR 2, o canvas saiu com metade da resolução e o terminal aparecia com o dobro do tamanho, cortado. O renderer DOM não tem esse problema e, com o counter-scale, fica nítido. Se o desempenho com muitos terminais pedir WebGL, o caminho é uma camada sobreposta sem `transform`, posicionada sobre os nós. A dependência `@xterm/addon-webgl` foi removida.
- **⌥Esc, e não Esc duas vezes, para sair do terminal.** O Claude Code usa Esc Esc (voltar a uma mensagem anterior), então o atalho do handoff tomaria a tecla do agente.
- **O tamanho do PTY é dado pelo nó:** quem define cols×rows é o cliente que redimensiona, e o servidor só aplica. Vários navegadores abertos seguem o mesmo valor pelo evento, sem disputa.

## 2026-10-15, W1 web: shell e canvas

- **Projeto `web/`:**
  - React 19, `@xyflow/react` 12, zustand 5 e Vite 8, com versões fixas no `package.json`;
  - o build vai direto para `internal/webui/dist`, que é embutido no binário;
  - `make web` refaz a UI.
- **Tema:**
  - os tokens do handoff foram copiados, e as fontes Geist, Geist Mono e Material Symbols vão no pacote, sem Google Fonts;
  - `data-theme` (escuro/claro) e `data-term` (noite/papel) ficam no `<html>`;
  - trocar o tema também troca a paleta do terminal, que continua ajustável no menu.
- **Estado (`src/state/store.ts`):**
  - o primeiro evento de `/ws/events` (`hello`) traz o estado completo;
  - depois, os eventos atualizam nós, arestas, runtime, pendências, voos (para o W3) e o registro de atividade;
  - a reconexão é automática, e a TopBar mostra "reconectando…".
- **Componentes:**
  - TopBar com workspace, andar "main", taxa, pausar/retomar, atividade, servidor e menu de tema;
  - Toolbar com as ferramentas Selecionar/Mover e os botões de criar terminal, nota e portal; desenho e grupo ficam desabilitados;
  - formulário de novo terminal, com nome único automático ("Claude 2");
  - minimapa com as cores de estado e controles de zoom (os dois se deslocam quando a gaveta abre);
  - ComposerPill;
  - ActivityDrawer com filtros por terminal, desfechos e limites, e a aba vertical quando fechada.
- **Canvas:**
  - React Flow com grade de pontos de 20 e zoom inicial de 80%; o viewport é lembrado no navegador;
  - nós arrastáveis pelo cabeçalho, com a posição salva via `PATCH`;
  - para ligar, arrasta-se a partir de uma alça lateral e solta-se em qualquer ponto do outro nó. Os destinos válidos ganham anel e os inválidos ficam a 40%;
  - Delete apaga o fio selecionado, que também mostra um botão "Desligar";
  - um toast avisa "A ↔ B ligados/desligados".
- **Nós:**
  - o TerminalNode tem faixa de papel, cabeçalho (nome, agente, papel, "teclado aqui", pílula de estado com cronômetro e menu com iniciar/parar, reiniciar, papel e remover) e rodapé; o corpo ainda é provisório, e o xterm entra no W2;
  - NoteNode mostra o texto da nota;
  - PortalNode é uma casca.
- **WireEdge:** âncoras nos lados que se olham, curva com caimento de 0,35 × distância (máximo de 120) e passagem pelos `ropePoints` (Catmull-Rom).
- **`tools/shot`:**
  - virou um módulo Go próprio, e o chromedp saiu do `go.mod` do squad;
  - agora imprime os erros e o console da página.
- **Teste:** `squad serve` isolado (com `SQUAD_HOME` no scratchpad) mais a disposição da referência via API. As capturas foram comparadas com o `Squad.dc.html`. Um `squad ask` real entre dois shells apareceu como pendente na gaveta, com o Planejador em "Esperando resposta".

**Decisões:**
- **Build da UI commitado:** o `dist` vai para o git, para que `go build` e `go install` saiam com o canvas sem Node.
  - A fonte Material Symbols (4 MB) é commitada uma vez só, porque o nome do arquivo leva o hash.
  - Dá para reduzir a fonte depois, com um subconjunto dos ícones usados.
- **Atalhos:** o handoff usa ⌥P tanto para "pausar conversas" quanto para "criar portal". Ficou assim:
  - ⌥ para os atalhos globais (⌥A gaveta, ⌥P pausar, ⌥K composer);
  - letras simples para as ferramentas do canvas (V, H, T, N, P), como mostram os tooltips da Toolbar;
  - as letras simples não agem quando o foco está num campo ou no terminal.
- **Remover nós:** só pelo menu do nó, com confirmação, porque remover um terminal encerra o processo. A tecla Delete apaga só fios.
- **Seletores do zustand 5 devem devolver referências estáveis:** um `?? []` dentro do seletor causava um laço infinito de renderização (React #185). A correção foi o `useNodes()`, com uma lista vazia constante.

## 2026-10-15, B5 notas

- **`internal/notes`:**
  - cada nota é um `.md` em `~/.squad/workspaces/<id>/notes/`;
  - **nome estável por padrão**: um slug do título com sufixo `-2`, `-3`… em colisões, sem renomear pela primeira linha como o Maestri faz;
  - um watcher (`fsnotify`) publica `note.edited` com `author: external` para edições fora do Squad, e as escritas pelo Squad publicam o nome do autor (agente ou `user`).
- **CLI:** `squad note list|read|write|edit|create|help`.
  - Só alcança notas ligadas ao terminal, inclusive cadeias nota↔nota.
  - O `read` sai numerado, com `[N lines total]` e intervalo opcional.
  - O `edit` exige trecho **único** (409 se ausente ou repetido).
  - O `create` liga a nota ao criador e a posiciona à esquerda dele.
  - Uma nota bloqueada dá 423.
- **API para a interface:** `GET/PUT /api/notes/{id}`; nós de nota criados por `POST /api/nodes` recebem o arquivo automaticamente.
- **README:** início rápido para testar sem a interface (curl na API).

**Decisão:** apagar o nó de uma nota **não apaga** o arquivo `.md`. É não destrutivo; fica para a interface oferecer "apagar também o arquivo".

## 2026-10-15, B4 adaptadores de agentes

- **`internal/agents`:**
  - **Claude Code:** o Squad gera `~/.squad/run/claude-hooks.json` com os hooks `Stop`, `StopFailure` e `UserPromptSubmit` chamando `squad hook …`, e a linha de lançamento recebe `--settings <arquivo>`. Os hooks de vários arquivos de settings são somados, então os do usuário (por exemplo, o RTK) continuam.
  - **Codex:** `-c notify=["<squad>","hook","codex-notify"]`, com o evento `agent-turn-complete` e `last-assistant-message`. **Não testado com conta real**, só com payload de exemplo.
  - **Gemini, shell e comandos próprios:** só a heurística de tela.
- **Mensagem final:** vem de `last_assistant_message` do hook `Stop`. Em versões antigas, a reserva é a última mensagem do transcript.
- **Monitor:**
  - `SignalStart` (o turno começa pelo hook) e `Signal` (o turno termina pelo hook);
  - um turno iniciado por hook só termina pelo hook, ou depois de 60 s sem mudança na tela;
  - para agentes com hooks, o barramento ignora o fim de turno detectado pela tela e desliga o fallback de tela estável.
- **`squad hook <nome> [payload]`:** repassa o payload ao servidor e nunca falha de forma visível, para não atrapalhar o agente.
- **`squad install-skill`:** instala `~/.claude/skills/squad/SKILL.md` (escrita por mim) com os comandos, o envelope e o `reply`.
- **Teste real com Claude Code** (`go test -tags e2e -run TestRealClaude ./internal/app/`): o Shell perguntou ao Claude e recebeu `pong` em 6,4 s, pelo hook `claude-stop`, sem lixo de tela.

**Fica para depois:** `squad mcp` (servidor MCP), priorizando a web.

**Precisa de você:**
- testar com o Codex quando houver conta (`agent: codex`);
- decidir se a skill deve ser instalada automaticamente no primeiro `serve`. Hoje é manual (`squad install-skill`), para não mexer em `~/.claude` sem pedir.

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
