# Protótipos (base da proposta Go, `docs/05-proposta-arquitetura-go.md`)

Código descartável, escrito para medir. **Não é o código do produto**, mas vários pedaços servem de ponto de partida.

## `go/`

Módulo Go `spike`. Compile com `go build -o bin/<nome> ./<nome>`. Os binários já estão em `bin/`, para macOS (arm64) e Windows (amd64).

| Pasta | O que faz |
|---|---|
| `rec/` | Roda uma TUI dentro de um PTY (`xpty`: Unix ou ConPTY) espelhado pelo emulador `vt`. Devolve as respostas do emulador ao PTY, cola texto com colagem delimitada quando o programa ativa o modo 2004 e grava os bytes crus e snapshots da tela no meio da sessão. |
| `replay/` | Reproduz um `.raw` no `vt` até um offset e imprime a tela (sem filtro). |
| `replayf/` | Igual, com o **filtro de OSC** antes do emulador. Também conta os títulos recebidos. |
| `oscfilter/` | Pacote reaproveitável: separa sequências OSC do fluxo, tratando o conteúdo como bytes opacos. Corrige o bug do `vt` com UTF-8 em títulos e entrega os eventos (título, OSC 7, OSC 9;4). |
| `bench/` | Mede memória por terminal e vazão de parsing do `vt` (variável `SB` = linhas de scrollback). |
| `portal/` | Navegador sem Playwright: `chromedp` + Chrome/Edge. Faz snapshot de acessibilidade com refs `@eN`, `Input.insertText` e clique por ref com eventos confiáveis, e captura um quadro de screencast. |
| `patches/vt-v0.1.0-datasize.patch` | Troca o buffer fixo de 4 MB do `vt` por 64 KB (1,12 MB por terminal em vez de 5,25 MB). |

Para usar o patch:

1. Copie `github.com/charmbracelet/x/vt@v0.1.0` do cache de módulos para uma pasta `vtfork/`.
2. Aplique o patch (`patch -p1 -d vtfork < patches/vt-v0.1.0-datasize.patch`).
3. Rode `go mod edit -replace github.com/charmbracelet/x/vt=./vtfork`.

Exemplos:

```sh
# Gravar o Claude Code numa pasta já confiada (um prompt mínimo)
bin/rec -max 60 -cols 120 -rows 36 -dir ~/projeto -out claude \
  -steps "wait:7|snap:start|paste:Responda apenas: pong|wait:0.2|enter|wait:0.25|enter|wait:18|snap:answer|text:/exit|wait:0.3|enter|wait:3" -- claude

# Comparar o vt (com filtro) com a referência xterm.js
bin/replayf ../recordings/claude.raw 120 36 5617 > /tmp/vt.txt
python3 ../xterm-ref/compare.py /tmp/vt.txt ../recordings/claude.answer.xterm11.txt

# Portal (no Windows, sem -browser, ele usa o Edge sozinho)
bin/portal -out /tmp/frame.jpg
```

## `xterm-ref/`

A referência de fidelidade com `@xterm/headless`, o mesmo motor do xterm.js do frontend. Instale com `npm install`.

- `replay.mjs` / `replay11.mjs`: imprimem a tela de um `.raw` até um offset; o `11` usa larguras do Unicode 11.
- `compare.py`: compara duas telas linha a linha.
- `cmp.sh` / `cmp11.sh`: atalhos que leem o offset do arquivo `.snaps`.
- `bench.mjs`: o mesmo benchmark de memória do Go, para comparar.

## `recordings/`

Sessões reais gravadas pelo `rec`, que servem de **sementes para os golden tests** do pacote `internal/screen`.

| Sessão | Programa | Snapshots (`.snaps`: rótulo e offset) | Observação |
|---|---|---|---|
| `vim.*` | vim | `edit` | Tela alternativa, acentos e caracteres largos |
| `htop.*` | htop | `run` | Redesenho pesado, cores, mouse |
| `codex.*` | Codex 0.141 | `start` | Saída sincronizada (2026), 48 títulos com spinner |
| `claude.*` | Claude Code 2.1.300 | `start`, `answer` | Títulos com UTF-8 (o bug), emoji com seletor de variação |

O sufixo indica a origem de cada tela:

| Sufixo | Origem |
|---|---|
| `.vt.txt` | `vt` sem filtro |
| `.f.txt` | `vt` com filtro de OSC |
| `.xterm11.txt` | xterm.js com Unicode 11 (a referência) |
| `.xterm.txt` | xterm.js sem Unicode 11, só no `htop` |

`frame.jpg` é o quadro de screencast do portal.

## Validação no Windows (quando houver uma máquina ou VM)

1. Instale Claude Code e Codex e copie `go/bin/rec.exe` e `go/bin/portal.exe`.
2. **Colagem multilinha:** grave o Claude Code com `paste:linha1\nlinha2` (troque `\n` por uma quebra real no `-steps`). Confira na tela (`.vt.txt`) se as duas linhas ficaram num único prompt antes do Enter.
3. **Títulos (OSC) pelo ConPTY:** grave o Codex e veja, com `replayf`, se há títulos com spinner (`titles=` > 0).
4. **Edge:** rode `portal.exe` sem `-browser` e confira o snapshot, `clicado: Nathan` e os eventos `isTrusted:true`.
5. Se 2 ou 3 falharem, teste o ConPTY em modo de passagem direta (flags do `CreatePseudoConsole`) ou o `conpty.dll` do Windows Terminal. Ver `docs/05`, seção 9.
