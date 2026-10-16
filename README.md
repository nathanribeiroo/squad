# Squad

Ferramenta local para orquestrar agentes de código (Claude Code, Codex, Devin, Gemini) e shells num canvas infinito. Os terminais conversam entre si pelas conexões que você desenha. O servidor é um binário Go que escuta em `127.0.0.1`, e a interface React abre numa janela própria, como um app.

> Estado: backend e interface web da primeira rodada prontos (B0–B5, W1–W4): terminais vivos no canvas, conexões, conversa entre agentes animada nos fios, Composer com @menções, notas em Markdown, a gaveta de atividade, vários workspaces, o portal com a página ao vivo, que os agentes também usam (`squad portal`), papéis com instruções que chegam a cada agente, o agente gerente, MCP, os andares (git worktree), com seletor no topo, e os resumos de turno. Com eles, a segunda rodada está completa (veja o plano). Falta a validação completa no Windows. Plano e andamento: [`docs/PLAN.md`](docs/PLAN.md) e [`docs/DEVLOG.md`](docs/DEVLOG.md).

## Requisitos

- Go 1.26+ (para compilar).
- Node 22+ (só para compilar a interface web).
- Python com `fonttools` e `brotli` (só para regenerar a fonte de ícones, `make icons`).
- macOS e Linux testados. No Windows, o básico foi testado numa máquina real (terminais no Git Bash, `squad list`, hooks do Claude Code).

## Instalação

Com Go 1.26+:

```sh
GONOPROXY=github.com/nathanribeiroo/squad go install github.com/nathanribeiroo/squad/cmd/squad@main
squad version   # mostra o commit instalado
```

- **Por que o `GONOPROXY`:** sem ele, o `@main` passa pelo proxy do Go, que guarda a resposta em cache e pode entregar a versão anterior por um bom tempo depois de um merge. Com ele, o Squad vem direto do GitHub (precisa do `git`, que o Git Bash já traz), e o resto continua pelo proxy.
- **Um commit exato** também funciona: `go install github.com/nathanribeiroo/squad/cmd/squad@<commit>`.
- O `@latest` pega a última tag, que está bem atrás da `main`.

O binário vai para `$(go env GOPATH)/bin` (no Windows, `%USERPROFILE%\go\bin`) e já traz a interface web embutida, sem precisar de Node.

## Início rápido

```sh
make build                       # gera bin/squad
bin/squad install-skill          # ensina o Squad ao Claude Code e ao Devin que estiverem nesta máquina
bin/squad serve -dir ~/meu-projeto
```

**`squad install-skill`:** rode de novo a cada atualização do Squad, para a skill acompanhar.
- Sem `-agent`, instala para cada agente encontrado na máquina (a CLI no PATH ou a pasta de configuração): o Claude Code em `~/.claude/skills/squad` e o Devin em `~/.config/devin/skills/squad` (no Windows, `%APPDATA%\devin\skills\squad`). `-agent claude`, `-agent devin` ou `-agent all` escolhem.
- **No Devin, também registra o servidor MCP `squad`** em `~/.config/devin/mcp_config.json` (no Windows, `%APPDATA%\devin\mcp_config.json`), mantendo os seus outros servidores. O Devin só lê servidores MCP desse arquivo. Fora de um terminal do Squad, esse servidor não oferece ferramentas. O arquivo anterior fica guardado como `mcp_config.json.squad-backup`.

A mesma skill vem empacotada em [`squad-skill.zip`](squad-skill.zip) (pasta `squad/` com o `SKILL.md`), gerada por `make skill-zip`. Ela só faz sentido onde o comando `squad` existe, ou seja, dentro dos terminais do Squad.

O `serve` mostra uma URL com token e abre o Squad numa janela. Os dados ficam em `~/.squad` (troque com `SQUAD_HOME`) e a porta padrão é 7420 (`-port` ou `SQUAD_PORT`).

### Janela de app

