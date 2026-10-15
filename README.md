# Squad

Ferramenta local para orquestrar agentes de código (Claude Code, Codex, Devin, Gemini) e shells num canvas infinito. Os terminais conversam entre si pelas conexões que você desenha. O servidor é um binário Go que escuta em `127.0.0.1`, e a interface React abre no navegador.

> Estado: backend e interface web da primeira rodada prontos (B0–B5, W1–W4): terminais vivos no canvas, conexões, conversa entre agentes animada nos fios, Composer com @menções, notas em Markdown e a gaveta de atividade. Ficam para depois: andares (git worktree), portal com navegador ao vivo, MCP e validação no Windows real. Plano e andamento: [`docs/PLAN.md`](docs/PLAN.md) e [`docs/DEVLOG.md`](docs/DEVLOG.md).

## Requisitos

- Go 1.26+ (para compilar).
- Node 22+ (só para compilar a interface web).
- macOS ou Linux testados; Windows compila (ConPTY) mas ainda não foi testado numa máquina Windows.

## Início rápido

```sh
make build                       # gera bin/squad
bin/squad install-skill          # opcional: ensina o Claude Code a usar o Squad (~/.claude/skills/squad)
bin/squad install-skill -agent devin   # idem para o Devin CLI (~/.config/devin/skills/squad)
bin/squad serve -dir ~/meu-projeto
```

A mesma skill vem empacotada em [`squad-skill.zip`](squad-skill.zip) (pasta `squad/` com o `SKILL.md`), gerada por `make skill-zip`. Ela só faz sentido onde o comando `squad` existe, ou seja, dentro dos terminais do Squad.

O `serve` mostra uma URL com token e abre o navegador. Os dados ficam em `~/.squad` (troque com `SQUAD_HOME`) e a porta padrão é 7420 (`-port` ou `SQUAD_PORT`).

### Interface

A interface já vem embutida no binário. Para mexer nela:

```sh
make web                                      # npm ci + build → internal/webui/dist (commitado)
cd web && SQUAD_TOKEN=<token> npm run dev     # Vite com proxy para o servidor em 7420
```

**Atalhos:**
- canvas: `V` selecionar, `H` mover, `T` novo terminal, `N` nota, `P` portal;
- globais: `⌥A` gaveta de atividade, `⌥P` pausar conversas, `⌥K` composer.

**Conexões:** arraste a partir da bolinha lateral de um nó e solte sobre outro nó. Um clique no fio o seleciona; para desligar, use Delete ou o botão "Desligar". Duplo clique no fio cria um ponto de curva, que pode ser arrastado; duplo clique no ponto o remove.

**Terminais:**
- clique para digitar ("teclado aqui"); `⌥Esc` devolve o teclado ao canvas;
- arraste pelo cabeçalho e redimensione pelas alças (o PTY acompanha);
- com zoom abaixo de 60% vira miniatura, e abaixo de 35%, cartão;
- a pinça ou ⌘/ctrl + roda sobre o terminal dá zoom no canvas.

**Composer (`⌥K`):**
- escreva para um ou mais terminais;
- `@` menciona nós ligados ao destino;
- cole imagens com ⌘V;
- `⏎` envia.

**Notas:** Prévia/Editar no cabeçalho; as tarefas podem ser marcadas com clique; o cadeado trava a nota para todos. Quando um agente edita a nota, o trecho alterado fica destacado.

### Testar a conversa sem a interface (pela API)

Em outro terminal, com o servidor rodando:

```sh
TOKEN=$(python3 -c "import json,os;print(json.load(open(os.path.expanduser('~/.squad/run/server.json')))['token'])")
API="curl -s -H Authorization:Bearer\ $TOKEN -H Content-Type:application/json"

# dois terminais: um shell e um Claude Code
SHELL_ID=$($API -d '{"kind":"terminal","terminal":{"name":"Shell","agent":"shell"}}' http://127.0.0.1:7420/api/nodes | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
CLAUDE_ID=$($API -d '{"kind":"terminal","terminal":{"name":"Claude","agent":"claude"}}' http://127.0.0.1:7420/api/nodes | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')

# ligar os dois
$API -d "{\"a\":\"$SHELL_ID\",\"b\":\"$CLAUDE_ID\"}" http://127.0.0.1:7420/api/edges
```

Para digitar no terminal "Shell", conecte um cliente WebSocket em `/ws/term/<id>`. Até a interface ficar pronta, o jeito mais simples é usar o próprio Claude: peça a ele, no terminal dele, que rode `squad list` e `squad ask "Shell" --raw "ls\n"`.

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
| `third_party/vt` | emulador de terminal (charmbracelet/x/vt, MIT) com patches |
| `docs/` | investigação do Maestri, propostas de arquitetura, spec da conversa, design (`design_handoff_squad/`), plano e diário |
| `spikes/` | protótipos e gravações reais de TUIs (base dos testes do emulador) |
| `research/` | ferramentas usadas na investigação |
