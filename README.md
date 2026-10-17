# Torre

A torre de controle dos seus agentes de código. Antes se chamava **Squad** (veja "Squad agora é Torre", abaixo).

Ferramenta local para orquestrar agentes de código (Claude Code, Codex, Devin, Gemini) e shells num canvas infinito. Os terminais conversam entre si pelas conexões que você desenha. O servidor é um binário Go que escuta em `127.0.0.1`, e a interface React abre numa janela própria, como um app.

> Estado: voos (terminais de agente) vivos no canvas, frequências com a conversa entre os agentes animada nos fios, Transmitir com @menções, NOTAMs em Markdown, o Rádio, vários workspaces, portais com a página ao vivo que os agentes também usam (`torre portal`), papéis com instruções e siglas, o voo líder, MCP, níveis de voo (git worktree), boletins e briefing, e sessões (cada uma um canvas inteiro, que pode trabalhar numa cópia isolada do repositório). O Squad virou Torre, com a interface redesenhada em fases (F0–F5). Falta a validação completa no Windows. Plano e andamento: [`docs/PLAN.md`](docs/PLAN.md) e [`docs/DEVLOG.md`](docs/DEVLOG.md).

## Requisitos

- Go 1.26+ (para compilar).
- Node 22+ (só para compilar a interface web).
- Python com `fonttools` e `brotli` (só para regenerar a fonte de ícones, `make icons`).
- macOS e Linux testados. No Windows, o básico foi testado numa máquina real (terminais no Git Bash, `torre list`, hooks do Claude Code).

## Instalação

Com Go 1.26+:

```sh
GONOPROXY=github.com/nathanribeiroo/squad go install github.com/nathanribeiroo/squad/cmd/...@main
torre version   # mostra o commit instalado
```

O `cmd/...` instala dois comandos: `torre` e `squad`, o mesmo programa com o nome antigo (veja abaixo).

- **Por que o `GONOPROXY`:** sem ele, o `@main` passa pelo proxy do Go, que guarda a resposta em cache e pode entregar a versão anterior por um bom tempo depois de um merge. Com ele, a Torre vem direto do GitHub (precisa do `git`, que o Git Bash já traz), e o resto continua pelo proxy.
- **Um commit exato** também funciona: `go install github.com/nathanribeiroo/squad/cmd/...@<commit>`.
- **O repositório e o módulo Go** continuam com o nome `squad`, para o caminho do `go install` não mudar.
- O `@latest` pega a última tag, que está bem atrás da `main`.

O binário vai para `$(go env GOPATH)/bin` (no Windows, `%USERPROFILE%\go\bin`) e já traz a interface web embutida, sem precisar de Node.

## Início rápido

```sh
make build                       # gera bin/torre
bin/torre install-skill          # ensina a Torre ao Claude Code e ao Devin que estiverem nesta máquina
bin/torre serve -dir ~/meu-projeto
```

**`torre install-skill`:** rode de novo a cada atualização da Torre, para a skill acompanhar.
- Sem `-agent`, instala para cada agente encontrado na máquina (a CLI no PATH ou a pasta de configuração): o Claude Code em `~/.claude/skills/torre` e o Devin em `~/.config/devin/skills/torre` (no Windows, `%APPDATA%\devin\skills\torre`). `-agent claude`, `-agent devin` ou `-agent all` escolhem.
- **No Devin, também registra o servidor MCP `torre`** em `~/.config/devin/mcp_config.json` (no Windows, `%APPDATA%\devin\mcp_config.json`), mantendo os seus outros servidores. O Devin só lê servidores MCP desse arquivo. Fora de um terminal da Torre, esse servidor não oferece ferramentas. O arquivo anterior fica guardado como `mcp_config.json.torre-backup`. O servidor `squad` registrado antes da troca de nome é substituído pelo `torre`.

A mesma skill vem empacotada em [`torre-skill.zip`](torre-skill.zip) (pasta `torre/` com o `SKILL.md`), gerada por `make skill-zip`. Ela só faz sentido onde o comando `torre` existe, ou seja, dentro dos terminais da Torre.

O `serve` mostra uma URL com token e abre a Torre numa janela. Os dados ficam em `~/.torre` (troque com `TORRE_HOME`) e a porta padrão é 7420 (`-port` ou `TORRE_PORT`).

### Squad agora é Torre

O nome mudou, e o que você já tinha continua funcionando:

- **Comando:** o novo é `torre`. O `squad` continua instalado, como o mesmo programa: avisa que o nome mudou e faz o mesmo. Scripts, configurações de agente e conversas antigas que chamam `squad ask` seguem funcionando.
- **Dados:** na primeira vez que o `torre serve` sobe, ele move `~/.squad` para `~/.torre` (workspaces, sessões, papéis, login, perfis das janelas). Se um `squad serve` antigo ainda estiver rodando, a pasta fica onde está e é usada assim mesmo: pare o antigo e suba de novo. No Windows, se algum arquivo estiver aberto (por exemplo, a janela do app de antes), a troca também espera o próximo início.
- **Variáveis:** `TORRE_HOME`, `TORRE_PORT`, `TORRE_SHELL`, `TORRE_BROWSER` e `TORRE_DEBUG`. Os nomes antigos (`SQUAD_…`) ainda valem, com um aviso no log do `serve`. Os terminais recebem as duas formas (`TORRE_URL` e `SQUAD_URL`, por exemplo).
- **Agentes:** o servidor MCP agora se chama `torre` (ferramentas `mcp__torre__…`). Rode `torre install-skill` uma vez: ele instala a skill `torre`, remove a `squad` e troca o registro do MCP no Devin. Se você liberou ferramentas `mcp__squad__…` nas suas configurações do Claude Code, libere as novas.
- **Andares e cópias de sessão** novos ficam em `<pasta-pai>/.torre/…`; os que já existem continuam em `.squad/`.
- **Login:** a janela e as abas continuam logadas (o cookie antigo vale e é trocado pelo novo).

