# Torre

A torre de controle dos seus agentes de código. Antes se chamava **Squad** (veja "Squad agora é Torre", abaixo).

Ferramenta local para orquestrar agentes de código (Claude Code, Codex, Devin, Gemini) e shells num canvas infinito. Os terminais conversam entre si pelas conexões que você desenha. O servidor é um binário Go que escuta em `127.0.0.1`, e a interface React abre numa janela própria, como um app.

> Estado: voos (terminais de agente) vivos no canvas, frequências com a conversa entre os agentes animada nos fios, Transmitir com @menções, NOTAMs em Markdown, o Rádio, vários workspaces, portais com a página ao vivo que os agentes também usam (`torre portal`), papéis com instruções e siglas, o voo líder, MCP, níveis de voo (git worktree), boletins e briefing, e sessões (cada uma um canvas inteiro, que pode trabalhar numa cópia isolada do repositório). O Squad virou Torre, com a interface redesenhada, a PRIORIDADE (o voo pede você, com resposta pela Torre no Claude e no Devin, também nas perguntas e nos planos, e som), a marca SUA VEZ, o Sinóptico, foco e encaixe, planos de voo e a pilha de NOTAMs. Falta a validação completa no Windows. Plano e andamento: [`docs/PLAN.md`](docs/PLAN.md) e [`docs/DEVLOG.md`](docs/DEVLOG.md).

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
- canvas: `V` selecionar, `H` mover, `T` novo voo, `N` NOTAM, `P` portal, `D` desenho, `G` grupo, `Delete` apaga o fio, o desenho ou a moldura selecionados, `Tab`/`⇧Tab` o próximo/anterior nó (na ordem de leitura, trazendo o nó para a tela), `⏎` dá o teclado ao voo selecionado;
- globais: `⌥S` Sinóptico ↔ Espaço, `⌥A` Rádio, `⌥P` espera geral, `⌥K` Transmitir, `⌥⏎` o próximo voo que pede você, `⌥F` foco, `⌥⇧←`/`⌥⇧→` encaixar, `⌥M` planos de voo, `⌥⇧M` salvar como plano, `⌥1`–`⌥9` workspaces;
- com um voo que pede você selecionado: `1` autorizar, `2` sempre, `3` negar.

**Foco, encaixe e ocultar** (também no menu ⋯ do voo):
- **Foco (`⌥F`):** o voo selecionado (ou o que tem o teclado) ocupa a área inteira, abaixo da barra do topo; `⌥F` ou Esc voltam ao canvas no mesmo zoom;
- **Encaixar (`⌥⇧←`/`⌥⇧→`):** o voo fica numa borda, com um divisor, e o canvas continua ao lado. As mesmas teclas desencaixam; a seta no cabeçalho troca de borda. O encaixe fica guardado por workspace, neste navegador;
- no foco e na borda, a tela do agente ganha o tamanho do painel (ele redesenha para as colunas e linhas novas) e volta ao tamanho do nó quando sai, mesmo se a página fechar. O nó no canvas mostra "Encaixado à esquerda"/"Em foco", com "Trazer de volta";
- **Ocultar a tela:** troca a tela do voo por um padrão listrado opaco (para gravar ou mostrar a tela sem expor o que o agente faz), no canvas, no retrato, na borda, no foco e no Sinóptico. O cabeçalho e o estado continuam à vista. Fica guardado por workspace, neste navegador.

**Primeira abertura:** numa instalação nova (sem workspace guardado), a Torre mostra três passos sobre um radar:
- **1 Enlace:** a página de login, para quando a janela abre sem o token (cole a URL que o `torre serve` mostrou). Com o `?token=` certo, ela pula direto para o passo 2;
- **2 Pasta:** a pasta do projeto (a do `torre serve`, já preenchida), com o que o git diz dela: repositório, branch, alterações, se os níveis de voo funcionam (precisam de um commit) e os `.env*` que vão para os níveis. Trocar a pasta aqui troca a do workspace;
- **3 Agentes:** os agentes e o navegador achados, com o comando de instalação dos que faltam, e **Começar vazio** (os planos de voo ainda não chegaram).
- "Pular" fecha os passos; eles não aparecem de novo neste navegador.

| Rota | O que faz |
|---|---|
| `GET /api/dirs/info?path=` | `exists`, `git`, `branch`, `commits`, `changes`, `envFiles` e as pastas dos workspaces (`recent`) |
| `GET /api/welcome` | `firstRun`: o servidor subiu sem nenhum workspace guardado |

**Sinóptico (`⌥S`, ou ESPAÇO · SINÓPTICO no topo):** a sessão inteira de relance, para deixar aberta o dia todo.
- **Radar:** cada voo é um símbolo no seu nível (a pista no centro, um anel por nível), na posição que tem no canvas: ▲ no ar, ● pede você (com o anel pulsando), △ aguarda, □ em solo, ○ hangar. Linhas âmbar tracejadas são pedidos abertos; verdes, respostas recentes. Os contadores ficam em cima e a legenda embaixo. Clique seleciona; duplo clique abre o voo no Espaço.
- **Fichas:** uma faixa de papel por voo, na ordem pede você › aguarda › no ar › em solo › hangar, com o que ele está fazendo, o agente, o nível e o tempo no estado.
- **Fila de pouso:** os níveis com commits, com as checagens (testes, revisão, tudo commitado, ninguém no ar) e "Autorizar pouso…". Testes e revisão são marcas que os voos do nível deixam (veja "Testes e revisão", nos níveis de voo).
- **Frequências:** os pares de voos, com pedido aberto (âmbar) ou resposta recente (verde), mais a taxa e os saltos. Num laço, os pares da cadeia ficam em âmbar cheio, e o radar liga os voos da cadeia.
- **Laço:** o mesmo cartão "ESPERA GERAL · LAÇO" do Espaço, no alto da coluna da direita. Ali, um clique no indicativo mostra o terminal do voo, e o duplo clique abre o voo no Espaço.
- **Prioridade:** o voo que pede você há mais tempo ("1 de N"), com a pergunta, o comando e AUTORIZAR (1), SEMPRE (2) e NEGAR (3). Sem prioridade, o cartão some.
- **Terminal:** a tela do voo selecionado, ao vivo; clique para digitar; o alfinete fixa o voo, que deixa de seguir a seleção.
- **Rodapé:** o Rádio passando (clique abre) e a barra Transmitir.
- A vista escolhida fica guardada no navegador.

