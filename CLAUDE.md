# Torre: guia para sessões do Claude Code

A Torre (antes chamada Squad) é um canvas (Go + React) onde agentes de código (Claude Code, Codex, Devin, Gemini) rodam em terminais reais e conversam pelos fios. Antes de começar qualquer etapa, leia:

- `docs/PLAN.md`: as etapas, o status de cada uma e o plano da segunda rodada (R1–R9).
- `docs/DEVLOG.md`: o que foi feito em cada etapa e por quê. Entradas novas vão no topo.
- `README.md`: como usar.
- `docs/01…05` (investigação e arquitetura), quando a etapa pedir.
- **Design:** `docs/torre/` (a identidade atual: `RENOMEACAO.md` com o vocabulário da interface, `TELAS.md`, `NOVIDADES-E-BACKEND.md`, tokens e mockups). O `docs/design_handoff_squad/` é o design anterior.
- **Nomes:** o produto, o binário, as pastas e as variáveis se chamam Torre (`torre`, `~/.torre`, `TORRE_*`). O repositório e o módulo Go continuam `squad`, e os identificadores internos do código também ficam (`floor`, `node`, `edge`, `role`, `manager`, `useSquad`). A interface usa o vocabulário da torre de controle (voo, frequência, nível, pista, pousar…).

## Como trabalhamos

- **Idioma:**
  - **Português:** com o usuário, nos PRs, no README e no DEVLOG.
  - **Inglês:** no código, nos comentários, nas mensagens de commit e nas mensagens da CLI.
- **Etapas grandes:** traga o plano detalhado primeiro e espere o "pode começar". Para uma decisão que a documentação não cobre, escolha a opção reversível, registre no DEVLOG e siga. Pergunte só o que é do usuário decidir.
- **Branch:** uma por etapa, a partir da `main` (`feat/<nome>`). Os commits vão em partes: servidor (Go), web (`web/src`) e, por último, o build da web com a documentação.
- **Ao terminar:**
  - abra um PR para o usuário aprovar. O corpo é em português, com "O que muda", "Testes" e "Não testado";
  - ele faz o merge. Não faça merge e não crie tags (o ambiente recusa push de tag);
  - o mesmo PR atualiza o README (uso), o DEVLOG (entrada nova) e o PLAN (status).
- **Na mensagem final ao usuário:**
  - o que mudou, o que foi testado de verdade e o que não foi;
  - como testar depois do merge: `GONOPROXY=github.com/nathanribeiroo/squad go install github.com/nathanribeiroo/squad/cmd/...@main` (instala o `torre` e o atalho `squad`; sem o `GONOPROXY`, o proxy do Go pode entregar a versão anterior por um bom tempo), `torre install-skill` (instala no Claude e no Devin, e registra o MCP no Devin; a primeira linha mostra a versão) e reiniciar o `torre serve`;
  - capturas de tela das mudanças de interface.
- **O usuário:** usa Windows com Git Bash (a política da empresa bloqueia o PowerShell), Claude Code e Devin. Testa o Claude no Mac e no Windows.
- **Todos os agentes:** toda funcionalidade vale para o Claude e o Devin, e para o Codex e o Gemini quando der.
  - Ao planejar e ao testar, passe pelo caminho de cada agente: lançamento, hooks, papel, MCP, permissões, skill.
  - O que ficar de fora para algum agente vai no PR, em "Não testado" ou numa nota.
  - O caminho do Devin se confere com o binário real (veja "Devin real", abaixo), não só com o agente falso.

## Validação

- **Go:** `make check` faz vet (Linux e Windows), testes e builds. Os testes do navegador precisam de `TORRE_BROWSER=/opt/pw-browsers/chromium`; sem ele são pulados.
- **Web:** `cd web && npx tsc -b && npm run build`. O build vai para `internal/webui/dist`, que fica no repositório.
- **Ícones:** a fonte tem só os ícones listados em `web/src/lib/icons.ts`. Depois de mudar a lista, rode `make icons`, que precisa de `pip install fonttools brotli` (num venv, se o Python do sistema não deixar). O `npm run build` falha se a fonte estiver desatualizada.
- **Skill:** o texto fica em `skillMD`, em `internal/cli/hook.go`. Depois de mudar, rode `make skill-zip`. Se só o texto da skill não mudou, o zip muda apenas nos horários; nesse caso, desfaça com `git checkout torre-skill.zip`.
- **Integração com agentes falsos:** `internal/testutil/fakeagent`, com os helpers `newEnv`, `agent`, `agentOfType` e `run` em `internal/app/app_test.go`.
- **Claude Code real:** `go test -tags e2e -run TestRealClaude ./internal/app/`. O Claude está logado no container. O teste passa pela tela de boas-vindas e pela pergunta de confiança na pasta.
- **Devin real (sem login):**
  - instale numa pasta isolada: baixe `https://cli.devin.ai/install.sh`, leia, e rode com `HOME=<pasta temporária> bash install.sh`. O binário fica em `<pasta>/.local/bin/devin`;
  - sem login não há sessão, mas `devin --help`, `devin mcp list`, `devin skills paths` e `devin --config <arquivo> mcp list` funcionam;
  - `PATH=<pasta>/.local/bin:$PATH go test -tags e2e -run TestRealDevinConfig ./internal/agents/` confere que o Devin lê a cópia da configuração e vê o MCP `torre`;
  - o `--config` do Devin **substitui** o `config.json` do usuário, e servidores MCP só valem no `mcp_config.json`.
