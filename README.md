# Squad

Ferramenta local para orquestrar agentes de código (Claude Code, Codex, Gemini) e shells num canvas infinito. Os terminais conversam entre si pelas conexões que você desenha. O servidor é um binário Go que escuta em `127.0.0.1`, e a interface React abre no navegador.

> Estado: em desenvolvimento. O plano e o andamento estão em [`docs/PLAN.md`](docs/PLAN.md) e [`docs/DEVLOG.md`](docs/DEVLOG.md).

## Início rápido

Requisitos: Go 1.26+, e Node 22+ só para compilar a interface web.

```sh
make build        # gera bin/squad
bin/squad version
make check        # vet, testes e build para macOS/Linux e Windows
```

## Estrutura

| Pasta | Conteúdo |
|---|---|
| `cmd/squad` | o binário único: servidor (`serve`) e a CLI usada pelos agentes (`ask`, `list`, `check`, …) |
| `internal/` | pacotes do servidor (terminal, tela, conversa, armazenamento, …) |
| `third_party/vt` | emulador de terminal (charmbracelet/x/vt, MIT) com um patch de memória |
| `docs/` | investigação do Maestri, propostas de arquitetura, spec da conversa, design (`design_handoff_squad/`) |
| `spikes/` | protótipos e gravações reais de TUIs (base dos testes do emulador) |
| `research/` | ferramentas usadas na investigação |