**Configurações** (⚙ no topo; fecha com Esc ou clique fora):
- **Aparência:** tema da interface (Escuro, Claro ou Sistema, que segue o computador) e dos terminais (Noite ou Papel);
- **Agentes:** cada agente com a versão que a Torre achou (ou "não achado", com o comando de instalação), o caminho e o **comando padrão**, editável: ele abre o agente nos voos sem comando próprio, em todos os workspaces (fica em `~/.torre/agents.json`; a seta volta ao nome do agente). Também mostra o navegador dos portais. "Procurar de novo" refaz a busca;
- **Limites** (deste workspace, em todas as sessões; valem na hora): saltos encadeados, pedidos por minuto, voos por líder e portais por voo. O botão "Limites" do Rádio abre aqui;
- **Resumos:** quem escreve os resumos e os boletins, como antes;
- **Papéis…** abre o painel de papéis;
- **Atalhos:** a lista;
- **Avisos**, quando um voo pede você, neste ou em outro workspace:
  - o número no título da janela;
  - uma notificação do sistema, quando a janela não está na frente (o navegador pede licença);
  - um som curto, ligado por padrão;
  - **Repetir enquanto esperar** (desligado por padrão): o som de novo a cada 2 minutos, até 3 vezes, enquanto o pedido continuar sem resposta.
  - **Quando um voo termina e espera você** (padrão: depois de 1 min; também Não, Na hora, 3 min e 10 min): um turno seu terminou e o voo espera. Com a janela da Torre atrás, avisa uma vez por turno, com a notificação do sistema (se ligada) e um toque só, diferente do da PRIORIDADE. Veja **SUA VEZ**, abaixo.
  - As escolhas valem por navegador.
  - **Quando o som começa a valer:**
    - **na janela de app** que o `torre serve` abre, desde o começo;
    - **numa aba comum do navegador**, depois do primeiro clique ou tecla na página (antes disso, o navegador não deixa tocar);
    - com a janela atrás de outras ou minimizada, ele continua tocando.

| Rota | O que faz |
|---|---|
| `GET /api/agents` (`?refresh=1` procura de novo) | os agentes (`found`, `version`, `path`, `defaultCommand`, `installHint`) e o navegador |
| `PUT /api/agents/<agente>` `{"defaultCommand"}` | troca o comando padrão (`""` volta ao nome) |
| `GET/PUT /w/<id>/api/limits` `{"maxHops", "perMinute", "maxTeam", "portalsPerAgent"}` | os limites do workspace (0 volta ao padrão) |
| `GET /api/plans` · `GET`/`DELETE /api/plans/<id>` · `POST /api/plans/import` (o arquivo) | os planos de voo, de todos os workspaces |
| `POST /w/<id>/api/plans` `{"name", "scope": "selection"\|"level"\|"session", "nodes", "floor", "notesContent"}` | salva um plano deste canvas |
| `POST /w/<id>/api/plans/<plano>/apply` `{"floor", "commands"}` | decola o plano; `commands` são os comandos próprios que você revisou (sem eles, 409 `plan.review`) |

**PRIORIDADE (o voo pede você):** um agente parado num pedido de permissão (rodar um comando, editar um arquivo, usar uma ferramenta MCP), numa pergunta com opções ou pedindo para aprovar um plano deixa de aparecer como "No ar":
- **no voo:** contorno âmbar com halo, a faixa de estado âmbar piscando, "PEDE VOCÊ 0:24" no lugar do estado e, em cima da tela, a frase do pedido com o que está em jogo ("QUER EXECUTAR UM COMANDO · npm run e2e");
- **RESPONDER**, embaixo do voo: **Autorizar** (1), **Sempre** (2), **Negar** (3) e **Abrir tela**. Com o voo selecionado, as teclas 1, 2 e 3 respondem.
  - **Sempre** só aparece quando a opção do agente é "não perguntar de novo" para aquilo que ele pediu. Nunca é uma opção que troque o modo de permissão, que valha para todos os projetos ou para todas as ferramentas de um servidor MCP.
  - Passando o mouse em cada botão, aparece o texto do próprio agente; no Rádio e no Sinóptico, o do Sempre fica escrito ("Sempre = Yes, always allow `npm test` commands in `app`").
  - As ferramentas da Torre que pedem a sua aprovação a cada uso (`keys`, `new_shell`, `portal_eval`, `floor_land`) nunca ganham Sempre.
  - **Negar** encerra o turno do voo: quem pediu algo a ele recebe a tela logo, sem esperar um minuto;
