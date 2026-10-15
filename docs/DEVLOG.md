# Diário de desenvolvimento

O que foi feito em cada etapa, as decisões tomadas sem consulta e o que precisa de você. Entradas mais novas no topo.

## 2026-10-15, B0 esqueleto

- Repositório privado `nathanribeiroo/squad` criado, com tudo o que existia na `main` (docs, design, spikes, research). A pasta `sandbox/`, resto dos testes do Maestri, ficou fora do git.
- Módulo `github.com/nathanribeiroo/squad`, com Go 1.26.
- `third_party/vt`: cópia do `charmbracelet/x/vt` v0.1.0 com o patch do buffer de 64 KB, via `replace`.
- `cmd/squad` com o despacho de subcomandos, `internal/config` (`~/.squad`, porta 7420, arquivo de descoberta `run/server.json` com escrita atômica) e `internal/version`.
- `make check`: vet (incluindo Windows), testes e build para os dois sistemas.

**Decisão:** a porta padrão é 7420 e pode ser trocada por `SQUAD_PORT`. O diretório de dados é `~/.squad` e pode ser trocado por `SQUAD_HOME`.
