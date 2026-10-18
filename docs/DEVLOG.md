# Diário de desenvolvimento

O que foi feito em cada etapa, as decisões tomadas sem consulta e o que precisa de você. Entradas mais novas no topo.

## 2026-10-18, Permissões nos níveis

- **Pedido:** o item 18 da lista de pendências. O que você libera com SEMPRE num nível de voo não pode se perder no pouso, e o nível deve começar com o que a pista já libera. O plano foi aprovado com a caixa de levar as regras desmarcada.
- **Onde os agentes guardam o SEMPRE:**
  - **Claude:** em `.claude/settings.local.json` (`permissions.allow`, `ask`, `deny`);
  - **Devin:** em `.devin/config.local.json`, no mesmo formato.
  - Os dois ficam fora do git, na pasta do projeto. Um nível é outra pasta (um git worktree), então eles não iam junto.
- **Servidor (`internal/floors/permissions.go`, novo):**
  - **Criar:** ao criar um nível ou a cópia da sessão, os dois arquivos vêm da pista, como o `.env`, e entram em `copied`.
  - **Decolar:** quando um Claude ou um Devin decola no nível (ou na cópia), o gancho de lançamento soma ao arquivo do nível as regras que a pista ganhou desde então. Só soma, nunca tira; as outras chaves do arquivo ficam como estão.
  - **Ver:** `GET /api/floors/{id}/changes` e `GET /api/copy/changes` devolvem `rules`, as regras que o nível tem e a pista não.
  - **Levar:** `POST /api/floors/{id}/land` e `POST /api/copy/merge` aceitam `permissions: true` e somam essas regras aos arquivos da pista. A resposta diz quantas (`permissions`). O pouso pela CLI e pelo MCP não monta esse campo, então um agente nunca as leva.
  - **Não é alteração:** os dois arquivos nunca contam como alteração não commitada, nem no pouso nem na contagem do nível. Um branch que os commita continua recusado no merge, como um `.env` copiado.
  - **Falhas:** se levar as regras falhar depois do merge, o pouso para com o nível no lugar e diz que o merge foi feito.
- **Interface:** no "Autorizar pouso" do nível e no da sessão em cópia, a seção "N permissões novas" lista cada regra (permitir, perguntar, negar; Claude ou Devin) e traz a caixa "Levar essas permissões para a pista" (na cópia, "para a pasta do projeto"). O aviso do pouso termina com "· N permissões levadas".
- **Correção de passagem, SEMPRE do Claude:** algumas telas do Claude escrevem "Yes, and don’t ask again for: git tag \*" com apóstrofo tipográfico. A Torre não reconhecia essa opção, e o botão SEMPRE não aparecia nessas perguntas. Agora o apóstrofo é normalizado.
- **Achado no Claude real (2.1.302):** num nível, o Claude grava o SEMPRE no arquivo da **pasta principal**, não no do worktree, e lê os dois.
  - **Conferido em três rodadas de `claude -p`:** com a regra só na pasta principal, ele roda o comando; só no worktree, também roda; sem a regra, pede permissão.
  - **Consequência:** o SEMPRE do Claude num nível já vale na pista na hora, e o pouso não tem o que levar dele.
  - **Ele também põe** `**/.claude/settings.local.json` no ignore global do git.
- **Decisões sem consulta:**
  - **O arquivo do Claude fica no mecanismo** mesmo assim: não atrapalha e cobre versões que gravem no worktree e regras escritas à mão.
  - **A soma ao decolar espera** uma criação ou um pouso de nível em andamento (os dois usam a mesma trava). Isso leva segundos, e o servidor não segura nenhuma outra trava nesse caminho.
- **Correção de passagem, terminal:** escrever num terminal já fechado (`Raw`) podia esperar para sempre. A fila tinha espaço, mas ninguém mais a lia. Apareceu na limpeza do teste do Claude real depois do pouso, que fecha o voo. Agora devolve erro.
- **Testado aqui:**
  - **`internal/floors`:**
    - a cópia na criação;
    - a soma ao decolar;
    - as regras novas;
    - não ser alteração;
    - levar e não levar;
    - o branch que commita o arquivo;
    - um arquivo que não é JSON;
    - a sessão em cópia.
  - **`internal/attention`:** a pergunta do Claude com "don’t".
  - **`internal/terminal`:** `Raw` depois de fechar. Falhava sem a correção.
  - **Claude real** (`TestRealClaudeFloorPermissions`, novo): um nível com o arquivo da pista, uma regra nova na pista antes de o Claude decolar, o SEMPRE pela Torre num `git tag`, a regra no arquivo da pista, nada a levar e o pouso. `TestRealClaudePermission` também passa.
  - **Na tela:** um nível com três regras novas do Devin, o diálogo com a caixa desmarcada, marcar, pousar, o aviso "· 3 permissões levadas" e as regras no arquivo da pista.
  - **Validação:** `make check` com o navegador, `tsc` e o build. Uma rodada do `go test ./...` falhou uma vez em `TestFramesHaveViewportSize` (portal, depende de tempo); a seguinte passou.
- **Correção de passagem, teste do Devin:**
  - **Sintoma:** `TestDevinAskAndRespond` (de `feat/devin-prioridade`) falhava no `make check`, mas não rodando sozinho.
  - **Causa:** o teste mandava a segunda pergunta sem esperar o fim do turno da primeira, e a segunda ficava com o fim desse turno.
  - **Correção:** agora o teste espera a primeira resposta.
  - **O comportamento do barramento é antigo:** uma pergunta entregue a um voo que ainda está no turno fica com o fim desse turno. Ficou como sugestão à parte.
- **Não testado:**
  - **Devin real:** sem login, não conferi onde o Devin grava o SEMPRE num worktree nem se ele lê o arquivo copiado. O formato do arquivo é o que vimos no Devin real em `feat/devin-prioridade`. Se ele gravar na pasta principal, como o Claude, a regra já vale na pista; se gravar no worktree, o pouso oferece levá-la.
  - **O diálogo da sessão em cópia:** só o do nível foi visto na tela. O caminho do servidor tem teste, e o componente é o mesmo.
  - **Windows.**
  - **Codex e Gemini:** ficaram de fora, como combinado.

## 2026-10-18, Histórico do Rádio

- **Pedido:** o item 12 do handoff ("Histórico do Rádio guardado"), que estava em "Para depois". Os quatro padrões propostos foram aceitos:
  - 500 linhas por sessão;
  - textos até 16 KB;
  - "Limpar o Rádio" com confirmação;
  - edições de NOTAM agrupadas.
- **Como era:**
  - o Rádio era montado só no navegador, a partir dos eventos ao vivo, com até 200 linhas;
  - ao recarregar, voltavam só os pedidos pendentes e o último boletim de cada voo;
  - o texto inteiro de um pedido sumia ao reiniciar o servidor, e a resposta nem chegava ao Rádio.
- **Servidor (`internal/radio`, novo):**
  - **Como monta:** um serviço por workspace (por sessão aberta) assina os eventos do workspace já ao ser criado e monta as linhas.
  - **O que entra:**
    - os pedidos e o desfecho;
    - as suas mensagens;
    - os retidos e o laço;
    - as edições de NOTAM;
    - o fim de turno;
    - os pedidos de permissão respondidos.
  - **Dados, não frases:** as linhas guardam dados (desfecho, nomes, `hold`, `by`, a ferramenta e a resposta dada), e a interface escreve as frases em português.
- **Onde guarda (`radio.jsonl`):**
  - **O arquivo:** fica na pasta da sessão, ao lado do `cards.json`.
  - **Linhas que mudam:** uma linha que muda (um pedido respondido, um boletim resumido) é escrita de novo com a mesma chave, e a última vale.
  - **Tamanho:** o arquivo é reescrito quando chega ao dobro das 500 linhas guardadas.
  - **Arquivo corrompido:** uma linha cortada por uma queda é ignorada.
  - **Textos:** o texto inteiro de cada pedido e da resposta ficam no arquivo (até 16 KB cada). O cabeçalho "[reply from …]" é tirado da resposta.
- **Barramento:** além do texto dos pedidos recentes, passa a guardar a resposta deles (`ReplyText`). A rota `GET /api/requests/{id}` usa o barramento e, depois de reiniciar, o histórico, e devolve `reply`.
- **Pedidos de permissão respondidos:**
  - o app acompanha o que cada voo pede (no aviso de estado do monitor) e, quando o pedido acaba, registra a linha com o que foi pedido e com a resposta dada pela interface ("autorizado", "sempre", "negado");
  - quando a resposta foi na tela do voo, a linha diz "respondido".
- **Rotas e eventos:**
  - `GET /api/radio?before=&limit=` (200 por página, com `more`) e `DELETE /api/radio`, só da interface;
  - os eventos `radio.item` (uma linha nova ou mudada) e `radio.cleared`.
- **Interface:**
  - **Carregar:** o Rádio carrega a primeira página ao conectar, junto com os boletins. As linhas que chegam durante o carregamento entram também.
  - **Eventos:** o navegador deixou de montar as linhas a partir dos eventos, e as animações dos fios continuam iguais.
  - **Botões:** "Mais antigos" no fim, "Ver a resposta" ao lado de "Ver o pedido inteiro", e a lixeira no cabeçalho para limpar.
  - **Data:** as linhas de outro dia mostram a data ("17/10 14:32").
- **Correções de passagem:**
  - cada salvamento automático de uma NOTAM virava uma linha nova; agora as edições da mesma NOTAM pelo mesmo autor, em até 2 minutos, são uma linha;
  - uma linha longa (data, nomes de voos que já saíram do canvas) empurrava a etiqueta para fora do Rádio; agora ela desce de linha.
- **Decisão sem consulta:**
  - **Fim de turno:** entra como linha do Rádio a cada turno, e não só o último de cada voo, como no `cards.json`.
  - **O botão de limpar** só aparece com o Rádio não vazio.
- **Testado aqui:**
  - **`internal/radio`** (também com o detector de corridas): cada tipo de linha, a troca de estado de um pedido, os textos, reabrir o arquivo, o agrupamento das NOTAMs, o limite com a reescrita, as páginas, limpar, os eventos e uma linha cortada.
  - **Agentes falsos** (`TestRadioKept`):
    - um pedido com o texto e a resposta;
    - a sua mensagem;
    - um pedido de permissão autorizado no Claude e um negado no Devin;
    - a rota com páginas;
    - o arquivo ao lado do canvas;
    - limpar.
  - **Claude real** (`TestRealClaudeRadio`, novo): a resposta "pong" fica guardada, sem o cabeçalho. `TestRealClaudePermission` também passa.
  - **Validação:** `make check` com o navegador, `tsc` e o build.
  - **Na tela, com agentes falsos:**
    - o histórico;
    - "Ver a resposta" e "Ver o pedido inteiro";
    - reiniciar o servidor e o histórico continuar;
    - "Mais antigos" (de 200 para 273 linhas);
    - limpar com a confirmação;
    - a data nas linhas de ontem;
    - duas sessões com históricos separados.
- **Não testado:**
  - **Devin real:** não usei a conta. As linhas vêm dos mesmos eventos para todos os agentes, e o modo Devin do agente falso passou no teste.
  - **Windows:** o Rádio no Windows.
  - **Uso longo:** um histórico de dias com muitos voos.

## 2026-10-18, Responder ao Devin pela Torre, e o som dos avisos

- **Pedido:**
  - o item "Responder pela Torre no Devin…" do "Para depois", com ênfase no Devin (o Codex e o Gemini ficam para depois);
  - junto, um som quando um voo pede você, para todos os agentes.
- **Devin real:**
  - **Instalação e login:** o Devin foi instalado numa pasta isolada e logado com a conta gratuita do usuário, pelo login manual (`devin auth login --force-manual-token-flow`). Eu mandei o link, ele abriu no navegador dele e me passou o código, que vale uma vez só.
  - **Modelo e gasto:** todo Devin de teste rodou com `--model swe-1-6-slow`, como ele pediu. Foram 10 pedidos curtos: 5 no levantamento, 3 no teste automático e 2 na tela. A cota estava em 95% depois do levantamento.
  - **No fim:** `devin auth logout` e a pasta apagada.
- **O que o levantamento mostrou (Devin 3000.11.92):**
  - **A tela de escolha** numera as opções sem ponto ("❭ 1 Yes  (Approve once)") e usa a seta ❭. Por isso a Torre não lia a pergunta do Devin.
  - **Pergunta com "?":** não há. Em cima das opções fica o cabeçalho da ferramenta ("⏺ Running command", "⏺ Editing ./README.md", "⏺ Fetching https://example.com", "⏺ mcp__torre__keys(…)").
  - **As teclas:** o número escolhe e confirma na hora, sem Enter. Testado com 1 (aprovar uma vez), 3 (sempre neste projeto), 7 e 8 ("No").
  - **O que o hook manda:** `exec` (`command`), `edit` (`file_path`), `webfetch` (`url`) e `mcp__torre__…` (os argumentos), com os mesmos campos do Claude Code.
  - **Fim do turno depois de "No":** nem o Devin nem o Claude Code mandam o hook de fim de turno.
  - **Os "sempre" de um comando:**
    - "Yes, allow `touch` commands" vale para esta sessão;
    - "always … in `proj`" grava `Exec(touch)` em `<projeto>/.devin/config.local.json` (e o Devin põe o arquivo no `.git/info/exclude`);
    - "always … in all projects" grava na configuração global.
  - **O "sempre" de uma busca na web** (`Fetch(domain:example.com)`) vai para a configuração global. Dentro da Torre, ela é a cópia que a Torre passa com `--config`.
  - **A pergunta com opções do Devin** (a ferramenta `ask_user_question`) não dispara o hook de permissão, só o `PreToolUse`. Na tela, ela tem a linha "? Not ready to answer, help me out!" embaixo. A Torre mostrava esse Devin como "trabalhando", e não "pede você".
- **Servidor (`internal/attention`):**
  - **Ler a tela:** a seta ❭ e o número sem ponto.
  - **O cabeçalho ⏺ como pergunta,** quando ele está mais perto das opções do que uma linha com "?". O texto que você digitou pode terminar em "?".
  - **O rodapé "help me out".**
  - **O Devin entra em `verified`.**
  - **Nunca viram SEMPRE:** as opções que valem para mais do que foi pedido ("in all projects", "all web fetches", "calling tools on" o servidor inteiro) e as que valem só para a sessão.
  - **Só há botões quando existe uma opção de autorizar.** Antes, o "No, exit" da pergunta de confiança na pasta, sozinho, virava um NEGAR (no Claude também).
- **Servidor (o que vai para a interface):**
  - **`labels`:** o texto do agente em cada opção.
  - **`AskEveryUse`:** o hook marca as ferramentas `mcp__torre__…` fora das seguras (`keys`, `new_shell`, `portal_eval`, `floor_land`). Elas nunca ganham SEMPRE, no Claude e no Devin, e a rota de resposta recusa o que a interface não ofereceu.
- **NEGAR encerra o turno:** como nenhum dos dois manda o hook de fim depois de "No", o turno terminava só depois de 60 s de silêncio, e quem pediu algo ao voo esperava um minuto à toa. Agora, depois de um NEGAR dado pela interface, o turno termina no silêncio normal, com o motivo `denied`, que conta como final para quem espera.
- **Decisão sem consulta, o que é SEMPRE no Devin:**
  - **Num comando:** "always … in `<projeto>`". A regra fica no projeto, fora do git, e vale dentro e fora da Torre. "In all projects" iria para a cópia da configuração e não valeria fora da Torre.
  - **Numa busca na web:** aquele endereço.
  - **Numa ferramenta MCP:** aquela ferramenta.
- **Interface:**
  - os botões mostram o texto do agente ao passar o mouse; no Rádio e no Sinóptico aparece "Sempre = …";
  - os nomes das ferramentas do Devin viraram as frases do cartão ("Quer executar um comando").
- **Som (Configurações → Avisos):**
  - **Ligado por padrão.** As escolhas passam a ser salvas uma a uma, então uma opção que você nunca mexeu segue o padrão. Quem salvou Avisos antes (qualquer opção) ficou com o som desligado e precisa ligar lá.
  - **Outro workspace:** toca quando um voo de outro workspace passa a pedir você, com a notificação do sistema, se ligada.
  - **Um áudio só para a página,** preparado no primeiro clique ou tecla. Antes disso, numa aba comum, o navegador não deixa tocar.
  - **A janela de app** abre o Chromium com `--autoplay-policy=no-user-gesture-required` e toca desde o começo.
  - **"Repetir enquanto esperar"** (desligado por padrão): a cada 2 minutos, até 3 vezes, enquanto o pedido estiver sem resposta.
- **Testado aqui:**
  - **Telas reais do Devin como casos de teste** (`TestParseDevin`, `TestSetPrompt`): comando, edição, busca, MCP, a pergunta, a confiança na pasta e o prompt com texto digitado.
  - **Agente falso no modo Devin** (`TestDevinAskAndRespond`):
    - SEMPRE com o texto do Devin;
    - `keys` sem SEMPRE (a rota recusa);
    - NEGAR encerrando o turno em segundos;
    - a pergunta com opções em PRIORIDADE.
  - **Devin real** (`TestRealDevinPermission`, novo, `-tags e2e`; pula sem login):
    - AUTORIZAR criou o arquivo;
    - NEGAR não criou;
    - SEMPRE criou e gravou `Exec(touch)` no projeto.
  - **Claude real:** `TestRealClaude` e `TestRealClaudePermission` passam.
  - **Devin real, configuração:** `TestRealDevinConfig` passa.
  - **Validação:** `make check` com o navegador, `tsc` e o build.
  - **Na tela, com o Devin real:** o cartão com os três botões, o Rádio com "Sempre = …", AUTORIZAR pela tela e o som tocando uma vez.
  - **Na tela, com agentes falsos e dois workspaces** (o som contado no navegador):
    - numa aba sem clique, o áudio fica suspenso;
    - na janela de app, toca;
    - depois de um clique, o pedido no outro workspace toca;
    - a repetição toca de novo aos 2 minutos.
- **Não testado:**
  - **Ouvir o som de verdade** (aqui não há saída de áudio), no Mac e no Windows.
  - **O app do Dock no Mac** (o Safari): pode pedir um clique antes do primeiro som.
  - **Responder pela Torre no Windows**, com o Devin e com o Claude.
  - **Codex e Gemini** continuam só com "Abrir tela".
  - **A aprovação de plano do Devin** ("Yes, implement plan") e **a pergunta com opções:** entram em PRIORIDADE, sem botões.
  - **Um SEMPRE dado num nível de voo** fica no `.devin/config.local.json` da pasta do nível e some ao pousar. O arquivo não está na lista de cópia dos níveis.

## 2026-10-18, Pilha de NOTAMs

- **Pedido:** a pilha que estava em "Para depois" (quadro 09 de `docs/torre/TELAS.md`), com três decisões suas:
  - empilhar dá acesso: um fio ligado à pilha vale para todas as NOTAMs dela, e a Torre confirma antes, dizendo quem ganha acesso;
  - tirar da pilha mantém os fios;
  - o cadeado continua por NOTAM.
- **Como fica guardada:** cada NOTAM da pilha tem `stack` (o id da base, também na própria base) e `stackPos` (0 na base), no `workspace.json`.
  - A base guarda os fios e a caixa da pilha; as outras NOTAMs não têm fios e ficam com a caixa da base.
  - O canvas desenha um nó só, a base, e os fios continuam valendo como sempre.
  - O código fica em `internal/store/stacks.go`.
- **Empilhar** (`Store.Stack`):
  - os fios das NOTAMs que entram passam para a base, sem repetir;
  - um fio entre duas NOTAMs da pilha some;
  - uma pilha solta sobre outra entra inteira, em cima, na ordem dela.
  - Recusado com `code`: o que não é NOTAM (`notam.stack_kind`), outro nível (`notam.stack_level`) e a própria pilha (`notam.stack_self`).
- **Um fio novo** para qualquer NOTAM da pilha termina na base; um fio entre duas NOTAMs da mesma pilha é recusado.
- **Mover ou redimensionar a base** leva a pilha; mudar de nível também.
- **Tirar da pilha** (`Store.Unstack`): as NOTAMs que saem ficam ao lado da pilha, em fila, com cópias dos fios da pilha.
  - Se a base sai, a próxima NOTAM vira a base e recebe os fios.
  - Uma pilha com uma NOTAM só deixa de ser pilha.
  - "Desfazer a pilha" solta todas.
  - Remover uma NOTAM empilhada tira só ela, pelo mesmo caminho.
- **O acesso dos agentes:**
  - **Notas:** `notes.accessible` segue a pilha como já seguia as notas encadeadas: quem está ligado à pilha lê e escreve cada NOTAM dela pelo nome, pela CLI e pelo MCP.
  - **Listas:** o `torre list` e o `torre note list` mostram `stack: "Plano"` (o nome da base).
  - **Menções:** as @menções aceitam as NOTAMs da pilha, no servidor (`compose.go`) e no Transmitir.
  - **Skill e MCP:** os textos explicam a pilha (`make skill-zip`).
- **Rotas, só da interface** (empilhar dá acesso, como ligar um fio na tela): `POST /w/<id>/api/notes/{id}/stack {onto}` e `POST /w/<id>/api/notes/{id}/unstack {all}`. Elas publicam os fios removidos e criados e as NOTAMs atualizadas, e a tela de todas as janelas se atualiza sozinha.
- **Planos de voo:**
  - um plano guarda `stack` e `stackPos` (pela chave da NOTAM no plano), e `Check` confere que a base é uma NOTAM do plano no mesmo nível;
  - selecionar a pilha leva todas as NOTAMs dela;
  - decolar empilha de novo.
- **Pousar um nível** desce as pilhas inteiras para a pista.
- **Interface:**
  - **Empilhar:** arrastar uma NOTAM e soltar com o centro dela dentro de outra NOTAM do nível. A confirmação ("Empilhar Arquitetura sobre Plano?", com quem passa a ler) só aparece quando algum voo ganha acesso. Cancelar, ou um erro, devolve a NOTAM ao lugar.
  - **A pilha na tela:** a faixa "PILHA · N NOTAMs", com as abas e "‹ i/N ›", a NOTAM da aba inteira e as folhas atrás.
  - **Edição de um agente:** quando um agente edita uma NOTAM que não está à mostra, a aba dela ganha um ponto e a faixa avisa "REV escreveu em Arquitetura" por 30 s, com o indicativo. O clique abre a NOTAM; a pilha não troca sozinha.
  - **Menu ⋯:** "Tirar da pilha" e "Desfazer a pilha".
  - **Ícones novos:** `call_split` e `layers_clear` (`make icons`).