- **numa pergunta ou num plano**, RESPONDER mostra as opções do próprio agente, cada uma com o seu número (as teclas 1–9 com o voo selecionado) e a descrição ao passar o mouse:
  - **Outra resposta…** (ou **O que mudar no plano…**): um campo para escrever a sua resposta, quando a pergunta aceita texto ("Type something." no Claude, "Other (type your own)" no Devin; "Tell Claude what to change" no plano do Claude). Enter envia; a quebra de linha vira espaço;
  - **Ver o plano** mostra o plano inteiro antes de você escolher (o Claude e o Devin mandam o plano no hook);
  - ficam só na tela: as perguntas de múltipla escolha (as caixinhas), as opções que desligam todas as permissões ("bypass") e a pergunta de confiança na pasta;
  - uma pergunta depois da outra (o Claude faz várias numa chamada) aparece uma por vez, e depois "Submit answers";
- **fora da tela:** a faixa "PRIORIDADE · TST em FL1 quer executar um comando · IR ATÉ ELE ⌥⏎" no topo do canvas, um ponto âmbar piscando na aba do nível, o número no título da janela ("(1) squad · Torre") e, em outro workspace, o ponto do botão de workspaces fica âmbar e o seletor diz "1 pede você";
- **⌥⏎** vai para o voo que pede há mais tempo: abre o nível, centra o voo e dá a ele o teclado;
- **no Rádio:** o pedido fica preso no topo ("TST → VOCÊ · Pede você · 0:24"), com os mesmos botões e "Ver no espaço", e o filtro **Pedem você**. Depois de respondido, fica a linha "Pediu você · respondido em 0:46"; numa pergunta ou num plano, com o que você escolheu ("Fez uma pergunta: Qual formato? → ISO 8601");
- responder direto na tela do voo funciona igual: o estado sai quando o pedido some da tela;
- **um pedido de outro voo espera:** a Torre não cola uma mensagem num voo que está perguntando (o texto viraria a resposta do pedido); ela entra depois que você responde.

| Agente | Como a Torre sabe | Responder pela Torre |
|---|---|---|
| Claude Code | hook `PermissionRequest` (a ferramenta e o comando; a pergunta, `AskUserQuestion`; o plano, `ExitPlanMode`) e a tela | sim: a tecla da opção, conferida com o Claude Code 2.1.302, nas permissões, nas perguntas e no plano |
| Devin | hook `PermissionRequest` (comando, edição, busca na web, MCP; o plano, `exit_plan_mode`) e a tela; a pergunta com opções do Devin, só pela tela | sim: a tecla da opção, conferida com o Devin 3000.11.92, nas permissões, nas perguntas e no plano (o modo de plano liga com alt+P). **Sempre** num comando vale para aquele comando neste projeto (o Devin grava em `.devin/config.local.json`, fora do git); numa busca na web, para aquele endereço (grava na configuração que a Torre passa ao Devin) |
| Codex, Gemini | a pergunta na tela (opções numeradas com o cursor) | ainda não: só **Abrir tela** |
| Shell, comando próprio | não se aplica | — |

O hook só avisa: a Torre nunca decide uma permissão pelo agente. A resposta pela interface é uma rota só da interface (`POST /w/<id>/api/nodes/<id>/attention/respond`, com `{"choice": "allow" | "always" | "deny"}` ou, numa pergunta ou num plano, `{"option": "2"}` e `{"option": "3", "text": "…"}` no campo de texto), que agentes não alcançam nem pela CLI nem pelo MCP; ela só digita num agente, só uma resposta que a interface ofereceu e só quando a pergunta na tela ainda é a mesma que a interface mostrou.

**SUA VEZ:** quando termina um turno seu (você escreveu no terminal, usou Transmitir ou Continuar), o voo mostra "EM SOLO · SUA VEZ", e a ficha dele sobe no Sinóptico, logo depois de quem pede você. Não conta o turno que respondeu a outro voo (a resposta volta sozinha para quem pediu), o de um voo com pedido na fila e o que você mesmo encerrou com Negar. A marca some quando o voo volta a trabalhar; vale para o Claude, o Devin, o Codex e o Gemini (shells e comandos próprios não têm turno). O aviso, com a janela atrás, fica em Configurações → Avisos.

**Confirmações:** remover um voo, uma NOTAM, uma sessão, um workspace ou um papel, começar uma conversa nova, trocar o papel de um voo, reiniciar quem usa um papel, descartar um nível e trocar de sessão com voos no ar abrem um diálogo da Torre:
- o título é a pergunta ("Remover o voo PLN?"), a lista diz o que se perde (✕) e o que fica (✓), e o botão é o verbo ("Remover voo", "Trocar mesmo assim");
- a borda diz a gravidade: vermelha quando perde trabalho, âmbar quando interrompe, clara quando só reinicia;
- quando há alterações sem commit a perder, o botão só funciona depois de marcar "Entendo que esse trabalho se perde.";
- Esc cancela; Enter confirma, menos nas vermelhas.

**Espaço vazio:** uma sessão sem voos na pista mostra "Ponha a primeira equipe no ar.", com **Usar um plano de voo** (`⌥M`), **Novo voo** (`T`) e **Escrever o plano numa NOTAM** (`N`).