O Squad abre numa janela própria, sem abas e sem barra de endereço, com o ícone dele na barra de tarefas e o nome do workspace no título.
- **Como:** é o modo app do Chrome, do Edge ou do Brave (o primeiro encontrado; o Edge vem em todo Windows), com um perfil separado em `~/.squad/window`. Ele não carrega as suas extensões e lembra o tamanho da janela.
- **Fechar a janela não para o Squad:** os agentes continuam trabalhando. Para reabrir, rode `squad serve` de novo (em qualquer pasta). Para encerrar, use Ctrl-C no terminal do `serve`.
- **Login:** o token não muda mais a cada reinício (fica em `~/.squad/session-token`), então a janela volta sozinha quando o servidor reinicia. Apagar esse arquivo desconecta todas as janelas e abas no próximo início. Se uma janela abrir sem login, ela pede a URL que o `serve` mostrou.
- **Outras formas:** `squad serve -open browser` abre uma aba no navegador padrão, como antes; `-open none` (ou `-no-open`) não abre nada. Sem Chrome, Edge nem Brave, abre a aba.
- **Mac com Safari:** o Safari não tem modo app por comando. No macOS 14 ou mais novo, abra o Squad no Safari e use **Arquivo → Adicionar ao Dock**, mantendo o nome **Squad**. Daí em diante, o `squad serve` abre esse app do Dock, que mostra o último workspace aberto (os outros ficam no seletor do topo). Com o Chrome ou o Edge instalado no Mac, a janela de app funciona sem esse passo.

**Workspaces:** cada pasta de projeto é um workspace, com o seu canvas, terminais e notas (`~/.squad/workspaces/<id>/`).
- `squad serve -dir <pasta>` abre o workspace da pasta (`.` se omitida) e o cria na primeira vez.
- Com o servidor já rodando, o mesmo comando abre ou cria o workspace naquele servidor, em vez de subir outro. Assim, `cd outro-projeto && squad serve` funciona a qualquer momento.
- Todos os workspaces ficam rodando: trocar de workspace na interface só troca o que a tela mostra, e os agentes dos outros continuam trabalhando.

### Shell dos terminais

Cada terminal abre um shell, e o agente é digitado dentro dele. A ordem de escolha é:

- **macOS/Linux:** `$SHELL` (login), `zsh`, `bash`, `sh`;
- **Windows:** **Git Bash** (achado ao lado do `git.exe` ou em `Program Files\Git`; nunca o `bash.exe` do WSL), `pwsh`, `powershell`, `cmd`.

A variável `SQUAD_SHELL` passa à frente dessa lista (ex.: `SQUAD_SHELL=pwsh.exe` ou um caminho completo). Se um shell existir mas não abrir, por exemplo o PowerShell bloqueado por política de grupo, o terminal tenta o próximo da lista.

### Interface

A interface já vem embutida no binário. Para mexer nela:

```sh
make web                                      # npm ci + build → internal/webui/dist (commitado)
cd web && SQUAD_TOKEN=<token> npm run dev     # Vite com proxy para o servidor em 7420
```

**Ícones:** a fonte Material Symbols é cortada para os ícones listados em `web/src/lib/icons.ts` (cerca de 9 KB em vez de 4 MB). Para usar um ícone novo, acrescente o nome nessa lista e rode `make icons`. O `npm run build` recusa uma fonte desatualizada.

**Atalhos:**
- canvas: `V` selecionar, `H` mover, `T` novo terminal, `N` nota, `P` portal;
- globais: `⌥A` gaveta de atividade, `⌥P` pausar conversas, `⌥K` composer.

**Conexões:** arraste a partir da bolinha lateral de um nó e solte sobre outro nó. Um clique no fio o seleciona; para desligar, use Delete ou o botão "Desligar". Duplo clique no fio cria um ponto de curva, que pode ser arrastado; duplo clique no ponto o remove.

**Terminais:**
- no formulário de criação (`T`), **Pasta** aceita um caminho absoluto, `~/…` ou relativo à pasta do workspace, com sugestões enquanto você digita (↑/↓, Enter, Tab completa). **Shell** lista os shells instalados; "Padrão" segue a ordem descrita em "Shell dos terminais";
- **Configurar…**, no menu `⋯` do terminal, edita esses campos; mudar o que o inicia (agente, comando, pasta ou shell) salva e reinicia o terminal;
- clique para digitar ("teclado aqui"); `⌥Esc` devolve o teclado ao canvas;
- arraste pelo cabeçalho e redimensione pelas alças (o PTY acompanha);
- com zoom abaixo de 60% vira miniatura, e abaixo de 35%, cartão;
- a pinça ou ⌘/ctrl + roda sobre o terminal dá zoom no canvas.