- **Decisões sem consulta:**
  - **A posição de quem sai:** à direita da pilha, uma ao lado da outra, para não cair por cima de nada da pilha.
  - **"Desfazer" mantém os fios em todas,** como "Tirar da pilha".
  - **Arrastar uma pilha sobre outra junta as duas,** em vez de recusar.
- **Correção de passagem:** o código em linha (`` `assim` ``) nas NOTAMs saía como um retângulo preto ilegível, porque o papel da NOTAM não definia `--surface-3`. Agora usa o papel um tom mais escuro.
- **Testado aqui:**
  - **Store:** `TestStackWiresAndPlace`, `TestUnstack`, `TestStackFloors`.
  - **Agentes falsos:** `TestStackReachesEveryNote`: ler antes e depois de empilhar, escrever numa NOTAM da pilha, um voo sem fio recusado, `list` e `note list`, menção, tirar da pilha e remover a base.
  - **Planos e níveis:** `TestPlanKeepsStacks` e `TestLandKeepsStacks`.
  - **Claude Code real:** `TestRealClaudeStack`: ligado só à base, ele leu a outra NOTAM e respondeu com o código dela.
  - **Validação:** `make check` com o navegador, `tsc` e o build.
  - **Na tela, com agentes falsos:**
    - empilhar três NOTAMs, com a confirmação;
    - passar pelas abas;
    - o aviso de edição pelo REV e o clique nele;
    - tirar da pilha e o menu;
    - as menções do Transmitir com as NOTAMs da pilha.
- **Não testado:**
  - **Devin real:** sem conta não há sessão. O Devin passa pelas mesmas rotas do servidor (CLI e MCP) que o Claude, e nada mudou na configuração, nos hooks ou no lançamento dele. Codex e Gemini, pelo mesmo motivo.
  - **Windows e Mac:** arrastar para empilhar no Windows e no Mac, com mouse e touchpad.
  - **Limitação conhecida:** abrir a aba de uma NOTAM que um agente acabou de editar não destaca o trecho alterado. A NOTAM monta de novo, sem o texto anterior para comparar; o aviso na faixa e o ponto na aba mostram que ela mudou.

## 2026-10-17, Torre: fim da rodada (F0–F5d)

- **Como andou:** cada fase numa branch a partir da `feat/torre` e mesclada nela quando ficou pronta (F0a, F0b, F1, F2, F3, F4a, F4b, F4c, F5a, F5b, F5c, F5d), sem esperar aprovação, como você pediu. Um PR leva a `feat/torre` para a `main`.
- **Testado no fim, na `feat/torre`:** `make check` com o navegador, `TestRealClaude` e `TestRealClaudePermission` (o Claude Code real: pergunta, hook, resposta pela interface) e `TestRealDevinConfig` (o Devin real lê a configuração com o hook `PermissionRequest` e o MCP `torre`).
- **Precisa de você:**
  - **Windows (Git Bash):** `torre install-skill` (instala o `torre` e troca a skill `squad`), a migração de `~/.squad` para `~/.torre` na primeira subida, um Claude pedindo permissão (o cartão PRIORIDADE e AUTORIZAR pela Torre), `⌥F`/`⌥⇧←` com o Alt do Windows, e um plano de voo decolando;
  - **Mac:** o `Torre.app` no Dock e o mesmo roteiro;
  - **Devin com login:** a PRIORIDADE no Devin (ele entra no estado pelo hook, mas a Torre ainda não responde por ele; veja o PLAN).
- **O que ficou para depois** está no PLAN, seção Torre, "Para depois", cada item com a referência no handoff.

## 2026-10-17, Torre F5d: planos de voo

- **Pacote `internal/plans`:** um plano é um JSON em `~/.torre/plans/<id>.json` com voos (agente, nome, indicativo, papel, líder, comando próprio, pasta relativa, posição e tamanho), NOTAMs (nome e, se a pessoa quiser, o conteúdo), portais (nome e endereço), frequências (pares de chaves) e níveis (nome e branch). As posições partem do canto do plano, em cada nível. `Check` valida tudo, inclusive o arquivo importado (agentes conhecidos, chaves únicas, fios para itens do plano, até 60 itens e 8 níveis, 4 MB).
- **Rotas do hub** (planos de todos os workspaces): `GET /api/plans`, `GET`/`DELETE /api/plans/{id}` e `POST /api/plans/import`. O importado fica marcado (`imported`).
- **Rotas do workspace**, só da interface (decolar inicia processos): `POST /w/<id>/api/plans` (a seleção, o nível aberto ou a sessão inteira) e `POST /w/<id>/api/plans/{id}/apply`.
- **Segurança:** decolar exige `commands` com cada comando próprio do plano, a lista que a interface mostrou em "Antes de decolar"; sem ela, 409 `plan.review`. Os voos de agente rodam o comando de Configurações → Agentes, como qualquer voo novo.
- **Decolar:** os itens vão para o nível aberto, à direita do que já existe lá (160 px depois do último nó). Os níveis do plano viram níveis novos com o nome dele ("Login 2" quando o nome, o branch ou a pasta já existem). Nomes repetidos ganham número, e o indicativo é numerado pelo store (IMP → IM2). Um papel que não existe nesta Torre, uma pasta que não existe e níveis num workspace sem git viram avisos, e o resto decola.
- **Decisão sem consulta: o que salvar.** No canvas só dá para selecionar no nível aberto, então o diálogo oferece três escopos: a seleção, tudo no nível aberto e a sessão inteira com os níveis (o que faz o plano "duas frentes em paralelo" do mockup).
- **Decisão sem consulta:** a galeria começa vazia, sem os planos de exemplo do mockup (ficou no PLAN).
- **Interface:** a galeria (`⌥M`, o botão novo na barra de ferramentas, "Usar um plano de voo" no espaço vazio e "Com um plano de voo" na primeira abertura), com miniatura, composição, importar, exportar e remover (com confirmação); "Antes de decolar", com o comando de cada voo e o aviso para planos importados; e "Salvar como plano" (`⌥⇧M`).
- **Testado aqui:** `TestPlanSaveAndApply` (salvar a sessão com o conteúdo da NOTAM, recusa sem revisão, decolar com nomes repetidos, NOTAM com conteúdo, frequências), `TestPlanLevelWithoutGit`, `TestPlanLevels` (num repositório: o nível "Login" e, na segunda vez, "Login 2"), `TestPlansImportListDelete` e, na tela, salvar a sessão de teste como "Trio com portal", importar um plano com `npm run dev`, a revisão e decolar a equipe ao lado da original.
- **Não testado:** decolar um plano com Claude e Devin reais (os voos nascem como qualquer voo novo, pelo mesmo `AddNode`); importar pela janela de arquivo (testado pela rota).

## 2026-10-17, Torre F5c: @ELEMENTOS no portal

- **Servidor:** o WebSocket do portal aceita `{"type":"elements","on":true|false}`. Ligada, a camada recebe `{"type":"elements","elements":[{ref, role, name, x, y, w, h}],"snapshot":bool}` na hora e de novo a cada segundo quando algo muda (rolagem, snapshot novo, página nova). As caixas são as de agora (`Page.Boxes`, `DOM.getBoxModel` de cada ref), não as do momento do snapshot.
- **Decisão sem consulta:** a camada mostra só as refs do último `torre portal snapshot`. Ligar a camada não tira um snapshot, porque um snapshot novo troca as refs, e o agente que está usando `@e7` passaria a clicar em outra coisa. Sem snapshot, aparece "Sem refs: eles aparecem quando um voo lê a página".
- **Interface:** o botão "@ ELEMENTOS" no cabeçalho do portal (quadro 10) liga e desliga a camada. Os contornos são tracejados, com a etiqueta "@e7 botão" (o papel traduzido), na cor do papel do último voo que agiu na página (ou de um na frequência dele, ou de quem criou o portal), escurecida. A camada não pega cliques.
- **Testado aqui:** `Page.Boxes` antes de qualquer snapshot (nada) e depois de rolar (o link que estava fora da tela aparece no lugar novo, sem snapshot novo), o WebSocket com a camada (antes e depois de um snapshot) e, na tela, um portal com uma página de teste lida pelo PLN (`torre portal snapshot` com o indicativo de um agente falso).
- **Não testado:** páginas com centenas de elementos (cada segundo faz uma chamada ao Chrome por ref; o snapshot já limita a 300).

## 2026-10-17, Torre F5b: foco, encaixe, ocultar e Tab

- **Uma tela por voo:** o terminal tem uma tela só (um xterm), então ela fica num lugar de cada vez: no foco, senão na borda, senão no nó (`web/src/lib/screens.ts`). O nó de um voo no foco ou na borda mostra "Em foco"/"Encaixado à esquerda" com "Trazer de volta"; o retrato e a ficha continuam iguais.
- **Foco (`⌥F`):** cobre o palco (o Espaço ou o Sinóptico), abaixo da barra do topo, e dá o teclado ao voo. `⌥F` de novo, Esc ou o botão do cabeçalho voltam; o zoom do canvas não muda. Age sobre o voo com o teclado, senão o selecionado.
- **Encaixe (`⌥⇧←`/`⌥⇧→`):** um painel de 46% (de 420 a 1100 px) com o divisor de 2 px do quadro 04, e o canvas ao lado, com a barra de ferramentas, o Transmitir e o minimapa dentro da parte que sobra. As mesmas teclas desencaixam. **Decisão sem consulta:** um voo encaixado por vez; o encaixe fica no navegador, por workspace (como o zoom de cada nível), e volta ao abrir.
- **Tamanho da tela:** no foco e na borda, o terminal ganha as colunas e linhas do painel (o mesmo `PATCH` do redimensionar), e volta às do nó ao sair. Ao fechar a página com o painel aberto, a volta vai num `fetch` com `keepalive`.
- **Ocultar a tela** (menu ⋯): listras opacas, não desfoque, no nó, no retrato, no painel e no Sinóptico; "Mostrar a tela" desfaz. **Decisão sem consulta:** guardado no navegador, por workspace (o handoff deixa o `redacted` no nó como opcional; ficou no PLAN).
- **Tab / ⇧Tab:** o próximo nó do nível, na ordem de leitura (faixas de 160 px de altura, depois da esquerda para a direita), trazido para a tela no mesmo zoom quando está fora. **Enter** dá o teclado ao voo selecionado (num zoom pequeno, aproxima antes). Só com a página em foco: um botão ou um campo com o foco ficam com o Tab e o Enter deles.
- **Do terminal:** `⌥F` e `⌥⇧←/→` chegam à Torre mesmo com o teclado num voo ou num portal (como `⌥A`, `⌥P` e `⌥K`). No Windows, `Alt+F` deixa de ir para o shell (avançar uma palavra no readline); o PLAN lembra que trocar os atalhos ainda não existe.
- **Testado aqui:** `tsc`, build e, no navegador com agentes falsos, encaixar o IMP (a tela foi de 80×24 para 96×53), desencaixar (voltou a 80×24), o foco em tela cheia, voltar, ocultar pelo menu e Tab passando por PLN → IMP → REV → NOTAM.
- **Não testado:** com o Claude e o Devin reais redesenhando no tamanho novo (é o mesmo redimensionar do canto do nó, que já funcionava com os dois); o `keepalive` ao fechar uma janela de app de verdade.

## 2026-10-17, Torre F5a: extras

- **Indicativo na CLI e no MCP:** `torre ask IMP "…"`, `check`, `wait`, `connect` e `close` acham o voo pelo indicativo, em qualquer caixa; o nome vale primeiro. O `torre list` mostra `callsign: IMP` no fim de cada linha (no fim para não mudar o começo das linhas, que os agentes já conhecem). O texto da skill e do MCP diz isso.
- **Renomear nível:** `PATCH /w/<id>/api/floors/{id} {name}` e "Renomear…" no menu do nível. Só o nome muda: o id, o número, o branch e a pasta ficam. As regras são as do nível novo (não pode ser "pista", nem um nome de outro nível).
- **Cancelar pedido:** `POST /w/<id>/api/requests/{id}/cancel` e o botão no pedido aberto do Rádio. Quem pediu recebe um 409 dizendo que você cancelou e que `torre wait` pega a resposta se o outro voo continuar. **Decisão sem consulta:** o outro voo não é interrompido (interromper um agente no meio do trabalho é outra decisão, que o usuário toma na tela dele).
- **Pedido inteiro no Rádio:** o barramento guarda o texto dos 200 últimos pedidos (`GET /w/<id>/api/requests/{id}`), e "Ver o pedido inteiro" aparece quando a prévia veio cortada. Os pedidos que já estavam abertos quando a página abriu agora também entram no Rádio (antes só apareciam no contador).
- **NOTAM redimensionável:** pelas bordas, com mínimo de 280×200, como o portal. O servidor já guardava `w` e `h`.
- **Transmitir a um nível:** no "+ destino", "Todos os voos de FL1 · Login" (e da PISTA) para cada nível com dois voos ou mais. Acrescenta os voos do nível aos destinos.
- **Conflito antes do pouso:** `GET /api/floors/{id}/changes` (e o da cópia da sessão) traz `conflicts`, de `git merge-tree --write-tree` contra o branch de origem, só quando os dois lados andaram. As checagens do diálogo mostram "✓ sem conflitos" ou "✕ 1 conflito" com os arquivos. Com o git anterior ao 2.38, `conflicts` vem `null`, e o diálogo volta a dizer "checados ao pousar".
- **Rotas só da interface:** as três rotas novas ficam nas rotas de sessão do workspace, longe do `/cli` e do MCP.
- **Testado aqui:** `TestAskByCallsign`, `TestRequestTextAndCancel` (texto inteiro, cancelar, cancelar de novo dá `request.gone`), `TestRenameAndCleanPrecheck` e o conflito previsto em `TestConflictsChangeNothing`, `make check` com o navegador; na interface, com agentes falsos e um repositório de teste: o Rádio com o pedido inteiro e o cancelamento (o `torre ask` recebeu a mensagem de cancelado), o Composer com "Todos os voos de PISTA", renomear o FL1 pela tela, o pouso com "1 conflito · README.md" e a NOTAM redimensionada e salva.
- **Não testado:** o indicativo com o Claude e o Devin reais (o caminho é o mesmo do nome, no servidor).

## 2026-10-17, Torre F4c: primeira abertura

- **Passo 1, Enlace:** a página de login do Go foi redesenhada sobre o radar, com os passos no topo, o campo da URL e "só 127.0.0.1 · o token fica num cookie". O botão continua "Entrar" (o teste do login procura o texto).
- **Quando aparece:** o hub guarda se subiu sem nenhum workspace (`GET /api/welcome`, `firstRun`). **Decisão sem consulta:** quem já usava o Squad não vê os passos (a pasta migrada já tem workspaces); e, depois de vistos ou pulados, eles não voltam no mesmo navegador.
- **Passo 2, Pasta** (`GET /api/dirs/info`): a pasta do `serve` já preenchida, com o git em até 3 s (repositório, branch, alterações, commit para os níveis) e os `.env*` do topo. Trocar a pasta muda a do workspace inicial, que ainda não tem voos.
- **Passo 3, Agentes:** a busca da F4a, com o comando de instalação. "Com um plano de voo" fica desligado até a F5d.
- **Decisão sem consulta: "Recentes"** são as pastas dos workspaces existentes. A Torre não guarda um histórico de pastas.
- **Testado aqui:** `TestWelcomeAndDirInfo` (instalação nova e segunda subida; um repositório com commit, alterações e dois `.env`; uma pasta sem git; uma que não existe), o login (`internal/server`), e capturas dos três passos num servidor com `TORRE_HOME` novo.
- **Não testado:** trocar a pasta no passo 2 por uma que já é de outro workspace (o hub recusa como antes; a mensagem aparece num aviso).

## 2026-10-17, Torre F4b: Sinóptico

- **Vista nova** (quadro 01), trocada por `⌥S` ou por ESPAÇO · SINÓPTICO na barra. A escolha fica no navegador. Tudo vem do estado que o canvas já tem; não há rota nova.
- **Radar:** um anel por nível (a pista no centro), as posições reais do canvas escaladas dentro do anel do nível, a varredura girando em 6 s, os símbolos por estado, as linhas de pedido aberto e de resposta recente, os contadores e a legenda. Clique seleciona; duplo clique abre o voo no Espaço, centrado.
- **Coluna do meio:** as fichas em papel (pede você › aguarda › no ar › em solo › hangar; o que o voo faz vem da pergunta, de quem ele aguarda, do título da tela ou da última mensagem do boletim), a fila de pouso (níveis com commits, com as checagens e "Autorizar pouso…") e as frequências.
- **Coluna da direita:** o cartão PRIORIDADE (com "1 de N", a pergunta e as respostas; as teclas 1, 2 e 3 respondem a ele) e o terminal do voo selecionado, ao vivo, com o alfinete.
- **Rodapé:** o Rádio passando (40 s por volta; parado com movimento reduzido) e o Transmitir.
- **Decisão sem consulta:** no Sinóptico, as teclas do canvas (T, N, P…) não fazem nada; ele não tem canvas.
- **Fila de pouso:** só o que existe (commits, alterações sem commit, voos no ar); "✓ testes · ✓ revisão" do mockup ficam no PLAN, e o conflito previsto vem na F5a.
- **Testado aqui:** `tsc`, build e capturas com o Claude real pedindo permissão. A tecla 1 no Sinóptico autorizou e o comando rodou.
- **Não testado:** um workspace com vários níveis e muitos voos (o radar foi visto com a pista e três voos).

## 2026-10-17, Torre F4a: Configurações

- **Painel de 1200×780** no lugar do menu ⚙, com abas: Aparência, Agentes, Limites, Resumos, Papéis…, Atalhos e Avisos. Fecha com Esc e clique fora.
- **Aparência:** o tema "Sistema" é novo e segue o computador enquanto a página está aberta. Quem já tinha escolhido Escuro ou Claro continua com a escolha.
- **Agentes** (`GET /api/agents`, `PUT /api/agents/{agent}`):
  - a busca acha o primeiro termo do comando padrão no PATH e roda `--version` (até 4 s, em paralelo); o resultado vale 5 min, e "Procurar de novo" refaz;
  - o comando padrão editável fica em `~/.torre/agents.json` e vale para os voos que decolarem depois, em todos os workspaces. É uma rota só da interface (decide o que roda em cada voo de agente), uma linha de até 300 caracteres;
  - o Novo voo usa a mesma busca para o "✓ instalado". Antes vinha da busca dos resumos.
- **Limites** (`GET/PUT /w/<id>/api/limits`):
  - guardados no workspace e compartilhados pelas sessões, como a configuração dos níveis e a dos resumos;
  - valem na hora: o barramento, o líder (`torre new`) e os portais (`torre portal create`) leem o valor novo;
  - 0 volta ao padrão do servidor (6 saltos, 30 por minuto, `-max-team` do `serve` e 3 portais);
  - o Rádio recebe o evento `limits.changed`.
- **Avisos**, guardados por navegador: o número no título (já existia na F3, agora pode ser desligado), uma notificação do sistema quando a janela não está na frente e dois toques curtos (WebAudio, sem arquivo de som).
- **Decisão sem consulta: os atalhos só são listados**, como na revisão do handoff (trocar fica no PLAN).
- **Testado aqui:** o comando padrão, a busca e a linha de lançamento com um "claude" falso no PATH (`TestDefaultCommandAndDetect`), a API de limites (`TestLimitsAPI`: em vigor no barramento, no líder e nos portais, faixa recusada, volta ao padrão), a suíte inteira, `tsc`, build e capturas das abas.
- **Não testado:** a notificação do sistema e o som num navegador de verdade (o Chromium sem tela daqui não mostra notificações); a busca no Windows (as versões vêm de `.cmd` do npm, que o Go roda).

## 2026-10-17, Torre F3: PRIORIDADE, o voo pede você

- **Spike com o Claude Code 2.1.302 real** (no modo de permissão padrão, num PTY):
  - o hook `PermissionRequest` chega antes do diálogo, com `tool_name` e `tool_input` (o comando, o arquivo); o `Notification` chega junto, com `notification_type: "permission_prompt"`;
  - a tela mostra "Do you want to proceed?" e as opções numeradas: `1. Yes`, `2. Yes, and always allow access to … from this project`, `3. Yes, and switch to auto mode`, `4. No`;
  - a tecla do número escolhe e confirma na hora. Negar interrompe o turno **sem** o hook `Stop` nem o `PostToolUse`.
- **Estado novo `ask`** (`internal/activity`): vem antes de `wait` (`err`/`off` > `ask` > `wait` > `work`…).
  - **Entra:** pelo hook `PermissionRequest` (Claude e Devin; o hook só avisa e nunca devolve uma decisão), ou por um prompt lido na tela parada há 1,5 s.
  - **O que conta como prompt** (`internal/attention`): uma pergunta terminada em "?" sobre duas ou mais opções numeradas, uma delas com o cursor, no fim da tela (abaixo, só o rodapé "Esc to cancel"). Assim, uma lista numerada na resposta do agente não conta, nem um prompt velho em cima do shell quando o agente sai.
  - **Sai:** quando o prompt some da tela (duas amostras), quando chega o hook de começo ou fim de turno, quando o processo para, ou (sem prompt legível, como no Devin) quando o voo volta a produzir saída depois de ficar parado.
  - Shells e comandos próprios nunca são lidos.
  - Um turno parado esperando você não termina por silêncio, então quem fez `torre ask` continua esperando em vez de receber a tela.
- **O que se responde:** cada opção vira `allow`, `always` ou `deny`, ou nada. "Yes, and switch to auto mode" e "allow all edits during this session" trocam o modo de permissão e **nunca** são oferecidas; SEMPRE só quando a opção é "always allow"/"don't ask again" (decisão da revisão do handoff).
- **Rota só da interface:** `POST /w/<id>/api/nodes/<id>/attention/respond`.
  - Só agentes; só se a pergunta na tela ainda é a mesma que a interface mostrou (senão, 409 `attention.changed`); só as teclas verificadas.
  - **Decisão sem consulta:** só o Claude está em `verified`. O Devin, o Codex e o Gemini entram em PRIORIDADE mas mostram só "Abrir tela", até alguém conferir as teclas na CLI real (fica no PLAN). Os rótulos do Devin 3000.11.92, lidos do binário ("Yes, allow …", "Yes, always allow …", "No, deny"), já são classificados.
