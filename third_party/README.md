# third_party

## vt

Cópia de `github.com/charmbracelet/x/vt` v0.1.0 (licença MIT, ver `vt/LICENSE`), usada via `replace` no `go.mod` da raiz.

Alteração local (`spikes/go/patches/vt-v0.1.0-squad.patch`):

- `emulator.go`: o buffer de dados do parser cai de 4 MB fixos para 64 KB. Isso reduz a memória por terminal de ~5,25 MB para ~1,12 MB. Medição em `docs/05-proposta-arquitetura-go.md`.
- `emulator.go`: a flag `closed` passou a ser `atomic.Bool`. `Read`, `Write` e `Close` rodam em goroutines diferentes, e o detector de corrida (`go test -race`) acusava o `bool` comum.

Bug conhecido do upstream, contornado fora daqui (`internal/screen`, filtro de OSC): bytes 0x80–0x9F dentro de um OSC (UTF-8 em títulos) são tratados como controles C1.

Quando o upstream aceitar as correções, remova a cópia e o `replace`.