**Composer (`⌥K`):**
- escreva para um ou mais terminais;
- `@` menciona nós ligados ao destino;
- cole imagens com ⌘V;
- `⏎` envia.

**Workspaces:**
- o botão no topo, à esquerda, abre o seletor: cor, nome, pasta, agentes ativos e pedidos pendentes de cada um. Um ponto azul no botão indica atividade em outro workspace;
- `⌥1`–`⌥9` trocam de workspace, e a URL guarda o workspace aberto (`?ws=<id>`), então dá para abrir dois em abas diferentes;
- "Novo workspace" pede a pasta (com sugestões), um nome opcional e a cor. O lápis de cada linha edita esses campos; mudar a pasta vale para terminais novos ou reiniciados;
- "Remover workspace" encerra os terminais dele e move o canvas e as notas para `~/.squad/trash/`. O último workspace não pode ser removido;
- `squad ask`, `list` e `check` só enxergam terminais do mesmo workspace.

**Portal (`P`):** mostra um site ao vivo, como uma aba de navegador dentro do canvas.
- A página é aberta por um Chrome, Edge ou Chromium sem janela, controlado pelo Squad (`SQUAD_BROWSER` aponta outro executável). Só abre quando um portal precisa. Os logins e cookies ficam em `~/.squad/browser`, compartilhados entre os workspaces.
- Clique na página para usá-la: o teclado vai para o site ("teclado aqui") até `⌥Esc`, e a roda do mouse rola a página (pinça ou ⌘/ctrl + roda dão zoom no canvas). Colar digita o texto.
- Barra de endereço, voltar, avançar e recarregar. Seguir links atualiza o endereço salvo no canvas.
- O tamanho da página acompanha o nó (redimensione pelas alças). Com o zoom abaixo de 60%, o portal mostra o último quadro e para de transmitir.
- Cada portal tem um nome ("Portal", "Portal 2"…). Duplo clique no nome (ou ⋯ → Renomear) troca; os agentes usam esse nome para escolher o portal.
- **Agentes no portal:** um terminal ligado ao portal usa a página com `squad portal` (tabela abaixo). Enquanto ele age, o cursor dele aparece na página com o nome e a cor do papel, e o fio mostra "clicando", "preenchendo", "digitando"…

**Papéis:** instruções que o agente recebe ao abrir (⚙ → Papéis…, ou "Editar papéis…" no menu do terminal).
- Vêm cinco prontos: Planejador, Arquiteto, Implementador, Revisor e Testador. As instruções podem ser editadas, e "Restaurar padrão" volta às do Squad. O Testador, por exemplo, testa interfaces web pelo portal.
- **Arquiteto** (`arch`): desenha a solução técnica e não escreve o código do produto.
  - Lê o código e a documentação, e registra o desenho numa nota "Arquitetura: …": problema, solução com os módulos e arquivos que mudam, contratos (tipos, rotas, formato dos dados), alternativas descartadas, riscos e as partes que podem andar em paralelo.
  - O Planejador o chama quando a mudança mexe na estrutura (módulo novo, API, formato de dados, dependência) ou vai ser dividida em frentes; numa tarefa pequena, dispensa.
  - O Planejador liga a nota a quem implementa e revisa, e usa as partes para decidir os andares. O Implementador segue a nota, e o Revisor confere se a mudança segue o desenho. Antes de aterrissar, o Arquiteto pode fazer uma revisão de arquitetura.
  - Também dá para usá-lo sem o Planejador: ligue-o a um Implementador e peça o desenho.