- **Mensagens esperam:** o barramento não cola um pedido num voo em `ask` (o texto viraria a resposta, e "pedido 7f3a" tem dígitos que escolheriam uma opção). Ele entra depois da resposta, até o limite de espera.
- **API:** `TerminalRuntime.attention` (`kind`, `tool`, `text`, `detail`, `source`, `since`, `choices`) e `WorkspaceSummary.attention`. **Decisão sem consulta:** a fila da prioridade não foi para o `hello`; a interface a monta do `runtime`.
- **Interface:**
  - no voo, o contorno âmbar com halo, a faixa piscando a 1 Hz (só esse estado pisca), "PEDE VOCÊ 0:24", a faixa de 26 px com a frase e o comando, e a barra RESPONDER embaixo, fora do nó;
  - a faixa "PRIORIDADE · TST em FL1 quer executar um comando · IR ATÉ ELE ⌥⏎" quando o voo não está na tela, o ⌥⏎, as teclas 1/2/3 com o voo selecionado, o número no título da janela e o ponto âmbar na aba do nível, no botão de workspaces e "1 pede você" no seletor;
  - no Rádio, o pedido preso no topo com os botões, o filtro "Pedem você" e o histórico "Pediu você · respondido em 0:46"; o briefing chama o estado de "esperando a permissão ou a resposta do usuário".
- **Testado aqui:**
  - `internal/attention`: os prompts do Claude (comando, edição, MCP), os do Devin, Gemini e Codex como os conhecemos, prosa e shell;
  - com o agente falso: a transição pelo hook, a resposta pela rota (a tecla chega), o pedido de outro voo que espera e chega depois, um agente sem teclas verificadas só pela tela, um comando próprio nunca lido, e a saída do processo;
  - **com o Claude real:** `TestRealClaudePermission` (o voo pede, a rota autoriza, o comando roda e a resposta volta pelo `Stop`); e na interface, clicando AUTORIZAR no Rádio, o arquivo foi criado;
  - o Devin real lê a configuração com o hook novo (`TestRealDevinConfig`);
  - capturas do voo pedindo, da faixa fora da tela, do Rádio e do depois.
- **Não testado:** o Devin pedindo permissão numa sessão de verdade (precisa de conta); o Codex e o Gemini (não instalados aqui); notificações do sistema (vão na F4a, em Avisos).

## 2026-10-17, Torre F2: confirmações, espaço vazio e erros em português

- **`ConfirmDialog`** (quadro 15): substitui as 10 `window.confirm` e as confirmações embutidas nos menus.
  - Cobre remover voo, NOTAM, sessão, workspace e papel; nova conversa; trocar o papel; reiniciar quem usa um papel; descartar as alterações de um papel; restaurar um papel; trocar de sessão com voos no ar; remover uma sessão com alterações sem commit na cópia. Descartar um nível ganhou a mesma borda e o mesmo botão.
  - A pergunta é o título, com o indicativo ("Remover o voo PLN?"); a lista diz o que se perde (✕) e o que fica (✓); o botão é o verbo.
  - A borda dá a gravidade: vermelha (perde trabalho), âmbar (interrompe), clara (só reinicia).
  - "Entendo que esse trabalho se perde." só quando há trabalho sem commit; Esc cancela; Enter não confirma as vermelhas (o foco começa em Cancelar).
  - `confirmDialog(...)` devolve uma promessa e pode ser chamada de qualquer lugar (inclusive de `lib/roles.ts`). Os menus que a abrem não fecham com o clique dentro dela, e o Esc dela não fecha o painel de baixo.
- **Trocar de sessão com voos no ar:** era uma caixa dentro do menu; agora é o diálogo "Trocar de sessão?", com os indicativos de quem para no meio e o que volta (a arremetida, com "Continuar").
- **Espaço vazio** (quadro 20): "ESPAÇO AÉREO VAZIO · workspace", "Ponha a primeira equipe no ar." e os botões de 52 com o atalho. **Decisão sem consulta:** "Usar um plano de voo ⌥M" aparece desligado até a F5d.
- **Arremetida e espera geral** (quadro 21): o voo interrompido mostra o chip "arremetida" no cabeçalho, e a faixa do topo usa indicativos. Na espera geral, cada frequência ganha o quadrado de 26 com ∞ (antes era "∞ retido").
- **Sem enlace** (quadro 19): depois de 4 s sem o servidor, um cartão "O torre serve parou? Tentando de novo em 3 s…", com a contagem da próxima tentativa. Uma queda curta só mostra "RECONECTANDO…" na barra.
- **Erros em português:**
  - pacote novo `internal/apierr`: um erro ganha `code` e `params`, e a resposta da API fica `{"error", "code", "params"}`. O `error` continua em inglês, porque os agentes, a CLI e os logs o leem;
  - 57 erros marcados na origem: pasta, workspace, sessão, voo, NOTAM, papel, nível, cópia da sessão, git e resumos;
  - a web traduz pelo `code` (`web/src/api/errors.ts`); sem `code` conhecido, mostra a mensagem do servidor.
- **Testado aqui:** testes Go novos (`apierr`, o `code` de "papel repetido" e de "último workspace" pela API, o de "pasta não encontrada"), a suíte inteira, vet no Linux e no Windows, `tsc`, build e capturas: espaço vazio, "Remover o voo PLN?", "Já existe um voo chamado Planejador" traduzido no Novo voo e o cartão sem enlace (derrubando o servidor com a página aberta).
- **Não testado:** o cartão de laço da espera geral (fica para depois, veja o PLAN); o Enter nas confirmações vermelhas com leitor de tela.

## 2026-10-17, Torre F1: redesenho do que existe

- **Indicativo do voo (`callsign`):**
  - até 4 letras e números, único no canvas da sessão (`workspace.json`, `TerminalData.callsign`);
  - vem da sigla do papel; o segundo voo com a mesma sigla ganha número (IMP, IM2, IM3… e IM10, cortando a sigla para caber em 4);
  - sem papel, vem das consoantes do nome (`ids.Callsign`: Claude → CLD, Gemini → GMN, Ana → NAA);
  - escolhido na criação, é usado como está e numerado se já existe; numa edição, um repetido é recusado (409);
  - trocar o papel troca o indicativo junto só se ele tinha vindo da sigla ou do nome; um escolhido fica;
  - voos de antes ganham indicativo quando o workspace abre.
- **Sigla do papel (`RoleDef.abbr`):** até 3 letras, única. Os prontos têm PLN, ARQ, IMP, REV e TST. Um papel sem sigla ganha as consoantes do nome (numeradas se repetem); uma sigla repetida escolhida por você é recusada. Papéis de antes ganham a sigla quando a biblioteca abre.
- **Número do nível (`Floor.number`):** FL1, FL2… por sessão (`workspace.json`, `floorSeq`); um número não volta depois que o nível pousa ou é descartado. Níveis de antes são numerados na ordem em que existem. O `torre floor list` mostra o número.
- **Decisão sem consulta: indicativo e número por sessão**, como o plano aprovado (as sessões têm canvas próprio desde o S2). Duas sessões podem ter um IMP cada.
- **Telas redesenhadas** (quadros 02–04, 06–17 e 22 do `docs/torre/TELAS.md`):
  - **topo:** caminho logo › workspace › sessão › níveis; à direita, ESPERA GERAL, RÁDIO com a contagem, ENLACE e o relógio;
  - **níveis:** abas com PISTA ou FL{n}, nome, branch, estado do git e ▲ quando há voo no ar; o cartão "Nível vazio"; Novo nível com o número; "Autorizar pouso · FL2" com as checagens; Descartar FL2;
  - **voo:** faixa de estado à esquerda, cabeçalho com quadradinho do papel, indicativo, nome, agente, chips (líder, teclado) e estado em texto; rodapé com nível, branch, pasta e "criado por PLN"; tarja de Hangar e Pane com INICIAR; retrato (zoom < 60%) e faixa de progresso em papel (zoom < 35%);
  - **frequências:** tracejado correndo durante um pedido, pontilhado quando retido, pílulas quadradas e DESLIGAR;
  - **Transmitir:** a barra fechada com o indicativo do voo selecionado; aberto, com chips de destino;
  - **Rádio:** filtros (Todos, Pendentes, um chip por voo), pedidos abertos no topo com "Ver no espaço", linhas em indicativos com o desfecho, rodapé com a taxa e o limite;
  - **boletim** (BOLETIM · duração), **briefing** (PEDE ATENÇÃO em âmbar, POR VOO com o indicativo), **NOTAM** (etiqueta, papel e tinta), **portal**, **minimapa** com a varredura, **barra de ferramentas**, **arremetida**;
  - **Novo voo:** 420 de largura, agentes em blocos com "✓ instalado"/"✕ não achado", NOME + INDICATIVO, PAPEL, PASTA, AVANÇADO (comando, shell, líder) e DECOLAR; em Configurar, o aviso âmbar e SALVAR E REINICIAR;
  - **menu ⋯ do voo:** "Parar · vai ao hangar", "Nova conversa…", "Líder", a seção PAPEL e "Remover…";
  - **Papéis:** 1200×780, lista de 280 com a sigla, editor com NOME + SIGLA, faixa verde "Salvo. PLN recebe…" com REINICIAR AGORA, rodapé "neste workspace: PLN";
  - **workspaces e sessões:** linhas com a cor, o nome com ⌥N, a pasta e "2 no ar · 1 pendente"; ATUAL na sessão aberta.
  - **avisos (toasts):** ok, aguarde e erro, com ícone.
- **Decisão sem consulta: "✓ instalado" no Novo voo** vem da mesma busca no PATH que os resumos já faziam (`summary` → `engines`). A F4a traz `GET /api/agents`, com a versão.
- **Decisão sem consulta: os chips "teclado" e "líder"** ficaram no cabeçalho (o handoff mostra os dois ali), e o estado virou texto sem fundo, com a cor do estado.
- **README:** o texto de uso foi revisto com o vocabulário novo e ganhou "Indicativos e níveis".
- **Testado aqui:** testes Go novos (`TestCallsigns`, `TestFloorNumbers`, `TestCallsignsFromRoles` pela API, e os de papéis com sigla), a suíte inteira, `tsc`, build da web e capturas no escuro e no claro: canvas, Novo voo (com Avançado), Papéis, workspaces, sessões, menu ⋯, Rádio e um Claude real com o papel Revisor (REV).
- **Não testado:** o Devin real no formulário (o "instalado" depende do PATH do servidor); renderização no Windows e no Mac; o retrato e a faixa de progresso com agentes trabalhando (os estados aparecem na F3, com a PRIORIDADE).

## 2026-10-17, Torre F0b: base visual e vocabulário

- **Tokens:** os da Torre (`docs/torre/tokens/torre-tokens.css`) em `web/src/theme/tokens.css`:
  - fundo `#030705`, canvas `#050b08` com pontos `#15301f`, superfícies verdes, texto `#cfeedd`, estados com faixa (`--st-*`), texto (`--stt-*`) e fundo (`--stb-*`), papel e tinta para as NOTAMs, cores de workspace (`--ws-*`) e tema claro em papel creme;
  - cantos retos em tudo; só os pontos de estado continuam redondos.
- **O que o arquivo do handoff não tinha** (completado aqui, marcado "added" no CSS):
  - o fundo tingido de TAXIANDO, PÁTIO e HANGAR;
  - as cores de papel no tema claro para Arquiteto, Testador e os papéis novos (o handoff só dá Planejador, Implementador e Revisor; os outros foram escurecidos na mesma medida);
  - "sem papel";
  - os nomes antigos que o código usava (`--grid`, `--note-bg`, `--shadow`, as alturas), que viraram apelidos dos novos.
- **Texto dos estados:** onde a cor do estado era a do texto, agora é a de texto (`--stt-*`), mais clara no escuro. A faixa sólida (`--st-*`) fica para fundos e bordas, como pede o handoff.
- **NOTAM em papel nos dois temas:** dentro dela, as cores de texto e de linha viram tinta e papel. Antes, o título sumia no escuro.
- **Fontes:** B612 e B612 Mono (400 e 700), embutidas como o Geist era.
- **Decisão sem consulta: os terminais continuam com Geist Mono.**
  - A 11 px, o B612 Mono tem 7,15 px por coluna: 80 colunas dão 572 px, mais que o voo padrão de 560 px.
  - O subconjunto latino dele não tem os símbolos dos agentes (⏺, ●, ✓, braille).
  - É a alternativa que o próprio arquivo de tokens sugere. O resto da interface usa B612 Mono nos dados.
- **Temas do terminal:** Noite e Papel recalibrados (handoff), ainda independentes do tema da interface.
- **Ícones:** 25 novos (os 22 da lista do handoff mais `link`, `stop` e `sync`, que os mockups usam e a lista esquecia). A fonte foi regerada com `make icons`; são 108 ícones.
- **Vocabulário em toda a interface:**
  - voo, frequência, nível, pista, pousar e pedir pouso, espera geral e liberar todos;
  - Rádio, Transmitir, NOTAM, boletim, briefing, líder, arremetida, Enlace;
  - estados: Taxiando, Pátio, No ar, Aguarda, Em solo, Pane (com a saída), Hangar, com os ícones `flight` e `flight_land`;
  - retido mostra o motivo ("espera geral" ou "limite por minuto"), e o fim de turno no Rádio aparece como "Em solo".
- **Para os agentes:**
  - as instruções prontas dos papéis falam em nível de voo, pista e pousar (os comandos continuam `torre floor create/land`), e o material do briefing também;
  - `--floor pista` vale como `--floor ground`.
- **README:** ganhou a tabela de vocabulário. O texto de uso inteiro é revisto na F1, quando as telas mudam.
- **Testado aqui:** `tsc`, build da web, `go test` do `summary`, `roles` e `store`, e capturas no tema escuro e no claro (canvas, voos, frequência, NOTAM, barra e Transmitir).
- **Não testado:** como o B612 fica no Windows e no Mac (a renderização de fonte muda); a F1 refaz cada tela com as medidas dos mockups.

## 2026-10-17, Torre F0a: Squad agora é Torre

- **Pedido do usuário:** trocar o nome para Torre e redesenhar a interface a partir do handoff (agora em `docs/torre/`). O plano por fases foi aprovado com as recomendações, e as fases entram na branch `feat/torre` sem esperar a aprovação de cada uma (veja o PLAN).
- **O que a revisão do handoff achou no código** (decidido com as recomendações aprovadas):
  - **hook `Notification`:** o handoff diz que ele "já é tratado"; não é. O Claude registra só `Stop`, `StopFailure`, `UserPromptSubmit` e `SessionStart`, e o Devin só `Stop`, `UserPromptSubmit` e `SessionStart`. O Claude 2.1.302 e o Devin 3000.11.92 têm `PermissionRequest` e `Notification`, que a F3 vai usar;
  - **SEMPRE não é sempre a tecla 2:** neste container, a opção 2 do Claude apareceu como "Yes, and switch to auto mode". A F3 lê as opções na tela e só oferece SEMPRE quando é "não perguntar de novo";
  - **níveis e indicativos são por sessão**, não por workspace: desde o S2, os andares são da sessão;
  - **erros em português:** a API ganha `code` e a interface traduz; os agentes e a CLI continuam em inglês;
  - **variáveis que o handoff não listava:** `SQUAD_PORT`, `SQUAD_DEBUG`, `SQUAD_CLI` e as que o servidor põe nos terminais;
  - **lugares do nome fora da lista:** o app do Dock no Mac, o cookie, o backup do Devin, o zip da skill e o agente falso dos testes.
- **Comando:**
  - o código da CLI foi para `internal/cli`;
  - `cmd/torre` e `cmd/squad` são dois `main` de poucas linhas. O `squad` é o mesmo comando: avisa no stderr que o nome mudou, menos em `hook` e `mcp`, que os agentes rodam;
  - a instalação vira `go install …/cmd/...@main`, que instala os dois.
- **Pasta de dados:**
  - `TORRE_HOME`, depois `SQUAD_HOME`, depois `~/.torre`;
  - o `torre serve` move `~/.squad` para `~/.torre` na primeira vez, com tudo dentro (workspaces, sessões, papéis, token de login, perfis);
  - se um `squad serve` antigo ainda roda dali, a pasta não se move e é usada como está (o `serve` novo abre o workspace no servidor antigo, como já fazia com um servidor rodando);
  - se mover falhar (no Windows, um arquivo aberto, como a janela de app de antes), usa `~/.squad` desta vez e tenta de novo no próximo início.
- **Variáveis:**
  - `TORRE_PORT`, `TORRE_SHELL`, `TORRE_BROWSER` e `TORRE_DEBUG`, com reserva nos nomes `SQUAD_…` e aviso no log;
  - os terminais recebem `TORRE_URL`, `_TOKEN`, `_TERMINAL_ID`, `_TERMINAL_NAME` e `_CLI`, e também as `SQUAD_…`. Assim, um `squad` antigo que tenha ficado no PATH, e agentes que retomam conversas de antes, continuam funcionando. O mesmo vale para `TORRE_FLOOR` no comando de preparo;
  - o servidor aceita o cabeçalho `X-Squad-Token` além do novo.
- **Agentes:**
  - servidor MCP `torre` (`mcp__torre__*`) no Claude, no Codex e no Devin; hooks chamando `torre hook`;
  - o `install-skill` instala a skill `torre`, remove a `squad` (só se for a que o Squad escreveu) e, no Devin, troca a entrada `squad` do `mcp_config.json` pela `torre` (só se for a do Squad: o binário `squad` com o argumento `mcp`);
  - textos da skill, dos papéis prontos, da ajuda da CLI e do MCP com `torre`.
- **Outros:**
  - andares e cópias de sessão novos em `<pasta-pai>/.torre/…`;
  - `TERM_PROGRAM=Torre`;
  - no Mac, o app do Dock `Torre.app`, ou o `Squad.app` de antes;
  - o cookie `torre_session`: o antigo continua valendo e é trocado na primeira página.
- **Interface:**
  - logo da torre (SVG do handoff) com "TORRE";
  - favicon e ícones PNG (192, 512 e 180, desenhados a partir do SVG);
  - manifest e cor da barra de título (`#07100b` / `#f6f2e4`);
  - título "workspace · Torre";
  - as chaves do navegador `squad.*` (workspace, andar, posição do canvas, temas) passam para `torre.*` na primeira carga;
  - página de login com a marca nova (a primeira abertura completa é a F4c).
- **Decisões tomadas sem consulta:**
  - **o atalho `squad` é um binário completo**, e não um script que chama o `torre`: funciona igual no Windows, sem depender de achar o outro no PATH;
  - **as chaves do navegador ficam com ponto** (`torre.theme`), como eram, e não `torre:` como no handoff;
  - **o `.squad-backup` do Devin vale como backup:** se ele existe, não se cria um `.torre-backup`, porque o original do usuário já está guardado.
- **Testado aqui:**
  - testes do `config`: a migração com servidor antigo rodando e sem ele, uma vez só, e os nomes antigos das variáveis;
  - testes do `cli`: o atalho `squad` e a remoção da skill antiga;
  - teste do Devin: a entrada antiga do MCP trocada, e uma do usuário com o mesmo nome mantida;
  - teste do servidor: o cookie antigo aceito e trocado;
  - `go vet` (Linux e Windows) e todos os testes, com o navegador;
  - `TestRealClaude` e `TestRealClaudeRole` (hooks e papel pelo `torre hook`), e `TestRealDevinConfig` (o Devin vê o MCP `torre`);
  - de ponta a ponta, com o binário: um `~/.squad` de teste foi movido para `~/.torre` no primeiro `torre serve`, com o mesmo token. Um terminal recebeu as duas famílias de variáveis, e `squad list` (com o aviso) e `torre list` funcionaram dentro dele.
- **Não testado:** Windows (a migração com arquivo preso, `torre.exe` e `squad.exe`), o `Squad.app` no Mac, o Devin com login, Codex e Gemini.

## 2026-10-17, S4: sessão numa cópia isolada

- **Pedido do usuário:** criar uma sessão nova enquanto a sessão 1 está no meio do trabalho, sem misturar os arquivos, com a opção de um branch novo a partir de um branch escolhido, e sem encher a tela de opções.
- **Nova sessão, "Onde a sessão trabalha":**
  - "Na pasta do projeto" ou "Numa cópia isolada";
  - na cópia, "A partir de" (o branch atual vem marcado) e "Branch novo" (o nome da sessão sem acentos, ou `sessao-<dia><mês>-<hora>`, com `-2`, `-3` se já existir);
  - se a sessão atual tem agentes trabalhando ou arquivos alterados sem commit, aparece o aviso ("Carrinho tem 1 arquivo alterado sem commit"), e a cópia já vem marcada;
  - numa pasta que não é repositório git, a escolha não aparece.
- **Como a cópia funciona (servidor):**
  - é um `git worktree` num branch novo, feito como um andar: do último commit do branch escolhido, com os arquivos da lista de cópia (`.env*`) e o comando de preparo, que roda quando a sessão abre (e de novo, se ela fechar antes de ele terminar). Fica em `<pasta-pai>/.squad/sessions/<projeto>--<branch>`;
  - o canvas da sessão guarda a cópia (`copy` no `workspace.json` da sessão, fora da parte compartilhada). O store ganhou a "pasta de trabalho": a cópia, quando existe, ou a pasta do workspace;
  - os terminais resolvem as pastas a partir dela, e uma pasta do projeto dada por inteiro aponta para a cópia;
  - os andares nascem da cópia e se chamam `<projeto>--<branch>--<andar>`, ao lado dos outros;
  - o estado do git no seletor (o térreo) é o da cópia, com os commits ainda não mesclados;
  - o Devin recebe `--respect-workspace-trust false` na cópia, como nos andares.
- **Mesclar:**
  - no menu do térreo (clicando no branch da sessão) ou em "Mesclar…" no painel; reaproveita o diálogo de aterrissar: commits, branch de destino (o de origem vem marcado), conflitos;
  - **a cópia continua**: a sessão segue trabalhando e pode mesclar de novo. O Claude e o Devin guardam as conversas por pasta, então apagar a cópia perderia as conversas da sessão;
  - é recusado com alteração sem commit na cópia, ou na pasta onde o branch de destino está aberto. É o caso da pasta do projeto com o trabalho da sessão 1: o diálogo diz isso em português e lista os arquivos. Antes, essa recusa (que vale também para aterrissar um andar) aparecia em inglês, com a mensagem do servidor.
- **Remover a sessão:** apaga a cópia e mantém o branch, com o que foi commitado. Com alterações sem commit na cópia, o servidor recusa (409 com os arquivos) e o painel lista os arquivos, com "Remover mesmo assim".
- **Decisões tomadas sem consulta:**
  - **o branch fica** ao remover a sessão: assim nada commitado se perde, e apagar o branch é fácil depois;
  - **remover o workspace ou mudar a pasta dele** é recusado enquanto houver sessão em cópia, como já era com os andares. Remover as cópias junto apagaria trabalho sem perguntar;
  - **o branch novo precisa ser novo:** abrir um branch existente numa sessão (como o `--existing` dos andares) fica para depois, se você quiser.