**Planos de voo:** uma equipe guardada para decolar de novo, em qualquer workspace, com conversas novas.
- **Salvar** (`⌥⇧M`, ou o cartão tracejado da galeria): a seleção no Espaço, tudo no nível aberto, ou a sessão inteira com os níveis. Guarda os voos (agente, nome, indicativo, papel, líder, comando próprio, pasta relativa, posição e tamanho), as NOTAMs (o nome e, se quiser, o conteúdo), os portais (nome e endereço), as frequências entre eles, os níveis (nome e branch), os grupos (abertos) e os desenhos. Um grupo selecionado leva o que está dentro dele. Fica em `~/.torre/plans`.
- **Galeria** (`⌥M`, o botão na barra de ferramentas, ou "Com um plano de voo" na primeira abertura): os planos com uma miniatura e a composição ("PLN · IMP · REV · 1 NOTAM · 3 frequências"); **Importar** um arquivo; em cada cartão, exportar o arquivo e remover.
- **Antes de decolar:** a lista do que cada voo vai rodar (o comando do agente, de Configurações → Agentes, ou o comando próprio do plano, em âmbar). Um plano importado com comandos próprios mostra o aviso. O servidor recusa decolar se a interface não mandar a lista de comandos que mostrou.
- **Decolar a equipe:** os itens vão para o nível aberto, ao lado do que já está lá; os níveis do plano viram níveis novos (com o nome dele, ou "Login 2" se já existir). Nomes que já existem ganham um número, e o indicativo também (IMP → IM2). O que não dá para criar (um papel que não existe nesta Torre, uma pasta que não existe, níveis sem git) vira um aviso, e o resto decola.

**Sem enlace:** se o `torre serve` parar, a barra mostra "Reconectando…"; depois de alguns segundos, um cartão diz "O torre serve parou? Tentando de novo em 3 s…". Os voos ficam no último estado conhecido e voltam sozinhos quando o servidor sobe.

**Erros em português:** as mensagens de erro da interface (pasta não encontrada, nome repetido, sigla em uso, nível com alterações…) chegam em português. A API manda `code` e `params` junto com o `error` em inglês, que é o que os agentes e a CLI continuam lendo.

**Frequências:** arraste a partir da bolinha lateral de um nó e solte sobre outro nó: os dois ficam na mesma frequência. Um clique no fio o seleciona; para tirar da frequência, use Delete ou o botão "Desligar". Duplo clique no fio cria um ponto de curva, que pode ser arrastado; duplo clique no ponto o remove.
- **Os fios pendem como uma corrente:** tracejados e finos, com 3% de folga. Os voos, as NOTAMs, os portais e os grupos recolhidos que estão no caminho são sólidos (o topo é chão e os lados são paredes), então o fio passa por cima ou por baixo deles e contorna as bordas, sem passar por trás. O fio não se enrola num nó: quando ele contorna mais da metade de um nó (por exemplo, ao levar uma NOTAM em volta de outra), passa para o lado mais curto. Os pontos de curva seguram a corrente como pregos. Quando um voo, uma NOTAM ou um ponto de curva se move, o fio balança e assenta. Com "reduzir movimento" ligado no sistema, ele só pende, sem balançar.
- **Estados do fio:** parado, tracejado cinza; com um pedido passando, âmbar, com o tracejado correndo de quem pediu para quem responde (a pílula fica no ponto mais baixo da corrente); na espera geral, pontilhado; num laço, âmbar contínuo. Na espera geral (`⌥P`), o botão do topo vira "Liberar todos" em âmbar e cada frequência ganha um quadrado com ∞.

**Laço:** quando uma cadeia de pedidos passa do limite de saltos (Configurações → Limites; padrão 6), a Torre põe tudo em espera geral e mostra, no alto do canvas, o cartão "ESPERA GERAL · LAÇO" com a cadeia (por exemplo, "PLN → IMP → REV → TST → ARQ passou de 3 saltos"). Um clique num indicativo centraliza o voo; o nome aparece ao passar o mouse. Os fios da cadeia ficam em âmbar cheio com "∞ laço · 3 saltos · espera geral". "Liberar todos" (`⌥P`) solta os pedidos retidos e o cartão some; o limite continua o mesmo, e "Limites…" abre a configuração. O voo que passou do limite recebe a cadeia na recusa e é orientado a avisar você. A espera geral que você liga pelo botão não mostra cartão.

**Indicativos e níveis:**
- cada voo tem um **indicativo** de até 4 letras, único no canvas da sessão. Vem da **sigla** do papel (PLN, IMP; o segundo Implementador é IM2) ou, sem papel, das consoantes do nome (Claude → CLD). Dá para escolher outro em Novo voo ou em Configurar…;
- o indicativo aparece no cabeçalho do voo, no Rádio, no Transmitir, no briefing, no "criado por" e na barra de quem aguarda resposta. Os agentes podem chamar um voo pelo nome ou pelo indicativo (`torre ask "Bia"` ou `torre ask IMP`), e o `torre list` mostra os dois;
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
- aberto: os destinos em chips (indicativo e agente; "+ destino" acrescenta outro, ou **Todos os voos de FL1**, de cada nível com dois voos ou mais), o texto e "⏎ transmitir";
- `@` menciona nós na frequência do destino;
- cole imagens com ⌘V.

**Rádio (`⌥A`):** os pedidos entre os voos, em indicativos ("14:32 PLN → IMP · c8d0").
- Filtros: Todos, Pendentes e um chip por voo. Os pedidos ainda abertos ficam no topo, com **Ver no espaço**, que leva ao voo, e **Cancelar pedido**: quem pediu recebe "cancelado" na hora, e o outro voo não é interrompido (se continuar trabalhando, `torre wait` pega a resposta).
- Um pedido longo aparece cortado; **Ver o pedido inteiro** mostra o texto todo, e **Ver a resposta** mostra o que voltou (a mensagem final do agente, ou a tela dele).
- Cada linha termina no desfecho: Pendente, Respondido em…, Entregue, Cancelado, Retido (espera geral ou limite por minuto), NOTAM editado, Em solo (fim de turno) ou "Pediu você · autorizado em 0:04" (ou sempre, negado; "respondido" quando foi na tela do voo).
- Edições seguidas da mesma NOTAM pelo mesmo autor, em até 2 minutos, viram uma linha só.
- **O histórico fica guardado**, por sessão, em `radio.jsonl`, na pasta da sessão (`~/.torre/workspaces/<id>/sessions/<sessão>/`):
  - ele volta ao recarregar a página, ao reiniciar o `torre serve` e ao reabrir a sessão; uma sessão nova começa com o Rádio vazio;
  - ficam as últimas 500 linhas; o texto de cada pedido e de cada resposta, até 16 KB;
  - aparecem 200 por vez, e **Mais antigos**, no fim da lista, traz as anteriores;
  - linhas de outro dia mostram a data ("17/10 14:32");
  - **Limpar o Rádio** (a lixeira no cabeçalho, com confirmação) apaga o histórico da sessão.

