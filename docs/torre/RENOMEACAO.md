# Renomeação: Squad → Torre

Duas camadas mudam:

1. **A marca**, dentro e fora da interface: Squad vira Torre.
2. **O vocabulário da interface**: os termos que a pessoa lê passam para a metáfora da torre de controle.

A lista de onde o nome aparece hoje está em `referencia-squad-atual/FUNCIONALIDADES.md`, seção 26.

---

## 1. Marca

### 1.1 Na interface

| Hoje | Torre |
|---|---|
| Wordmark "squad" + grade 2×2 | "TORRE" (B612 700, espaçamento .1em, maiúsculas) + o SVG da torre (README, "Logo") |
| `<title>` "loja-demo · Squad" | "loja-demo · Torre"; com voos pedindo você: "(1) loja-demo · Torre" |
| `manifest.webmanifest`: "Squad", "Agentes de código em terminais reais, num canvas." | "Torre", "A torre de controle dos seus agentes de código." (ou mantenha a descrição atual com o nome novo) |
| `favicon.svg`, `icon-192.png`, `icon-512.png`, `apple-touch-icon.png` | A torre sobre `#030705`, vidro `#7fe0a0`, farol `#e05a2b` |
| Página de login "squad" (servida pelo Go) | Redesenhada como o passo 1 da primeira abertura (quadro 19) |
| Cor da barra de título da janela de app | `#07100b` (escuro) / `#f6f2e4` (claro) |
| "Restaurar o papel X como o Squad o traz?" | "Restaurar o papel X como a Torre o traz?" |
| "lixeira do Squad (~/.squad/trash)" | "lixeira da Torre (~/.torre/trash)" |
| "Volta às instruções que o Squad traz" | "Volta às instruções que a Torre traz" |
| "Nenhuma CLI de agente foi encontrada no PATH do Squad" | "…no PATH da Torre" |

Gênero: **a** Torre.

### 1.2 Fora da interface (o produto inteiro)

| Hoje | Torre | Compatibilidade |
|---|---|---|
| Binário e comando `squad` | `torre` | Por uma versão, instalar também `squad` como atalho que chama `torre` e avisa no stderr que o nome mudou. |
| Subcomandos `squad serve`, `ask`, `list`, `check`, `wait`, `reply`, `note`, `portal`, `roles`, `new`, `connect`, `close`, `floor`, `install-skill`, `version`, `mcp`, `hook` | Os mesmos, com `torre` na frente | Os subcomandos continuam em inglês, porque são a interface dos agentes. Opcional: `torre level` como sinônimo de `torre floor`. |
| `~/.squad` (e `SQUAD_HOME`) | `~/.torre` (e `TORRE_HOME`) | Na primeira execução, se `~/.torre` não existe e `~/.squad` existe, **mover** (ou copiar e deixar um aviso) e registrar no log. Ler `SQUAD_HOME` se `TORRE_HOME` não estiver definido. |
| Variáveis `SQUAD_*` (`SQUAD_SHELL`, `SQUAD_BROWSER`…) | `TORRE_*` | Ler `TORRE_*` primeiro e cair para `SQUAD_*`, com um aviso de descontinuação. |
| Andares `<pai>/.squad/floors/<proj>--<andar>` | `<pai>/.torre/floors/<proj>--<nível>` | Os níveis novos nascem no caminho novo. Os existentes continuam onde estão (o `Floor.path` já guarda o caminho). |
| Cópias de sessão `<pai>/.squad/sessions/…` | `<pai>/.torre/sessions/…` | Igual: só as novas no caminho novo. |
| Servidor MCP `squad` (ferramentas `mcp__squad__*`) | `torre` (`mcp__torre__*`) | Atualizar o `--mcp-config` do Claude, o `mcp_servers` do Codex e o `mcp_config.json` do Devin. As listas de ferramentas liberadas e as de aprovação passam para os nomes novos. |
| Skill `squad` (`~/.claude/skills/squad` e a pasta de skills do Devin) | `torre` | `torre install-skill` instala a nova e remove a antiga. Na primeira execução, se só a antiga existir, avisar para reinstalar. |
| Hooks dos agentes (`squad hook …` em Stop, SessionStart…) | `torre hook …` | Regravar os hooks dos agentes que a Torre configura. Por uma versão, aceitar chamadas do `squad hook`. |
| Envelope `[de: Planejador · pedido 7f3a]` | Igual (não leva o nome do produto) | Opcional: o indicativo no lugar do nome, `[de: PLN · pedido 7f3a]`, se o indicativo entrar (`NOVIDADES-E-BACKEND.md`, item 2). |
| Textos dos papéis prontos e da skill que citam `squad ask`, `squad note`… | `torre ask`, `torre note`… | Revisar `roles.json` dos papéis prontos e o texto da skill. Os papéis editados pela pessoa não são reescritos, mas o `squad` continua funcionando por uma versão. |
| `README` do projeto, `go.mod`/módulo, nomes de pacote, CI, release | Torre | O nome do módulo Go e do repositório fica a seu critério; mudar só se não quebrar a distribuição. |
| Cookie de sessão, chaves de `localStorage` (posição e zoom por workspace) | Prefixo novo (`torre:`) | Migrar as chaves antigas na primeira carga, para ninguém perder a posição do canvas. |