### Vocabulário da interface

A interface usa a metáfora da torre de controle (a lista completa está em [`docs/torre/RENOMEACAO.md`](docs/torre/RENOMEACAO.md)). Os comandos dos agentes continuam em inglês (`torre floor land`, por exemplo).

| Antes | Agora |
|---|---|
| terminal (de agente ou shell) | voo |
| fio, ligar | frequência, pôr na mesma frequência |
| andar · térreo | nível de voo (FL1, FL2…) · pista |
| aterrissar, mesclar a sessão | pousar, pedir pouso |
| pausar conversas · retomar | espera geral · liberar todos |
| Atividade · Composer | Rádio · Transmitir |
| nota · cartão de turno · resumo do workspace | NOTAM · boletim · briefing |
| gerente · interrompido · servidor | líder · arremetida · enlace |
| Iniciando, Ocioso, Trabalhando, Esperando resposta, Terminou, Parado, Descarregado | Taxiando, Pátio, No ar, Aguarda, Em solo, Pane, Hangar |

### Janela de app

A Torre abre numa janela própria, sem abas e sem barra de endereço, com o ícone dela na barra de tarefas e o nome do workspace no título.
- **Como:** é o modo app do Chrome, do Edge ou do Brave (o primeiro encontrado; o Edge vem em todo Windows), com um perfil separado em `~/.torre/window`. Ele não carrega as suas extensões e lembra o tamanho da janela.
- **Fechar a janela não para a Torre:** os agentes continuam trabalhando. Para reabrir, rode `torre serve` de novo (em qualquer pasta). Para encerrar, use Ctrl-C no terminal do `serve`.
- **Login:** o token não muda mais a cada reinício (fica em `~/.torre/session-token`), então a janela volta sozinha quando o servidor reinicia. Apagar esse arquivo desconecta todas as janelas e abas no próximo início. Se uma janela abrir sem login, ela pede a URL que o `serve` mostrou.
- **Outras formas:** `torre serve -open browser` abre uma aba no navegador padrão, como antes; `-open none` (ou `-no-open`) não abre nada. Sem Chrome, Edge nem Brave, abre a aba.
- **Mac com Safari:** o Safari não tem modo app por comando. No macOS 14 ou mais novo, abra a Torre no Safari e use **Arquivo → Adicionar ao Dock**, mantendo o nome **Torre**. Daí em diante, o `torre serve` abre esse app do Dock (um app que você criou com o nome **Squad**, antes da troca, continua valendo), que mostra o último workspace aberto (os outros ficam no seletor do topo). Com o Chrome ou o Edge instalado no Mac, a janela de app funciona sem esse passo.

**Workspaces:** cada pasta de projeto é um workspace, com o seu canvas, terminais e notas (`~/.torre/workspaces/<id>/`).
- `torre serve -dir <pasta>` abre o workspace da pasta (`.` se omitida) e o cria na primeira vez.
- Com o servidor já rodando, o mesmo comando abre ou cria o workspace naquele servidor, em vez de subir outro. Assim, `cd outro-projeto && torre serve` funciona a qualquer momento.
- Todos os workspaces ficam rodando: trocar de workspace na interface só troca o que a tela mostra, e os agentes dos outros continuam trabalhando.

### Shell dos terminais

Cada terminal abre um shell, e o agente é digitado dentro dele. A ordem de escolha é:

- **macOS/Linux:** `$SHELL` (login), `zsh`, `bash`, `sh`;
- **Windows:** **Git Bash** (achado ao lado do `git.exe` ou em `Program Files\Git`; nunca o `bash.exe` do WSL), `pwsh`, `powershell`, `cmd`.

A variável `TORRE_SHELL` passa à frente dessa lista (ex.: `TORRE_SHELL=pwsh.exe` ou um caminho completo). Se um shell existir mas não abrir, por exemplo o PowerShell bloqueado por política de grupo, o terminal tenta o próximo da lista.

### Interface

A interface já vem embutida no binário. Para mexer nela:

```sh
make web                                      # npm ci + build → internal/webui/dist (commitado)
cd web && TORRE_TOKEN=<token> npm run dev     # Vite com proxy para o servidor em 7420
```

**Ícones:** a fonte Material Symbols é cortada para os ícones listados em `web/src/lib/icons.ts` (cerca de 9 KB em vez de 4 MB). Para usar um ícone novo, acrescente o nome nessa lista e rode `make icons`. O `npm run build` recusa uma fonte desatualizada.

**Atalhos:**
- canvas: `V` selecionar, `H` mover, `T` novo voo, `N` NOTAM, `P` portal;
- globais: `⌥A` Rádio, `⌥P` espera geral, `⌥K` Transmitir, `⌥1`–`⌥9` workspaces.

**Confirmações:** remover um voo, uma NOTAM, uma sessão, um workspace ou um papel, começar uma conversa nova, trocar o papel de um voo, reiniciar quem usa um papel, descartar um nível e trocar de sessão com voos no ar abrem um diálogo da Torre:
- o título é a pergunta ("Remover o voo PLN?"), a lista diz o que se perde (✕) e o que fica (✓), e o botão é o verbo ("Remover voo", "Trocar mesmo assim");
- a borda diz a gravidade: vermelha quando perde trabalho, âmbar quando interrompe, clara quando só reinicia;
- quando há alterações sem commit a perder, o botão só funciona depois de marcar "Entendo que esse trabalho se perde.";
- Esc cancela; Enter confirma, menos nas vermelhas.

**Espaço vazio:** uma sessão sem voos na pista mostra "Ponha a primeira equipe no ar.", com **Novo voo** (`T`) e **Escrever o plano numa NOTAM** (`N`). "Usar um plano de voo" aparece desligado até os planos de voo chegarem.

