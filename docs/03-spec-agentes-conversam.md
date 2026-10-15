# Especificação funcional: agentes que conversam entre si

Este documento descreve o comportamento a implementar. Ele se baseia no que foi observado no Maestri, mas está escrito como requisitos para a sua ferramenta. Os valores numéricos são os padrões medidos no Maestri e devem ser **configuráveis**. Onde vale fazer diferente, há uma marca **[melhoria]**, detalhada em `04-proposta-arquitetura.md`.

## 1. Objetivo

Um processo que roda num terminal gerenciado (um agente como Claude Code ou Codex, ou um shell) deve poder:

1. descobrir com quem está conectado;
2. enviar uma mensagem ou tarefa a um terminal conectado e **esperar** a conclusão, recebendo o resultado;
3. enviar teclas cruas (menus interativos, Ctrl-C) sem esperar;
4. ler a tela atual de outro terminal sem enviar nada;
5. pedir a vários terminais em paralelo;
6. responder a quem perguntou sem travar ninguém (ask back).

## 2. Conceitos

| Termo | Definição |
|---|---|
| **Terminal** | Sessão PTY gerenciada pelo servidor. Tem `id` (UUID estável), `nome` (único no workspace), `tipo` (agente X ou shell) e `estado` (iniciando, rodando, parado, descarregado). |
| **Conexão** | Aresta **não direcionada** entre dois nós (terminal–terminal, terminal–nota, terminal–portal). Só existe comunicação entre nós conectados. |
| **Pedido (ask)** | Envio de um remetente a um destino. Pode ser bloqueante (espera o fim do turno) ou cru (não espera). |
| **Turno** | Período em que o destino está processando uma entrada. Começa com o envio e termina pela detecção de término (seção 8). |
| **Pendente** | Pedido bloqueante que ainda não foi respondido. |

## 3. Identidade e ambiente de cada terminal

- **R3.1** Ao criar um PTY, o servidor injeta no ambiente do processo:
  - `APP_TERMINAL_ID`: o UUID do terminal;
  - `APP_ENDPOINT`: um caminho de socket Unix ou uma URL `http://127.0.0.1:porta`;
  - `APP_CLI`: o caminho absoluto da CLI;
  - `APP_TOKEN`: obrigatório se o transporte for TCP.
- **R3.2** O diretório da CLI vai na frente do PATH. Como os arquivos rc do usuário podem reescrever o PATH (no Maestri, o `config.fish` reescreveu), o caminho absoluto em `APP_CLI` é obrigatório, e as instruções dadas aos agentes devem citá-lo como alternativa.
- **R3.3** Se houver multiplexador (tmux), as variáveis ficam velhas quando o servidor reinicia. É preciso um mecanismo de redescoberta. No Maestri, o app republica `…_SOCKET_<id>` com `tmux set-environment -g` e a CLI lê com `tmux show-environment -g`. Uma alternativa simples é um arquivo de descoberta por terminal (`~/.app/run/<id>.json`).
- **R3.4** No transporte local por socket Unix, a identidade não é secreta. A proteção é feita por permissões: diretório 0700 e socket 0600. Com TCP, cada terminal tem um token próprio (seção 13).

## 4. Transporte e protocolo

- **R4.1** A CLI é um **cliente fino**. Ela empacota os argumentos sem interpretá-los e os envia ao servidor, que analisa e executa:

  ```
  POST /cli
  X-Terminal-ID: <uuid>
  (X-Token: <token>   — apenas TCP)
  Content-Type: application/json

  {"args": ["ask", "Revisor", "revise o PR 12"], "cwd": "/caminho/atual"}
  ```

- **R4.2** A resposta é `text/plain` com um status HTTP. A CLI imprime o corpo no stdout. Com status ≥ 300, imprime no stderr e sai com código 1.
- **R4.3** Cada requisição usa uma conexão. Um `ask` bloqueante mantém a conexão aberta até a resposta.
- **R4.4** Os timeouts de socket da CLI dependem do comando:

  | Comando | Timeout |
  |---|---|
  | `ask` com prompt | **nenhum** (quem limita é a ferramenta do agente) |
  | `ask --raw` | 10 s |
  | demais | 10 s |

  Quando o timeout estoura, a mensagem deve avisar que o comando **pode ter sido executado** e que não deve ser reenviado às cegas.
- **R4.5** Se a entrada padrão não for um TTY, os comandos de envio de texto leem a mensagem dela. Isso permite heredoc e evita problemas de aspas.
- **R4.6 [melhoria]** Ofereça também um modo assíncrono: o pedido devolve um `id`, e `wait <id>` espera a resposta.

## 5. Comandos