- **`squad roles`** (ou a ferramenta MCP `roles`, liberada): qualquer agente vê os papéis disponíveis, com o id, se é gerente e o que faz, inclusive os que você criar. O `squad new --help` mostra a mesma lista.
- Se você já tinha criado um papel com o nome de um que passou a vir pronto (por exemplo, "Arquiteto"), o seu continua, com as mesmas instruções, como "Arquiteto (personalizado)".
- Dá para criar papéis com nome, cor, ícone e instruções. Eles valem para todos os workspaces (`~/.squad/roles.json`). Um papel em uso não pode ser removido.
- Como cada agente recebe o papel: Claude Code por `--append-system-prompt-file`; Codex por `developer_instructions`; Devin pelo hook `SessionStart`; Gemini como primeira mensagem (`-i`). Shells não têm papel, e comandos próprios mostram o papel só como etiqueta.
- Trocar o papel de um agente aberto reinicia o terminal, com aviso. Depois de editar um papel, o painel oferece reiniciar os agentes que o usam.

**Agente gerente:** um agente em modo gerente monta a própria equipe.
- O Planejador já vem em modo gerente. A opção fica no papel ("Gerente", em Papéis…), e cada terminal pode ligá-la ou desligá-la no menu ⋯ → "Modo gerente". O chip "gerente" aparece no cabeçalho.
- `squad new "Bia" --role impl` cria um terminal ao lado do gerente, já ligado a ele e com o papel. O comando só volta quando o agente está pronto para receber a primeira mensagem.
- O que um agente criou mostra "criado por Plano". Esses terminais não são gerentes, e cada gerente cria no máximo 6 (`squad serve -max-team N`).
- Qualquer agente pode abrir um portal: `squad portal create localhost:5173 "App"` cria o portal ligado a quem pediu (até 3 por agente).
- Pelo MCP (abaixo), o gerente trabalha sem pedir aprovação a cada passo, no Claude Code e no Devin. Pela CLI, cada comando `squad` pede aprovação como qualquer outro comando.

**Andares:** cópias isoladas do repositório do workspace, para frentes de trabalho em paralelo que mexem nos mesmos arquivos.
- Cada andar é um `git worktree` num branch próprio, com um canvas próprio. A pasta fica **fora do projeto**, como no Maestri: `<pasta-pai do repositório>/.squad/floors/<projeto>--<andar>`.
- **Na interface:** o seletor no topo tem o térreo (o branch atual) e uma aba por andar, cada uma com "limpo", "● 3 alterações", "2 commits", "preparando" ou "preparo falhou". Um ponto azul marca agentes trabalhando ali. O `+` cria um andar. Clicar no andar aberto abre o menu: aterrissar, log do preparo, copiar o caminho e descartar. O andar aberto fica na URL (`&floor=`), e cada andar guarda o próprio zoom.
- Terminal, nota e portal criados na tela vão para o andar aberto. O rodapé de cada terminal mostra o branch e a pasta de verdade.
- Um fio para um nó de outro andar aparece como etiqueta embaixo do nó (`⇄ Plano · térreo`). Ela pulsa enquanto um pedido passa por ele, e um clique leva ao outro andar, centrado no nó.
- O andar parte do último commit (alterações não commitadas ficam no térreo). Recebe os arquivos versionados, mais os não versionados da lista de cópia do workspace (padrão `.env*`), e roda o comando de preparo, se houver (ex.: `npm install`). A lista e o comando ficam em Novo andar → Preparação (ou `PUT /w/<id>/api/floors/config`). O preparo roda no shell padrão (Git Bash no Windows), e a saída fica no "Log do preparo".
- Os terminais do andar abrem na cópia; pastas relativas, e mesmo uma pasta do térreo dada por inteiro, apontam para a cópia. Fios podem ligar terminais de andares diferentes: o gerente no térreo conversa com quem está no andar.
- **Aterrissar** fecha os terminais e portais do andar e apaga a pasta; as notas descem para o térreo. O branch fica. Com `--merge`, antes mescla o branch no branch de origem (`--into outro` para outro), com commit de merge. Se o branch de destino não estiver aberto em nenhuma pasta, a mescla acontece sem tocar em pasta nenhuma. É recusado se o andar tiver alteração não commitada, se a pasta de destino tiver alteração em arquivo versionado, ou se o branch do andar tiver commitado um arquivo copiado do térreo (como o `.env`, que pode ter segredos). Com conflito, nada muda, e a resposta lista os arquivos.
- O diálogo de aterrissar mostra os commits do andar, as alterações que ainda impedem e onde mesclar, e diz o que fecha. Num conflito, ele lista os arquivos.
- **Descartar** (só pelo usuário, no menu do andar) apaga tudo: terminais, pasta (mesmo com trabalho não commitado) e o branch, se foi o andar que o criou.
- **O Planejador decide sozinho:** quando divide a implementação em duas ou mais frentes em paralelo no mesmo repositório, cria um andar por frente. O Revisor e o Testador de cada frente vão para o andar dela. Com uma frente só, ele trabalha no térreo. No fim, aterrissa uma frente por vez com merge, e você aprova cada uma. Quem editou o papel Planejador antes continua com a própria versão; "Restaurar padrão" traz a regra.
- Gerentes criam e aterrissam andares (só os que eles criaram); qualquer agente lista. Pelo MCP, `floors` (listar, criar) vem liberada, e `floor_land` pede aprovação.
- Um workspace com andares não pode ser removido nem mudar de pasta antes de aterrissar ou descartar os andares.