**Sem enlace:** se o `torre serve` parar, a barra mostra "Reconectando…"; depois de alguns segundos, um cartão diz "O torre serve parou? Tentando de novo em 3 s…". Os voos ficam no último estado conhecido e voltam sozinhos quando o servidor sobe.

**Erros em português:** as mensagens de erro da interface (pasta não encontrada, nome repetido, sigla em uso, nível com alterações…) chegam em português. A API manda `code` e `params` junto com o `error` em inglês, que é o que os agentes e a CLI continuam lendo.

**Frequências:** arraste a partir da bolinha lateral de um nó e solte sobre outro nó: os dois ficam na mesma frequência. Um clique no fio o seleciona; para tirar da frequência, use Delete ou o botão "Desligar". Duplo clique no fio cria um ponto de curva, que pode ser arrastado; duplo clique no ponto o remove. Enquanto um pedido passa, o fio corre tracejado. Na espera geral (`⌥P`), o botão do topo vira "Liberar todos" em âmbar e cada frequência ganha um quadrado com ∞.

**Indicativos e níveis:**
- cada voo tem um **indicativo** de até 4 letras, único no canvas da sessão. Vem da **sigla** do papel (PLN, IMP; o segundo Implementador é IM2) ou, sem papel, das consoantes do nome (Claude → CLD). Dá para escolher outro em Novo voo ou em Configurar…;
- o indicativo aparece no cabeçalho do voo, no Rádio, no Transmitir, no briefing, no "criado por" e na barra de quem aguarda resposta. Os agentes continuam se chamando pelo nome (`torre ask "Bia"`);
- trocar o papel de um voo troca o indicativo junto, se ele veio da sigla; um indicativo escolhido por você fica;
- cada papel tem uma **sigla** de até 3 letras (Papéis → SIGLA). Os prontos são PLN, ARQ, IMP, REV e TST; um papel novo sem sigla ganha as consoantes do nome;
- os **níveis de voo** são numerados por sessão: FL1, FL2… O número não volta a ser usado depois que o nível pousa ou é descartado. A pista é o branch da sessão.

**Voos:**
- **Novo voo** (`T`): o agente em blocos (Claude Code, Codex, Devin, Gemini, Shell, Comando), com "✓ instalado" ou "✕ não achado" para cada CLI; **Nome** e **Indicativo**; **Papel** (com "editar papéis…"); **Pasta** (opcional). Em **Avançado**: o comando, o shell e "Líder". **Decolar** cria o voo no nível aberto;
- **Pasta** aceita um caminho absoluto, `~/…` ou relativo à pasta do workspace, com sugestões enquanto você digita (↑/↓, Enter, Tab completa). **Shell** lista os shells instalados; "Padrão" segue a ordem descrita em "Shell dos terminais";
- **Configurar…**, no menu `⋯` do voo, edita esses campos; mudar o que o inicia (agente, comando, pasta ou shell) mostra o aviso e o botão vira **Salvar e reiniciar**;
- **o voo na tela:** a faixa à esquerda tem a cor do estado (No ar, Aguarda, Em solo…); o quadradinho antes do indicativo tem a cor do papel. O rodapé mostra o nível, o branch, a pasta e quem criou o voo. Parado, o voo mostra "Hangar" ou "Pane" com o botão **Iniciar**;
- clique para digitar (chip "teclado"); `⌥Esc` devolve o teclado ao canvas;
- arraste pelo cabeçalho e redimensione pelas alças (o PTY acompanha);
- com zoom abaixo de 60%, o voo vira um retrato da tela, e abaixo de 35%, uma **faixa de progresso** em papel, com o indicativo grande e o estado;
- a pinça ou ⌘/ctrl + roda sobre o voo dá zoom no canvas.

**Conversas que continuam:**
- cada voo de agente lembra a conversa em que está. Ao reabrir o `torre serve`, ou em **Reiniciar**, o agente volta para a mesma conversa:

| Agente | Como a Torre sabe a conversa | Como retoma |
|---|---|---|
| Claude | escolhe o id ao abrir (`--session-id`) | `claude --resume <id>` |
| Devin | o id que os hooks dele informam | `devin --resume <id>` |
| Codex | o `thread-id` do aviso de fim de turno | `codex resume <id>` |
| Gemini | escolhe o id ao abrir, nas versões com `--session-id` | `gemini --resume <id>` |

- se o agente não acha a conversa (apagada, ou de outra pasta), a própria linha de comando começa uma nova (`retomar || começar`), e a tela avisa "Não deu para retomar a conversa";
- uma conversa em que ninguém escreveu ainda não tem o que retomar: o agente que ainda não recebeu nenhuma mensagem começa uma conversa nova, sem aviso;
- **Nova conversa…**, no menu `⋯`, reinicia o agente sem o contexto (a conversa antiga continua guardada no agente). Trocar o papel também começa uma conversa nova, para o papel valer desde o início;
- **Copiar comando de retomada**, no mesmo menu, copia `claude --resume <id>` (ou o do agente): rodado na pasta do voo, abre a mesma conversa fora da Torre;
- **arremetida:** se a Torre fechar com um agente no meio de um turno, ao reabrir aparece a faixa âmbar "Arremetida · CLD foi interrompido no meio do turno". **Continuar** manda "continue de onde parou" a cada um; **Dispensar** esquece. O que o agente estava escrevendo naquele instante se perde, e ele refaz esse passo. O voo também mostra "arremetida" no cabeçalho. Funciona no Claude e no Devin, que avisam o início e o fim de cada turno;
- trocar a pasta ou o agente de um voo começa uma conversa nova, porque os agentes guardam as conversas por pasta.

**Transmitir (`⌥K`):**
- fechado, é a barra embaixo do canvas: "TRANSMITIR →" com o indicativo do voo selecionado;
- aberto: os destinos em chips (indicativo e agente; "+ voo" acrescenta outro), o texto e "⏎ transmitir";
- `@` menciona nós na frequência do destino;
- cole imagens com ⌘V.