- **Testado aqui:**
  - testes do `floors`:
    - a cópia a partir de outro branch, sem o trabalho não commitado da pasta do projeto;
    - o preparo ao abrir e o estado do git;
    - um andar nascendo da cópia;
    - mesclar: recusa com alteração na cópia e na pasta de destino; mesclar no branch de origem com a cópia continuando;
    - remover mantendo o branch;
  - teste do `hub`: sessão em cópia pela API, terminal rodando na cópia, branch repetido recusado, pasta do workspace travada, remoção com 409 e com `confirm`;
  - teste do Devin com a flag na cópia;
  - `make check`, `TestRealClaude` e `TestRealDevinConfig`.
- **Claude real**, num repositório de teste:
  - a sessão "Carrinho" teve um Claude que acrescentou "TODO: carrinho com cupom" ao README, sem commit;
  - no formulário de nova sessão, o aviso apareceu e a cópia veio marcada. Criei "Login novo" com os mesmos terminais;
  - o Claude da sessão nova abriu na cópia, sem a pergunta de confiança. Perguntado, disse que o README não tem o TODO, e fez o commit "login" na cópia;
  - a pasta do projeto ficou só com o TODO, sem o `login.txt`;
  - "Mesclar" em `main` foi recusado (a pasta do projeto tem o README alterado); em `dev`, mesclou, e a cópia continuou;
  - de volta ao "Carrinho", tudo estava como antes;
  - remover "Login novo" com um arquivo sem commit na cópia mostrou a lista e "Remover mesmo assim". Depois disso, a cópia sumiu, e o branch `login-novo` ficou.
- **Devin real:** um terminal Devin na sessão em cópia abriu na pasta da cópia com `--respect-workspace-trust false` (sem login, ele para na tela de entrar).
- **Observação:** a pergunta de confiança do Claude não aparece na cópia quando a pasta do projeto já é confiada. No teste, foi preciso confiar no repositório de teste uma vez.
- **Não testado:**
  - Devin com login trabalhando na cópia;
  - Codex e Gemini;
  - Windows: o `git worktree` e a remoção da cópia (com o mesmo cuidado dos andares para pastas presas por um instante);
  - um comando de preparo de verdade (`npm install`) numa cópia, que só foi testado com `echo`.

## 2026-10-17, S3: sessões na interface

- **Topo:** a sessão aberta aparece entre o workspace e os andares ("squad › Login novo ▾ › térreo"): workspace, sessão e andar, do maior para o menor. Clicando, abre o painel de sessões.
- **Painel de sessões:**
  - cada sessão mostra o nome, a criação, a última abertura, quantos terminais, notas, portais e andares tem, e os agentes; a aberta tem a marca "atual";
  - uma sessão sem nome ("Nova sessão · 17/10 14:32") aparece em cinza até o agente dar o título;
  - ações: **Abrir**, **Renomear** (no lugar, Enter salva e Esc desiste), **Remover** e **Nova sessão** (nome opcional e "Começar com os mesmos terminais, sem as conversas", que só aparece quando o térreo tem terminais).
- **Agentes trabalhando:** a recusa do servidor (409) vira um aviso dentro do painel, com os nomes de quem trabalha e os pedidos em andamento, e os botões **Cancelar** e **Trocar mesmo assim**, que reenvia com `confirm`.
- **Seletor de workspaces:** passar o mouse num workspace (250 ms) ou clicar na seta com o número de sessões mostra as sessões dele ao lado. Clicar numa delas abre a sessão (com o mesmo aviso, se houver agentes trabalhando) e mostra aquele workspace.
- **Novo workspace:** campo "Nome da sessão (opcional)".
- **Como a tela segue a sessão:**
  - o estado guarda a sessão aberta do workspace na tela;
  - quando ela muda (evento `sessions` ou a lista de workspaces), o canvas, a atividade e os cartões são limpos e os eventos reconectam;
  - isso vale também quando a troca vem de outra janela ou da API;
  - o nome no topo vem primeiro da lista de sessões, que chega na hora; a lista de workspaces chega um instante depois;
  - o estado do git no seletor de andares é buscado de novo, e o enquadramento do canvas (posição e zoom) fica guardado por sessão.
- **Decisões tomadas sem consulta:**
  - **Abrir é um botão**, não um clique na linha: abrir outra sessão para os agentes desta, então não pode ser um clique acidental;
  - **Remover** pede confirmação do navegador, como remover um workspace;
  - **"Última abertura"**, não "última atividade": o servidor guarda quando a sessão foi aberta;
  - **os agentes aparecem como etiquetas de texto** ("claude", "devin"), como no cabeçalho do terminal: o Squad não tem ícones de agente;
  - **ícone novo:** `history`, na sessão do topo.
- **Correção encontrada no teste: conversa vazia (servidor).**
  - **O problema:**
    - um Claude que nunca recebeu mensagem tem um id de conversa (`--session-id`), mas o Claude só grava a conversa na primeira mensagem;
    - ao voltar para a sessão, o `--resume` falhava, a linha começava uma nova, e a tela avisava "Não deu para retomar a conversa";
    - isso acontecia em toda troca de sessão, para cada terminal ainda sem uso.
  - **A correção:**
    - o terminal ganha a marca `empty`. Ela é ligada quando uma conversa começa (id novo escolhido pelo Squad, ou o hook de início com `source` `startup` ou `clear`) e desligada no primeiro turno;
    - com a marca, o terminal começa uma conversa nova direto, sem tentar retomar e sem aviso;
    - "Copiar comando de retomada" some do menu enquanto a conversa está vazia;
    - o Codex não precisa dela: o id dele só chega no fim de um turno.
  - **Terminais de antes da correção:** não têm a marca, então um terminal que nunca foi usado ainda pode mostrar o aviso uma vez. Depois disso, fica marcado.
  - **Devin:** a marca vem do hook de início com `source: startup`. Sem login, não deu para conferir esse valor; se o Devin usar outro, ele continua como antes (tenta retomar e, se não achar, começa uma nova).
- **Testado aqui:**
  - testes do S1 com o agente falso, ajustados:
    - reiniciar sem nenhum turno começa uma conversa nova (id novo, sem `--resume`, sem aviso);
    - depois de um turno, retoma;
    - depois de uma retomada que falhou, a próxima abertura não tenta de novo;
    - o Devin retoma depois de um turno;
  - `make check` (com os testes do navegador), `TestRealClaude` e `TestRealDevinConfig`;
  - **Claude real num `serve` isolado**, com a tela aberta pelo Chromium:
    - montei três sessões no workspace do Squad (Planejador, Implementador e Revisor Devin com uma nota e fios; uma sem nome com um Claude; "Revisão do hub" com dois Claudes e uma nota);
    - painel com as três, renomear no lugar e criar com "os mesmos terminais" (Revisor e Testador voltaram, com conversas novas);
    - mandei o Planejador ler e resumir os arquivos do `internal/hub`. No meio do turno, "Abrir" outra sessão mostrou "Planejador está trabalhando"; **Cancelar** manteve tudo; **Trocar mesmo assim** trocou o canvas e o nome no topo;
    - voltando para a sessão, apareceu "Planejador foi interrompido", com a conversa de antes na tela;
    - remover uma sessão pelo painel;
    - workspace novo "loja" pelo formulário, com a sessão "Checkout", e as sessões do workspace "squad" ao lado dele no seletor. Abrir "Revisão do hub" dali trocou de workspace e de sessão de uma vez;
    - uma sessão sem nome, com um Claude perguntado sobre login por link mágico, virou "Trocar login por link mágico" no topo;
    - ida e volta entre uma sessão já usada e uma com terminais sem uso: nenhum aviso, e o nome do topo mudou em menos de 0,1 s.
- **Observação:** o Devin sem login fica numa tela com contador, que o monitor lê como trabalho. Por isso, aqui, o seletor pediu confirmação por causa do Revisor Devin. Com login, a tela dele fica parada entre os turnos.
- **Não testado:**
  - Devin com login: a confirmação com um Devin trabalhando e a conversa vazia;
  - Codex e Gemini;
  - Windows e Mac reais: passar o mouse no seletor, renomear no lugar e a confirmação do navegador ao remover.

## 2026-10-17, S2: sessões no servidor

- **O que é uma sessão:** um canvas inteiro do workspace (terminais, notas, portais, fios, posições, andares, cartões de turno), numa pasta própria em `~/.squad/workspaces/<id>/sessions/<sessão>/`. O `sessions.json` do workspace lista as sessões e diz qual está aberta.
- **Como a troca funciona:**
  - o `hub` monta o `app` do workspace sobre o canvas da sessão aberta. Abrir outra fecha esse `app` (terminais, portais, monitor) e monta outro sobre o canvas da outra sessão;
  - as notas, os anexos e os cartões já derivavam a pasta do arquivo do canvas, então seguem a sessão sem mudança;
  - o endereço `/w/<id>/` é o mesmo;
  - ao fechar, o barramento de eventos agora é fechado, e a tela, que já reconectava sozinha, recarrega o estado da sessão nova;
  - conferido com a tela aberta.
- **O que as sessões compartilham:** o nome, a pasta, a cor e as configurações de andares e de resumos (`store.Shared`). Elas ficam no canvas da sessão aberta, e a sessão que abre recebe essa parte. Assim, renomear o workspace numa sessão vale para todas.
- **Agentes trabalhando:**
  - sair de uma sessão com agentes no meio de um turno, ou com pedidos esperando, é recusado (409, com os nomes) sem `confirm`;
  - com `confirm`, eles param e voltam como interrompidos (S1);
  - "trabalhando" é o turno aberto do monitor, só para agentes. Um shell com um servidor de desenvolvimento não conta.
- **Nova sessão:**
  - vazia, ou com os terminais do térreo da sessão atual: novos ids, sem conversa, com papéis, posições e os fios entre eles. Notas e portais ficam de fora;
  - os terminais de andares ficam de fora, porque os andares são da sessão de origem.
- **Nome:**
  - sem nome, a sessão nasce como "Nova sessão · 17/10 14:32";
  - no fim do primeiro turno do orquestrador, o Squad pede um título ao mesmo tipo de agente, com o mesmo mecanismo dos resumos (`claude -p`, `devin -p`, `codex exec`, `gemini -p`). O orquestrador é o terminal gerente; se não houver gerente, o primeiro agente que terminar um turno;
  - se os resumos estiverem desligados ou o agente falhar, o título vem das primeiras palavras do primeiro pedido;
  - o nome dado por você nunca é trocado.
- **Cartões de turno:** antes ficavam só na memória e sumiam ao reiniciar o Squad. Agora ficam em `cards.json`, na pasta da sessão, e voltam com ela.
- **Remover:**
  - a sessão aberta não pode ser removida;
  - uma sessão com andares também não: é preciso abri-la e aterrissar ou descartar os andares;
  - a pasta vai para a lixeira, e as conversas continuam guardadas nos agentes.
  - Remover o workspace leva a pasta dele inteira. Mudar a pasta do workspace e removê-lo agora olham os andares de todas as sessões.
- **Migração:**
  - um workspace de antes das sessões vira a "Sessão 1" na primeira abertura: o canvas, as notas, os anexos e os cartões vão para a pasta da sessão;
  - o `sessions.json` é gravado antes de o canvas mudar de lugar, então uma migração interrompida termina na próxima abertura.
- **Testado aqui:**
  - testes do `hub`:
    - canvas completo ida e volta (nós, fios, posições, conteúdo da nota, terminal rodando de novo);
    - nome do workspace seguindo as sessões;
    - renomear, remover para a lixeira, recarregar o `hub`;
    - nova sessão com os mesmos terminais (papel mantido, sem conversa, um fio);
    - recusa e `confirm` com agente trabalhando;
    - migração com nota;
    - nome pelo primeiro pedido;
  - `-race` nos pacotes mexidos e `make check`.
- **Claude real:**
  - com o binário da `main`, criei um workspace com um Claude e uma nota, e pedi para guardar 7319;
  - com o binário novo, o workspace virou a "Sessão 1" (nota e canvas dentro da pasta da sessão), e o Claude retomou a conversa;
  - abri uma sessão 2 sem nome (canvas vazio), pus um Claude e pedi os movimentos do cavalo: o `claude -p` deu o nome "Movimentos do cavalo em b1";
  - com a tela aberta, abri a sessão 1 pela API: a tela trocou sozinha, com o Claude, o "Eu" e a nota "Regras";
  - perguntei o número: 7319.
- **Não testado:**
  - o título escrito pelo `devin -p` (precisa de login) e o retomar do Devin na troca de sessão (o mesmo do S1);
  - Codex e Gemini;
  - Windows real: a migração e a lixeira usam `os.Rename`, mas com as sessões fechadas não há arquivo preso.

## 2026-10-17, S1: conversas que continuam

- **Pedido do usuário:**
  - fechar o `squad serve` (ou reiniciar um terminal) e voltar com cada agente na conversa em que estava;
  - depois, sessões por workspace, cada uma com o seu canvas.
  - O plano combinado (S1 a S4) está no PLAN. Este é o S1.
- **O que os agentes oferecem:** conferido com os binários reais: Claude 2.1, Devin 3000.11, Codex 0.163 e Gemini 0.64 (instalados só para ler a ajuda).

| Agente | Id da conversa | Retomar |
|---|---|---|
| Claude | o Squad escolhe (`--session-id`); os hooks avisam se mudar (`/clear`, `/resume`) | `--resume <id>` |
| Devin | o `session_id` dos hooks (o Devin não aceita escolher) | `--resume <id>` |
| Codex | o `thread-id` do aviso de fim de turno | `codex resume <id>` (subcomando, logo depois do programa) |
| Gemini | o Squad escolhe (`--session-id`), se a versão tiver; o Squad confere na ajuda uma vez | `--resume <id>` |

- **O agente não é o processo do terminal:** o Squad digita a linha do agente num shell. Por isso, uma retomada que falha não aparece como terminal encerrado: o shell continua vivo.
  - **Solução:** a própria linha cuida da falha, no formato `retomar || começar nova`. No PowerShell 5.1, que não tem `||`, vira `; if ($LASTEXITCODE -ne 0) { … }`.
  - **Mesmo id:** no Claude e no Gemini, a conversa nova começa com o mesmo id, então o id guardado continua valendo.
  - **Conferido no Claude real:** ele sai com 1 e "No conversation found" quando não acha a conversa, e com 0 no `/exit` e no Ctrl-C duplo. O `||` só age na falha.
  - **O aviso:** um hook `SessionStart` novo no Claude (o Devin já tinha) informa o `source`. Pedido de retomada que volta com `startup` vira o aviso "Não deu para retomar a conversa de X: começou uma nova".
- **Interrompidos:**
  - os hooks de início e fim de turno marcam o terminal como ocupado (`busy`, gravado no `workspace.json`). A marca vem dos hooks, e não da tela: a tela do agente abrindo apagaria a marca;
  - um terminal que reabre retomando a conversa e ainda ocupado volta como **interrompido**;
  - a tela mostra "Claude foi interrompido no meio do trabalho", com **Continuar** (manda "O seu trabalho foi interrompido (o terminal reiniciou). Continue de onde parou.") e **Dispensar**;
  - a marca some quando o agente trabalha de novo;
  - funciona no Claude e no Devin, que têm hook de início de turno. O Codex e o Gemini não têm, então ficam de fora.
- **Proteções:**
  - o id e a marca de ocupado são do servidor: o `PATCH` de nó, que substitui todos os dados do terminal, não os apaga nem os troca;
  - trocar a pasta ou o agente começa uma conversa nova, porque os agentes guardam as conversas por pasta;
  - um nó criado pela API começa sem conversa;
  - o id que chega pelos hooks é validado (letras, números, `.`, `_`, `-`, sem começar com `-`) e vai entre aspas na linha.
- **Papel:**
  - **Devin:** não recebe o papel de novo ao retomar (`source: resume`), porque a conversa já tem o papel;
  - **Gemini:** o papel vai como primeira mensagem, então só entra numa conversa nova;
  - **Trocar o papel:** agora começa uma conversa nova, para o papel valer desde o início. Antes também reiniciava do zero.
- **Interface:**
  - no menu do terminal, **Nova conversa** (com confirmação) e **Copiar comando de retomada** (`claude --resume <id>`, para abrir a conversa fora do Squad, na pasta do terminal);
  - a faixa de interrompidos e o aviso de retomada que falhou.
- **Skill:** uma regra para quando o Squad reinicia no meio de uma espera: usar `squad wait` ou `squad check` antes de mandar o pedido de novo.
- **Bug achado nos testes:** o estado do terminal ia com `interrupted` em `omitempty`. Quando virava falso, o campo sumia, e a tela, que junta o estado novo ao antigo, continuava mostrando a faixa. Corrigido.
- **Efeito colateral bom:** neste container, os agentes lançados pelo Squad herdavam variáveis da minha sessão e gravavam a conversa num arquivo com o id dela. Com `--session-id`, cada um usa o seu.
- **Testado aqui:**
  - agentes falsos: a linha de cada agente (nova, retomada, `||`, id inválido, linha do usuário com `--continue`, Codex via `npx`, Gemini antigo), os hooks com o id, o Claude retomando ao reiniciar, "Nova conversa", o `PATCH` sem apagar o id, a pasta nova, a falha com o mesmo id e o aviso, o Devin com o id vindo dos hooks, o interrompido com Continuar e Dispensar;
  - `make check`.
- **Claude real, num `serve` de teste:**
  - pedi para guardar 4271;
  - dei uma lista de 80 cidades e fechei o `serve` no item 40;
  - ao reabrir, o Claude voltou com `--resume <id>`, mostrando a conversa, e a faixa apareceu;
  - cliquei em Continuar: ele recomeçou a lista (a resposta pela metade não fica salva) e terminou. A faixa sumiu;
  - perguntei o número: 4271;
  - com um id inexistente gravado, ele começou uma conversa nova com o mesmo id, e a tela avisou.
- **Devin real:** sem login, ele para na tela de entrar na conta, então não deu para testar a retomada.
- **Precisa de você (Devin com login):**
  1. abrir um terminal Devin, conversar, reiniciar o `serve` e conferir que ele volta na conversa (`devin --resume <id>`);
  2. fechar o `serve` no meio de um turno e usar o Continuar;
  3. não sei se o Devin sai com erro quando não acha a conversa. Se ele ficar aberto mostrando um erro, o `||` não entra, e você vê o erro dentro do Devin; nesse caso, me avise.

## 2026-10-16, Planejador espera o resultado (`squad wait`)

- **Problema (visto no teste do Arquiteto):** o Planejador respondeu "ainda esperando o Arq" a quem pediu e terminou o trabalho depois, sozinho; a resposta final ficou só na tela dele.
- **Causa:** o `squad ask` espera enquanto o agente trabalha, mas volta depois de 2 min sem atividade, com a tela e `[squad: timeout]`. O Arq estava parado num pedido de permissão. Depois disso o pedido deixava de existir: a resposta do Arq, quando viesse, não tinha para onde ir. O Planejador só podia ler a tela (`check`) ou mandar de novo (pedido duplicado). Por isso o ajuste só no texto não bastava.
- **`squad wait "Nome"`** (e a ferramenta MCP `wait`, liberada no Claude e no Devin porque não manda nada):
  - espera o turno atual do agente acabar e devolve a resposta final dele, sem mandar mensagem;
  - se ele não está num turno, devolve na hora a última resposta, com há quanto tempo e o começo do pedido que a gerou, ou a tela;
  - tem o mesmo limite do `ask`: segue enquanto há atividade e volta depois de 2 min sem nenhuma, com a tela e o aviso de que ele pode estar esperando o usuário;
  - quem espera aparece como "esperando" no canvas.
- **Turno aberto:** antes, 60 s de silêncio desligavam o "trabalhando" de um agente com hooks, e não havia como saber que o turno continuava. Agora o monitor lembra que um turno começou e só um hook o termina (`InTurn`). No Codex, que não tem hook de início, o turno conta a partir da mensagem entregue.
- **O timeout do `ask` avisa:** se o turno do agente não acabou, a mensagem diz para não reenviar e usar `squad wait "Nome"`.
- **Mensagens durante o `wait`:** se o agente esperado pergunta algo a quem espera, a pergunta chega como resultado do `wait` (`[de: … · pedido …]`), em vez de ir para a fila de um terminal ocupado. Os dois ficariam se esperando até o limite de 30 min. Quem esperava responde com `squad reply` e espera de novo. Os ciclos com `wait` no meio também são liberados, como já eram com `ask`. Esperar por quem espera por você é recusado.
- **Textos:**
  - o Planejador espera os resultados antes de responder, não termina o turno com andamento e só para antes se algo depender do usuário, dizendo o quê;
  - a skill ganhou o comando e a mesma regra para qualquer agente que recebeu um pedido;
  - o MCP (agora 17 ferramentas) e a ajuda da CLI citam o `wait`.
- **Limite do cliente MCP:** o Claude Code corta uma ferramenta MCP depois de `MCP_TOOL_TIMEOUT` (aqui no container, 60 s). O `ask` cortado vira "timed out", e o pedido é cancelado. Agora o `squad mcp` lê essa variável e faz o `ask` e o `wait` voltarem um pouco antes do limite, com a tela do agente esperado e a orientação: se ela mostra uma pergunta ou um pedido de permissão, avise o usuário; se não, chame `wait` de novo. Sem isso, com um limite menor que os 2 min do servidor, o aviso de inatividade nunca chegaria, e o agente esperaria em loop por alguém parado numa permissão. Não sei o padrão dessa variável no Claude Code da sua máquina, nem se o Devin tem um limite parecido; se não houver limite, nada muda.
- **Bug encontrado:** o `squad roles` (do PR anterior) não estava na lista de comandos do binário. Pela CLI dava "unknown command"; só a ferramenta MCP funcionava. Corrigido, com um teste que confere que todo comando da ajuda dos terminais chega ao servidor.
- **Todos os agentes:**
  - Claude e Devin têm hook de início e de fim: o `wait` sabe que o turno continua mesmo parado numa permissão;
  - Codex só tem o de fim: o turno conta a partir da mensagem entregue pelo Squad;
  - Gemini não tem hook: o `wait` segue a tela, como o `ask`;
  - a ferramenta MCP vale para Claude, Devin e Codex, e a CLI para todos.
- **Testado aqui:**
  - agentes falsos com hooks: `ask` que volta por tempo com o agente ainda no turno (e o aviso), `wait` até o hook de fim, a última resposta depois, o estado "esperando" no canvas, a pergunta do agente esperado chegando pelo `wait` e a resposta voltando por `squad reply`, e a recusa do ciclo;
  - pelo cliente MCP de verdade: a ferramenta `wait` e o corte antes do `MCP_TOOL_TIMEOUT`, com a tela e o `wait` depois;
  - `make check`, com `-race` nos testes da conversa, e o Devin real lendo a configuração com a ferramenta nova liberada (`TestRealDevinConfig`).