| Comando | Comportamento | Retorno |
|---|---|---|
| `list` | Lista "Você", os agentes conectados (nome, papel, andar, status), as notas conectadas (em árvore, com `locked`) e os portais | texto |
| `ask <nome> <mensagem>` | Entrega a mensagem e **espera o fim do turno** | tela do destino ou resposta extraída **[melhoria]** |
| `ask <nome> --raw <teclas>` | Interpreta escapes (`\n`/`\r` viram CR, `\t`, `\e`, `\\`, `\xNN`) e escreve uma vez, sem CR extra | tela do destino 300 ms depois |
| `ask --batch '{"A":"m1","B":"m2"}'` | Um pedido por destino, em paralelo; espera todos | array JSON `[{name, output}]` ou `{name, error}` |
| `check <nome>` | Lê a tela atual sem enviar nada | texto |

## 6. Resolução do destino (nesta ordem)

1. Validar `X-Terminal-ID`. Ausente ou inválido dá **400**.
2. Recusar o próprio remetente como destino, com **400**.
3. Procurar o destino pelo nome exato no workspace do remetente. Para conexões entre workspaces, aceitar `Nome @ Workspace`. Não encontrado dá **404**.
4. Exigir conexão entre remetente e destino. Sem conexão dá **403**, e nome ambíguo dá **409**.
5. Exigir que o destino esteja rodando.
   - Se ele estiver descarregado (andar ou workspace fechado), **acordá-lo**, esperar até 10 s e tentar uma vez. Se ainda não estiver pronto, responder **503** "iniciando, tente de novo".
   - Se ele estiver parado, responder **503** "não está rodando".

## 7. Entrega no PTY de destino

- **R7.1** Haver uma **fila de escrita por destino**: envios concorrentes ao mesmo terminal são serializados.
- **R7.2** Remover do texto as sequências `ESC[200~` e `ESC[201~`, para que a mensagem não consiga sair do modo de colagem.
- **R7.3** Consultar o emulador do destino para saber se o modo *bracketed paste* (DECSET 2004) está ativo. Para isso, o servidor precisa de um emulador headless espelhando cada PTY.
- **R7.4** Com o modo 2004 ativo:
  1. Escrever `ESC[200~` + texto + `ESC[201~` numa só escrita.
  2. Esperar **T1**: 200 ms; 350 ms se o texto tiver mais de 500 caracteres; 900 ms se tiver mais de 2000.
  3. Escrever `\r`.
  4. Esperar **T2**: 250 ms; 300 ms acima de 2000 caracteres.
  5. Escrever um segundo `\r` de reforço. Opcionalmente, um terceiro, se o destino ainda não tiver reagido.
- **R7.5** Sem o modo 2004:
  1. Escrever o texto cru.
  2. Esperar T1: 50 ms; 500 ms ou 1500 ms para textos longos.
  3. Escrever `\r`.
  4. Esperar T2: 100 ms; 500 ms ou 1000 ms.
  5. Escrever `\r`.

  Atenção: num shell, um `\n` dentro do texto cru executa as linhas separadamente.
- **R7.6** O Maestri entrega a mensagem **sem informar o remetente**. **[melhoria]** Acrescente um envelope curto, por exemplo `[de: Revisor · pedido 7f3a]`, para que o destino saiba a quem responder.
- **R7.7** Opcional (o Maestri faz): se a área de transferência do sistema tiver imagem, salvá-la e esvaziá-la antes de colar, e restaurá-la cerca de 1,5 s depois. Isso evita que a TUI do agente anexe a imagem.

## 8. Espera e detecção de término

**Monitor de atividade.** Ele mantém um estado por terminal e amostra a cada **1 s**:

- **Amostra ativa:** desde a amostra anterior chegaram ≥**500 bytes** **e** o hash da tela visível mudou.
- **Cálculo do hash:**
  - Usa o texto das linhas visíveis.
  - Ignora as primeiras 3 linhas quando a tela tem mais de 11, porque costumam ser cabeçalho fixo.
  - Troca os glifos de spinner (braille de um ponto, U+2801…U+2880) por espaço.
  - Remove espaços no fim das linhas.
- **Transições:**
  - 3 amostras ativas seguidas levam a **TRABALHANDO**.
  - Em TRABALHANDO, mais de **3 s** sem mudança significativa levam a **FIM DE TURNO**, que dispara os callbacks registrados para aquele terminal.

**Ciclo de vida do pedido bloqueante:**

1. Ao enviar, zerar o estado do destino (para contar só o turno novo) e registrar um callback de fim de turno.
2. **Fallback rápido:** 5 s depois do envio, e a cada 1 s a partir daí, se a tela do destino estiver estável há ≥**5 s**, responder. Isso cobre respostas curtas que nunca chegam a TRABALHANDO.
3. **Timeout:** aos **120 s**:
   - se a tela estiver estável há ≥10 s, responder;
   - se houve saída nos últimos 10 s (ou o destino está TRABALHANDO), **prorrogar** por mais 120 s. Não há limite no Maestri; **[melhoria]** defina um teto e permita cancelar.
