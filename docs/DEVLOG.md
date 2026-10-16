# Diário de desenvolvimento

O que foi feito em cada etapa, as decisões tomadas sem consulta e o que precisa de você. Entradas mais novas no topo.

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