- **Claude real, num worktree do Squad, com a skill nova:**
  - **o mesmo pedido do teste do Arquiteto.** O `ask` do Planejador ao Arq foi cortado pelo limite de 60 s do Claude deste container. Ele chamou `wait`, foi cortado de novo e chamou `wait` outra vez, que trouxe a resposta final do Arq. Depois criou o Impl, ligou a nota e respondeu uma vez só, com o resultado;
  - **um pedido que para numa permissão:** "crie um Arquiteto e peça a ele para rodar `go vet` e `go version`". Desta vez o Planejador usou a CLI pelo Bash. O Arq parou no pedido de permissão do `go vet` e eu segurei por 4 min. O Planejador ficou esperando no `squad ask`, sem responder nada; depois que aprovei, ele respondeu uma vez, com a saída dos dois comandos.
- **Observado:** num pedido de permissão, o Claude Code pisca o marcador "●" da ferramenta. O Squad conta isso como atividade, então o `ask` não volta em 2 min: espera até a aprovação, até 30 min. Para o Planejador, é o comportamento certo, porque quem aprova é você, no canvas. Mas o canvas mostra o Arq como "trabalhando", não "esperando você". Para mostrar isso, dá para usar o hook `Notification` do Claude, que avisa quando ele pede permissão. Fica como ideia; ainda não sei se o Devin tem algo parecido.
- **Não testado:** o corte novo pelo `MCP_TOOL_TIMEOUT` com o Claude real (no segundo teste, o Planejador usou a CLI; o corte foi testado com o cliente MCP de verdade e um agente falso); uma sessão real do Devin (sem login aqui); o Codex e o Gemini reais.

## 2026-10-16, papel Arquiteto

- **Pedido do usuário:** um papel de arquiteto (e um de product manager, que ele decidiu deixar para depois). O Arquiteto não depende do PM: o PM, quando vier, entra só com o papel novo e o texto do Planejador.
- **O papel (`arch`, cor índigo, ícone `architecture`):** desenha a solução numa nota "Arquitetura: …" (problema, módulos e arquivos, contratos, alternativas, riscos, partes paralelas) e não escreve o código do produto. Não é gerente nem obrigatório.
- **Ajustes nos papéis prontos:**
  - o Planejador chama o Arquiteto quando a mudança mexe na estrutura ou vai ser dividida, liga a nota à equipe e usa as partes para decidir os andares;
  - o Implementador segue a nota;
  - o Revisor confere se a mudança segue o desenho;
  - o Testador não mudou. Quem editou esses papéis continua com a própria versão; "Restaurar padrão" traz os ajustes.
- **Conflito ao carregar a biblioteca:**
  - um papel do usuário com o mesmo nome de um papel pronto novo ganha o sufixo " (personalizado)";
  - um papel do usuário com o mesmo id ganha outro id. Só acontece se alguém tivesse criado um papel chamado "arch".
- **Os agentes enxergam os papéis:** `squad roles` e a ferramenta MCP `roles` (liberada no Claude e no Devin, porque só leem) listam id, nome, se é gerente e a primeira linha das instruções, inclusive dos papéis do usuário. O `squad new --help` mostra a mesma lista. A skill ganhou a seção "Roles", e os textos do MCP e da CLI citam o `arch`.
- **Limite da equipe:** continua 6 por gerente. Arquiteto, Revisor e duas frentes de Implementador + Testador cabem.
- **Testado aqui:**
  - teste unitário dos cinco papéis, dos ajustes e do conflito de nome e de id;
  - `squad roles` para um agente que não é gerente, e o `squad new --help`;
  - a ferramenta MCP `roles` pelo cliente MCP de verdade, agora com 16 ferramentas;
  - o Devin real lê a configuração com a ferramenta nova liberada (`TestRealDevinConfig`).
- **Claude real, num worktree do próprio Squad:** pedi ao Planejador "GET /api/version no servidor e a versão no rodapé da web; monte a equipe e peça o desenho a um Arquiteto, crie o Implementador e ligue a nota, sem implementar".
  - Ele criou o "Arq" com o papel Arquiteto.
  - O Arq parou num pedido de permissão do Claude Code para um `ls`/`grep`. O Planejador não respondeu por ele: me perguntou, como a skill manda, e eu aprovei.
  - O Arq leu o código e criou a nota "Arquitetura: versão no servidor e rodapé" (7.300 caracteres: o contrato, de onde vem a versão, a autenticação, os arquivos e as partes; ele recomendou uma branch só, por ser pequena).
  - O Planejador criou o "Impl" e ligou a nota a ele e a si.
- **Observado:** o Planejador respondeu um "ainda esperando" antes de o desenho ficar pronto e terminou o trabalho depois, sozinho. O resultado foi o pedido, mas a resposta intermediária confunde. Fica para ajustar no texto do Planejador se aparecer de novo.
- **Não testado:** uma sessão real do Devin (sem login aqui), e o Codex e o Gemini reais.

## 2026-10-16, nomes de nota

- **Pedido do usuário:** dar nome às notas, em vez de "Nota" e `nota-2.md`. Ele renomeia as próprias notas depois de criar; os agentes nomeiam na criação e não renomeiam.
- **O risco de renomear:** um agente que já conhecia o nome antigo receberia "nota não encontrada" na próxima leitura. **Solução:** a nota guarda os nomes antigos (`aliases`), e o arquivo não muda depois de criado. O agente acha a nota pelo nome antigo, e a resposta avisa o nome atual ("this note is now named …"). O usuário escolheu isso em vez de travar o nome.
- **Rascunho:** a nota criada na interface nasce sem arquivo, com um nome provisório único. O primeiro nome confirmado dá nome ao arquivo. O rascunho termina no que vier primeiro: nome confirmado, conteúdo escrito, fio ligado (o serviço de notas escuta `edge.added`, e a busca dos agentes também termina rascunhos) ou o servidor reiniciando. Assim o arquivo nunca é renomeado no disco, o que no Windows falharia com o arquivo aberto num editor.
- **Nomes:** únicos no workspace, sem diferenciar maiúsculas, contando os nomes antigos e os nomes de arquivo; até 60 caracteres, uma linha, sem começar com `-`. A busca dos agentes ignora maiúsculas e prefere o nome atual, depois um antigo, depois o arquivo. Antes, a busca diferenciava maiúsculas, e a checagem de repetição só olhava as notas ligadas ao agente.
- **Notas de agente:** guardam o autor (`createdBy`), como os portais. Na interface, aparecem com "por Bia" e sem "Renomear", e a API recusa renomeá-las. Agora nascem num espaço livre perto do agente (antes, sempre à esquerda dele, por cima do que estivesse ali).
- **Proteção:** o `PATCH /api/nodes` trocava título e arquivo de uma nota sem checagem nenhuma. Agora ele muda só o cadeado; nomear passa pela rota nova, `PATCH /api/notes/{id}`.
- **Todos os agentes:** a CLI (`squad note`), a ferramenta `note` do MCP, o `squad list` (que mostra `formerly: …`) e o `squad connect` por nome antigo são os mesmos para Claude, Devin, Codex e Gemini. A skill ganhou uma seção "Notes".
- **Testado aqui:**
  - teste de integração do rascunho, dos quatro gatilhos, dos nomes únicos e dos nomes antigos, da recusa em notas de agente e do `PATCH` protegido; o MCP recusa nome repetido e acha a nota em qualquer caixa;
  - interface, pelo Chromium: nota nova com o campo selecionado, nome repetido com aviso, Esc com o nome provisório, renomear com duplo clique;
  - Claude real: leu a nota pelo nome; renomeei pela interface; pedi pelo nome antigo e ele achou, escreveu e disse o nome novo. Criou "Bugs encontrados" (com "por Ana", num espaço livre) e teve recusada uma nota "roteiro de testes", nome já usado.
- **Não testado:** uma sessão real do Devin (sem login aqui). Ele usa a mesma CLI e o mesmo MCP.

## 2026-10-16, instalação da versão certa

