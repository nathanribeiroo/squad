# Squad

Ferramenta local para orquestrar agentes de código (Claude Code, Codex, Devin, Gemini) e shells num canvas infinito. Os terminais conversam entre si pelas conexões que você desenha. O servidor é um binário Go que escuta em `127.0.0.1`, e a interface React abre no navegador.

> Estado: backend e interface web da primeira rodada prontos (B0–B5, W1–W4): terminais vivos no canvas, conexões, conversa entre agentes animada nos fios, Composer com @menções, notas em Markdown, a gaveta de atividade e vários workspaces. Ficam para depois: andares (git worktree), portal com navegador ao vivo, MCP e validação no Windows real. Plano e andamento: [`docs/PLAN.md`](docs/PLAN.md) e [`docs/DEVLOG.md`](docs/DEVLOG.md).

## Requisitos

- Go 1.26+ (para compilar).
- Node 22+ (só para compilar a interface web).
- Python com `fonttools` e `brotli` (só para regenerar a fonte de ícones, `make icons`).
- macOS e Linux testados. No Windows, o básico foi testado numa máquina real (terminais no Git Bash, `squad list`, hooks do Claude Code).

## Instalação

Com Go 1.26+:

```sh
go install github.com/nathanribeiroo/squad/cmd/squad@latest   # ou @v0.1.0
```

O binário vai para `$(go env GOPATH)/bin` (no Windows, `%USERPROFILE%\go\bin`) e já traz a interface web embutida, sem precisar de Node.

## Início rápido

```sh
make build                       # gera bin/squad
bin/squad install-skill          # opcional: ensina o Claude Code a usar o Squad (~/.claude/skills/squad)
bin/squad install-skill -agent devin   # idem para o Devin CLI (~/.config/devin/skills/squad)
bin/squad serve -dir ~/meu-projeto
```

A mesma skill vem empacotada em [`squad-skill.zip`](squad-skill.zip) (pasta `squad/` com o `SKILL.md`), gerada por `make skill-zip`. Ela só faz sentido onde o comando `squad` existe, ou seja, dentro dos terminais do Squad.

O `serve` mostra uma URL com token e abre o navegador. Os dados ficam em `~/.squad` (troque com `SQUAD_HOME`) e a porta padrão é 7420 (`-port` ou `SQUAD_PORT`).

**Workspaces:** cada pasta de projeto é um workspace, com o seu canvas, terminais e notas (`~/.squad/workspaces/<id>/`).
- `squad serve -dir <pasta>` abre o workspace da pasta (`.` se omitida) e o cria na primeira vez.
- Com o servidor já rodando, o mesmo comando abre ou cria o workspace naquele servidor, em vez de subir outro. Assim, `cd outro-projeto && squad serve` funciona a qualquer momento.
- Todos os workspaces ficam rodando: trocar de workspace na interface só troca o que a tela mostra, e os agentes dos outros continuam trabalhando.

### Shell dos terminais

Cada terminal abre um shell, e o agente é digitado dentro dele. A ordem de escolha é:

- **macOS/Linux:** `$SHELL` (login), `zsh`, `bash`, `sh`;
- **Windows:** **Git Bash** (achado ao lado do `git.exe` ou em `Program Files\Git`; nunca o `bash.exe` do WSL), `pwsh`, `powershell`, `cmd`.

A variável `SQUAD_SHELL` passa à frente dessa lista (ex.: `SQUAD_SHELL=pwsh.exe` ou um caminho completo). Se um shell existir mas não abrir, por exemplo o PowerShell bloqueado por política de grupo, o terminal tenta o próximo da lista.

### Interface

A interface já vem embutida no binário. Para mexer nela:

```sh
make web                                      # npm ci + build → internal/webui/dist (commitado)
cd web && SQUAD_TOKEN=<token> npm run dev     # Vite com proxy para o servidor em 7420
```

**Ícones:** a fonte Material Symbols é cortada para os ícones listados em `web/src/lib/icons.ts` (cerca de 9 KB em vez de 4 MB). Para usar um ícone novo, acrescente o nome nessa lista e rode `make icons`. O `npm run build` recusa uma fonte desatualizada.

**Atalhos:**
- canvas: `V` selecionar, `H` mover, `T` novo terminal, `N` nota, `P` portal;
- globais: `⌥A` gaveta de atividade, `⌥P` pausar conversas, `⌥K` composer.

**Conexões:** arraste a partir da bolinha lateral de um nó e solte sobre outro nó. Um clique no fio o seleciona; para desligar, use Delete ou o botão "Desligar". Duplo clique no fio cria um ponto de curva, que pode ser arrastado; duplo clique no ponto o remove.

**Terminais:**
- no formulário de criação (`T`), **Pasta** aceita um caminho absoluto, `~/…` ou relativo à pasta do workspace, com sugestões enquanto você digita (↑/↓, Enter, Tab completa). **Shell** lista os shells instalados; "Padrão" segue a ordem descrita em "Shell dos terminais";
- **Configurar…**, no menu `⋯` do terminal, edita esses campos; mudar o que o inicia (agente, comando, pasta ou shell) salva e reinicia o terminal;
- clique para digitar ("teclado aqui"); `⌥Esc` devolve o teclado ao canvas;
- arraste pelo cabeçalho e redimensione pelas alças (o PTY acompanha);
- com zoom abaixo de 60% vira miniatura, e abaixo de 35%, cartão;
- a pinça ou ⌘/ctrl + roda sobre o terminal dá zoom no canvas.