**Rádio (`⌥A`):** os pedidos entre os voos, em indicativos ("14:32 PLN → IMP · c8d0").
- Filtros: Todos, Pendentes e um chip por voo. Os pedidos ainda abertos ficam no topo, com **Ver no espaço**, que leva ao voo.
- Cada linha termina no desfecho: Pendente, Respondido em…, Entregue, Cancelado, Retido (espera geral ou limite por minuto), NOTAM editado ou Em solo (fim de turno).
- O rodapé mostra quantas mensagens passaram no último minuto, o limite e o máximo de saltos.

**Workspaces:**
- o botão no topo, à esquerda, abre o seletor: a cor, o nome com o atalho (`⌥1`…), a pasta e o que está acontecendo ("2 no ar · 1 pendente"). Um ponto no botão indica atividade em outro workspace;
- `⌥1`–`⌥9` trocam de workspace, e a URL guarda o workspace aberto (`?ws=<id>`), então dá para abrir dois em abas diferentes;
- "Novo workspace" pede a pasta (com sugestões), um nome opcional e a cor. O lápis de cada linha edita esses campos; mudar a pasta vale para voos novos ou reiniciados;
- "Remover workspace" encerra os voos dele e move o canvas e as NOTAMs para `~/.torre/trash/`. O último workspace não pode ser removido;
- `torre ask`, `list` e `check` só enxergam voos do mesmo workspace.

**Portal (`P`):** mostra um site ao vivo, como uma aba de navegador dentro do canvas.
- A página é aberta por um Chrome, Edge ou Chromium sem janela, controlado pela Torre (`TORRE_BROWSER` aponta outro executável). Só abre quando um portal precisa. Os logins e cookies ficam em `~/.torre/browser`, compartilhados entre os workspaces.
- Clique na página para usá-la: o teclado vai para o site (chip "teclado") até `⌥Esc`, e a roda do mouse rola a página (pinça ou ⌘/ctrl + roda dão zoom no canvas). Colar digita o texto.
- Barra de endereço, voltar, avançar e recarregar. Seguir links atualiza o endereço salvo no canvas.
- O tamanho da página acompanha o nó (redimensione pelas alças). Com o zoom abaixo de 60%, o portal mostra o último quadro e para de transmitir ("pausado · zoom").
- Cada portal tem um nome ("Portal", "Portal 2"…). Duplo clique no nome (ou ⋯ → Renomear) troca; os agentes usam esse nome para escolher o portal.
- **Agentes no portal:** um voo na frequência do portal usa a página com `torre portal` (tabela abaixo). Enquanto ele age, o cursor dele aparece na página com o nome e a cor do papel, e o fio mostra "clicando", "preenchendo", "digitando"…

**Papéis:** instruções que o agente recebe ao decolar (⚙ → Papéis…, ou "Editar papéis…" no menu do voo).
- O painel lista os papéis com o ícone, o nome, a sigla e "editado". O editor tem **Nome**, **Sigla**, **Cor**, **Ícone**, **Líder** e as **Instruções** (com o contador). O rodapé mostra quem usa o papel neste workspace.
- Vêm cinco prontos: Planejador (PLN), Arquiteto (ARQ), Implementador (IMP), Revisor (REV) e Testador (TST). As instruções podem ser editadas, e "Restaurar padrão" volta às da Torre. O Testador, por exemplo, testa interfaces web pelo portal.
- **Arquiteto** (`arch`): desenha a solução técnica e não escreve o código do produto.
  - Lê o código e a documentação, e registra o desenho numa NOTAM "Arquitetura: …": problema, solução com os módulos e arquivos que mudam, contratos (tipos, rotas, formato dos dados), alternativas descartadas, riscos e as partes que podem andar em paralelo.
  - O Planejador o chama quando a mudança mexe na estrutura (módulo novo, API, formato de dados, dependência) ou vai ser dividida em frentes; numa tarefa pequena, dispensa.
  - O Planejador põe a NOTAM na frequência de quem implementa e revisa, e usa as partes para decidir os níveis de voo. O Implementador segue a NOTAM, e o Revisor confere se a mudança segue o desenho. Antes de pousar, o Arquiteto pode fazer uma revisão de arquitetura.
  - Também dá para usá-lo sem o Planejador: ponha-o na frequência de um Implementador e peça o desenho.
- **`torre roles`** (ou a ferramenta MCP `roles`, liberada): qualquer agente vê os papéis disponíveis, com o id, se é líder e o que faz, inclusive os que você criar. O `torre new --help` mostra a mesma lista.
- Se você já tinha criado um papel com o nome de um que passou a vir pronto (por exemplo, "Arquiteto"), o seu continua, com as mesmas instruções, como "Arquiteto (personalizado)".
- Dá para criar papéis com nome, sigla, cor, ícone e instruções. Eles valem para todos os workspaces (`~/.torre/roles.json`). Um papel em uso não pode ser removido. Duas siglas iguais são recusadas.
- Como cada agente recebe o papel: Claude Code por `--append-system-prompt-file`; Codex por `developer_instructions`; Devin pelo hook `SessionStart`; Gemini como primeira mensagem (`-i`). Shells não têm papel, e comandos próprios mostram o papel só como etiqueta.
- Trocar o papel de um agente aberto reinicia o voo, com aviso. Depois de salvar um papel, a faixa verde "Salvo. PLN recebe as instruções novas ao reiniciar" oferece **Reiniciar agora**.

**Líder:** um agente líder monta a própria equipe.
- O Planejador já vem como líder. A opção fica no papel ("Líder", em Papéis…), e cada voo pode ligá-la ou desligá-la no menu ⋯ → "Líder" (ou em Novo voo → Avançado). O chip "líder" aparece no cabeçalho.
- `torre new "Bia" --role impl` cria um voo ao lado do líder, já na frequência dele e com o papel. O comando só volta quando o agente está pronto para receber a primeira mensagem.
- O que um agente criou mostra "criado por PLN". Esses voos não são líderes, e cada líder cria no máximo 6 (`torre serve -max-team N`).
- Qualquer agente pode abrir um portal: `torre portal create localhost:5173 "App"` cria o portal na frequência de quem pediu (até 3 por agente).
- Pelo MCP (abaixo), o líder trabalha sem pedir aprovação a cada passo, no Claude Code e no Devin. Pela CLI, cada comando `torre` pede aprovação como qualquer outro comando.