| Rota (só da interface) | O que faz |
|---|---|
| `GET /w/<id>/api/radio?before=<hora>&limit=<n>` | as linhas do Rádio da sessão aberta, da mais nova para a mais antiga (200 por padrão), e `more` quando há mais antigas |
| `DELETE /w/<id>/api/radio` | limpa o Rádio da sessão |
| `GET /w/<id>/api/requests/<pedido>` | o texto inteiro de um pedido e a resposta (`reply`), quando guardada |
- O rodapé mostra quantas mensagens passaram no último minuto, o limite e o máximo de saltos.

**Consumo:** o que cada voo gastou e os limites do plano de cada agente, no botão CONSUMO do topo (entre RÁDIO e ENLACE).
- **O botão** mostra a janela mais cheia ("5H 83%", em âmbar a partir de 80% e em vermelho a partir de 95%) ou, sem limites conhecidos, os tokens do workspace. Numa tela estreita, fica só o ícone e o número.
- **A caixa** abre com um clique e fecha com Esc ou clicando fora:
  - **uma seção por agente** dos voos do workspace (sem voos de agente, as contas já vistas): as janelas do plano, com a barra, a porcentagem e quando renova, e "visto há…", que diz de quando é o número. A conta é a mesma em todos os workspaces;
  - **uma linha por voo**, de todos os níveis: entrada (fora do cache), saída, cache lido, cache criado, quanto do contexto está ocupado e o custo. Clicar na linha leva ao voo. O total fica embaixo.
- **De onde vem:** tudo é lido no seu computador, do que o agente já grava. Nada vai para a rede, e nenhuma credencial é lida.

| Agente | Tokens e contexto | Limites da conta | Custo |
|---|---|---|---|
| Claude Code | a status line que a Torre põe no Claude e a transcrição da conversa | sessão de 5 h e semana (Pro e Max), a partir da primeira resposta | a estimativa do Claude a preços da API (no Pro e no Max ela não é cobrada) |
| Devin | o `sessions.db` do Devin (`~/.local/share/devin/cli`; no Windows, `%APPDATA%\devin\cli`) e o rodapé "Context: …" da tela | cota diária e semanal, quando você abre `/usage` no voo; numa conta de empresa, os ACUs que o `/usage` mostrar | ACUs ou créditos da sessão |
| Codex | os arquivos da conversa (`~/.codex/sessions`) | as janelas do plano do ChatGPT | — |
| Gemini | os arquivos da sessão (`~/.gemini/tmp`) | o Gemini não informa | — |

- **A sua status line continua:** a Torre roda a do seu `settings.json` (do projeto ou o seu) e mostra o que ela imprimir.
- **Rota** (só da interface): `GET /w/<id>/api/usage` dá os voos e as contas. As contas ficam em `~/.torre/usage-accounts.json`, para aparecerem depois de reiniciar.

**Workspaces:**
- o botão no topo, à esquerda, abre o seletor: a cor, o nome com o atalho (`⌥1`…), a pasta e o que está acontecendo ("2 no ar · 1 pendente"). Um ponto no botão indica atividade em outro workspace;
- `⌥1`–`⌥9` trocam de workspace, e a URL guarda o workspace aberto (`?ws=<id>`), então dá para abrir dois em abas diferentes;
- "Novo workspace" pede a pasta (com sugestões), um nome opcional e a cor. O lápis de cada linha edita esses campos; mudar a pasta vale para voos novos ou reiniciados;
- **Remover** (a lixeira de cada linha, ou "Remover workspace" no lápis) encerra os voos dele e move o canvas e as NOTAMs para `~/.torre/trash/`. O último workspace não pode ser removido: a lixeira avisa;
- `torre ask`, `list` e `check` só enxergam voos do mesmo workspace.

**Portal (`P`):** mostra um site ao vivo, como uma aba de navegador dentro do canvas.
- A página é aberta por um Chrome, Edge ou Chromium sem janela, controlado pela Torre (`TORRE_BROWSER` aponta outro executável). Só abre quando um portal precisa. Os logins e cookies ficam em `~/.torre/browser`, compartilhados entre os workspaces.
- Clique na página para usá-la: o teclado vai para o site (chip "teclado") até `⌥Esc`, e a roda do mouse rola a página (pinça ou ⌘/ctrl + roda dão zoom no canvas). Colar digita o texto.
- Barra de endereço, voltar, avançar e recarregar. Seguir links atualiza o endereço salvo no canvas.
- O tamanho da página acompanha o nó (redimensione pelas alças). Com o zoom abaixo de 60%, o portal mostra o último quadro e para de transmitir ("pausado · zoom").
- Cada portal tem um nome ("Portal", "Portal 2"…). Duplo clique no nome (ou ⋯ → Renomear) troca; os agentes usam esse nome para escolher o portal.
- **Agentes no portal:** um voo na frequência do portal usa a página com `torre portal` (tabela abaixo). Enquanto ele age, o cursor dele aparece na página com o nome e a cor do papel, e o fio mostra "clicando", "preenchendo", "digitando"…
- **@ ELEMENTOS**, no cabeçalho do portal, mostra o que os voos veem: um contorno tracejado em cada elemento do último `torre portal snapshot`, com a etiqueta ("@e7 botão"), na cor do papel de quem usa a página. Os contornos acompanham a rolagem e não pegam cliques. Sem snapshot (ou depois que a página muda de endereço), aparece "Sem refs". Ligar a camada não tira um snapshot novo, para não trocar as refs que um agente está usando.

