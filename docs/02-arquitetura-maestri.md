# Arquitetura do Maestri (reconstruída)

Os diagramas resumem o que está em `01-investigacao-maestri.md`. As setas com linha cheia foram observadas; as tracejadas foram inferidas pelo estático.

## Visão geral: processos, PTYs, CLI, IPC e persistência

```mermaid
flowchart LR
  subgraph APP["Maestri.app (um processo, sem sandbox)"]
    UI["Canvas SwiftUI/AppKit<br/>terminais renderizados em Metal"]
    TM["TerminalManager<br/>SwiftTermProvider"]
    MON["TerminalActivityMonitor<br/>amostra a cada 1 s"]
    IAS["InterAgentServer<br/>HTTP/1.0 sobre sockets BSD"]
    RT["Roteador da CLI<br/>list, ask, check, note, portal, role, floor"]
    PEND["Tabela de asks pendentes<br/>ask back e quebra de ciclos"]
    WQ["Fila de escrita por terminal<br/>colagem delimitada + CR"]
    NOTES["Watcher de notas"]
    FLOORS["Gestor de andares<br/>git worktree + cp -c"]
    PORT["Portais<br/>WKWebView, simulador, adb"]
    OMBRO["Ombro<br/>FoundationModels + ferramentas"]
    WIRE["Maestri Wire<br/>TLS/WebSocket para o app companheiro"]
  end

  subgraph PTYS["PTYs (um por terminal)"]
    SH1["shell -l<br/>MAESTRI_SOCKET, TERMINAL_ID, CLI"]
    AG1["agente: claude / codex / gemini"]
    SH2["shell -l"]
  end

  CLI["CLI maestri<br/>copiada para TMPDIR/maestri-cli"]
  SOCK[("TMPDIR/maestri-xxxx/maestri.sock")]
  TCP[("127.0.0.1:porta<br/>terminais remotos + token")]

  subgraph DISK["Persistência"]
    WS[("~/.maestri/workspaces/ID/workspace.json")]
    ND[("…/notes/*.md")]
    SB[("…/terminals/ID.scrollback")]
    PREF[("~/.maestri/preferences.json<br/>rolePresets")]
    ROLE[("projeto/.maestri/roles/ID/<br/>CLAUDE.md = AGENTS.md")]
    FL[("pasta-pai/.maestri/floors/projeto--andar<br/>git worktree")]
    SK[("~/.claude/skills/maestri*")]
  end

  UI --- TM
  TM -->|forkpty via SwiftTerm| SH1
  TM -->|forkpty| SH2
  SH1 --> AG1
  TM -->|digita a linha de lançamento| SH1
  AG1 -->|executa| CLI
  CLI -->|"POST /cli + X-Terminal-ID"| SOCK
  SOCK --> IAS
  TCP -.-> IAS
  IAS --> RT
  RT --> PEND
  RT --> WQ
  WQ -->|bytes no master do PTY| SH2
  MON -->|fim de turno| PEND
  PEND -->|"200 + tela renderizada"| IAS
  RT --> NOTES
  NOTES --- ND
  RT -.-> FLOORS
  FLOORS --- FL
  RT -.-> PORT
  TM --- WS
  TM --- SB
  TM -->|materializa papel| ROLE
  PREF --- TM
  OMBRO -.-> MON
  WIRE -.-> UI
  AG1 -.->|lê instruções| SK
```

## Sequência: `ask` bloqueante

```mermaid
sequenceDiagram
  autonumber
  participant A as Agente A (terminal A)
  participant C as CLI maestri
  participant S as InterAgentServer
  participant Q as Fila de escrita de B
  participant B as PTY de B (agente B)
  participant M as ActivityMonitor

  A->>C: maestri ask "B" "tarefa"
  C->>S: POST /cli {args, workingDirectory}<br/>X-Terminal-ID = A (sem timeout de socket)
  S->>S: valida A, resolve "B", exige conexão A-B
  S->>S: cria PendingAsk(A→B), callback de fim de turno,<br/>fallback em 5 s, timeout em 120 s
  S->>Q: enfileira a mensagem
  Q->>B: ESC[200~tarefa ESC[201~ (se modo 2004)
  Q->>B: CR (~200 ms), CR de reforço (~250 ms)
  B-->>M: saída (amostras de 1 s: bytes + hash da tela)
  alt turno detectado (≥3 amostras ativas, >3 s sem mudança)
    M->>S: fim de turno de B
  else resposta curta
    S->>S: fallback: tela estável ≥5 s
  else 120 s esgotados
    S->>S: houve saída nos últimos 10 s? prorroga +120 s
  end
  S-->>C: 200 text/plain = tela renderizada de B
  C-->>A: imprime no stdout
```

## Sequência: ask back (resposta direta)

```mermaid
sequenceDiagram
  participant A as Terminal A
  participant S as Servidor
  participant B as Agente B
  A->>S: ask "B" "faça X e responda com maestri ask A"
  Note over S: PendingAsk P = A→B (A bloqueado)
  S->>B: entrega a mensagem (colagem + CR)
  B->>S: ask "A" "resultado completo"
  Note over S: existe pendente com origem A e destino B<br/>(é uma resposta direta)
  S-->>A: libera P com uma nota padrão:<br/>a resposta chega como novo prompt
  S->>A: entrega "resultado completo" no PTY de A
  Note over S: o ask B→A segue normal<br/>(espera A ficar ocioso)
  Note over S: ciclo maior (A→B→C→A)?<br/>busca em profundidade nos pendentes<br/>e libera o pendente que bloqueia
```

## Detecção de término: estados por terminal

```mermaid
stateDiagram-v2
  [*] --> Ocioso
  Ocioso --> Ativo: amostra com ≥500 B e hash de tela diferente
  Ativo --> Ativo: nova amostra ativa
  Ativo --> Trabalhando: 3 amostras ativas seguidas
  Ativo --> Ocioso: amostra sem atividade
  Trabalhando --> Trabalhando: mudança significativa nos últimos 3 s
  Trabalhando --> FimDeTurno: mais de 3 s sem mudança significativa
  FimDeTurno --> Ocioso: dispara callbacks dos asks pendentes (e o Ombro)
  note right of Ocioso
    O hash usa as linhas visíveis,
    pula as 3 primeiras se houver mais de 11
    e ignora glifos braille de spinner.
    O ask tem um fallback próprio:
    tela estável por 5 s libera.
  end note
```

## Andares: criar e aterrissar

```mermaid
flowchart TB
  G["Térreo: projeto em main"] -->|"floor create X"| W["git worktree add --no-checkout<br/>pasta-pai/.maestri/floors/proj--x (branch x)"]
  W --> CP["cp -c -R de cada item do térreo<br/>(menos 36 pastas de build)"]
  CP --> RS["git reset: índice = HEAD"]
  RS --> F["Andar: canvas próprio + terminais no worktree"]
  F -->|"floor land X"| L1{"andar e térreo limpos?"}
  L1 -->|não| ERR["recusa: commit ou stash antes"]
  L1 -->|sim, sem --into| K["remove worktree e andar<br/>mantém o branch x"]
  L1 -->|"sim, --into main"| MG["merge sem fast-forward<br/>ou merge-tree + commit-tree + update-ref"]
  MG --> K
```