- **O que aconteceu:** no Windows, o `squad install-skill` instalou só a skill do Claude, e com `-agent devin` não registrou o MCP. A causa não era o código: o `go install …@main` passa pelo proxy do Go, que ainda respondia com o commit anterior (o merge do PR #15) quase uma hora depois do merge do PR #16. O usuário instalou a versão antiga.
- **Correção na instrução:** `GONOPROXY=github.com/nathanribeiroo/squad go install github.com/nathanribeiroo/squad/cmd/squad@main` busca o Squad direto do GitHub e o resto pelo proxy. Testei aqui: com o proxy em cache, essa forma instalou o commit novo. Um commit exato (`@b0350d2`) também funciona. O README trocou o `@latest`, que pega a tag antiga v0.1.0.
- **`install-skill` mais claro:** a primeira linha mostra a versão do `squad`, e ele avisa qual agente não encontrou e como instalar nele mesmo assim (`-agent devin`).

## 2026-10-16, paridade do Devin

- **Pedido do usuário:** tudo o que o Squad faz tem que valer para o Devin tanto quanto para o Claude (e para os outros agentes quando der). Ele usa os dois.
- **Como conferi:** instalei o Devin CLI real (v3000.11.92) numa pasta isolada. Sem login não dá para abrir uma sessão, mas dá para ler a ajuda e as configurações (`devin mcp list`, `devin skills paths`).
- **Três falhas encontradas:**
  - **MCP:** desde a v3000.3, o Devin só lê servidores MCP do `mcp_config.json`. O `mcpServers` que o Squad punha no `--config` era ignorado (o `devin mcp list` não mostrava o `squad`). Os terminais Devin estavam sem as ferramentas MCP e pediam aprovação a cada comando `squad`.
  - **Configuração do usuário:** pela ajuda do Devin, o `--config` **substitui** o `~/.config/devin/config.json`. Dentro do Squad, o modelo, as permissões salvas e os hooks pessoais do usuário não valiam.
  - **Skill:** o `squad install-skill` sem `-agent` instalava só no Claude, e as instruções pós-merge só citavam esse comando. A skill do Devin ficava para trás.
- **Correções:**
  - **Cópia da configuração:** a cada terminal Devin aberto, o Squad lê o `config.json` do usuário (JSON com comentários e vírgulas sobrando, como o Devin aceita) e grava uma cópia em `~/.squad/run/devin-config.json` com os hooks do Squad depois dos do usuário e as ferramentas MCP seguras liberadas (as mesmas do Claude). As permissões que o Devin salvar nessa cópia durante uma sessão do Squad são mantidas. Se o arquivo do usuário não abrir, vai só a parte do Squad, como antes. Sem `mcpServers` na cópia.
  - **MCP registrado (decisão do usuário):** o `squad install-skill` registra o servidor `squad` no `mcp_config.json` do Devin, mantendo os outros servidores e guardando o arquivo anterior como `.squad-backup`. Fora de um terminal do Squad, o `squad mcp` responde sem ferramentas, com um aviso, em vez de falhar.
  - **Skill para todos:** sem `-agent`, o `install-skill` instala para cada agente encontrado (a CLI no PATH ou a pasta de configuração); `-agent all` instala nos dois.
  - **Andares (decisão do usuário):** o Devin abre num andar com `--respect-workspace-trust false`, sem a pergunta de confiança na pasta. O Claude não pergunta num worktree de um repositório confiado.
  - **Skill:** o aviso sobre telas de permissão cita o Devin e o Codex, não só o Claude.
- **Testado aqui:**
  - unitários: a cópia mantém o modelo, as permissões e os hooks do usuário; não duplica os hooks do Squad; mantém o que o Devin salvou; o JSON com comentários; o registro do MCP mantém os outros servidores e não sobrescreve um arquivo quebrado;
  - com o Devin real: ele lê a cópia (grava nela a versão do esquema) e o `devin mcp list` mostra o `squad` ao lado dos servidores do usuário (`go test -tags e2e -run TestRealDevinConfig ./internal/agents/`);
  - pelo `squad serve`: um terminal Devin no térreo abre com a cópia, e num andar abre dentro da cópia do repositório, com `--respect-workspace-trust false`;
  - o `squad mcp` fora do Squad responde ao `initialize` e ao `tools/list` sem ferramentas.
- **Precisa de você (Devin com login):**
  - rodar `squad install-skill` e reiniciar o `squad serve`;
  - num terminal Devin, pedir `use a ferramenta squad list`: ela deve rodar sem pedir aprovação. Isso confirma que o Devin passa as variáveis do terminal (`SQUAD_*`) ao servidor MCP; o binário não mostra sinal de filtrá-las, mas não deu para ver numa sessão;
  - o cartão de fim de turno, o papel e os estados do Devin (os hooks dentro de uma sessão com conta);
  - se o modelo e as permissões que você configurou no Devin continuam valendo dentro do Squad;
  - um andar com um Devin: ele não deve perguntar se confia na pasta.
- **Codex e Gemini (fora deste PR):** o Codex recebe MCP, papel e fim de turno, mas não tem skill instalável pelo Squad nem a pergunta de confiança tratada nos andares. O Gemini recebe só o papel.

## 2026-10-16, janela de app

- **Pedido do usuário:** abrir o Squad numa janela, como um app, sem mostrar a URL. No Mac, ele usa o Safari.
- **Escolha: o modo app do navegador** (`--app=<url>`) do Chrome, do Edge ou do Brave, achado por `browser.Find` (o mesmo dos portais). Uma janela nativa (WebView2 dentro do `squad.exe`) foi descartada: no Linux e no macOS exigiria compilar com C, e o `go install` deixaria de ser simples.
- **Perfil próprio** em `~/.squad/window` (separado do perfil dos portais, `~/.squad/browser`): a janela não se mistura com o navegador do usuário e não carrega extensões. O tamanho 1440×900 vai só na primeira vez; testei que o `--window-size` passaria por cima do tamanho lembrado.
- **Processo:** no Linux e no macOS, o navegador vai para outro grupo de processos, para o Ctrl-C do `serve` não fechar a janela (no Windows, um programa gráfico já não recebe o Ctrl-C). Fechar a janela não para o servidor.
- **Login que sobrevive ao reinício:** antes, o token mudava a cada `squad serve`. Agora ele fica em `~/.squad/session-token` (permissão 0600), e o cookie dura um ano. Sem isso, o app do Dock do Safari, que não tem barra de endereço, ficaria sem login a cada reinício. Decisão reversível: o arquivo de descoberta (`run/server.json`) já guardava o token, e o servidor continua só em `127.0.0.1`, com checagem de Host e Origin. Apagar o arquivo desconecta tudo.
- **Página de login:** sem sessão, a página pede a URL que o `serve` mostrou, num campo, já que a janela não tem barra de endereço. Um token antigo mostra a mesma página, com o aviso.
- **Safari:** não tem modo app por comando. O caminho é o "Adicionar ao Dock" (macOS 14+), feito uma vez. Depois, o `squad serve` abre `~/Applications/Squad.app` com `open -a`, sem URL (não sei se um app do Dock aceita uma URL, então ele abre no último workspace). Com Chrome ou Edge no Mac, vale a janela de app.
- **Web:** ícone (a grade do logo) em SVG e PNG, `apple-touch-icon`, `manifest.webmanifest`; título "workspace · Squad"; `theme-color` acompanha o tema, para a barra de título combinar com o topo; `translate="no"`, para o navegador não oferecer tradução. Os ícones e o manifest são servidos sem sessão, porque o navegador os busca sem cookie.
- **Flags:** `-open app` (padrão), `-open browser` (a aba de antes) e `-open none` (o `-no-open` continua valendo).
- **Testado aqui:** Chromium de verdade num display virtual (Xvfb): a janela abre sem barra de endereço, com o título do workspace; sobrevive ao servidor parado e reconecta quando ele volta; um segundo `squad serve -dir` abre outra janela; a página de login aceita a URL colada.
- **Precisa de você:** conferir no Windows (Edge) e, no Mac, o app do Dock do Safari. Aqui não há Windows nem Mac. Também não consegui confirmar que o tamanho da janela é lembrado: sem gerenciador de janelas, o Chromium gravou o tamanho mas não o restaurou.

## 2026-10-16, R7, R8 e R9 cortados

- **Decisão do usuário:** o portal Android (R7), os terminais remotos (R8) e o Playwright (R9) não são necessários e saem da rodada. A segunda rodada termina no R6. O que vem depois fica para combinar; os candidatos estão em "Fora da rodada", no PLAN.

## 2026-10-16, resumos de turno (R6)

- **Decisão do usuário:** cartões automáticos com a mensagem final do agente, de graça (opção A), e resumo sob demanda escrito por uma CLI de agente (opção C), sem nota. O texto de um terminal fica com o agente dele; numa máquina só com Devin, o Devin resume.
- **Fim de turno para todos:**
  - o monitor de atividade passou a avisar todo fim de turno (`OnAnyTurnEnd`), com a hora de início e o pedido que iniciou o turno;
  - o pedido vem do hook `UserPromptSubmit` do Claude e do Devin (campo `prompt`). Para os outros agentes, vem da última mensagem enviada ao terminal (`ask` ou Composer).
- **Cartões** (`internal/summary`):
  - só os agentes (Claude, Codex, Devin, Gemini) ganham cartão;
  - nos agentes com hook, o cartão espera o hook (com a mensagem final) e ignora a tela parada. No Gemini, sem hook, usa as últimas 40 linhas da tela;
  - os cartões ficam em memória, um por terminal, e chegam pelo evento `turn.card`.
- **Resumir e resumo do workspace:**
  - **Claude:** `claude -p --model haiku --tools "" --no-session-persistence --setting-sources "" --strict-mcp-config --output-format json --system-prompt-file …`, com o material pela entrada padrão;
    - testei: 6,7 s, nada no histórico de sessões, nenhum hook do usuário. O preço de lista seria de US$ 0,0008 por resumo; numa assinatura, desconta do limite do plano;
    - o `--bare` ficou de fora, porque ignora o login e exige chave de API. O `--system-prompt-file` funciona, embora não apareça na ajuda;
  - **Devin:** `devin -p --prompt-file … --respect-workspace-trust false`, pela documentação dele;
  - **Codex:** `codex exec --sandbox read-only --output-last-message …`;
  - **Gemini:** `gemini -p`, com o material pela entrada padrão;
  - **Isolamento:** sempre numa pasta temporária vazia, com o texto em arquivo ou na entrada padrão, nunca como argumento (nada de aspas no Windows). Limite de 90 s, e o ambiente sai sem as marcas de uma sessão de agente pai;
  - **Material como dado:** o material vai entre `<material>` e `</material>`, e as instruções dizem que é só dado. Foi a própria Ana (Claude) quem apontou o risco, ao ler o arquivo das instruções;
  - **Resposta:** um JSON (`resumo` e `passos`, ou `visao`, `terminais` e `atencao`), lido mesmo dentro de um bloco de código.
- **Ajustes por workspace:** cartões ligados e quem resume (automático, claude, devin, codex, gemini ou desligado), com as CLIs encontradas no PATH (`/api/summary/settings`).
- **Interface:**
  - **o cartão:** fica embaixo do terminal, para não cobrir a tela do agente (as etiquetas de andar descem para dar lugar), com a duração e o começo da mensagem;
  - **o painel:** abre para cima, sobre o terminal (para baixo saía da tela). Mostra o resumo, os passos, o pedido e a mensagem final em Markdown;
  - **"Resumir":** mostra "resumindo…" enquanto roda;
  - **os passos:** abrem o Composer com o texto pronto para aquele terminal;
  - **a Atividade:** mostra o turno, inclusive os que terminaram antes de a página abrir, e ganhou o botão "Resumo", que abre a visão geral do workspace (clicar num nome leva ao terminal, no andar dele);
  - **⚙ → Resumos…** abre os ajustes;
  - **o diálogo genérico** saiu de `FloorDialogs` para `Dialog`;
  - **a Atividade** mostrava "· main" fixo e agora mostra o branch do andar aberto.
- **Decisões tomadas sem consulta** (reversíveis):
  - os cartões começam ligados (o PLAN dizia "desligado", mas isso valia para um modelo pago) e ficam só em memória;
  - o ajuste é por workspace;
  - "Automático" no resumo do workspace usa o agente mais comum entre os terminais;
  - a instrução pede de 1 a 3 próximos passos e só deixa a lista vazia quando não há mesmo nada útil a pedir. Na primeira versão, o modelo devolveu passos vazios para uma pergunta só informativa.
- **Testes:**
  - `internal/summary`, com CLIs falsas no PATH:
    - o cartão vem do hook, e não da tela, para quem tem hook;
    - shells não ganham cartão;
    - o "Automático" escolhe a CLI do agente do terminal; com CLI fixa, inclusive não instalada, ou desligado;
    - os argumentos do `claude` e do `devin`;
    - o resumo do workspace e a leitura do JSON;
  - `internal/app`, com o agente falso no papel de Claude (hook `Stop` e `UserPromptSubmit` com o pedido): o cartão pelo caminho real, o "Resumir" pela API com um `claude` falso e o material que ele leu, e os ajustes.
- **Com o Claude real, pelo Chromium:**
  - a Ana (Implementador) leu arquivos do projeto e o cartão apareceu, com a duração e a mensagem;
  - o "Resumir" chamou o `claude -p` de verdade e trouxe o resumo e 3 passos;
  - o passo clicado abriu o Composer para a Ana com o texto;
  - o resumo do workspace descreveu a Ana e o shell "Eu";
  - os ajustes mostraram o Claude instalado e as outras CLIs não encontradas.
- **Precisa de você:** o `devin -p` na sua máquina (aqui não há Devin, Codex nem Gemini) e os cartões do Devin, que dependem do hook `Stop` dele.

## 2026-10-16, Planejador usa andares sozinho

- **Por quê:** até aqui, os andares só eram usados quando alguém pedia. O Planejador tinha as ferramentas, mas as instruções dele não diziam quando usá-las. O padrão das ferramentas de agentes em paralelo é isolar cada tarefa paralela automaticamente e deixar a mescla para uma pessoa revisar.
- **A regra** (nas instruções padrão do Planejador; o usuário escolheu só a regra, sem ajuste por workspace):
  - com duas ou mais frentes de implementação em paralelo no mesmo repositório, cada frente ganha um andar. O Implementador e, se houver, o Revisor e o Testador daquela frente ficam no andar dela, para verem o código dela. O Planejador fica no térreo;
  - com uma frente só, nada de andar: o preparo custa (`npm install`, disco, portas), e quem revisa precisa ver o mesmo código;
  - quem está no andar faz commit do que terminou. Depois da revisão e dos testes, o Planejador aterrissa uma frente por vez com `--merge`, e o usuário aprova cada uma. Num conflito, um agente daquele andar mescla o branch de origem, resolve e faz commit, e o Planejador aterrissa de novo.
- **Quem já editou o Planejador** continua com a própria versão (os papéis editados ficam em `roles.json`). "Restaurar padrão" traz a regra.
- **Com o Claude real**, sem falar em andares no pedido:
  - "duas mudanças independentes em paralelo, por dois implementadores; só monte a equipe": ele criou os andares Frutas e Cores e um Implementador em cada (`squad new … --floor`), ficou no térreo e descreveu a aterrissagem com merge, uma de cada vez, com aprovação;
  - "uma tarefa só": ele criou o Implementador no térreo e explicou que não criou andar porque não havia frente paralela.

## 2026-10-16, andares: interface (R5)

- **Seletor no topo** (o item 5 do design, que era só um marcador):
  - a aba do térreo mostra o branch atual; cada andar ganha uma aba com "limpo", "● N alterações", "N commits" (ainda não aterrissados), "preparando" ou "preparo falhou";
  - um ponto azul marca agentes trabalhando naquele andar;
  - o `+` cria um andar;
  - clicar no andar aberto abre o menu: aterrissar, log do preparo, copiar o caminho, descartar;
  - com muitos andares, as abas rolam, e o `+` fica sempre visível. Quando falta espaço, o caminho no topo encolhe primeiro e o texto "pedidos por min" some abaixo de 1500 px.
- **Estado:**
  - o andar aberto fica por workspace, no navegador e na URL (`&floor=`);
  - cada andar guarda o próprio zoom e posição;
  - um andar que some (aterrissado pelo gerente) leva a tela de volta ao térreo, com aviso.
- **Canvas por andar:**
  - o canvas mostra só os nós do andar aberto;
  - terminal, nota e portal criados na tela vão para o andar aberto. As sugestões de pasta do formulário vêm da cópia (`/api/dirs?floor=`);
  - o rodapé do terminal mostra o branch e a pasta reais; antes, "⎇ main" era fixo;
  - um andar vazio mostra um cartão: branch, pasta, estado do preparo (com o log), o comando para o gerente e o botão "Novo terminal neste andar".
- **Fios entre andares** (o design não cobre; escolha reversível): viram etiquetas embaixo do nó, `⇄ Plano · térreo`. A etiqueta pulsa enquanto um pedido passa pelo fio, e um clique leva ao outro andar, centrado no nó. Com o zoom afastado, ela mantém um tamanho legível.
- **Diálogos:**
  - **criar:** nome; o branch vem do nome e pode ser editado, e um branch que já existe abre como está. Mostra de onde o andar parte. "Preparação" (recolhida) edita a lista de cópia e o comando de preparo do workspace;
  - **aterrissar:**
    - mostra os commits do andar e quantos commits novos a origem tem;
    - lista as alterações não commitadas, que desabilitam o botão;
    - oferece "Mesclar em [branch]" (a origem por padrão; desmarcado quando não há commits) ou "Só guardar o branch";
    - diz o que fecha e o que desce para o térreo;
    - num conflito ou com o destino sujo, mostra os arquivos no próprio diálogo, com o que fazer;
  - **descartar:** lista o que se perde (pasta com alterações, branch com commits não mesclados). Se houver trabalho a perder, pede para marcar "Entendo que esse trabalho se perde";
  - **log do preparo:** a saída do comando, atualizada a cada 2 s enquanto ele roda.
- **Servidor:**
  - `GET /api/floors/status` traz o branch do térreo e, para o térreo e cada andar, as alterações e os commits à frente. A interface consulta ao abrir, a cada 10 s com a aba visível, quando os andares mudam e logo depois de um agente terminar um turno;
  - `GET /api/branches` lista os branches.
- **Skill:** quando um agente criado pelo gerente para numa pergunta de permissão, o gerente avisa o usuário em vez de responder com `keys`. No teste real, o Planejador tentou aprovar a pergunta da Bia com `keys`. A aprovação do `keys` segurou, e eu recusei como usuário.
- **Ícones novos:** `call_merge`, `commit`, `content_copy`, `flight_land`, `warning`.
- **Testes:**
  - Go: `Status` e `Branches` com git real (alterações sem contar os copiados, commits à frente, pasta sumida, pasta sem repositório). Os endpoints também passaram pelo teste de integração;
  - `make check`, `tsc` e o build.
- **Na interface, com o Claude real** (num worktree de teste do próprio Squad, pelo Chromium):
  - o Planejador criou o andar Login e a Bia nele;
  - abri o andar pela URL e vi a pergunta de permissão da Bia na tela. Aprovei clicando no terminal e apertando Enter, duas vezes (arquivo e commit);
  - a aba passou de "1 alteração" a "1 commit";
  - no térreo, o Plano mostrou a etiqueta `⇄ Bia · Login`, e o Login mostrou `⇄ Plano · térreo`;
  - pelo `+`, criei o andar Cadastro com um comando de preparo lento: a aba e o cartão mostraram "preparando", e o log mostrou a saída;
  - aterrissei o Login pelo diálogo:
    - com um rascunho não commitado, o botão ficou desabilitado e o arquivo listado;
    - sem o rascunho, "Mesclar e aterrissar" fez o commit de merge com dois pais, fechou a Bia e voltou ao térreo;
  - o Cadastro, com o mesmo trecho do README mudado nos dois lados, mostrou o conflito no `README.md`, e o térreo não mudou;
  - "Novo terminal neste andar" abriu um Claude na cópia (o processo tinha a pasta da cópia como cwd), sem pergunta de confiança;
  - o descarte pela interface pediu a confirmação e apagou a pasta e o branch.
- **Precisa de você (Windows):** o seletor e os diálogos com um andar de verdade, e o preparo pelo Git Bash.

## 2026-10-16, andares: servidor (R4)

- **O que é:** um andar é uma cópia isolada do repositório do workspace (um `git worktree` num branch próprio), com um canvas próprio. Serve para frentes de trabalho em paralelo que mexem nos mesmos arquivos.
- **Onde fica a cópia** (decisão do usuário): fora do projeto, como no Maestri, em `<pasta-pai do repositório>/.squad/floors/<projeto>--<andar>`. Se o repositório estiver direto na pasta do usuário, a cópia cai em `~/.squad/floors`, ao lado dos dados do Squad, sem conflito. As pastas `floors` e `.squad` são apagadas quando ficam vazias.
- **Modelo:** os andares ficam dentro do workspace (`floors` no `workspace.json`, mais o campo `floor` em cada nó), como os `floors[]` do Maestri.
  - A alternativa seria um workspace por andar, mas os fios não cruzam workspaces, e o gerente no térreo precisa falar com quem está no andar.
  - Assim o barramento não mudou: um fio entre andares funciona como qualquer outro, e os nomes continuam únicos no workspace.
- **Criar** (`internal/floors`):
  - o branch é o nome sem acentos (`Página de Login` → `pagina-de-login`), ou `--branch B`. Um branch que já existe só abre com `--existing`;
  - o andar parte do último commit do térreo. Sem cópia instantânea no Windows, ele recebe o checkout dos arquivos versionados e os não versionados que casam com a lista de cópia do workspace (padrão `.env*`), sem sobrescrever o que o checkout trouxe;
  - o comando de preparo (ex.: `npm install`) roda em segundo plano, no shell padrão (Git Bash no Windows). Ele roda num grupo de processos (Unix) ou Job Object (Windows), para que parar o comando pare também o que ele abriu. O limite é de 30 min, e a saída vai para um log;
  - `squad floor create` espera o preparo por até 45 s e diz se terminou, falhou ou continua.
- **Aterrissar**, nas regras do Maestri:
  - é recusado se o andar tiver alteração não commitada (arquivos copiados não contam), uma operação pela metade (merge, rebase) ou a pasta fora do branch do andar;
  - sem `--merge` não mescla: fecha os terminais, apaga a pasta e mantém o branch;
  - com `--merge` (ou `--into B`), o merge é `--no-ff`, com a mensagem "Merge branch 'x' into y". Se o destino está aberto numa pasta, essa pasta não pode ter alteração em arquivos versionados. Se não está aberto em nenhuma, o Squad usa `merge-tree --write-tree`, `commit-tree` e `update-ref`, sem tocar em pasta nenhuma (git 2.38+);
  - num conflito, o Squad faz `merge --abort` e devolve a lista de arquivos; nada muda.
- **Descartar** (só pelo usuário, `DELETE /api/floors/<id>`): fecha tudo, apaga a pasta e o branch, se foi o andar que o criou.
- **Decisões tomadas sem consulta** (reversíveis):
  - **Mescla de arquivo copiado:** é recusada se o branch do andar commitou um arquivo copiado do térreo. Isso apareceu no teste: um `git add .` no andar commitou o `.env`, o merge falhou, e um segredo iria para o branch base;
  - **Notas:** as notas do andar descem para o térreo ao aterrissar ou descartar, porque guardam conhecimento, não código. Terminais e portais fecham;
  - **Agentes:** gerentes criam andares e aterrissam só os que criaram. Ninguém aterrissa o andar em que está, e descartar fica com o usuário. Pelo MCP, `floors` (listar e criar) vem liberada e `floor_land` pede aprovação, porque mescla branches e fecha terminais;
  - **Workspace com andares:** não pode ser removido nem mudar de pasta (os worktrees pertencem ao repositório).
- **Servidor:**
  - os terminais de um andar abrem na cópia. Pastas relativas resolvem na cópia, e uma pasta do térreo dada por inteiro vira a mesma pasta na cópia. `/api/dirs?floor=<id>` sugere pastas da cópia;
  - `squad new --floor "Nome"` (padrão: o andar de quem cria; `--floor ground` é o térreo);
  - portais e notas criados por um agente ficam no andar dele;
  - API `/api/floors`: listar, criar, configuração, alterações (arquivos e commits, para o diálogo do R5), log do preparo, aterrissar e descartar. O evento `floors.updated` traz a lista;
  - `squad list` mostra o andar de cada um, e a nota do gerente sugere andares. A skill ganhou a seção "Floors", e o MCP passou de 13 para 15 ferramentas.
- **Web, só o mínimo:** o canvas mostra o térreo, sem os nós e fios dos andares, e já tem os tipos e o evento. O seletor, criar e alternar, as alterações pendentes e o diálogo de aterrissar ficam no R5. A barra do topo já tem o marcador estático "main · limpo" do design, que o R5 liga aos andares.
- **Achado:** o Claude Code não pergunta se confia na pasta quando ela é um worktree de um repositório já confiado. Testei: a cópia em `.squad/floors` abriu direto, e uma pasta comum perguntou. Então os agentes do andar abrem sem essa pergunta.
- **Testes:**
  - `internal/floors`, com git de verdade:
    - criar (com acento no nome, `.env` copiado, preparo, log);
    - alterações e recusa com trabalho não commitado ou térreo sujo;
    - merge com o destino aberto (commit com dois pais) e sem pasta;
    - conflito nos dois caminhos, sem deixar rastro;
    - descartar (com branch próprio e com branch existente);
    - subpasta do workspace, preparo que falha e preparo interrompido;
    - pasta que não é repositório;
  - `internal/app`, com agentes falsos:
    - o gerente cria o andar e põe agentes nele (`pwd` mostra a cópia, inclusive com uma pasta do térreo dada por inteiro);
    - `squad list` e `squad floor list`;
    - portal criado no andar;
    - recusas (não gerente, `.env` commitado, trabalho não commitado, Bia aterrissando);
    - aterrissar com merge pela CLI;
    - criar, listar e descartar pela API;
  - `internal/store`, `internal/hub` (recusa remover ou mover um workspace com andares), `internal/agents` e MCP (15 ferramentas, `floor_land` não liberada).
- **Com o Claude Code real** (num worktree de teste do próprio Squad):
  - pedi ao Planejador, de um shell ligado a ele: "crie o andar Rodape, ponha a Bia nele, peça um arquivo commitado e aterrisse mesclando";
  - ele usou `floors` e `new` pelo MCP, sem aprovação;
  - a Bia, na cópia, pediu aprovação para criar o arquivo e para o `git commit` (o modo manual dela). Aprovei como usuário;
  - o Planejador chamou `floor_land`, que pediu aprovação;
  - resultado: um commit de merge com os dois pais no branch de origem. A pasta do andar e o `.squad` vazio foram apagados, e o branch `rodape` ficou;
  - depois, com um andar e um terminal nele, a interface mostrou só os nós do térreo (captura no PR).
- **Precisa de você (Windows):** criar um andar, abrir um agente nele e aterrissar. O ponto a observar é a remoção da pasta logo depois de fechar os terminais: o Squad tenta de novo por até 5 s, porque o Windows pode segurar a pasta por um instante. Vale também testar o preparo pelo Git Bash.

## 2026-10-16, MCP (R3)

- **`squad mcp`:** servidor MCP por stdio, feito com o SDK oficial em Go (`modelcontextprotocol/go-sdk` v1.8.0). O agente o abre dentro do terminal, e cada chamada vai ao servidor do Squad como aquele terminal, pelo mesmo caminho da CLI (`callServer`). São 13 ferramentas, com argumentos tipados:
  - `list`, `ask`, `ask_batch`, `check`, `reply`;
  - `note` (list/read/write/edit/create);
  - `portal` (snapshot, click, fill, type, key, scroll, goto, back, forward, reload, screenshot, console, create, close, list);
  - `new`, `connect`, `close`;
  - `keys`, `portal_eval`, `new_shell`.
- **Segurança:**
  - as dez primeiras vêm liberadas no Claude Code (`permissions.allow` no `--settings` que o Squad já passa). As chamadas delas levam a marca `safe`, e com ela o **servidor** recusa tudo o que rodaria um comando:
    - mensagem para um shell ou comando próprio (seria executada como linha de comando);
    - teclas (`--raw`);
    - `new` com linha de comando ou shell;
    - JavaScript no portal;
  - `keys`, `new_shell` e `portal_eval` pedem aprovação a cada uso;
  - nomes que começam com `-` são recusados, para um argumento não virar opção (`--command=…`);
  - a decisão fica no servidor, então mexer nos argumentos não contorna a regra.
- **Configuração por agente:**
  - **Claude Code:** `--mcp-config ~/.squad/run/claude-mcp.json`;
  - **Codex:** `-c mcp_servers.squad.*`, com `env_vars` (o Codex só repassa algumas variáveis a servidores stdio) e `tool_timeout_sec=1800` (o padrão é 60 s, e um `ask` pode levar minutos);
  - **Devin:** `mcpServers` no mesmo `--config` dos hooks;
  - **Gemini:** não tem opção por sessão; fica com a CLI.
- **Por que não liberar a CLI:** `Bash(squad *)` liberado deixaria `squad new --agent shell --command …` rodar qualquer coisa sem aprovação. Pelo MCP, a separação segura/insegura existe por ferramenta.
- **Testes:**
  - `internal/agents`: linha do Claude e do Codex, config do MCP, ferramentas liberadas e as que não são;
  - `internal/app`: um cliente MCP do SDK abre o `squad mcp` de verdade e chama as ferramentas contra o servidor e agentes falsos. Cobre `list`, `ask`, `ask_batch`, as recusas (mensagem ao shell, que não recebeu nada; `new` com shell; nome `--raw`; `eval` pela ferramenta `portal`), `keys`, `new_shell`, `note`, `connect` e `close`.
- **Com o Claude Code real, sem `--allowedTools`:**
  - "use só as ferramentas MCP: veja quem está ligado (list), crie um Implementador Bia (new) e pergunte o papel dela (ask)": em 16 s, sem nenhum pedido de aprovação, a Ana usou `mcp__squad__list`, `mcp__squad__new` e `mcp__squad__ask` e trouxe a resposta da Bia. Isso confirma que o `squad mcp` recebe a identidade do terminal pelo ambiente;
  - pedido para usar `keys` no shell: o Claude Code parou em "squad — Keys Tool (MCP)… Do you want to proceed?", e o shell não recebeu nada. Depois da aprovação, `echo oi-das-teclas` rodou no shell.

## 2026-10-16, agente gerente e portais criados por agentes (R2)

- **Por quê:** com os papéis, o Planejador já sabia que devia delegar, mas só podia falar com os terminais que o usuário tivesse criado e ligado a ele. Agora ele monta a equipe.
- **Modo gerente:**
  - vem do papel (campo `manager`; o Planejador vem ligado) e pode ser trocado no terminal (`manager` no nó; nulo segue o papel);
  - os terminais que um gerente cria não são gerentes, para que uma equipe não cresça sozinha;
  - cada gerente cria no máximo 6 terminais (`squad serve -max-team N`).
- **`internal/team`:**
  - `squad new "Nome" [--role] [--agent] [--cwd] [--command]` cria o terminal num lugar livre ao lado do gerente: à direita primeiro, depois embaixo, à esquerda e em cima, anel por anel (`store.FreeSpot`). O terminal herda o agente, a pasta e o shell do gerente, nasce ligado a ele e inicia;
  - o comando só volta quando o agente novo está pronto (tela parada por 2 s, sem trabalhar; no máximo 45 s, dentro dos 60 s da CLI). Sem isso, um `squad ask` logo depois cairia no shell antes de o agente abrir. Se não ficar pronto, a resposta diz isso e mostra o fim da tela;
  - `squad connect "A" ["B"]` liga dois nós pelo nome (terminais, portais, notas; sem diferenciar maiúsculas);
  - `squad close "Nome"` remove só o que o próprio gerente criou;
  - quem não é gerente recebe 403 com a explicação (menu ⋯ → Modo gerente).
- **Portais criados por agentes:** `squad portal create URL ["Nome"]`, para qualquer agente, abre o portal ao lado de quem pediu, já ligado a ele, até 3 por agente. `squad portal close` fecha um que o agente criou.
- **Servidor:** criar, ligar e remover nós virou método do servidor (`AddNode`, `AddEdge`, `RemoveNode`), usado pela API e pelos agentes. Assim os eventos chegam ao canvas igual. Cada nó guarda quem o criou (`createdBy`).
- **`squad list`** diz ao gerente o que ele pode fazer ("You are a manager: … 2 of 6 created"). A skill ganhou a seção "Building a team". As instruções padrão do Planejador dizem para criar quem faltar com `squad new`, e as do Testador, para abrir um portal com `squad portal create`.
- **Interface:**
  - chip "gerente" no cabeçalho;
  - "Modo gerente" no menu ⋯ dos agentes;
  - opção "Gerente" no editor de papéis;
  - "criado por Plano" no rodapé do terminal e "por Plano" no cabeçalho do portal, com a cor do papel de quem criou.
- **Correção:** `squad portal list` quebrava num servidor sem navegador; `browser.Manager.Get` agora aceita o gerenciador nulo.
- **Segurança:** os comandos `squad` continuam pedindo aprovação no Claude Code. Liberá-los de vez (por exemplo, no `--settings` do Squad) deixaria um agente rodar qualquer comando sem aprovação, com `squad new --agent shell --command …`. Fica a critério do usuário (`--allowedTools`).
- **Testes:**
  - `internal/store` (lugar livre);
  - `internal/app`, com agentes falsos: criar, esperar ficar pronto e o primeiro `ask` chegar ao agente; erros, limite, criados não são gerentes, connect, close e modo gerente desligado; portais criados, nomes, limite e só quem criou fecha.
- **Com o Claude Code real:**
  - um Planejador (com `--allowedTools "Bash(squad *)"`) recebeu: "monte uma equipe com um Implementador Bia e um Testador Caio, abra um portal App em localhost:8765, ligue o Caio a ele e pergunte o papel de cada um";
  - em 22 s ele rodou `squad new` duas vezes, `squad portal create`, `squad connect` e `squad ask --batch`. Respondeu: "Bia: Sou a Bia, a Implementadora do Squad… / Caio: Sou o Testador do Squad…";
  - no canvas, Bia e Caio apareceram ao lado do Plano, com "criado por Plano", e o portal App, com "por Plano", ligado ao Caio.

## 2026-10-16, papéis com instruções (R1)

- **Por quê:** até aqui o papel era só cor e etiqueta; o agente não recebia nada. Um Planejador aberto no canvas não sabia que devia delegar.
- **`internal/roles`, a biblioteca:**
  - os quatro papéis de sempre (ids `plan`, `impl`, `rev`, `test`, os mesmos que os terminais já gravavam) ganharam instruções padrão. O Testador testa interfaces web pelo portal (`squad portal snapshot/click/fill/console/screenshot`);
  - o usuário edita as instruções e, se quiser, restaura o padrão. Também cria papéis com nome, cor (8 opções), ícone (12 opções) e instruções de até 4.000 caracteres;
  - o arquivo é global (`~/.squad/roles.json`) e guarda só os papéis novos e os editados. Assim, um papel que ninguém editou acompanha os padrões das próximas versões;
  - o hub serve `GET/POST /api/roles`, `PUT/DELETE /api/roles/{id}` e `POST /api/roles/{id}/reset`, e manda a lista em `/ws/workspaces` a cada mudança;
  - remover um papel em uso é recusado, com a lista dos terminais ("Ana (proj)").
- **Como cada agente recebe o papel** (texto: "Você está no terminal "Ana" do Squad, com o papel "Testador"…", seguido das instruções):
  - **Claude Code:** `--append-system-prompt-file`, com um arquivo por terminal em `~/.squad/run/roles/`, apagado quando o terminal é removido. O flag funciona no modo interativo (documentação do Claude Code);
  - **Codex:** `-c developer_instructions="…"` (chave oficial da config). O texto vai como string TOML só em ASCII: acentos, aspas, `%`, `$` e `!` viram `\uXXXX`, para que nenhum shell (sh, PowerShell, cmd) o altere na linha de lançamento;
  - **Devin:** o hook `SessionStart` (no mesmo `--config` dos outros hooks) chama `squad hook devin-session-start`, e o servidor responde com `additionalContext` no formato do Claude Code, que o Devin documenta. O `squad hook` passou a imprimir a resposta do servidor;
  - **Gemini:** não tem opção para instruções extras, então o papel vai como primeiro prompt (`gemini -i "$SQUAD_ROLE"`), a partir de uma variável de ambiente para a linha ficar curta. No cmd, as quebras de linha viram espaços;
  - **shells e comandos próprios:** nada. Digitar instruções num shell poderia executá-las (com o Gemini ausente, por exemplo, o texto cairia no bash). Por isso a ideia inicial de mandar uma "primeira mensagem" quando o agente ficasse pronto foi descartada.
- **`squad list`** mostra o nome do papel (`role: "Testador"`), e a skill diz para dar o trabalho a quem tem o papel certo.
- **Interface:**
  - ⚙ → "Papéis…" abre a biblioteca: lista à esquerda e editor à direita (nome, cor, ícone, instruções com contador), "Restaurar padrão" nos editados e "Remover" nos novos;
  - o rodapé mostra quem usa o papel no workspace. Depois de salvar, oferece reiniciar esses agentes ("Ana recebe as instruções novas ao reiniciar · Reiniciar agora");
  - no menu do terminal, os papéis vêm da biblioteca, com "Editar papéis…". Trocar o papel de um agente aberto pede confirmação e reinicia o terminal. Shells não têm papel;
  - no formulário do terminal, o papel vem da biblioteca, com o aviso "O terminal reinicia…" quando muda. Nos comandos próprios aparece "o papel fica só como etiqueta";
  - cor, ícone e nome do papel vêm da biblioteca em todo o canvas (faixa, etiqueta, cartão, Composer, gaveta, notas e cursor do portal). Há quatro cores novas (teal, indigo, magenta, laranja) nos dois temas.
- **Testes:**
  - `internal/roles`: padrões, criar, editar, restaurar, remover, ids com acento ("Revisão de UX" → `revisao-de-ux`), nomes únicos e o arquivo;
  - `internal/hub`: API, recusa de papel em uso e lista ao vivo;
  - `internal/agents`: linha de lançamento de cada agente (a string do Codex decodifica de volta ao texto, sem metacaracteres de shell) e hook `SessionStart`;
  - `internal/app`: `squad list` com os papéis e o hook do Devin pelo binário `squad` de verdade.
- **Teste com o Claude Code real** (`go test -tags e2e -run TestRealClaude ./internal/app/`):
  - um papel "responda só em MAIÚSCULAS" fez o Claude responder `PONG`; sem papel, `pong`;
  - o teste agora passa pela tela de boas-vindas e pela pergunta de confiança na pasta (a do repositório) numa máquina nova.
- **Na interface, com o Chrome headless e um Claude Code real:**
  - "Ana" (Claude, Testador) respondeu ao `squad ask` de um shell: "Sou a Ana, Testadora do Squad: testo o que foi feito… Numa interface web, uso um portal ligado a mim (`squad portal snapshot`, `click`…)";
  - pelo formulário, o papel dela passou para Revisor ("Salvar e reiniciar"), e ela respondeu: "Sou a Revisora: leio as mudanças de código e aponto bugs…".

## 2026-10-15, portal: agentes (P2)

- **`squad portal`** (extensão do `agentbus`, como `squad note`): `snapshot`, `click`, `fill`, `type`, `key`, `scroll`, `goto`, `back`, `forward`, `reload`, `screenshot`, `eval`, `console` e `list`. O agente só enxerga portais ligados a ele.
  - **`snapshot`:** a árvore de acessibilidade do Chrome vira uma lista de elementos com referência (`@e3 button "Enviar" [282,218 81x38]`), ordenados pela posição na tela, com valor, marcado/desmarcado e "offscreen". No máximo 300 elementos. As referências valem até a página mudar de documento; uma referência velha pede um snapshot novo.
  - **Ações:** `click` rola o elemento para a tela e clica no centro; `fill` limpa o campo e digita; `key` aceita nomes e combinações (`Enter`, `Control+a`, `Shift+Tab`…). Depois de cada ação, a resposta diz se a página mudou de endereço ("The page is now … Take a new snapshot").
  - **`screenshot`** salva um PNG em `attachments/` do workspace e imprime o caminho; `console` mostra as últimas 200 mensagens e erros da página; `eval` roda JavaScript com limite de 10 s.
  - **Qual portal:** com um só ligado, é ele; com vários, o primeiro argumento é o nome (`squad portal "Docs" click @e2`, sem diferenciar maiúsculas). `squad list` e as @menções do Composer citam os portais pelo nome.
- **Nome do portal:** "Portal", "Portal 2"… dados na criação (os antigos ganham nome ao abrir). Duplo clique no nome ou ⋯ → Renomear; nomes repetidos são recusados. Trocar o endereço mantém o nome.
- **Na tela:** cada ação publica `portal.action` (quem, o quê, onde). O portal mostra o cursor do agente na página, com o nome e a cor do papel, deslizando de um ponto ao outro; teclas, digitação e navegação aparecem num selo no canto. O fio entre o agente e o portal acende na cor do papel, com a pílula "clicando", "preenchendo", "digitando"… por 2,5 s.
- **Correção do P1 (imagem distorcida):** quando a primeira carga da página corria junto com o início do screencast, o Chromium passava a capturar a janela headless (780×441) em vez da página (por exemplo, 638×479). A imagem ficava esticada, e os cliques no canvas caíam fora do lugar. Agora o quadro com tamanho errado é descartado, o tamanho da página é aplicado de novo e o screencast recomeça. Teste novo: `TestFramesHaveViewportSize`, que falha sem a correção.
- **Correção:** o binário `squad` não repassava o comando `portal` ao servidor ("unknown command").
- **Skill:** a seção "Portals" explica o ciclo snapshot → ação → snapshot e os comandos (`squad install-skill` atualiza).
- **Testes:** `internal/browser` (ações dos agentes e tamanho dos quadros) e `internal/app` (`TestPortalCommands`: comandos, nomes, ambiguidade, evento, screenshot, console e endereço gravado), com o Chromium de verdade. Na interface, com o Chrome headless: renomear o portal e um agente "Ana" (Testador) preenchendo, clicando e teclando, com o cursor e a pílula na cor do papel.

## 2026-10-15, portal ao vivo (P1)

- **Teste prévio:** três abas de um Chromium sem janela transmitiram ao mesmo tempo, cerca de 20 quadros por segundo cada. O navegador abriu em 1,3 s e cada aba em 0,2 s. Daí a escolha de um navegador para o servidor inteiro, com uma aba por portal.
- **`internal/browser`:**
  - procura Chrome, Edge, Chromium ou Brave (`SQUAD_BROWSER` força um executável) e abre o navegador só quando um portal precisa, com o perfil em `~/.squad/browser`;
  - fecha 30 s depois da última aba e reabre sob demanda;
  - `Page`: navegar, voltar e avançar (a página em branco inicial não conta), recarregar, tamanho da página, estado (URL, título, carregando, erro, histórico), mouse e teclado com eventos confiáveis, colar e `Evaluate`;
  - o screencast (JPEG) só roda enquanto alguém assiste, e um espectador lento pula quadros em vez de acumular atraso.
- **`internal/portal`** (um por workspace):
  - `GET /w/<id>/ws/portal/{nó}` envia o estado em JSON e os quadros em binário (8 bytes com a hora da pintura, em ms, e depois o JPEG); recebe mouse, teclado, colar e voltar/avançar/recarregar;
  - editar o endereço navega; navegar dentro da página grava o endereço novo no canvas, sem navegar de novo (a última URL aplicada fica guardada por aba, para que mover o nó não recarregue a página);
  - redimensionar o nó redimensiona a página;
  - remover o portal ou o workspace fecha a aba.
- **Web:**
  - transmite só no zoom "ao vivo"; abaixo disso, mostra o último quadro ("pausado");
  - "● ao vivo · N ms" fica âmbar acima de 400 ms. O primeiro quadro de cada conexão é o último guardado e não entra na conta, porque o Chrome só pinta quando a página muda;
  - as posições do mouse são convertidas da imagem na tela para a página em qualquer zoom; o teclado segue as regras dos terminais (`⌥Esc`);
  - o nó do portal ganhou alças de redimensionar;
  - sem navegador, aparece a mensagem com a solução.
- **Testes:**
  - `internal/browser` e `internal/portal` rodam contra um navegador de verdade quando há um (`SQUAD_BROWSER`; sem navegador, são pulados). Cobrem quadros, cliques e teclas, colar, histórico, erro de conexão, tamanho, URL gravada, mover sem recarregar, fechar a aba e o navegador ocioso. Também com `-race`;
  - na interface, com o Chrome headless: contador clicado três vezes, nome digitado, "Enviar", rolagem, link para outra página com a URL salva, voltar, sem navegador e zoom reduzido.
- **Binário:** cerca de 2,6 MB a mais por causa do `chromedp`.

**Precisa de você (Windows):** abrir um portal com um site e ver se o Edge (ou o Chrome) sobe sem janela. Se a política da empresa bloquear o `msedge.exe` nesse modo, o portal mostra o erro; aí vale testar com `SQUAD_BROWSER` apontando para o Chrome.

## 2026-10-15, vários workspaces

- **Uma instância por workspace:** cada workspace é um `app.App` completo (canvas, terminais, `agentbus`, notas, monitor de atividade), como antes. O pacote novo `internal/hub` carrega todos de `~/.squad/workspaces/*` e monta cada um em `/w/<id>/`. Por isso, nada dentro do workspace mudou: pausa, limite por minuto, notas e `squad ask` já ficam separados.
- **`/cli` e `/hooks`:** o hub descobre o workspace pelo `X-Terminal-ID`, e o token do terminal continua sendo conferido pelo workspace.
- **API do hub:**
  - `GET/POST /api/workspaces`, `POST /api/workspaces/open` (acha ou cria pela pasta), `PATCH/DELETE /api/workspaces/{id}`;
  - `GET /api/dirs` relativo à pasta do usuário;
  - `/ws/workspaces` com o resumo (terminais, agentes ativos, pedidos pendentes), agrupado a cada 300 ms.
  - `/api/…` sem `/w/<id>` responde 404 explicando o caminho novo.
- **Remover:** encerra os terminais e move a pasta do workspace para `~/.squad/trash/<id>-<data>`, com novas tentativas no Windows enquanto o watcher de notas solta a pasta. O último workspace não pode ser removido.
- **Cores:** chaves (`teal`, `blue`, …) desenhadas com tokens do tema. Um workspace novo recebe a menos usada; os sem cor (o `default` antigo) ganham uma ao carregar.
- **`server`:** `Handler` foi dividido em `APIHandler` e `TerminalHandler`, com `Guard`, `Session` e `UI` como funções. O redirecionamento do `?token=` mantém os outros parâmetros (`?ws=`). A interface agora é registrada em `/` (um `GET /` conflitava com `/api/` no `ServeMux`) e recusa métodos que não sejam GET e HEAD.
- **`squad serve -dir`:** abre ou cria o workspace da pasta (`.` por padrão). Com um servidor já rodando, pede o workspace a ele e abre o navegador, em vez de falhar com "already running".
- **Web:**
  - prefixo `/w/<id>` em toda a API do workspace;
  - a lista vem de `/ws/workspaces`, e o workspace na tela é o do `?ws=`, depois o último usado, depois o primeiro;
  - a troca limpa o estado do workspace anterior, reabre `/ws/events`, fecha os sockets dos terminais (o `term/session` observa `activeWs`, para evitar um ciclo de imports com o store) e restaura o enquadramento salvo daquele canvas;
  - seletor no topo, `⌥1`–`⌥9` (também com o teclado num terminal), criar, editar e remover.
- **Testes:**
  - `internal/hub`: criar, abrir, editar e remover, lixeira, recarga, cor do workspace antigo, roteamento por workspace, `check` de um terminal com o mesmo nome em dois workspaces, credenciais erradas, stream da lista e redirecionamento do token;
  - todos com `-race`;
  - interface conferida com o Chrome headless: um terminal trabalhando no p1 enquanto o p2 está na tela, o ponto de atividade, `⌥2`, a volta com o terminal ainda trabalhando, criar, renomear (ao vivo na barra) e remover.

**Decisões (combinadas):** todos os workspaces sobem com o servidor; `serve -dir` com servidor rodando abre o workspace nele; remover manda para a lixeira. A gaveta de atividade é por workspace e recomeça ao trocar (o servidor só guarda os pedidos pendentes).

## 2026-10-15, pasta e shell por terminal, fonte de ícones

- **Pasta do terminal:** absoluta, `~/…` ou relativa ao workspace (`ResolveCwd` em `internal/server`). A relativa segue o workspace se a pasta dele mudar. Criar ou editar com uma pasta inexistente dá 400 (`folder not found: …`), em vez de um terminal que não abre.
- **`GET /api/dirs?path=…`:** subpastas do trecho digitado, escritas do jeito que foram digitadas (relativas continuam relativas). Pastas ocultas só aparecem com `.`; até 50 sugestões.
- **`GET /api/shells`:** shells instalados com nome amigável (Git Bash, PowerShell 7, Windows PowerShell, cmd; no Unix, os candidatos mais o `/etc/shells`, sem `tmux`/`screen`), o padrão primeiro.
- **Iniciar e reiniciar** agora remontam o terminal a partir do nó (`StartTerminal`), para valer o que foi editado. O `Manager.Restart` reaproveitava a spec antiga.
- **Web:**
  - `TerminalForm` substitui o `NewTerminalForm` e também edita ("Configurar…" no menu do terminal), com "Salvar e reiniciar" quando muda agente, comando, pasta ou shell;
  - `DirInput` faz as sugestões de pasta;
  - o rodapé do terminal mostra a pasta resolvida, cortando o começo do caminho e não o fim;
  - `~` também abrevia `C:\Users\<nome>`.
- **Ícones:** `web/src/lib/icons.ts` lista os 63 ícones usados (tipo `IconName`). `scripts/subset-icons.py` (fontTools) mantém só as ligaduras deles, fixa peso 400, grade 0 e opsz 20 e mantém o eixo FILL: 4 MB viram 8,8 KB. O binário cai de 13,5 MB para 9,5 MB. `npm run build` falha se a lista e a fonte divergirem (`make icons`).
- **Testes:** `TestResolveCwd`, `TestTerminalFolder` (pasta relativa, inexistente, edição + reinício), `TestDirSuggestions`, `TestDirsAndShellsEndpoints`, `TestShellOptionsLabels`, `TestEtcShells`. A interface foi conferida com o Chrome headless: sugestões de pasta, Tab, criação, "Configurar…" com reinício e os ícones (incluindo o preenchido).

**Decisão:** a pasta é guardada do jeito que foi digitada (sem a barra final), e não resolvida, para que as relativas acompanhem o workspace.

## 2026-10-15, Windows: Git Bash como shell padrão

No primeiro teste num Windows real, nenhum terminal abria: `failed to create process: This program is blocked by group policy`. A máquina bloqueia o PowerShell, que era o primeiro da lista.

- **Ordem no Windows:** Git Bash, `pwsh`, `powershell`, `cmd`. O Git Bash (`<Git>\bin\bash.exe`) é procurado ao lado do `git.exe` no PATH, em `%ProgramFiles%\Git` e em `%LOCALAPPDATA%\Programs\Git`. Um `bash` sem caminho nunca passa pelo PATH, porque cairia no `System32\bash.exe` do WSL, onde o `squad.exe` e os agentes do Windows não funcionam.
- **Git Bash** abre com `--login -i`, `CHERE_INVOKING=1` (o profile não troca para `$HOME`) e `MSYS2_PATH_TYPE=inherit` (mantém o PATH do Windows). As duas variáveis só entram se o usuário não as tiver definido.
- **`SQUAD_SHELL`** (todas as plataformas) passa à frente do padrão. O shell escolhido no nó continua tendo prioridade.
- **Fallback:** se o processo do shell não puder ser criado, o terminal tenta o próximo candidato. A linha do agente é montada com as aspas do primeiro candidato; o fallback só a quebra se cair até o `cmd` com um caminho entre aspas.
- **Testes:** `shell_test.go` simula máquinas Windows (Git em `cmd\`, em `mingw64\bin`, instalação por usuário, sem Git, `SQUAD_SHELL`, WSL no PATH); `TestFallsBackWhenShellCannotStart` usa um "shell" que não executa.

**Precisa de você (Windows):** confirmar que o Git Bash abre na pasta do workspace, que `squad list` funciona dentro dele e que o `claude` abre com os hooks (`/hooks`).

## 2026-10-15, A1 adaptador do Devin CLI

Feito como o Codex: segue a documentação oficial do Devin CLI (docs.devin.ai/cli) e foi testado só com o agente falso.

- **Tipo `devin`** em `internal/agents` (`Hooks: true`, comando padrão `devin`).
- **Hooks:** ao subir, o servidor grava `~/.squad/run/devin-hooks.json` com `{"hooks": {"Stop": …, "UserPromptSubmit": …}}` no formato do Claude Code, chamando `squad hook devin-stop` e `squad hook devin-prompt`. A linha de lançamento vira `devin --config <arquivo>`. O gerador do arquivo do Claude virou `writeHooks`, usado pelos dois.
- **Parser:** `devin-prompt` abre o turno; `devin-stop` usa o mesmo payload do `claude-stop` (`last_assistant_message`, enviado pelo Devin desde a v3000.5.20). O Devin não tem `StopFailure`.
- **Skill:** `squad install-skill -agent devin` grava em `~/.config/devin/skills/squad/` (`$XDG_CONFIG_HOME` se definido; `%APPDATA%\devin\skills` no Windows). O Devin não lê `~/.claude/skills`, só o `.claude/skills` do projeto.
- **Web:** "Devin" no seletor de agente, com nome padrão "Devin".
- **Testes:** parser e linha de lançamento em `agents_test.go`, `TestDevinHookRepliesWithFinalMessage` (agente falso com `FAKEAGENT_HOOK=devin`) e `TestInstallSkillDevin`.

**Decisão:** hooks injetados por `--config`, porque é o único jeito por terminal sem tocar na config do usuário nem no repositório dele. A documentação só diz "Configuration file path"; não diz se o arquivo **soma** com `~/.config/devin/config.json` ou **substitui** esse arquivo.

**Precisa de você (máquina com Devin):**
1. Abrir um terminal "Devin" no canvas e rodar `/hooks`: devem aparecer `Stop` e `UserPromptSubmit` vindos de `devin-hooks.json`.
2. Conferir se as suas configurações (modelo, permissões, tema) continuam valendo dentro do Squad. Se o `--config` **substituir** a config do usuário, o plano B é um plugin local do Devin (`hooks/hooks.json`) instalado uma vez com `devin plugins install`. Fora de um terminal do Squad, o `squad hook` não faz nada.
3. Ligar um Claude ao Devin e pedir `squad ask "Devin" "diga pong"`: a resposta deve vir como `[reply from Devin · request …]` com a mensagem final, não a tela. Se vier a tela, teste o encaminhamento à mão num terminal do Squad: `SQUAD_DEBUG=1 squad hook devin-stop '{"last_assistant_message":"x"}'` mostra o erro, se houver.
4. Mensagem com várias linhas: o envelope `[de: … · pedido …]` deve chegar inteiro na caixa do Devin e ser enviado com o Enter.
5. Provocar um erro (sem rede, por exemplo) e ver se o `Stop` dispara. Se não disparar, o pedido só termina no timeout; a correção seria aceitar o "idle" da tela para o Devin.
6. Na primeira execução numa pasta, o Devin pergunta se confia no workspace; responder no próprio terminal.
7. Com `--sandbox`, ver se o `squad ask` alcança `127.0.0.1` (o sandbox filtra rede).

## 2026-10-15, skill empacotada

- **`squad-skill.zip` na raiz:** a pasta `squad/` com o `SKILL.md`, gerada por `make skill-zip` a partir do próprio binário (`squad install-skill -dir bin/skill`). Assim o zip nunca diverge do texto que o `install-skill` grava. Se o texto da skill mudar em `cmd/squad/hook.go`, rode `make skill-zip` de novo.
- **Limpeza:** o binário `fakeagent` tinha sido commitado por engano na raiz no B3. Saiu do repositório e entrou no `.gitignore`. Os testes continuam compilando o agente falso a partir de `internal/testutil/fakeagent`.

## 2026-10-15, W4 web: Composer, notas e portal

- **Servidor: `POST /api/nodes/{id}/send`** (`internal/app/compose.go`). Entrega uma mensagem do Composer a um terminal como colagem + Enter, igual a digitar.
  - **Menções:** cada `@Nome` precisa estar ligado ao destino; senão, 400. A mensagem ganha uma linha final que diz ao agente o que é cada menção e como alcançá-la, por exemplo `[menções: @B = terminal ligado (squad ask "B" "…") · @Plano = nota ligada plano.md (squad note read "plano.md")]`.
  - **Imagens coladas** (data URL PNG, JPEG, GIF ou WebP): são salvas em `~/.squad/workspaces/<id>/attachments/` e o caminho vai no texto. O Claude Code anexa imagens referenciadas por caminho.
  - **Evento `message.sent`:** a gaveta mostra "Você → Implementador".
  - **Teste:** `TestComposerSend` confere o texto entregue pelo log do agente falso, os anexos, o evento e as recusas (menção não ligada, mensagem vazia, imagem inválida).
- **Composer** (⌥K ou clique na pílula; Esc fecha):
  - **Destinos:** chips "Para" com papel e agente, "+ destino" para enviar a vários ("Enviar a 2"), e aviso do estado do destino ("trabalhando · entra na fila"; "parado · inicie o terminal" bloqueia o envio).
  - **Texto:** `contentEditable` em que as menções viram chips inline (ícone de nota ou portal, quadrado da cor do papel para terminal).
  - **Menu @:** acima do Composer, com "Ligados a X" (só nós ligados a todos os destinos), filtro com o trecho sublinhado, nomes que começam com o texto digitado primeiro, ↑↓ e ⏎/Tab. "Não ligados" aparecem desabilitados, com "ligue para mencionar".
  - **Imagens:** colar (⌘V), soltar ou escolher arquivo; o cartão mostra miniatura e "1440×900 · colada".
  - **Envio:** ⏎ envia, ⇧⏎ quebra a linha; a colagem só aceita texto puro.
- **NoteNode:**
  - prévia em Markdown com renderer próprio, sem HTML injetado: títulos, listas, tarefas clicáveis, código, negrito, itálico e links http/mailto;
  - modo de edição (Prévia/Editar ou duplo clique), salvo 800 ms depois de parar de digitar e ao sair;
  - cadeado para travar e destravar (travada, ninguém escreve);
  - edição de um agente: o trecho alterado fica destacado na cor do papel do autor, com "Implementador escreveu há 4 s", por 30 s. Se o arquivo muda enquanto você edita, aparece "X alterou a nota · Recarregar" em vez de sobrescrever o rascunho.
- **PortalNode:** endereço editável (`PATCH portal.url`), botão para abrir no navegador e "sem transmissão". A imagem ao vivo via CDP fica para depois.
- **ComposerPill:** mira só em terminais (foco, seleção ou último destino).
- **Teste com eventos reais via CDP:**
  - menu @ com "Ligados a Planejador", chip, envio entregue ao agente falso e toast "Enviado a Planejador";
  - `squad note edit` vindo de um agente, que destacou a linha;
  - edição pela UI salva no `.md` e tarefa marcada com clique.

**Decisão:** como o agente recebe as menções. No Maestri elas viram tags próprias; no Squad o texto mantém o `@Nome` legível e ganha uma linha final de contexto com o comando que alcança cada nó. Não há formato novo para o agente aprender, e a skill do Squad já ensina `squad ask` e `squad note`.

## 2026-10-15, W3 web: fios e conversa

- **A conversa no fio, dirigida pelos eventos reais** (`request.sent`, `reply.sent`, `reply.received`, `request.cancelled`):
  1. **Viajando:** o fio acende em âmbar da origem ao destino (`pathLength` = 100, traço animado em 600 ms) e o envelope "✉ id →" percorre a curva (CSS `offset-path`).
  2. **Pendente:**
     - fio tracejado em âmbar; a alça da origem fica cheia e a do destino ganha um anel;
     - pílula com ampulheta, cronômetro e id no ponto mais baixo da curva.
  3. **Resposta:** o fio acende em verde do destino à origem, com o envelope "← resposta id".
     - Uma resposta que vem direto de um hook ou da tela estável chega só como `reply.received` (sem `reply.sent`); a animação de volta dispara pelos dois.
  4. **Recebida:** o marcador "✓ id · 0:07" some em cerca de 4 s.
  - Um pedido cancelado mostra "⊘ id cancelado".
  - Com `prefers-reduced-motion`, as animações vão direto ao estado final.
- **Faixas de remetente**, acima do terminal e nunca por cima:
  - "Pedido id de X · a resposta volta para ele", na cor do papel de X;
  - "Aguardando Y · pedido id · enviado há N s";
  - "Resposta de Y · id · 7s", que fica 60 s ou até o próximo pedido.
  - Quando a faixa aparece, a grade do terminal se reajusta (`ResizeObserver` no slot).
- **Pausa e laço:**
  - com as conversas pausadas, cada fio mostra um selo ⏸;
  - ao atingir o limite de saltos, a gaveta registra "Laço: limite de N saltos atingido… Conversas pausadas até você retomar".
- **Pontos de curva:**
  - duplo clique no fio adiciona um ponto onde ele menos alonga a curva;
  - o ponto pode ser arrastado (fio selecionado); duplo clique no ponto o remove;
  - a curva passa pelos pontos (`PATCH /api/edges/{id}`).
- **Taxa de pedidos:** "N/30 pedidos por min" agora é uma janela deslizante de 60 s no cliente (antes só subia).
- **Rótulos no fio:** num fio entre nós empilhados, o ponto mais baixo cai sobre o nó, então as pílulas vão para o meio da curva. O "Desligar" se afasta de pontos de curva próximos.
- **`tools/shot`:** ganhou os passos `dblclick` e `shot FILE` (capturas intermediárias na mesma página).
- **Teste:** uma conversa real entre dois agentes falsos (`fakeagent` com o hook do Claude):
  - o Planejador roda `squad ask Implementador …`, e o Implementador trabalha por 7 s e responde pelo hook `claude-stop`;
  - as quatro fases, as faixas, os estados (Esperando resposta → Trabalhando/Terminou), a caixa e a gaveta ("Respondido em 0:07") foram capturados quadro a quadro.

## 2026-10-15, W2 web: nó de terminal

- **Sessão de terminal (`web/src/term/session.ts`):**
  - cada terminal tem uma instância do xterm.js e um WebSocket `/ws/term/{id}`, guardados num registro fora do React; trocar o nível de zoom não derruba o buffer nem a conexão;
  - na mensagem `init`, a tela é limpa e redimensionada para o tamanho do PTY; a tela atual chega em seguida;
  - a reconexão é automática e fica imediata quando o terminal volta a rodar (iniciar ou reiniciar);
  - a entrada vai em binário;
  - a sessão é descartada quando o nó sai do workspace.
- **Texto nítido e mouse correto em qualquer zoom (counter-scale):**
  - o xterm não aceita ancestrais com `transform: scale` (o mouse cai na célula errada), e o React Flow aplica o zoom com um `scale`;
  - por isso, o slot do terminal é dimensionado `zoom` vezes maior e reduzido com `scale(1/zoom)`, com fonte 11 × zoom. A escala efetiva fica 1;
  - a métrica é ajustada pela grade medida: linha de 15 × zoom (handoff 11/15), sem estourar o slot;
  - o layout é refeito quando o gesto de zoom termina; durante o gesto, o conteúdo fica congelado, como pede o handoff.
- **Três níveis de zoom, com histerese de ±0,03:**
  - vivo (≥ 0,6), com o xterm;
  - miniatura (0,35–0,6), com um snapshot em HTML colorido (`serializeAsHTML`) a cada 3 s e as legendas em pixels de tela;
  - cartão (< 0,35), com nome, agente, papel, caixa e a banda de estado, em 240 px de design reduzidos ao espaço do nó.
- **Foco:**
  - um clique dá o teclado ao terminal, com anel e "teclado aqui";
  - ⌥Esc devolve o teclado ao canvas;
  - ⌥A, ⌥P e ⌥K continuam globais mesmo com o terminal focado.
- **Roda e pinça:**
  - a roda simples rola o terminal sem mover o canvas;
  - pinça ou ⌘/ctrl + roda sobre o terminal dão zoom no canvas em torno do ponteiro (antes, o navegador dava zoom na página).
- **Tamanho:**
  - o nó tem alças de redimensionar quando está selecionado;
  - ao soltar, o cliente calcula cols×rows pela métrica exata da fonte e envia `PATCH` com `w`, `h` e `terminal.cols/rows`;
  - um terminal sem tamanho salvo recebe a grade que cabe no nó: 81×24 em 560×440.
- **Servidor:** um `PATCH` que muda `terminal.cols/rows` redimensiona o PTY em execução. Todos os clientes seguem pelo `node.updated` (teste `TestPatchTerminalSizeResizes`).
- **Também no nó:**
  - caixa de pedidos pendentes no rodapé e no cartão;
  - aviso de processo encerrado ou com erro sobre o terminal;
  - menu com iniciar/parar, reiniciar, papel e remover.
- **`tools/shot`:** ganhou `-steps`, com eventos reais via CDP (clique, arraste, texto, teclas, roda com modificadores, eval). Os testes usaram esses eventos:
  - digitação no shell, com acentos, CJK e cores;
  - seleção com o mouse a 80% em DPR 1 e 2;
  - zoom por pinça sobre o terminal;
  - miniatura e cartão;
  - redimensionamento até 104×31, confirmado com `tput cols/lines` dentro do PTY.

**Decisões:**
- **Renderer DOM do xterm, não WebGL.** O addon WebGL mede o próprio canvas com um `ResizeObserver` (`device-pixel-content-box`), e o Chrome informa esse tamanho errado quando há um ancestral com `scale`. Em DPR 2, o canvas saiu com metade da resolução e o terminal aparecia com o dobro do tamanho, cortado. O renderer DOM não tem esse problema e, com o counter-scale, fica nítido. Se o desempenho com muitos terminais pedir WebGL, o caminho é uma camada sobreposta sem `transform`, posicionada sobre os nós. A dependência `@xterm/addon-webgl` foi removida.
- **⌥Esc, e não Esc duas vezes, para sair do terminal.** O Claude Code usa Esc Esc (voltar a uma mensagem anterior), então o atalho do handoff tomaria a tecla do agente.
- **O tamanho do PTY é dado pelo nó:** quem define cols×rows é o cliente que redimensiona, e o servidor só aplica. Vários navegadores abertos seguem o mesmo valor pelo evento, sem disputa.

## 2026-10-15, W1 web: shell e canvas

- **Projeto `web/`:**
  - React 19, `@xyflow/react` 12, zustand 5 e Vite 8, com versões fixas no `package.json`;
  - o build vai direto para `internal/webui/dist`, que é embutido no binário;
  - `make web` refaz a UI.
- **Tema:**
  - os tokens do handoff foram copiados, e as fontes Geist, Geist Mono e Material Symbols vão no pacote, sem Google Fonts;
  - `data-theme` (escuro/claro) e `data-term` (noite/papel) ficam no `<html>`;
  - trocar o tema também troca a paleta do terminal, que continua ajustável no menu.
- **Estado (`src/state/store.ts`):**
  - o primeiro evento de `/ws/events` (`hello`) traz o estado completo;
  - depois, os eventos atualizam nós, arestas, runtime, pendências, voos (para o W3) e o registro de atividade;
  - a reconexão é automática, e a TopBar mostra "reconectando…".
- **Componentes:**
  - TopBar com workspace, andar "main", taxa, pausar/retomar, atividade, servidor e menu de tema;
  - Toolbar com as ferramentas Selecionar/Mover e os botões de criar terminal, nota e portal; desenho e grupo ficam desabilitados;
  - formulário de novo terminal, com nome único automático ("Claude 2");
  - minimapa com as cores de estado e controles de zoom (os dois se deslocam quando a gaveta abre);
  - ComposerPill;
  - ActivityDrawer com filtros por terminal, desfechos e limites, e a aba vertical quando fechada.
- **Canvas:**
  - React Flow com grade de pontos de 20 e zoom inicial de 80%; o viewport é lembrado no navegador;
  - nós arrastáveis pelo cabeçalho, com a posição salva via `PATCH`;
  - para ligar, arrasta-se a partir de uma alça lateral e solta-se em qualquer ponto do outro nó. Os destinos válidos ganham anel e os inválidos ficam a 40%;
  - Delete apaga o fio selecionado, que também mostra um botão "Desligar";
  - um toast avisa "A ↔ B ligados/desligados".
- **Nós:**
  - o TerminalNode tem faixa de papel, cabeçalho (nome, agente, papel, "teclado aqui", pílula de estado com cronômetro e menu com iniciar/parar, reiniciar, papel e remover) e rodapé; o corpo ainda é provisório, e o xterm entra no W2;
  - NoteNode mostra o texto da nota;
  - PortalNode é uma casca.
- **WireEdge:** âncoras nos lados que se olham, curva com caimento de 0,35 × distância (máximo de 120) e passagem pelos `ropePoints` (Catmull-Rom).
- **`tools/shot`:**
  - virou um módulo Go próprio, e o chromedp saiu do `go.mod` do squad;
  - agora imprime os erros e o console da página.
- **Teste:** `squad serve` isolado (com `SQUAD_HOME` no scratchpad) mais a disposição da referência via API. As capturas foram comparadas com o `Squad.dc.html`. Um `squad ask` real entre dois shells apareceu como pendente na gaveta, com o Planejador em "Esperando resposta".

**Decisões:**
- **Build da UI commitado:** o `dist` vai para o git, para que `go build` e `go install` saiam com o canvas sem Node.
  - A fonte Material Symbols (4 MB) é commitada uma vez só, porque o nome do arquivo leva o hash.
  - Dá para reduzir a fonte depois, com um subconjunto dos ícones usados.
- **Atalhos:** o handoff usa ⌥P tanto para "pausar conversas" quanto para "criar portal". Ficou assim:
  - ⌥ para os atalhos globais (⌥A gaveta, ⌥P pausar, ⌥K composer);
  - letras simples para as ferramentas do canvas (V, H, T, N, P), como mostram os tooltips da Toolbar;
  - as letras simples não agem quando o foco está num campo ou no terminal.
- **Remover nós:** só pelo menu do nó, com confirmação, porque remover um terminal encerra o processo. A tecla Delete apaga só fios.
- **Seletores do zustand 5 devem devolver referências estáveis:** um `?? []` dentro do seletor causava um laço infinito de renderização (React #185). A correção foi o `useNodes()`, com uma lista vazia constante.

## 2026-10-15, B5 notas

- **`internal/notes`:**
  - cada nota é um `.md` em `~/.squad/workspaces/<id>/notes/`;
  - **nome estável por padrão**: um slug do título com sufixo `-2`, `-3`… em colisões, sem renomear pela primeira linha como o Maestri faz;
  - um watcher (`fsnotify`) publica `note.edited` com `author: external` para edições fora do Squad, e as escritas pelo Squad publicam o nome do autor (agente ou `user`).
- **CLI:** `squad note list|read|write|edit|create|help`.
  - Só alcança notas ligadas ao terminal, inclusive cadeias nota↔nota.
  - O `read` sai numerado, com `[N lines total]` e intervalo opcional.
  - O `edit` exige trecho **único** (409 se ausente ou repetido).
  - O `create` liga a nota ao criador e a posiciona à esquerda dele.
  - Uma nota bloqueada dá 423.
- **API para a interface:** `GET/PUT /api/notes/{id}`; nós de nota criados por `POST /api/nodes` recebem o arquivo automaticamente.
- **README:** início rápido para testar sem a interface (curl na API).

**Decisão:** apagar o nó de uma nota **não apaga** o arquivo `.md`. É não destrutivo; fica para a interface oferecer "apagar também o arquivo".

## 2026-10-15, B4 adaptadores de agentes

- **`internal/agents`:**
  - **Claude Code:** o Squad gera `~/.squad/run/claude-hooks.json` com os hooks `Stop`, `StopFailure` e `UserPromptSubmit` chamando `squad hook …`, e a linha de lançamento recebe `--settings <arquivo>`. Os hooks de vários arquivos de settings são somados, então os do usuário (por exemplo, o RTK) continuam.
  - **Codex:** `-c notify=["<squad>","hook","codex-notify"]`, com o evento `agent-turn-complete` e `last-assistant-message`. **Não testado com conta real**, só com payload de exemplo.
  - **Gemini, shell e comandos próprios:** só a heurística de tela.
- **Mensagem final:** vem de `last_assistant_message` do hook `Stop`. Em versões antigas, a reserva é a última mensagem do transcript.
- **Monitor:**
  - `SignalStart` (o turno começa pelo hook) e `Signal` (o turno termina pelo hook);
  - um turno iniciado por hook só termina pelo hook, ou depois de 60 s sem mudança na tela;
  - para agentes com hooks, o barramento ignora o fim de turno detectado pela tela e desliga o fallback de tela estável.
- **`squad hook <nome> [payload]`:** repassa o payload ao servidor e nunca falha de forma visível, para não atrapalhar o agente.
- **`squad install-skill`:** instala `~/.claude/skills/squad/SKILL.md` (escrita por mim) com os comandos, o envelope e o `reply`.
- **Teste real com Claude Code** (`go test -tags e2e -run TestRealClaude ./internal/app/`): o Shell perguntou ao Claude e recebeu `pong` em 6,4 s, pelo hook `claude-stop`, sem lixo de tela.

**Fica para depois:** `squad mcp` (servidor MCP), priorizando a web.

**Precisa de você:**
- testar com o Codex quando houver conta (`agent: codex`);
- decidir se a skill deve ser instalada automaticamente no primeiro `serve`. Hoje é manual (`squad install-skill`), para não mexer em `~/.claude` sem pedir.

## 2026-10-15, B3 conversa entre agentes

- **`internal/activity`, o monitor:**
  - amostra periódica (1 s): uma amostra é ativa com ≥500 B de saída **e** mudança no hash da tela, ou com título de spinner;
  - 3 amostras ativas seguidas = "trabalhando"; mais de 3 s sem mudança = fim de turno;
  - sinais explícitos (hooks) encerram o turno na hora;
  - estados da interface: boot, idle, work, wait, done, err, off.
- **`internal/agentbus`, a conversa:**
  - `list`, `ask` (bloqueante), `ask --raw`, `ask --batch`, `check`, `reply <id>` e `help`, todos via `POST /cli`;
  - envelope `[de: Nome · pedido id]` (sem envelope para shells);
  - pendentes com fim de turno, fallback de tela estável (5 s), timeout de 120 s prorrogável e teto de 30 min;
  - cancelamento quando o cliente desiste ou a conexão ou o nó somem;
  - limite de saltos (6, com pausa automática), limite de taxa (30/min, pedidos retidos) e pausar/retomar;
  - eventos `request.*` e `reply.*`.
- **`internal/app`:** monta servidor, monitor e barramento; rotas `POST /api/conversations/pause|resume`; `terminal.state` passa a levar `activity`.
- **CLI `squad`:** cliente fino (`SQUAD_URL`, `SQUAD_TERMINAL_ID`, `SQUAD_TOKEN`); mensagem pelo stdin (heredoc); sem timeout no `ask` bloqueante.
- **Testes de integração com agentes falsos conversando pela CLI real:**
  - resposta no fim do turno e pelo fallback;
  - erros 400/403/404/503;
  - resposta direta, `reply` por id e ciclo A→B→C→A liberado;
  - `--raw`, `check` e `--batch`;
  - limite de saltos, pausa e cancelamento por remoção de conexão.

  Passam com `-race`, repetidos.

**Decisões (melhorias sobre o Maestri):**
- **Resposta direta:** se B manda `ask A` enquanto A espera por B, a mensagem **é a resposta** do pedido de A. Ela chega completa ao comando bloqueado de A, e B recebe "OK" na hora. O Maestri colava como prompt novo em A e deixava B esperando o turno de A.
- **`squad reply <id>`:** responde explicitamente pelo id do envelope.
- **Turno com hook:** quando o fim de turno vem com a mensagem final (hooks, na B4), a resposta é essa mensagem, e não a tela.
- **Fallback desligável:** o fallback de tela estável pode ser desligado por tipo de agente (os que têm hooks), para um agente que pensa em silêncio não liberar a espera antes da hora.
- **Teto na espera:** o timeout tem teto (30 min), e o pedido é cancelado se o cliente desconecta.
- **Saída da CLI em inglês:** é lida principalmente pelos agentes. A interface fica em português.

**Observação:** a detecção de "trabalhando" depende de saída contínua entre as amostras. Um agente que imprime em rajadas espaçadas mais que o intervalo de amostragem pode não ser marcado como trabalhando; aí vale o fallback, ou o hook.

## 2026-10-15, B2 servidor

- **`internal/store`:** workspace com nós (terminal, nota, portal) e arestas não direcionadas (`ropePoints`), em JSON atômico no envelope `{type, schemaVersion, payload}`. Validações: nome único de terminal, sem aresta para si mesmo nem duplicada. Consultas de vizinhos e conexão.
- **`internal/events`:** pub/sub que descarta assinantes lentos.
- **`internal/server`:**
  - **segurança:** só escuta em 127.0.0.1; verifica `Host` (contra DNS rebinding) e `Origin`; sessão por cookie `HttpOnly`/`SameSite=Strict`, criado a partir de `/?token=…`, ou por `Authorization: Bearer`; token por terminal (`X-Terminal-ID` + `X-Squad-Token`) para a CLI e os hooks;
  - **REST:** `/api/state`, CRUD de nós e arestas, `start`/`stop`/`restart`;
  - **WebSocket:** `/ws/events` (começa com `hello` e o estado completo) e `/ws/term/{id}` (`init`, render inicial em binário, saída em binário; entrada em binário; `resize` em texto);
  - cada terminal recebe `SQUAD_URL`, `SQUAD_TERMINAL_ID`, `SQUAD_TERMINAL_NAME`, `SQUAD_TOKEN` e `SQUAD_CLI`.
- **`internal/webui`:** interface embutida no binário (por ora, página provisória).
- **`squad serve`:** abre o workspace padrão, escuta em 127.0.0.1:7420, grava `~/.squad/run/server.json`, mostra a URL com token, abre o navegador e sobe os terminais. No Ctrl-C, encerra tudo e limpa a descoberta.

**Decisões:**
- **Workspace único (`default`)** por enquanto, com a pasta do projeto definida na primeira execução (`-dir`). O modelo já comporta vários; o seletor de workspaces fica para depois.
- **Terminais morrem com o servidor.** Mantê-los vivos entre reinícios (pty-host ou tmux) fica para depois, como prevê a `05`.

## 2026-10-15, B1 núcleo de terminal

- **`internal/screen`:**
  - interface `Screen` sobre o `vt`, com filtro de OSC (título, cwd, progresso);
  - hash normalizado para detectar atividade;
  - modos rastreados (2004, tela alternativa) e `Render()` para pintar um cliente que conecta;
  - as respostas do emulador são drenadas internamente, para o `Write` nunca travar.
- **Golden tests** com as gravações reais (vim, htop, Codex, Claude): a tela bate com a referência do xterm.js. A única tolerância é para linhas com emoji + seletor de variação.
- **`internal/pty`:**
  - no Unix, `creack/pty` (nova sessão + terminal de controle);
  - no Windows, ConPTY do `xpty` dentro de um Job Object com `KILL_ON_JOB_CLOSE`;
  - parada da árvore: SIGHUP+SIGTERM no grupo e SIGKILL depois de 2 s;
  - código de saída 128+sinal.
- **`internal/terminal`:**
  - shell de login por plataforma;
  - linha de lançamento digitada (type-ahead);
  - ambiente sem os marcadores de sessão de agentes (`CLAUDECODE`, `CLAUDE_CODE_SESSION_ID`…);
  - fila de escrita serializada;
  - `Paste` com as temporizações da spec;
  - assinantes com render inicial + stream, sem lacuna nem duplicação;
  - `Manager` com criar, parar, reiniciar e remover.
- **Agente falso** (`internal/testutil/fakeagent`) para os testes.
- `go test -race` limpo.

**Decisões:**
- **PTY no Unix:** `creack/pty` em vez do `xpty`. O `xpty` não define `Setsid`/`Setctty` nem fecha o lado escravo no pai; sem isso, o shell fica sem controle de jobs e a leitura não termina quando o processo sai.
- **Enter de reforço só quando o destino fica calado:** na colagem, ele só é enviado se nenhuma saída chegou depois do primeiro Enter. Isso evita duplo envio em shells e em agentes que reagiram, e mantém a rede de segurança das TUIs que engolem o primeiro Enter. É uma melhoria sobre o Maestri, que manda sempre.
- **Segundo patch no `third_party/vt`:** `closed` virou `atomic.Bool`, porque o detector de corrida apontou o problema. Registrado em `third_party/README.md` e `spikes/go/patches/vt-v0.1.0-squad.patch`.

**Limitação:** no Unix, a parada atinge o grupo de processos. Um descendente que crie a própria sessão (daemon) escapa. Se for um problema, varrer a tabela de processos como o Maestri faz.

## 2026-10-15, B0 esqueleto

- Repositório privado `nathanribeiroo/squad` criado, com tudo o que existia na `main` (docs, design, spikes, research). A pasta `sandbox/`, resto dos testes do Maestri, ficou fora do git.
- Módulo `github.com/nathanribeiroo/squad`, com Go 1.26.
- `third_party/vt`: cópia do `charmbracelet/x/vt` v0.1.0 com o patch do buffer de 64 KB, via `replace`.
- `cmd/squad` com o despacho de subcomandos, `internal/config` (`~/.squad`, porta 7420, arquivo de descoberta `run/server.json` com escrita atômica) e `internal/version`.
- `make check`: vet (incluindo Windows), testes e build para os dois sistemas.

**Decisão:** a porta padrão é 7420 e pode ser trocada por `SQUAD_PORT`. O diretório de dados é `~/.squad` e pode ser trocado por `SQUAD_HOME`.