**Resumos de turno:** quando um agente (Claude, Codex, Devin, Gemini) termina um turno, aparece um cartão embaixo do terminal.
- **O cartão:** mostra "Turno · 2m10s" e a mensagem final do agente, que o hook dele já entrega, então não custa nada. O Gemini não tem hook, e o cartão usa o fim da tela. Clicar no texto abre o turno inteiro: o pedido que o iniciou e a mensagem final. O cartão some quando o próximo turno começa, ou com o ×, e o turno também entra na Atividade.
- **"Resumir"**, no cartão, pede a um agente um resumo de verdade, com até 3 próximos passos. Clicar num passo abre o Composer para aquele terminal, com o texto pronto.
- **"Resumo"**, no topo da Atividade, dá uma visão geral do workspace: o que cada agente fez e o que pede a sua atenção.
- **Quem escreve os resumos** (⚙ → Resumos…, por workspace):
  - **Automático:** cada terminal é resumido pela CLI do próprio agente: `claude -p` (com o modelo Haiku), `devin -p`, `codex exec` ou `gemini -p`. Sem ela, o Squad usa a primeira instalada. Assim o texto de um terminal Devin fica com o Devin.
  - Dá para fixar uma das CLIs ou desligar.
  - Sem nenhuma CLI no PATH, o botão "Resumir" não aparece, e os cartões continuam funcionando.
- **Custo:** cada resumo gasta um pouco do plano da ferramenta que o escreve, e só roda quando você clica.
- **Isolamento:** o resumo roda numa pasta vazia, sem ferramentas e sem mexer no projeto. No Claude, também sem as suas configurações, os seus hooks e sem salvar sessão no histórico.
- **Duração:** os cartões ficam em memória e somem quando o `squad serve` reinicia.

**Notas:** Prévia/Editar no cabeçalho; as tarefas podem ser marcadas com clique; o cadeado trava a nota para todos. Quando um agente edita a nota, o trecho alterado fica destacado.

**Nome das notas:**
- **Nota nova** (tecla N ou barra de ferramentas): já abre com o campo de nome selecionado. Enter confirma; Esc fica com o nome provisório ("Nota", "Nota 2"…). O arquivo nasce com o nome que você der ("Plano de testes" → `plano-de-testes.md`) e não muda mais.
- **Renomear** as suas notas: duplo clique no título, ou "Renomear" no menu ⋯. O arquivo fica, e **o nome antigo continua valendo para os agentes**: um agente que pedir a nota pelo nome antigo a encontra, e a resposta diz o nome novo.
- **Notas criadas por agentes** (`squad note create "Nome"`, ou a ferramenta `note` do MCP) ficam com o nome que o agente deu: aparecem com "por Bia" e sem "Renomear".
- **Nomes únicos no workspace**, sem diferenciar maiúsculas, contando os nomes antigos. Os agentes acham a nota pelo nome em qualquer caixa.

### Testar a conversa sem a interface (pela API)

Em outro terminal, com o servidor rodando:

```sh
TOKEN=$(python3 -c "import json,os;print(json.load(open(os.path.expanduser('~/.squad/run/server.json')))['token'])")
API="curl -s -H Authorization:Bearer\ $TOKEN -H Content-Type:application/json"
# a API de cada workspace fica em /w/<id>/; aqui, o primeiro da lista
WS=$($API http://127.0.0.1:7420/api/workspaces | python3 -c 'import json,sys;print(json.load(sys.stdin)[0]["id"])')

# dois terminais: um shell e um Claude Code
SHELL_ID=$($API -d '{"kind":"terminal","terminal":{"name":"Shell","agent":"shell"}}' http://127.0.0.1:7420/w/$WS/api/nodes | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
CLAUDE_ID=$($API -d '{"kind":"terminal","terminal":{"name":"Claude","agent":"claude"}}' http://127.0.0.1:7420/w/$WS/api/nodes | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')

# ligar os dois
$API -d "{\"a\":\"$SHELL_ID\",\"b\":\"$CLAUDE_ID\"}" http://127.0.0.1:7420/w/$WS/api/edges
```

Para digitar no terminal "Shell", conecte um cliente WebSocket em `/w/$WS/ws/term/<id>`. Até a interface ficar pronta, o jeito mais simples é usar o próprio Claude: peça a ele, no terminal dele, que rode `squad list` e `squad ask "Shell" --raw "ls\n"`.

## Como a conversa funciona

Dentro de um terminal do Squad, o comando `squad` está no PATH:

| Comando | O que faz |
|---|---|
| `squad list` | quem você é e o que está ligado a você |
| `squad ask "Nome" "mensagem"` | entrega a mensagem (com o envelope `[de: Você · pedido 7f3a]`) e espera a resposta |
| `squad ask "Nome" --raw "teclas"` | digita teclas (`\n` Enter, `\x03` Ctrl-C…) e devolve a tela |
| `squad ask --batch '{"A":"…","B":"…"}'` | vários pedidos em paralelo |
| `squad check "Nome"` | lê a tela de um terminal ligado |
| `squad wait "Nome"` | espera o turno do agente acabar e devolve a resposta final, sem mandar nada (para depois de um `ask` que voltou por tempo esgotado; se ele já terminou, devolve a última resposta) |
| `squad reply <id> "…"` | responde o pedido `<id>` |
| `squad note list\|read\|write\|edit\|create` | notas ligadas a você |
| `squad portal snapshot` | lê a página de um portal ligado: título, URL e os elementos com referências `@e1`, `@e2`… |
| `squad portal click @e3` · `fill @e2 "texto"` · `type "texto"` · `key Enter` · `scroll down` | age na página (`click X Y` também vale) |
| `squad portal goto URL` · `back` · `forward` · `reload` | navega |
| `squad portal screenshot` · `console` · `eval "js"` | imagem PNG (salva no workspace), console do navegador, JavaScript |
| `squad portal create URL "Nome"` · `close "Nome"` | abre um portal ligado a você / fecha um que você criou |
| `squad roles` | os papéis disponíveis (id, nome, o que fazem) |
| `squad new "Nome" --role impl` | gerentes: cria um terminal ao lado, ligado e já pronto (`--agent`, `--cwd`, `--command`) |
| `squad connect "A" "B"` · `squad close "Nome"` | gerentes: liga dois nós / remove um terminal que criou |
| `squad floor list` | os andares: branch, estado, pasta |
| `squad floor create "Nome"` | gerentes: cria um andar a partir do último commit (`--branch B`, `--existing` para abrir um branch que já existe) |
| `squad new "Bia" --role impl --floor "Nome"` | gerentes: cria o terminal no andar, trabalhando na cópia (`--floor ground` é o térreo) |
| `squad floor land "Nome" [--merge \| --into BRANCH]` | gerentes: aterrissa um andar que criou, mesclando o branch antes se pedido |

Com mais de um portal ligado, diga qual: `squad portal "Docs" snapshot`. `squad portal help` mostra tudo.

**MCP (`squad mcp`):** os mesmos comandos como ferramentas MCP, com argumentos tipados. O Squad configura isso sozinho em cada terminal:
- **Claude Code:** `--mcp-config`.
- **Codex:** `mcp_servers`, com o tempo de espera em 30 min.
- **Devin:** pelo `mcp_config.json` dele, onde o `squad install-skill` registra o servidor (o Devin não aceita servidores MCP por sessão).
- **Gemini:** não tem como receber por sessão; use a CLI.