**Grupo (`G`):** uma moldura com nome e cor em volta de vários nós, para organizar uma equipe grande ("Front-end" com PLN, IMP e o portal). É só visual: os agentes não veem o grupo, e quem conversa com quem continua sendo decidido pelos fios.
- **Criar:** com nós selecionados, `G` põe a moldura em volta deles ("Grupo 1"); sem seleção, `G` e arraste um retângulo no canvas. `Esc` desiste.
- **Quem está dentro:** o que está inteiro dentro da moldura na hora. Para entrar ou sair, arraste o nó para dentro ou para fora. Uma moldura dentro de outra só anda junto.
- **Mover:** arraste pela aba do nome; tudo o que está dentro vai junto, com os fios. As alças (com a moldura selecionada) redimensionam a moldura sem mexer nos nós.
- **Renomear e cor:** duplo clique no nome, ou o menu `⋯` (Renomear, Cor, Recolher, Remover moldura). **Remover moldura** tira só a moldura; os nós ficam.
- **Recolher:** a moldura vira um cartão com o nome e os estados de dentro ("4 voos · 1 pede você · 2 no ar"). Os voos continuam no ar, só somem da tela. Os fios para eles saem do cartão (tracejados), com as pílulas de pedido. Quando alguém de dentro pede você, o cartão fica âmbar e a faixa PRIORIDADE diz "REV no grupo Front-end…": **IR ATÉ ELE** (`⌥⏎`), "Ver no espaço" e os indicativos do laço abrem o grupo antes. Arrastar o cartão leva junto o que está escondido; **Expandir** (ou duplo clique no cartão) devolve tudo para onde estava.

**Desenho (`D`):** setas, formas e texto à mão, para explicar o canvas ("IMP entrega para REV", um círculo em volta do que está travado). Ninguém lê os desenhos: não são NOTAMs nem fios.
- **A barra:** Caneta, Seta, Retângulo, Elipse e Texto, e as cores. Cada traço vira um objeto, e a ferramenta continua ligada até `Esc` ou `V`. No Texto, um clique começa a escrever; `Esc` ou clicar fora termina (vazio, ele some).
- **Depois:** com `V`, um clique na linha (ou no texto) seleciona, arrastar move, as alças redimensionam e `Delete` apaga. Duplo clique num texto edita.
- Os desenhos ficam atrás dos voos e só pegam o clique nas próprias linhas, então não atrapalham o terminal embaixo.

**Grupos e desenhos nos níveis e nos planos:** ficam no nível em que foram feitos. Pousar ou descartar o nível remove os dele (as NOTAMs continuam descendo para a pista). Os planos de voo guardam e decolam grupos e desenhos; salvar a seleção com um grupo selecionado leva o que está dentro dele.

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
- **Na interface:** a barra do topo tem a PISTA (o branch atual) e uma aba por nível (FL1, FL2…), cada uma com o nome, o branch e "limpo", "3 alt.", "2 commits", "preparando" ou "preparo falhou". Um ▲ marca voos no ar ali. O `+` cria um nível. Clicar no nível aberto abre o menu: pedir pouso, renomear, log do preparo, copiar o caminho e descartar. **Renomear** troca só o nome (o número, o branch e a pasta ficam), e os agentes passam a chamar o nível pelo nome novo. O nível aberto fica na URL (`&floor=`), e cada nível guarda o próprio zoom.
- **Novo nível:** o número (FL3) aparece ao lado do nome e do branch. Um nível sem voos mostra o cartão "Nível vazio", com o comando `torre new … --floor` e o botão "Novo voo neste nível".
- Voo, NOTAM e portal criados na tela vão para o nível aberto. O rodapé de cada voo mostra o nível, o branch e a pasta de verdade.
- Um fio para um nó de outro nível aparece como etiqueta embaixo do nó (`⇄ PLN · pista`). Ela pulsa enquanto um pedido passa por ele, e um clique leva ao outro nível, centrado no nó.
- O nível parte do último commit (alterações não commitadas ficam na pista). Recebe os arquivos versionados, mais os não versionados da lista de cópia do workspace (padrão `.env*`), e roda o comando de preparo, se houver (ex.: `npm install`). A lista e o comando ficam em Novo nível → Preparação (ou `PUT /w/<id>/api/floors/config`). O preparo roda no shell padrão (Git Bash no Windows), e a saída fica no "Log do preparo".
- Os voos do nível abrem na cópia; pastas relativas, e mesmo uma pasta da pista dada por inteiro, apontam para a cópia. Um voo pode estar na frequência de voos de outros níveis: o líder na pista conversa com quem está no nível.
- **Pousar** fecha os voos e portais do nível e apaga a pasta; as NOTAMs descem para a pista. O branch fica. Com `--merge`, antes mescla o branch no branch de origem (`--into outro` para outro), com commit de merge. Se o branch de destino não estiver aberto em nenhuma pasta, a mescla acontece sem tocar em pasta nenhuma. É recusado se o nível tiver alteração não commitada, se a pasta de destino tiver alteração em arquivo versionado, ou se o branch do nível tiver commitado um arquivo copiado da pista (como o `.env`, que pode ter segredos). Com conflito, nada muda, e a resposta lista os arquivos.
- **Autorizar pouso · FL2:** o diálogo mostra os commits do nível, as checagens (conflitos, tudo commitado, voos no ar, commits novos na origem), onde mesclar e o que fecha. Os conflitos com o branch de origem são checados antes de tentar (`git merge-tree`, sem tocar em pasta): "✓ sem conflitos" ou "✕ 1 conflito · README.md". Com um git anterior ao 2.38, ou mesclando em outro branch, eles são checados ao pousar, e o diálogo lista os arquivos.
- **Testes e revisão:** o Testador e o Revisor de um nível registram o resultado deles com `torre floor mark tests pass "12 testes passaram"` ou `torre floor mark review fail "falta tratar o erro"` (pelo MCP, a ferramenta `floors` com `action: mark`, que não pede aprovação).
  - **Quem marca:** os voos do próprio nível e o líder que o criou.
  - **A marca vale para o commit do nível naquele momento:** com um commit novo depois, ela fica "desatualizada" e precisa ser feita de novo. Alterações sem commit na hora não entram nela, e a resposta avisa.
  - **Onde aparece:** na fila de pouso do Sinóptico e no "Autorizar pouso", como ✓ (passou), ✕ (falhou), ⟳ (desatualizada) ou ○ (sem marca); passando o mouse, quem marcou, a nota, quando e o commit. O Rádio ganha uma linha a cada marca.
  - **Para os agentes:** `torre floor list` mostra as marcas, e `torre floor land` diz como elas estavam no pouso.
  - **Não bloqueiam o pouso:** quem decide é você, ao autorizar. Os papéis padrão do Testador e do Revisor marcam, e o do Planejador confere antes de pousar; quem editou esses papéis antes usa "Restaurar padrão" em Papéis para trazer a regra.