**Níveis de voo:** cópias isoladas do repositório do workspace, para frentes de trabalho em paralelo que mexem nos mesmos arquivos.
- Cada nível é um `git worktree` num branch próprio, com um canvas próprio. A pasta fica **fora do projeto**, como no Maestri: `<pasta-pai do repositório>/.torre/floors/<projeto>--<nível>`.
- **Na interface:** a barra do topo tem a PISTA (o branch atual) e uma aba por nível (FL1, FL2…), cada uma com o nome, o branch e "limpo", "3 alt.", "2 commits", "preparando" ou "preparo falhou". Um ▲ marca voos no ar ali. O `+` cria um nível. Clicar no nível aberto abre o menu: pedir pouso, log do preparo, copiar o caminho e descartar. O nível aberto fica na URL (`&floor=`), e cada nível guarda o próprio zoom.
- **Novo nível:** o número (FL3) aparece ao lado do nome e do branch. Um nível sem voos mostra o cartão "Nível vazio", com o comando `torre new … --floor` e o botão "Novo voo neste nível".
- Voo, NOTAM e portal criados na tela vão para o nível aberto. O rodapé de cada voo mostra o nível, o branch e a pasta de verdade.
- Um fio para um nó de outro nível aparece como etiqueta embaixo do nó (`⇄ PLN · pista`). Ela pulsa enquanto um pedido passa por ele, e um clique leva ao outro nível, centrado no nó.
- O nível parte do último commit (alterações não commitadas ficam na pista). Recebe os arquivos versionados, mais os não versionados da lista de cópia do workspace (padrão `.env*`), e roda o comando de preparo, se houver (ex.: `npm install`). A lista e o comando ficam em Novo nível → Preparação (ou `PUT /w/<id>/api/floors/config`). O preparo roda no shell padrão (Git Bash no Windows), e a saída fica no "Log do preparo".
- Os voos do nível abrem na cópia; pastas relativas, e mesmo uma pasta da pista dada por inteiro, apontam para a cópia. Um voo pode estar na frequência de voos de outros níveis: o líder na pista conversa com quem está no nível.
- **Pousar** fecha os voos e portais do nível e apaga a pasta; as NOTAMs descem para a pista. O branch fica. Com `--merge`, antes mescla o branch no branch de origem (`--into outro` para outro), com commit de merge. Se o branch de destino não estiver aberto em nenhuma pasta, a mescla acontece sem tocar em pasta nenhuma. É recusado se o nível tiver alteração não commitada, se a pasta de destino tiver alteração em arquivo versionado, ou se o branch do nível tiver commitado um arquivo copiado da pista (como o `.env`, que pode ter segredos). Com conflito, nada muda, e a resposta lista os arquivos.
- **Autorizar pouso · FL2:** o diálogo mostra os commits do nível, as checagens (tudo commitado, voos no ar, conflitos checados ao pousar), onde mesclar e o que fecha. Num conflito, ele lista os arquivos.
- **Descartar** (só pelo usuário, no menu do nível) apaga tudo: voos, pasta (mesmo com trabalho não commitado) e o branch, se foi o nível que o criou.
- **O Planejador decide sozinho:** quando divide a implementação em duas ou mais frentes em paralelo no mesmo repositório, cria um nível por frente. O Revisor e o Testador de cada frente vão para o nível dela. Com uma frente só, ele trabalha na pista. No fim, pousa uma frente por vez com merge, e você aprova cada uma. Quem editou o papel Planejador antes continua com a própria versão; "Restaurar padrão" traz a regra.
- Líderes criam e pousam níveis (só os que eles criaram); qualquer agente lista. Pelo MCP, `floors` (listar, criar) vem liberada, e `floor_land` pede aprovação.
- Um workspace com níveis não pode ser removido nem mudar de pasta antes de pousar ou descartar os níveis.

**Boletins e briefing:** quando um agente (Claude, Codex, Devin, Gemini) termina um turno, aparece um boletim embaixo do voo.
- **O boletim:** mostra "BOLETIM · 2m10s" e a mensagem final do agente, que o hook dele já entrega, então não custa nada. O Gemini não tem hook, e o boletim usa o fim da tela. Clicar no texto abre o turno inteiro: o pedido que o iniciou e a mensagem final. O boletim some quando o próximo turno começa, ou com o ×, e o turno também entra no Rádio.
- **"Resumir"**, no boletim, pede a um agente um resumo de verdade, com até 3 próximos passos ("→ IMP"). Clicar num passo abre o Transmitir para aquele voo, com o texto pronto.
- **Briefing**, no topo do Rádio, dá uma visão geral do workspace: um parágrafo, o que **pede atenção** (em âmbar) e uma linha **por voo**, com o indicativo, que leva ao voo.
- **Quem escreve os resumos** (⚙ → Resumos…, por workspace):
  - **Automático:** cada voo é resumido pela CLI do próprio agente: `claude -p` (com o modelo Haiku), `devin -p`, `codex exec` ou `gemini -p`. Sem ela, a Torre usa a primeira instalada. Assim o texto de um voo Devin fica com o Devin.
  - Dá para fixar uma das CLIs ou desligar.
  - Sem nenhuma CLI no PATH, o botão "Resumir" não aparece, e os boletins continuam funcionando.