| Ferramentas | Aprovação |
|---|---|
| `list`, `ask`, `ask_batch`, `check`, `wait`, `reply`, `note`, `portal`, `new`, `connect`, `close`, `floors`, `roles` | já liberadas no Claude Code e no Devin. O servidor recusa nelas tudo o que rodaria um comando: mensagem para um shell ou comando próprio, terminal com linha de comando, JavaScript na página. |
| `keys` (digitar num terminal), `new_shell` (terminal com comando, ex.: `npm run dev`), `portal_eval`, `floor_land` (mescla branches e fecha terminais) | o agente pede a sua aprovação a cada uso |

Com o Claude Code, o Codex e o Devin CLI, a resposta é a **mensagem final do turno** (via hook `Stop` ou `notify`), não a tela. Com outros programas, o Squad espera a tela parar de mudar.

**Tempo de espera:**
- o `ask` espera enquanto o agente trabalha (até 30 min) e volta depois de 2 min sem atividade nenhuma, com a tela e o aviso `[squad: timeout …]`. Isso acontece, por exemplo, quando o agente fica parado num pedido de permissão;
- se o turno dele ainda não acabou, o aviso manda usar `squad wait "Nome"`: ele espera esse mesmo turno acabar e devolve a resposta final, sem mandar outra mensagem;
- se, durante o `wait`, o agente esperado mandar uma pergunta a quem espera, ela chega como resultado do `wait` (`[de: … · pedido …]`). Quem esperava responde com `squad reply` e espera de novo;
- esperar por quem está esperando por você é recusado, porque nenhum dos dois terminaria;
- se o agente limita o tempo de uma ferramenta MCP (no Claude Code, a variável `MCP_TOOL_TIMEOUT`), as ferramentas `ask` e `wait` voltam um pouco antes do limite. Elas trazem a tela do agente esperado e dizem para chamar `wait` de novo ou, se a tela mostra uma pergunta ou um pedido de permissão, avisar o usuário.

O Devin é lançado com `devin --config ~/.squad/run/devin-config.json`. Esse `--config` **substitui** o `config.json` do usuário, então o arquivo é uma cópia do seu `~/.config/devin/config.json` (relida a cada terminal Devin aberto) com os hooks do Squad (`Stop`, `UserPromptSubmit` e `SessionStart`) depois dos seus e as ferramentas MCP seguras liberadas. O seu modelo, as suas permissões e os seus hooks continuam valendo. Permissões que você salvar como globais de dentro do Squad ficam nessa cópia. Num andar, o Devin abre com `--respect-workspace-trust false`, sem a pergunta de confiança na pasta: o andar é uma cópia do repositório que você abriu. A leitura dessa configuração foi testada com o Devin real; os hooks numa sessão com conta ainda não (veja `docs/DEVLOG.md`).

## Testes

```sh
make check                                                   # vet (incl. Windows), testes, builds
go test -tags e2e -run TestRealClaude -v ./internal/app/     # usa o Claude Code real (1 prompt mínimo)
```

## Estrutura

| Pasta | Conteúdo |
|---|---|
| `cmd/squad` | o binário único: servidor (`serve`), a CLI usada pelos agentes, `hook`, `install-skill` |
| `internal/` | `terminal`, `pty`, `screen` (emulador), `activity` (monitor), `agentbus` (conversa), `agents` (hooks), `notes`, `floors` (andares), `store`, `server`, `app` |
| `web/` | interface React (React Flow, xterm.js); o build vai para `internal/webui/dist` |
| `tools/shot` | captura de tela via Chrome headless (módulo Go próprio), usada para comparar a UI com o design |
| `third_party/vt` | emulador de terminal (charmbracelet/x/vt, MIT) com patches, como pacote do próprio módulo |
| `docs/` | investigação do Maestri, propostas de arquitetura, spec da conversa, design (`design_handoff_squad/`), plano e diário |
| `spikes/` | protótipos e gravações reais de TUIs (base dos testes do emulador) |
| `research/` | ferramentas usadas na investigação |