- **Permissões (o SEMPRE):** o Claude e o Devin guardam o que você libera com SEMPRE num arquivo da pasta do projeto, fora do git (`.claude/settings.local.json` e `.devin/config.local.json`).
  - **Ao criar o nível:** os dois arquivos vêm da pista;
  - **quando um Claude ou Devin decola nele:** ganha as regras que a pista ganhou depois (só soma, nunca tira);
  - **esses arquivos nunca contam como alteração:** não bloqueiam o pouso;
  - **no "Autorizar pouso":** se o nível ganhou regras, o diálogo as lista, e a caixa "Levar essas permissões para a pista" as soma aos arquivos da pista. Ela vem **desmarcada**, porque um voo também pode escrever esses arquivos: leia antes de levar. Descartar o nível joga as regras fora, e o pouso pela CLI ou pelo MCP nunca as leva;
  - **o Claude (2.1.302) é diferente:** num nível, ele já grava o SEMPRE no arquivo da pasta principal e lê os dois arquivos, então a regra vale na pista na hora e o pouso não tem o que levar dele.
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

**NOTAMs:** as notas do canvas, em papel. Prévia/Editar no cabeçalho; as tarefas podem ser marcadas com clique; o cadeado trava a NOTAM para todos. Quando um agente edita a NOTAM, o trecho alterado fica destacado. Para mudar o tamanho, arraste o canto de baixo, à direita (as duas risquinhas), ou, com a NOTAM selecionada, as bordas e os cantos (mínimo 280×200). Um nome comprido é cortado com "…" (o nome inteiro aparece ao passar o mouse), e os botões ficam sempre dentro da NOTAM. No modo Editar, ela grava sozinha 0,8 s depois que você para de digitar, e Esc volta à prévia.

**Nome das NOTAMs:**
- **NOTAM nova** (tecla N ou barra de ferramentas): já abre com o campo de nome selecionado. Enter confirma; Esc fica com o nome provisório ("Nota", "Nota 2"…). O arquivo nasce com o nome que você der ("Plano de testes" → `plano-de-testes.md`) e não muda mais.
- **Renomear** as suas NOTAMs: duplo clique no título, ou "Renomear" no menu ⋯. O arquivo fica, e **o nome antigo continua valendo para os agentes**: um agente que pedir a NOTAM pelo nome antigo a encontra, e a resposta diz o nome novo.
- **NOTAMs criadas por agentes** (`torre note create "Nome"`, ou a ferramenta `note` do MCP) ficam com o nome que o agente deu: aparecem com "por IMP" e sem "Renomear".
- **Nomes únicos no workspace**, sem diferenciar maiúsculas, contando os nomes antigos. Os agentes acham a NOTAM pelo nome em qualquer caixa.