- **Custo:** cada resumo gasta um pouco do plano da ferramenta que o escreve, e só roda quando você clica.
- **Isolamento:** o resumo roda numa pasta vazia, sem ferramentas e sem mexer no projeto. No Claude, também sem as suas configurações, os seus hooks e sem salvar sessão no histórico.
- **Duração:**
  - cada voo guarda o boletim do último turno, com o resumo, se você pediu um;
  - os boletins ficam no disco, com a sessão (`cards.json`, na pasta dela), e voltam quando o `torre serve` reinicia ou quando você reabre a sessão;
  - um resumo que estava sendo escrito quando a Torre fechou não continua: clique em "Resumir" de novo;
  - o × esconde o boletim só nesta janela: ao recarregar a página, ele volta.

**NOTAMs:** as notas do canvas, em papel. Prévia/Editar no cabeçalho; as tarefas podem ser marcadas com clique; o cadeado trava a NOTAM para todos. Quando um agente edita a NOTAM, o trecho alterado fica destacado. No modo Editar, ela grava sozinha 0,8 s depois que você para de digitar, e Esc volta à prévia.

**Nome das NOTAMs:**
- **NOTAM nova** (tecla N ou barra de ferramentas): já abre com o campo de nome selecionado. Enter confirma; Esc fica com o nome provisório ("Nota", "Nota 2"…). O arquivo nasce com o nome que você der ("Plano de testes" → `plano-de-testes.md`) e não muda mais.
- **Renomear** as suas NOTAMs: duplo clique no título, ou "Renomear" no menu ⋯. O arquivo fica, e **o nome antigo continua valendo para os agentes**: um agente que pedir a NOTAM pelo nome antigo a encontra, e a resposta diz o nome novo.
- **NOTAMs criadas por agentes** (`torre note create "Nome"`, ou a ferramenta `note` do MCP) ficam com o nome que o agente deu: aparecem com "por IMP" e sem "Renomear".
- **Nomes únicos no workspace**, sem diferenciar maiúsculas, contando os nomes antigos. Os agentes acham a NOTAM pelo nome em qualquer caixa.

### Sessões

Cada workspace tem sessões, e cada sessão é um canvas inteiro: voos, NOTAMs, portais, frequências, posições, níveis de voo e boletins. Só a sessão aberta tem processos rodando.
- **Nova sessão:** começa com o canvas vazio, ou com os mesmos terminais (com fios e papéis, mas sem as conversas).
- **Abrir uma sessão:** para os processos da sessão atual e volta tudo da outra como estava. Cada agente retoma a sua conversa, e quem foi interrompido no meio de um turno ganha o "Continuar".
- **Nome:**
  - você dá ao criar a sessão (ou ao criar o workspace, para a primeira);
  - sem nome, o fim do primeiro turno do orquestrador (o terminal gerente, ou o primeiro agente) dá um título, escrito pelo mesmo tipo de agente (`claude -p`, `devin -p`…);
  - se não houver agente disponível, o título vem do começo do primeiro pedido.
- **Arquivos:** na pasta do projeto, os arquivos são os mesmos para todas as sessões, e voltar a uma sessão não desfaz o que outra mudou. Para separar, a sessão pode trabalhar numa cópia isolada (abaixo).
- **No disco:** cada sessão fica em `~/.torre/workspaces/<id>/sessions/<sessão>/`, com o canvas, as notas, os anexos e os cartões. Um workspace de antes das sessões vira a "Sessão 1" na primeira abertura.

**Na tela:**
- **No topo:** depois do workspace aparece a sessão aberta (ex.: "torre › Login novo ▾"). Clicando nela, abre o painel de sessões.
- **Painel de sessões:**
  - cada sessão mostra o nome, quando foi criada e aberta pela última vez, quantos voos, NOTAMs, portais e níveis tem, e os agentes dela; a aberta tem a marca "ATUAL";
  - **Abrir**, **Renomear** (o lápis; Enter salva, Esc desiste) e **Remover** (a lixeira; não aparece na sessão aberta e fica desligada numa sessão com níveis);
  - **Nova sessão:** nome opcional e a caixa "Começar com os mesmos voos, sem as conversas". **Criar e abrir** guarda a sessão atual e abre a nova.
- **Voos no ar:** abrir outra sessão (ou criar uma) com agentes no meio de um turno mostra quem está no ar, com **Cancelar** ou **Trocar mesmo assim**. Trocando, o trabalho para no meio, e o voo volta em arremetida, com "Continuar", quando você reabrir a sessão.
- **Seletor de workspaces:** passe o mouse num workspace (ou clique na seta com o número de sessões) para ver as sessões dele ao lado; clicar numa delas abre a sessão e mostra aquele workspace.
- **Novo workspace:** o formulário tem o campo "Nome da sessão (opcional)", o nome da primeira sessão.
- **Uma sessão sem nome** aparece em itálico ("Nova sessão · 17/10 14:32") até o voo líder dar o título.
- **O enquadramento do canvas** (posição e zoom) fica guardado por sessão.
- **Indicativos e números de nível** são da sessão: outra sessão pode ter o seu próprio IMP e o seu próprio FL1.

**Sessão numa cópia isolada:** para começar uma sessão sem mexer no que outra deixou pela metade na pasta do projeto.
- **Onde a sessão trabalha**, em Nova sessão:
  - **Na pasta do projeto**, como as outras sessões;
  - **Numa cópia isolada**: um branch novo (o nome vem do nome da sessão, e dá para mudar), criado **a partir de** um branch que você escolhe (o padrão é o branch atual).