**Os mockups ainda mostram `squad` em alguns textos** (`squad ask` no terminal do PLN, `squad new "Bia" …` no nível vazio, `squad serve` na primeira abertura, `~/.squad/trash`, `.squad/floors`). Em todos eles, use `torre`. O `TORRE_BROWSER` do portal já está certo.

---

## 2. Vocabulário da interface

Vale para tudo o que a pessoa lê: rótulos, menus, toasts, diálogos, dicas e textos de ajuda. **Os identificadores do código podem ficar como estão** (`floor`, `node`, `edge`, `role`, `manager`, `activity`, `turn card`), para não aumentar o risco.

| Hoje | Torre | Observação |
|---|---|---|
| Terminal (de agente) | **Voo** | "Novo voo", "Remover o voo PLN?". Shell também é um voo ("voo de serviço" não é preciso). |
| Nome do terminal | Nome + **indicativo** (3 letras: PLN, IMP, REV, TST, ARQ…) | O indicativo é novo (`NOVIDADES-E-BACKEND.md`, item 2). |
| Fio / conexão | **Frequência** | "na mesma frequência", "fora da frequência", "Desligar". |
| Andar | **Nível de voo**, abreviado **FL1, FL2…** | "Novo nível de voo", "Descartar FL2?". |
| Térreo | **Pista** | Sempre com o branch: "PISTA main". |
| Aterrissar | **Pousar** / **Pedir pouso** / **Autorizar pouso** | O diálogo é "AUTORIZAR POUSO · FL1"; o botão, "MESCLAR E POUSAR" ou "POUSAR". A recusa é "POUSO NEGADO". |
| Mesclar (sessão em cópia) | **Pousar** | Mesmo diálogo do nível. |
| Pausar conversas / Retomar | **Espera geral** / **Liberar todos** | Botão da barra; com pausa ativa, o botão fica âmbar cheio. |
| Atividade (gaveta) | **Rádio** | |
| Composer | **Transmitir** | "TRANSMITIR →", "TRANSMITIR A 2", toast "Transmitido a IMP e REV". |
| Nota | **NOTAM** | O arquivo `.md` continua igual. |
| Cartão de turno | **Boletim** | "BOLETIM · 31s". |
| Resumo do workspace | **Briefing** | |
| Gerente | **Líder** | Chip "líder", "Modo líder" no menu. |
| Modelo (arranjo salvo) | **Plano de voo** | Funcionalidade nova (item 9). |
| Interrompido | **Arremetida** | Faixa "ARREMETIDA · PLN e REV foram interrompidos…". |
| Servidor / reconectando | **ENLACE** / **RECONECTANDO…** | |
| Caixa N (pedidos esperando resposta) | **caixa N** | Fica. |
| Workspace, Sessão, Portal, Papel, Pedido | iguais | |

### Estados

| `activity` | Hoje | Torre |
|---|---|---|
| `boot` | Iniciando | **TAXIANDO** |
| `idle` | Ocioso | **PÁTIO** |
| `work` | Trabalhando | **NO AR** |
| `wait` | Esperando resposta | **AGUARDA** (com quem: "AGUARDA IMP") |
| `ask` (novo) | — | **PRIORIDADE** no geral; **PEDE VOCÊ** no voo |
| `done` | Terminou | **EM SOLO** |
| `err` | Parado · erro / Parado · saída N | **PANE** / **PANE · saída N** |
| `off` | Descarregado | **HANGAR** |

### Desfechos no Rádio

| Hoje | Torre |
|---|---|
| Pendente · 0:42 | Pendente · 0:42 |
| Respondido em 0:09 | Respondido em 0:09 |
| Entregue | Entregue |
| Cancelado | Cancelado |
| Retido | Retido · espera geral / Retido · limite por minuto |
| Nota editada | NOTAM editado |
| Fim do turno | Em solo |
| — | **Pede você · 0:24** (novo) |

### Textos que mudam (exemplos; aplicar o padrão no resto)

- "Planejador ↔ Implementador ligados" → "PLN ↔ IMP na mesma frequência".
- "… desligados" → "PLN ↔ IMP desligados".
- "Enviado a Implementador" → "Transmitido a IMP".
- "Andar X criado: branch Y" → "FL2 criado: branch feat/cupom".
- "X aterrissado: Y mesclado em main (abc1234)" → "FL1 pousou: feat/export-csv em main (abc1234)".
- "Revisor foi interrompido no meio do trabalho…" → "ARREMETIDA · REV foi interrompido no meio do turno. A conversa voltou; falta só continuar."
- "Nenhuma conversa ainda. Ligue dois terminais e use squad ask." → "Rádio em silêncio. Ponha dois voos na mesma frequência e use torre ask."
- "Nota vazia. Clique em Editar, ou ligue um agente…" → "NOTAM vazia. Clique em Editar, ou ponha um voo na frequência dela para ele escrever."