**Pilha de NOTAMs:** várias NOTAMs no lugar de uma, para dar a um voo todo o material de um assunto com um fio só.
- **Empilhar:** arraste uma NOTAM e solte em cima de outra do mesmo nível. A de baixo é a base. Os fios das duas passam para a pilha, sem repetir, e um fio entre as duas some. Soltar uma pilha em cima de outra junta as duas.
- **Antes de empilhar:** se algum voo passa a ler NOTAMs que não lia, a Torre pergunta, com quem ganha acesso ("IMP e REV passam a ler e escrever as 3 NOTAMs da pilha"). Sem ninguém ganhando acesso, empilha direto.
- **Um fio ligado à pilha vale para todas as NOTAMs dela.** Um fio novo para qualquer NOTAM da pilha vai para a pilha.
- **Na tela:** a faixa "PILHA · 3 NOTAMs", uma aba por NOTAM, "‹ 1/3 ›" para passar, a NOTAM da aba aberta inteira e as folhas desenhadas atrás. Mover ou redimensionar leva a pilha toda.
- **Quando um agente escreve** numa NOTAM que não está à mostra, a aba dela ganha um ponto e a faixa avisa "REV escreveu em Arquitetura". O clique abre essa NOTAM. A pilha não troca de NOTAM sozinha.
- **Tirar da pilha** (menu ⋯): a NOTAM vai para o lado da pilha e continua com os fios dela (a pilha também). Se sair a base, a próxima NOTAM vira a base. Uma pilha que fica com uma NOTAM só deixa de ser pilha.
- **Desfazer a pilha** (menu ⋯): solta todas, cada uma com os fios da pilha.
- **O cadeado continua por NOTAM.** Remover uma NOTAM da pilha tira só ela.
- **Agentes:** leem e escrevem cada NOTAM pelo próprio nome, pela CLI e pelo MCP. O `torre list` e o `torre note list` mostram `stack: "Plano"` (o nome da base). As @menções do Transmitir também oferecem as NOTAMs da pilha. Só a interface empilha.
- **Planos de voo e níveis:** um plano guarda as pilhas e decola com elas; pousar um nível desce as pilhas inteiras para a pista.

| Rota (só da interface) | O que faz |
|---|---|
| `POST /w/<id>/api/notes/<nota>/stack` `{"onto"}` | empilha a nota (com a pilha dela) sobre a pilha de `onto`; devolve as notas da pilha |
| `POST /w/<id>/api/notes/<nota>/unstack` `{"all"}` | tira a nota da pilha (com `all`, desfaz a pilha); as que saem ficam com os fios |

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
  - **Abrir**, **Renomear** (o lápis; Enter salva, Esc desiste) e **Remover** (a lixeira). A lixeira fica apagada, e o clique diz o motivo, na sessão aberta (abra outra antes), na única sessão do workspace (remova o workspace) e numa sessão com níveis;
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
- **Pedir pouso da sessão:** clicando na PISTA (o branch da sessão), ou em **Pedir pouso…** no painel de sessões. O diálogo mostra os commits da sessão e mescla o branch dela no branch escolhido (o padrão é o de origem), com commit de merge. **A cópia continua**: a sessão segue trabalhando e pode mesclar de novo. É recusado se a cópia tiver alteração sem commit, ou se a pasta onde o branch de destino está aberto (a do projeto, por exemplo) tiver alteração sem commit; com conflito, nada muda, e o diálogo lista os arquivos para um agente da sessão resolver. As permissões do SEMPRE seguem a cópia como seguem um nível: o diálogo lista as regras novas, com a caixa (desmarcada) "Levar essas permissões para a pasta do projeto".
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
| `torre list` | quem você é e o que está ligado a você, com o estado de cada voo, o pedido que ele está atendendo, quantos esperam na fila dele e por quem ele espera |
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
| `torre floor mark tests\|review pass\|fail "nota"` | os voos do nível (e o líder que o criou): registram o resultado dos testes ou da revisão no último commit, para a fila de pouso |
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

**Fila de cada voo:** um agente (Claude Code, Devin, Codex, Gemini) recebe um pedido por vez, quando está livre. Livre quer dizer:
- não está trabalhando nem pedindo você;
- está parado há um instante (1,5 s), porque uma mensagem que você digitou com ele trabalhando começa logo depois do turno;
- e não tem outro pedido entregue sem resposta.

Se não estiver livre, o pedido espera na fila dele, por ordem de chegada, e entra quando o turno acaba. Assim cada pedido recebe a resposta do turno que ele mesmo começou. Antes, um pedido que chegava no meio do turno de outro ficava com a resposta desse outro.
- **Na tela:** o fio mostra "NA FILA", a faixa do voo diz "Na fila de IMP" (e, no destino, "1 na fila"), e a linha do Rádio diz "Na fila · Implementador termina o turno". Dá para cancelar pelo Rádio.
- **O que não entra na fila:** shells e comandos próprios (`--raw` também), a resposta a quem pergunta de volta, a mensagem para quem está em `torre wait` esperando quem mandou e as suas mensagens (Transmitir).
- **Laços:** se um voo ocupado pergunta (ou espera com `torre wait`) e há na fila dele um pedido de alguém da mesma cadeia, ele recebe esse pedido na hora, como resultado do comando, com a instrução de responder primeiro (`torre reply`) e mandar a mensagem dele de novo depois. Sem isso, um esperaria o turno do outro.
- **Limite:** a espera na fila conta para o limite de 30 min do pedido. Passou disso, o pedido volta dizendo que a mensagem não foi entregue.
- **No `torre list`:** cada voo ligado mostra o pedido que está atendendo e de quem, quantos esperam na fila dele e por quem ele mesmo espera. Para você mesmo, mostra quantos esperam o seu turno acabar e de quem. Uma linha de legenda explica os estados (`idle`, `work`, `wait`, `ask`…). Um pedido na fila aparece sem o número, e o `torre reply` recusa responder a um pedido que ainda não chegou: o agente responderia sem ter lido.
  ```
  - name: "Impl", agent: claude, state: work, callsign: IMP
    busy: answering request 7f3a from "Plano"; 1 more in its line (from "Revisor"): an ask now waits for those first
  - name: "Revisor", agent: devin, state: wait, callsign: REV
    waiting for "Impl" (#2 in its line)
  ```

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