- **Interface de ponta a ponta:**
  - compile o binário para uma pasta temporária com o nome **`torre`**: o terminal põe a pasta do binário no PATH, e com outro nome os agentes não acham o `torre`;
  - suba com `TORRE_HOME=<pasta temporária> TORRE_BROWSER=/opt/pw-browsers/chromium <pasta>/torre serve -dir /home/user/squad -port 78xx -no-open`;
    - **`TORRE_HOME`** separado isola os dados da Torre sem mexer no `HOME`, então o login do Claude continua valendo;
    - **`/home/user/squad`** como pasta do workspace já é confiada pelo Claude, então ele não pergunta;
  - crie nós e fios pela API (`POST /w/<ws>/api/nodes`, `/api/edges`, com `Authorization: Bearer <token>`; o token sai no log do `serve`);
  - para agir como um terminal, leia as variáveis `TORRE_*` do processo bash dele em `/proc/<pid>/environ` e rode o `torre` com elas. Assim dá para usar `torre ask` ou dar uma tarefa a um Claude real.
- **Janela de app (`-open app`, o padrão do `serve`):** suba um display virtual (`Xvfb :99 -screen 0 1600x1000x24 &`) e rode o `serve` com `DISPLAY=:99`. Como root, o Chromium precisa de `--no-sandbox`: aponte o `TORRE_BROWSER` para um script que chama `/opt/pw-browsers/chromium --no-sandbox --test-type "$@"`. A captura sai com `DISPLAY=:99 import -window root out.png`. Sem gerenciador de janelas, não há barra de título.
- **Capturas de tela:** use um programa Go pequeno com `chromedp` (já é dependência), com o Chromium em `/opt/pw-browsers/chromium`. Ele abre `http://127.0.0.1:<porta>/?token=…&ws=…`, executa passos (clicar, duplo clique, digitar, teclas, roda com ctrl para zoom, `eval`, esperar) e salva PNGs.
  - Ele não está no repositório: recrie no scratchpad.
  - Em 1440×900, o botão de ajustar à tela fica perto de (1405, 867).
- **Armadilhas:**
  - `pkill -f "<padrão>"` também mata o seu próprio shell quando o padrão aparece na linha de comando. Mate por PID (`ps -eo pid,args | awk …`);
  - `go build ./cmd/torre` na raiz deixa um binário `torre` solto: use `-o` ou apague antes de commitar;
  - os testes de `internal/app` têm `//go:build !windows`. Um arquivo de teste novo ali precisa da mesma linha, ou o vet do Windows quebra.

## Mapa do código