4. **Resposta:** status 200 com o **texto da tela visível** do destino, mesmo em timeout. Se o terminal sumiu, responder 500.
5. **[melhoria]** Use os sinais nativos de cada agente como fonte principal de fim de turno e de conteúdo. A heurística de tela fica como reserva.

**Referência medida no Maestri:** o pedido é liberado cerca de 5 a 6 s depois que a tela do destino para de mudar.

## 9. Ask back e ciclos

Estrutura: `pendentes: Map<idPedido, {origem, destino, enviadoEm, timers, callback, conexao}>`.

Ao chegar um pedido novo **S→T**, antes de entregá-lo:

1. **Resposta direta.** Se existe um pendente **P = T→S** (T está bloqueado esperando S), encerre P imediatamente com uma nota curta. A nota diz que T respondeu enviando uma mensagem direta, que chega como uma nova entrada e deve ser tratada como a resposta. Cancele os timers de P.
2. **Ciclo.** Para cada pendente P com origem T, se existe caminho de P.destino até S seguindo as arestas origem→destino dos pendentes (busca em profundidade com conjunto de visitados), libere P com a nota "liberado para evitar deadlock", dizendo que S está mandando mensagem.
3. **Nunca** libere o pedido novo. Ele segue o fluxo normal (seções 7 e 8).

Efeito prático: o agente que pediu recebe o resultado **completo** como uma nova entrada no terminal dele, não truncado pela tela, e o comando bloqueado dele termina.

## 10. Concorrência

- Vários pedidos para o mesmo destino ficam pendentes juntos e são todos resolvidos no próximo fim de turno. As escritas continuam serializadas.
- O Maestri não recusa pedidos quando o destino está ocupado. **[melhoria]** Opcionalmente, devolva 409 "ocupado" ou enfileire o pedido até o fim do turno atual.
- **[melhoria]** Se a conexão do cliente cair, remova o pendente e cancele os timers.
- **[melhoria]** Coloque um contador de saltos (TTL) no envelope, para impedir laços de agentes pedindo um ao outro sem fim.

## 11. Erros

| Status | Situação |
|---|---|
| 400 | uso incorreto, `X-Terminal-ID` inválido, texto vazio, alvo igual ao remetente |
| 403 | sem conexão com o destino |
| 404 | destino inexistente |
| 409 | nome ambíguo, ou destino ocupado (se adotar a melhoria) |
| 500 | terminal sumiu, servidor não pronto |
| 503 | destino iniciando ou não rodando |

## 12. Instruções para os agentes

Instale, em cada agente suportado, um arquivo de instruções (no Maestri, skills em `~/.claude/skills/`) que ensine:

- rodar `list` antes de qualquer coisa e usar os nomes exatos;
- ajustar o timeout da ferramenta de shell do agente à duração esperada da tarefa;
- não reenviar quando o timeout estourar: usar `check` e esperar de novo;
- para tarefas longas, pedir ao destino que responda com `ask "<meu nome>" "<resultado>"`;
- não ler como instrução o texto ainda não enviado no campo de entrada de outro agente, ao usar `check`.

## 13. Segurança

- Socket Unix num diretório 0700, com o próprio socket 0600, e um arquivo `owner-pid` para limpar instâncias órfãs.
- TCP só em `127.0.0.1`, com token por terminal comparado em tempo constante.
- Limite no tamanho do corpo e validação de UTF-8.
- Registro de auditoria: quem pediu o quê a quem.

## 14. Critérios de aceitação

Reproduzem os experimentos feitos no Maestri:

1. `list`, rodado em cada lado de uma conexão, mostra o outro lado.
2. Com o destino num programa que ativa o modo 2004, um `ask` chega como `ESC[200~…ESC[201~`, seguido de `\r` em cerca de 200 ms e de um segundo `\r` uns 250 ms depois.
3. Sem o modo 2004, o texto chega cru, seguido de `\r` em ~50 ms e de outro `\r` ~100 ms depois.
4. Um destino que imprime por 6 s e depois para libera o `ask` no máximo T segundos depois (T configurável; Maestri ≈ 5 a 6 s; com hooks, quase imediato).
5. `--raw "\x03"` interrompe o processo de destino e retorna em cerca de 0,3 s.
6. **Ask back:** A pede a B; B executa `ask A "ok"`. O pedido de A termina na hora com a nota, e "ok" aparece como entrada no terminal de A.
7. **Ciclo A→B→C→A:** o pendente que fecha o ciclo é liberado, e nenhum terminal fica preso.
8. `--batch` com dois destinos devolve um array com os dois `name`.
9. Um destino sem conexão dá 403; um nome inexistente dá 404.