- **A opção certa já vem marcada:** se a sessão atual tem agentes trabalhando ou arquivos alterados sem commit, aparece o aviso ("Carrinho tem 1 arquivo alterado sem commit") e a cópia vem marcada. Sem nada pendente, vem marcada a pasta do projeto.
- **A cópia** é feita como um nível de voo (`git worktree`), do último commit do branch escolhido: o que está sem commit fica na pasta do projeto. Ela recebe os arquivos da lista de cópia (`.env*`) e roda o comando de preparo, se houver. Fica ao lado do repositório, em `<pasta-pai>/.torre/sessions/<projeto>--<branch>`.
- **Na sessão:** os voos (Claude, Devin, Codex, Gemini, shells) abrem na cópia; o Devin não pergunta se confia na pasta, e o Claude também não. A PISTA, na barra dos níveis, mostra o branch da sessão; o rodapé dos voos mostra a pasta da cópia. Os níveis que a sessão criar nascem da cópia.
- **Pedir pouso da sessão:** clicando na PISTA (o branch da sessão), ou em **Pedir pouso…** no painel de sessões. O diálogo mostra os commits da sessão e mescla o branch dela no branch escolhido (o padrão é o de origem), com commit de merge. **A cópia continua**: a sessão segue trabalhando e pode mesclar de novo. É recusado se a cópia tiver alteração sem commit, ou se a pasta onde o branch de destino está aberto (a do projeto, por exemplo) tiver alteração sem commit; com conflito, nada muda, e o diálogo lista os arquivos para um agente da sessão resolver.
- **Remover a sessão** apaga a cópia; o branch continua no repositório, com o que foi commitado. Se a cópia tiver alterações sem commit, o painel lista os arquivos e pede confirmação.
- **Sem git:** numa pasta que não é um repositório git, a opção não aparece.
- Um workspace com sessões em cópia não pode ser removido nem mudar de pasta antes de remover essas sessões.

**Pela API** (com `Authorization: Bearer <token>`):

| Rota | O que faz |
|---|---|
| `GET /api/workspaces/<id>/sessions` | as sessões, da mais nova para a mais antiga, com a aberta marcada, quantos terminais, notas, portais e níveis (`floors`) cada uma tem e, numa sessão em cópia, o `branch` e a pasta (`copy`) |
| `POST /api/workspaces/<id>/sessions` `{"name", "keepTerminals", "confirm", "copy": {"branch", "from"}}` | abre uma sessão nova; com `copy`, numa cópia isolada |
| `POST /api/workspaces/<id>/sessions/<sessão>/open` `{"confirm"}` | abre outra sessão |
| `PATCH /api/workspaces/<id>/sessions/<sessão>` `{"name"}` | renomeia |
| `DELETE /api/workspaces/<id>/sessions/<sessão>` | manda a pasta da sessão para a lixeira (a aberta, ou uma com níveis, não pode) e apaga a cópia dela; com alterações sem commit na cópia, 409 com `files`, e `?confirm=true` remove mesmo assim |
| `GET /w/<id>/api/copy/changes`, `POST /w/<id>/api/copy/merge` `{"into"}`, `GET /w/<id>/api/copy/log` | a cópia da sessão aberta: o que ela tem para mesclar, mesclar (a cópia continua) e o log do preparo |

Sem `confirm`, sair de uma sessão com agentes trabalhando ou pedidos esperando resposta é recusado (409, com a lista de quem trabalha). Com `confirm`, eles param no meio do turno e voltam como interrompidos quando a sessão abrir de novo. A tela se atualiza sozinha ao trocar de sessão, também quando a troca vem de outra janela ou da API.

### Testar a conversa sem a interface (pela API)

Em outro terminal, com o servidor rodando:

```sh
TOKEN=$(python3 -c "import json,os;print(json.load(open(os.path.expanduser('~/.torre/run/server.json')))['token'])")
API="curl -s -H Authorization:Bearer\ $TOKEN -H Content-Type:application/json"
# a API de cada workspace fica em /w/<id>/; aqui, o primeiro da lista
WS=$($API http://127.0.0.1:7420/api/workspaces | python3 -c 'import json,sys;print(json.load(sys.stdin)[0]["id"])')

# dois terminais: um shell e um Claude Code
SHELL_ID=$($API -d '{"kind":"terminal","terminal":{"name":"Shell","agent":"shell"}}' http://127.0.0.1:7420/w/$WS/api/nodes | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')
CLAUDE_ID=$($API -d '{"kind":"terminal","terminal":{"name":"Claude","agent":"claude"}}' http://127.0.0.1:7420/w/$WS/api/nodes | python3 -c 'import json,sys;print(json.load(sys.stdin)["id"])')

# ligar os dois
$API -d "{\"a\":\"$SHELL_ID\",\"b\":\"$CLAUDE_ID\"}" http://127.0.0.1:7420/w/$WS/api/edges
```

Para digitar no terminal "Shell", conecte um cliente WebSocket em `/w/$WS/ws/term/<id>`. Até a interface ficar pronta, o jeito mais simples é usar o próprio Claude: peça a ele, no terminal dele, que rode `torre list` e `torre ask "Shell" --raw "ls\n"`.

## Como a conversa funciona

Dentro de um terminal da Torre, o comando `torre` está no PATH:

| Comando | O que faz |
|---|---|
| `torre list` | quem você é e o que está ligado a você |
| `torre ask "Nome" "mensagem"` | entrega a mensagem (com o envelope `[de: Você · pedido 7f3a]`) e espera a resposta |
| `torre ask "Nome" --raw "teclas"` | digita teclas (`\n` Enter, `\x03` Ctrl-C…) e devolve a tela |
| `torre ask --batch '{"A":"…","B":"…"}'` | vários pedidos em paralelo |
| `torre check "Nome"` | lê a tela de um terminal ligado |
| `torre wait "Nome"` | espera o turno do agente acabar e devolve a resposta final, sem mandar nada (para depois de um `ask` que voltou por tempo esgotado; se ele já terminou, devolve a última resposta) |
| `torre reply <id> "…"` | responde o pedido `<id>` |
| `torre note list\|read\|write\|edit\|create` | notas ligadas a você |
| `torre portal snapshot` | lê a página de um portal ligado: título, URL e os elementos com referências `@e1`, `@e2`… |
| `torre portal click @e3` · `fill @e2 "texto"` · `type "texto"` · `key Enter` · `scroll down` | age na página (`click X Y` também vale) |
| `torre portal goto URL` · `back` · `forward` · `reload` | navega |
| `torre portal screenshot` · `console` · `eval "js"` | imagem PNG (salva no workspace), console do navegador, JavaScript |
| `torre portal create URL "Nome"` · `close "Nome"` | abre um portal ligado a você / fecha um que você criou |
| `torre roles` | os papéis disponíveis (id, nome, o que fazem) |
| `torre new "Nome" --role impl` | líderes: cria um voo ao lado, ligado e já pronto (`--agent`, `--cwd`, `--command`) |
| `torre connect "A" "B"` · `torre close "Nome"` | líderes: põe dois nós na mesma frequência / remove um voo que criou |
| `torre floor list` | os níveis de voo: número (FL1…), branch, estado, pasta |
| `torre floor create "Nome"` | líderes: cria um nível a partir do último commit (`--branch B`, `--existing` para abrir um branch que já existe) |
| `torre new "Bia" --role impl --floor "Nome"` | líderes: cria o voo no nível, trabalhando na cópia (`--floor ground` ou `--floor pista` é a pista) |
| `torre floor land "Nome" [--merge \| --into BRANCH]` | líderes: pousa um nível que criou, mesclando o branch antes se pedido |

