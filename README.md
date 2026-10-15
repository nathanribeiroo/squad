# Squad

Ferramenta local para orquestrar agentes de código (Claude Code, Codex, Gemini) e shells num canvas infinito. Os terminais conversam entre si pelas conexões que você desenha. O servidor é um binário Go que escuta em `127.0.0.1`, e a interface React abre no navegador.

> Estado: o backend está pronto (terminais, conversa entre agentes, hooks do Claude Code, notas); a interface web está em desenvolvimento. Plano e andamento: [`docs/PLAN.md`](docs/PLAN.md) e [`docs/DEVLOG.md`](docs/DEVLOG.md).

## Requisitos

- Go 1.26+ (para compilar).
- Node 22+ (só para compilar a interface web).
- macOS ou Linux testados; Windows compila (ConPTY) mas ainda não foi testado numa máquina Windows.

## Início rápido

```sh
make build                       # gera bin/squad
bin/squad install-skill          # opcional: ensina o Claude Code a usar o Squad (~/.claude/skills/squad)
bin/squad serve -dir ~/meu-projeto
```

O `serve` mostra uma URL com token e abre o navegador. Os dados ficam em `~/.squad` (troque com `SQUAD_HOME`) e a porta padrão é 7420 (`-port` ou `SQUAD_PORT`).

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

Com o Claude Code, a resposta é a **mensagem final do turno** (via hook `Stop`), não a tela. Com outros programas, o Squad espera a tela parar de mudar.

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
| `third_party/vt` | emulador de terminal (charmbracelet/x/vt, MIT) com patches |
| `docs/` | investigação do Maestri, propostas de arquitetura, spec da conversa, design (`design_handoff_squad/`), plano e diário |
| `spikes/` | protótipos e gravações reais de TUIs (base dos testes do emulador) |
| `research/` | ferramentas usadas na investigação |