**Composer (`⌥K`):**
- escreva para um ou mais terminais;
- `@` menciona nós ligados ao destino;
- cole imagens com ⌘V;
- `⏎` envia.

**Workspaces:**
- o botão no topo, à esquerda, abre o seletor: cor, nome, pasta, agentes ativos e pedidos pendentes de cada um. Um ponto azul no botão indica atividade em outro workspace;
- `⌥1`–`⌥9` trocam de workspace, e a URL guarda o workspace aberto (`?ws=<id>`), então dá para abrir dois em abas diferentes;
- "Novo workspace" pede a pasta (com sugestões), um nome opcional e a cor. O lápis de cada linha edita esses campos; mudar a pasta vale para terminais novos ou reiniciados;
- "Remover workspace" encerra os terminais dele e move o canvas e as notas para `~/.squad/trash/`. O último workspace não pode ser removido;
- `squad ask`, `list` e `check` só enxergam terminais do mesmo workspace.

**Notas:** Prévia/Editar no cabeçalho; as tarefas podem ser marcadas com clique; o cadeado trava a nota para todos. Quando um agente edita a nota, o trecho alterado fica destacado.

### Testar a conversa sem a interface (pela API)

Em outro terminal, com o servidor rodando:

```sh
TOKEN=$(python3 -c "import json,os;print(json.load(open(os.path.expanduser('~/.squad/run/server.json')))['token'])")
API="curl -s -H Authorization:Bearer\ $TOKEN -H Content-Type:application/json"
# a API de cada workspace fica em /w/<id>/; aqui, o primeiro da lista
WS=$($API http://127.0.0.1:7420/api/workspaces | python3 -c 'import json,sys;print(json.load(sys.stdin)[0]["id"])')

# dois terminais: um shell e um Claude Code
SHELL_ID=$($API -d '{"kind":"terminal","terminal":{"name":"Shell","agent":"shell"}}' http://127.0.0.1:7420/w/$WS/api/nodes | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
CLAUDE_ID=$($API -d '{"kind":"terminal","terminal":{"name":"Claude","agent":"claude"}}' http://127.0.0.1:7420/w/$WS/api/nodes | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')

# ligar os dois
$API -d "{\"a\":\"$SHELL_ID\",\"b\":\"$CLAUDE_ID\"}" http://127.0.0.1:7420/w/$WS/api/edges
```

Para digitar no terminal "Shell", conecte um cliente WebSocket em `/w/$WS/ws/term/<id>`. Até a interface ficar pronta, o jeito mais simples é usar o próprio Claude: peça a ele, no terminal dele, que rode `squad list` e `squad ask "Shell" --raw "ls\n"`.

## Como a conversa funciona

Dentro de um terminal do Squad, o comando `squad` está no PATH:

| Comando | O que faz |
|---|---|
| `squad list` | quem você é e o que está ligado a você |
| `squad ask "Nome" "mensagem"` | entrega a mensagem (com o envelope `[de: Você · pedido 7f3a]`) e espera a resposta |
| `squad ask "Nome" --raw "teclas"` | digita teclas (`\n` Enter, `\x03` Ctrl-C…) e devolve a tela |
| `squad ask --batch '{"A":"…","B":"…"}'` | vários pedidos em paralelo |
| `squad check "Nome"` | lê a tela de um terminal ligado |
| `squad reply <id> "…"` | responde o pedido `<id>` |
| `squad note list\|read\|write\|edit\|create` | notas ligadas a você |

Com o Claude Code, o Codex e o Devin CLI, a resposta é a **mensagem final do turno** (via hook `Stop` ou `notify`), não a tela. Com outros programas, o Squad espera a tela parar de mudar.

O Devin é lançado com `devin --config ~/.squad/run/devin-hooks.json`, um arquivo só com os hooks `Stop` e `UserPromptSubmit` do Squad. A integração segue a documentação oficial e ainda não foi testada com uma conta real (veja `docs/DEVLOG.md`).

## Testes

```sh
make check                                                   # vet (incl. Windows), testes, builds
go test -tags e2e -run TestRealClaude -v ./internal/app/     # usa o Claude Code real (1 prompt mínimo)
```

## Estrutura

| Pasta | Conteúdo |
|---|---|
| `cmd/squad` | o binário único: servidor (`serve`), a CLI usada pelos agentes, `hook`, `install-skill` |
| `internal/` | `terminal`, `pty`, `screen` (emulador), `activity` (monitor), `agentbus` (conversa), `agents` (hooks), `notes`, `store`, `server`, `app` |
| `web/` | interface React (React Flow, xterm.js); o build vai para `internal/webui/dist` |
| `tools/shot` | captura de tela via Chrome headless (módulo Go próprio), usada para comparar a UI com o design |
| `third_party/vt` | emulador de terminal (charmbracelet/x/vt, MIT) com patches, como pacote do próprio módulo |
| `docs/` | investigação do Maestri, propostas de arquitetura, spec da conversa, design (`design_handoff_squad/`), plano e diário |
| `spikes/` | protótipos e gravações reais de TUIs (base dos testes do emulador) |
| `research/` | ferramentas usadas na investigação |