Com mais de um portal ligado, diga qual: `torre portal "Docs" snapshot`. `torre portal help` mostra tudo.

**MCP (`torre mcp`):** os mesmos comandos como ferramentas MCP, com argumentos tipados. A Torre configura isso sozinho em cada terminal:
- **Claude Code:** `--mcp-config`.
- **Codex:** `mcp_servers`, com o tempo de espera em 30 min.
- **Devin:** pelo `mcp_config.json` dele, onde o `torre install-skill` registra o servidor (o Devin não aceita servidores MCP por sessão).
- **Gemini:** não tem como receber por sessão; use a CLI.

| Ferramentas | Aprovação |
|---|---|
| `list`, `ask`, `ask_batch`, `check`, `wait`, `reply`, `note`, `portal`, `new`, `connect`, `close`, `floors`, `roles` | já liberadas no Claude Code e no Devin. O servidor recusa nelas tudo o que rodaria um comando: mensagem para um shell ou comando próprio, terminal com linha de comando, JavaScript na página. |
| `keys` (digitar num terminal), `new_shell` (terminal com comando, ex.: `npm run dev`), `portal_eval`, `floor_land` (mescla branches e fecha terminais) | o agente pede a sua aprovação a cada uso |

Com o Claude Code, o Codex e o Devin CLI, a resposta é a **mensagem final do turno** (via hook `Stop` ou `notify`), não a tela. Com outros programas, a Torre espera a tela parar de mudar.

**Tempo de espera:**
- o `ask` espera enquanto o agente trabalha (até 30 min) e volta depois de 2 min sem atividade nenhuma, com a tela e o aviso `[torre: timeout …]`. Isso acontece, por exemplo, quando o agente fica parado num pedido de permissão;
- se o turno dele ainda não acabou, o aviso manda usar `torre wait "Nome"`: ele espera esse mesmo turno acabar e devolve a resposta final, sem mandar outra mensagem;
- se, durante o `wait`, o agente esperado mandar uma pergunta a quem espera, ela chega como resultado do `wait` (`[de: … · pedido …]`). Quem esperava responde com `torre reply` e espera de novo;
- esperar por quem está esperando por você é recusado, porque nenhum dos dois terminaria;
- se o agente limita o tempo de uma ferramenta MCP (no Claude Code, a variável `MCP_TOOL_TIMEOUT`), as ferramentas `ask` e `wait` voltam um pouco antes do limite. Elas trazem a tela do agente esperado e dizem para chamar `wait` de novo ou, se a tela mostra uma pergunta ou um pedido de permissão, avisar o usuário.

O Devin é lançado com `devin --config ~/.torre/run/devin-config.json`. Esse `--config` **substitui** o `config.json` do usuário, então o arquivo é uma cópia do seu `~/.config/devin/config.json` (relida a cada terminal Devin aberto) com os hooks da Torre (`Stop`, `UserPromptSubmit` e `SessionStart`) depois dos seus e as ferramentas MCP seguras liberadas. O seu modelo, as suas permissões e os seus hooks continuam valendo. Permissões que você salvar como globais de dentro da Torre ficam nessa cópia. Num nível de voo, o Devin abre com `--respect-workspace-trust false`, sem a pergunta de confiança na pasta: o nível é uma cópia do repositório que você abriu. A leitura dessa configuração foi testada com o Devin real; os hooks numa sessão com conta ainda não (veja `docs/DEVLOG.md`).

## Testes

```sh
make check                                                   # vet (incl. Windows), testes, builds
go test -tags e2e -run TestRealClaude -v ./internal/app/     # usa o Claude Code real (1 prompt mínimo)
```

## Estrutura

| Pasta | Conteúdo |
|---|---|
| `cmd/torre`, `internal/cli` | o binário único: servidor (`serve`), a CLI usada pelos agentes, `hook`, `install-skill` (`cmd/squad` é o mesmo, com o nome antigo) |
| `internal/` | `terminal`, `pty`, `screen` (emulador), `activity` (monitor), `agentbus` (conversa), `agents` (hooks), `notes`, `floors` (níveis de voo), `store`, `server`, `app` |
| `web/` | interface React (React Flow, xterm.js); o build vai para `internal/webui/dist` |
| `tools/shot` | captura de tela via Chrome headless (módulo Go próprio), usada para comparar a UI com o design |
| `third_party/vt` | emulador de terminal (charmbracelet/x/vt, MIT) com patches, como pacote do próprio módulo |
| `docs/` | investigação do Maestri, propostas de arquitetura, spec da conversa, design (`design_handoff_squad/` e o da Torre, `torre/`), plano e diário |
| `spikes/` | protótipos e gravações reais de TUIs (base dos testes do emulador) |
| `research/` | ferramentas usadas na investigação |