| Onde | O quê |
|---|---|
| `cmd/torre`, `internal/cli` | CLI: `serve` (e a janela de app, `window.go`), comandos de terminal (`list`, `ask`, `note`, `portal`, `new`…), `hook`, `mcp`, `install-skill`. `cmd/squad` é o mesmo comando com o nome antigo |
| `internal/hub` | vários workspaces, `/api/workspaces`, `/api/roles`, `/api/agents`, `/api/plans`, `/ws/workspaces`, roteamento de `/cli` e `/hooks`; as sessões de cada workspace, a migração e a troca (`sessions.go`); a primeira abertura (`welcome.go`) |
| `internal/app` | monta um workspace: servidor, barramento, monitor de atividade, notas, portais, equipe; limites (`limits.go`), salvar e decolar planos de voo (`plans.go`), as rotas da pilha de NOTAMs (`stacks.go`) |
| `internal/plans` | planos de voo em `~/.torre/plans`: validar, salvar, importar |
| `internal/attention`, `internal/apierr` | PRIORIDADE: ler o prompt na tela e as teclas de cada resposta (permissões; as opções das perguntas e dos planos, com o campo de texto); erros da API com `code` (traduzidos em `web/src/api/errors.ts`) |
| `internal/server` | API HTTP e WebSocket, terminais, eventos; `AddNode`, `AddEdge` e `RemoveNode` servem a API e os agentes |
| `internal/store` | `workspace.json` (nós, fios, `createdBy`), `FreeSpot`; pilhas de NOTAMs (`stacks.go`: a base guarda os fios e a caixa); grupos e desenhos, só visuais (`visual.go`: o que está dentro de uma moldura, mover vários nós) |
| `internal/agentbus` | `torre list/ask/check/wait/reply`, limites, pausa, modo `Safe`; a fila de pedidos de cada voo e a entrega a quem está ocupado num laço (`queue.go`); o motivo da espera geral e a cadeia de um laço que passou do limite de saltos (`loop.go`) |
| `internal/agents` | linhas de lançamento, hooks, papel e MCP de cada agente, `SafeMCPTools`; a conversa de cada terminal, nova ou retomada (`conversation.go`); a cópia da configuração do Devin e o registro do MCP nele (`devin.go`) |
| `internal/roles`, `internal/team` | biblioteca de papéis; `torre new/connect/close` (gerente) |
| `internal/portal`, `internal/browser` | portais (CDP com chromedp): transmissão, `torre portal` |
| `internal/summary` | cartões de fim de turno e resumos (`claude -p`, `devin -p`, `codex exec`, `gemini -p`) |
| `internal/radio` | o Rádio de cada sessão: as linhas montadas dos eventos, em `radio.jsonl` na pasta da sessão, com o texto dos pedidos e das respostas |
| `internal/usage` | Consumo: o que cada voo gastou e os limites da conta de cada agente, lidos dos arquivos dos agentes; a status line do Claude (`claude.go`, e `internal/cli/statusline.go`, que roda a do usuário), o `sessions.db` e a tela do `/usage` do Devin (`devin.go`), o Codex e o Gemini; as contas, as mesmas em todos os workspaces (`usage.go`) |
| `internal/floors` | andares: git worktree, cópia e preparo, aterrissar (merge), descartar, `torre floor`, `/api/floors`; a cópia em que uma sessão trabalha, e mesclar sem fechar (`copy.go`); as permissões do SEMPRE do Claude e do Devin que seguem os níveis e voltam no pouso (`permissions.go`); as marcas de testes e revisão dos voos, `torre floor mark` (`checks.go`) |
| `internal/terminal`, `pty`, `screen`, `activity`, `notes` | terminais, PTY e ConPTY, emulador, estados, notas |
| `web/` | React + Vite + React Flow; estado em `src/state/store.ts`, canvas em `src/canvas` (o Desenho em `DrawLayer.tsx`; grupos e desenhos em `src/lib/visual.ts`), sessões em `src/components/SessionsMenu.tsx`, o Consumo em `src/components/UsageMenu.tsx` |

## Segurança (já decidido)

- **CLI:** os comandos `torre` pedem aprovação no Claude Code. Não libere `Bash(torre *)` (nem `Bash(squad *)`): `torre new --agent shell --command …` rodaria qualquer coisa.
- **MCP:** as ferramentas de `agents.SafeMCPTools` vêm liberadas. Nas chamadas delas, o servidor recusa tudo o que rodaria um comando. `keys`, `new_shell`, `portal_eval` e `floor_land` pedem aprovação.
- **Só da interface** (nunca pelo `/cli` nem pelo MCP): responder a PRIORIDADE (e o hook `PermissionRequest` nunca devolve uma decisão), os limites, o comando padrão dos agentes, cancelar pedidos, salvar e decolar planos de voo. Decolar um plano exige a lista dos comandos próprios que a interface mostrou.
- **Shells:** nunca digite texto num shell automaticamente. Por isso shells e comandos próprios não recebem as instruções do papel.

## Pendências

- **Validação no Windows real:** veja os "Precisa de você" no DEVLOG.
- **Consumo em contas de empresa:** a análise e o que falta estão no PLAN, na seção "Consumo em contas de empresa (para discutir)". O usuário vai trazer a tela do `/usage` do Devin da empresa e o tipo de conta do Claude antes do plano.
- **Próximo passo:** a Torre (F0–F5d, no PLAN) terminou. O que ficou de fora está em "Para depois", na seção Torre do PLAN; o que vem agora é decisão do usuário.
