# Brief de design: canvas de agentes

Oct 15, 2026 · @Nathan Ribeiro

Uma ferramenta local, aberta no navegador, em que agentes de código (Claude Code, Codex, Gemini) e shells viram nós num canvas infinito e conversam entre si pelas conexões que você desenha. A referência é o Maestri, app de desktop cuja versão de macOS foi investigada; este brief junta a ideia, essa referência e o que precisa ser desenhado no Claude Design.

## Conceito

Cada agente de IA roda num terminal real, posto como nó num canvas, e só conversa com quem estiver ligado a ele por uma conexão desenhada por você.

- **Para quem:** quem programa com dois ou mais agentes de código ao mesmo tempo (por exemplo, um planejador, um implementador e um revisor) e hoje alterna entre abas de terminal.
- **Problema:** os agentes não se falam, não dá para ver quem está trabalhando ou parado, e experimentos paralelos se atropelam no mesmo repositório.
- **Promessa:** montar uma equipe de agentes arrastando e ligando nós; ver em tempo real quem trabalha, quem espera quem e o que cada um respondeu; isolar experimentos em cópias do projeto e trazê-los de volta.
- **Forma:** servidor Node local (NestJS) com interface React, aberta no navegador em `http://127.0.0.1:<porta>`. Roda em macOS, Linux e Windows.
- **Nome:** a definir. O vocabulário deste brief (terminal, conexão, nota, portal, andar, papel) é provisório.

## Referência: o Maestri

O [Maestri](https://www.themaestri.app/) é um app de desktop, nativo no macOS e com versão para Windows (interface em Electron), feito por um desenvolvedor solo brasileiro ([Evert Junior, no Product Hunt](https://www.producthunt.com/products/maestri)), que se apresenta como "an infinite canvas where coding agents work in concert". A versão 0.53.1 foi investigada em 14 e 15 de outubro de 2026; os detalhes estão nos documentos 01 a 04 da pasta `docs/`.

| Conceito | Como é no Maestri | Na nossa versão |
| --- | --- | --- |
| Canvas infinito | Zoom, pan, grupos e desenhos à mão; um canvas por workspace (projeto) | Igual, em React Flow |
| Terminal como nó | Claude Code, Codex, Gemini, OpenCode ou shell, cada um num terminal real | Igual |
| Conexão | Linha arrastada entre dois nós, sem direção; guarda pontos de curva (`ropePoints`) e "abraçadeiras" (`cableTies`) | Igual; é ela que autoriza a conversa |
| Conversa entre agentes | O agente roda `maestri ask`; recebe de volta a tela do outro terminal | CLI e MCP; volta a última mensagem do agente |
| Papéis | Instruções reutilizáveis (líder, programador, revisor, testador), com cor e ícone | Igual |
| Notas | Post-its em Markdown que os agentes leem e escrevem | Igual, com nome estável |
| Composer | Caixa flutuante de prompt com @menções e colagem de captura de tela | Igual |
| Chat por terminal | Visão de chat com respostas formatadas, diagramas e arquivos arrastáveis | Fase posterior |
| Portais | Navegador, simulador iOS ou Android e celular, controlados pelos agentes | Navegador primeiro |
| Andares | Cópia isolada do projeto (git worktree), mesclada de volta depois ("aterrissar") | Igual |
| Partituras | Arranjo salvo de agentes, notas, portais e conexões, restaurável em outro lugar | Igual, com nome próprio |
| Modo Maestro | Um agente gerente cria e liga outros terminais | Igual |
| Ombro | Assistente local (Apple Intelligence) que resume turnos e sugere próximos passos | Opcional, com qualquer modelo |

**Visual do original, segundo o site:** Liquid Glass da Apple, convenções do macOS, modo escuro e terminais renderizados em Metal, nítidos em qualquer zoom. A metáfora central é maestro e orquestra.

**O que não copiar:** nome, logotipo, ícones, textos, telas e a metáfora musical (Maestri, Maestro, Partitura, Ombro). O Liquid Glass é linguagem de sistema da Apple; num app de navegador, vira imitação. Use o Maestri como referência de conceitos e fluxos, não de aparência, e não envie capturas de tela dele ao Claude Design.

## Visão geral

&#91;embedded content: wireframe do canvas · 3 agentes, nota, portal e Composer\]

As conexões não têm seta porque não têm direção; a tracejada marca um pedido do Planejador ao Implementador ainda sem resposta. A faixa colorida no cabeçalho do terminal é a cor do papel, e o estado aparece como ponto colorido mais texto.

## Objetos do canvas

O terminal de agente é o objeto central; os outros existem para alimentar, ligar ou isolar os agentes.

| Objeto | O que é | O que precisa aparecer | Ações principais |
| --- | --- | --- | --- |
| Terminal de agente | Claude Code, Codex, Gemini ou OpenCode rodando num terminal real | Nome, tipo de agente, papel (cor e etiqueta), estado do turno, andar, a tela viva | Digitar direto, enviar pelo Composer, parar, reiniciar, trocar papel, abrir em foco |
| Terminal de shell | Um shell comum (zsh, bash, fish) | Nome, estado (rodando, parado, código de saída), a tela viva | As mesmas, sem papel |
| Nota | Arquivo Markdown que pessoas e agentes editam | Título, prévia renderizada ou edição, cadeado quando bloqueada, aviso de que um agente acabou de editar | Editar, alternar prévia, bloquear, empilhar |
| Portal | Navegador controlado pelo servidor | URL, título, imagem ao vivo da página, sinal de que um agente está clicando | Navegar, recarregar, ligar a um agente |
| Conexão | Linha sem direção entre dois nós; autoriza a conversa | Quem liga quem, mensagem em trânsito, pedido pendente e há quanto tempo | Criar arrastando, apagar, pausar a conversa |
| Grupo | Moldura que junta nós | Nome e cor | Mover em bloco, recolher |
| Desenho | Setas, formas e texto à mão livre | Traço simples | Desenhar, apagar |

Quatro conceitos não são nós, mas aparecem no canvas:

| Conceito | O que é | Onde aparece | Ações principais |
| --- | --- | --- | --- |
| Workspace | Um canvas por projeto, com pasta de trabalho | Seletor no topo; nome, cor, número de agentes ativos | Criar, alternar por atalho |
| Andar | Cópia isolada do projeto (branch + git worktree), com canvas próprio | Camada do canvas; branch, alterações pendentes, se pode aterrissar | Criar, alternar, aterrissar, descartar |
| Papel | Instrução reutilizável que define a função do agente | Cor e etiqueta no terminal; biblioteca própria | Criar, editar, atribuir (reinicia o agente) |
| Modelo | Arranjo salvo de nós, conexões e papéis (a Partitura do Maestri) | Galeria de modelos | Salvar da seleção, importar, revisar os comandos antes de rodar |

## Estados

&#91;embedded content: estados do terminal e desfechos de um pedido\]

Em destaque, na cor de espera do wireframe, os dois lados da mesma espera: o terminal Esperando é o que tem um pedido Pendente. O contorno tracejado marca o desfecho que só existe na nossa versão; os outros vêm do Maestri.

## Telas e fluxos a desenhar

Comece pelas telas de F0 e F1: o canvas com terminais e a conversa entre agentes são o produto; o resto se apoia neles.

| Fase | Tela ou fluxo | O que o design precisa resolver |
| --- | --- | --- |
| F0 | Primeira abertura | Colar o token de sessão (o servidor só aceita o navegador autenticado), escolher a pasta do projeto, mostrar quais agentes estão instalados |
| F0 | Canvas principal | Barra superior (workspace e andar), barra de ferramentas (terminal, nota, portal, desenho), minimapa, estado vazio |
| F0 | Nó de terminal | Cabeçalho com nome, agente, papel e estado; a tela viva; três níveis de zoom (ver Interações) |
| F0 | Novo terminal | Escolher agente ou shell, pasta, papel e comando; aviso quando o agente não está instalado |
| F1 | Criar conexão | Arrastar de um nó a outro, realce dos destinos válidos, confirmação discreta |
| F1 | Conversa em andamento | Mensagem viajando pela conexão, pedido pendente com cronômetro, resposta chegando, remetente visível no destino |
| F1 | Painel de atividade | Linha do tempo de quem pediu o quê a quem, filtros por agente, botão de pausar a conversa |
| F2 | Nota | Edição e prévia, bloqueio, pilha de notas, destaque do trecho que um agente acabou de escrever |
| F2 | Composer | Caixa flutuante; @menções apenas dos nós ligados ao destino; menções como chips; colar imagem |
| F3 | Biblioteca de papéis | Lista, editor (nome, cor, ícone, instrução), atribuição com aviso de que o agente reinicia |
| F4 | Andares | Seletor, criar (nome e branch), estado do andar, aterrissar com ou sem mesclar, recusa quando há alterações, lista de conflitos |
| F5 | Portal | Barra de URL, imagem ao vivo, camada opcional com os elementos que o agente enxerga (`@e1`, `@e2`…) |
| F6 | Modelos | Salvar seleção como modelo, galeria, importação que lista os comandos antes de rodar |
| F6 | Agente gerente | Marcar quais terminais o gerente criou e ligou |
| F6 | Resumo de turno | Cartão curto ao fim de um turno, com próximos passos sugeridos |
| Todas | Erros e reconexão | Agente ausente, terminal encerrado com erro, servidor fora do ar, WebSocket reconectando |
| Todas | Configurações | Atalhos, tema dos terminais, agentes e seus comandos, limites de conversa |

## Interações-chave

A conexão é a interação que define o produto: só nós ligados se enxergam, nos pedidos, nas @menções do Composer e na lista de cada agente.

- **Conectar:** arrastar de uma alça do nó até outro nó. Apagar a conexão corta a conversa na hora.
- **Conversar:** quando A pede algo a B, a mensagem chega em B como se tivesse sido digitada (colagem e Enter). A conexão mostra o pedido em trânsito, o tempo de espera e a resposta voltando. Se B responder com uma mensagem direta para A ("ask back"), A é liberado na hora.
- **Zoom em três níveis:** perto, o terminal vivo, legível e digitável; no meio, uma miniatura do texto atualizada aos poucos; longe, um cartão com nome, agente, papel e estado. Durante o gesto, o conteúdo fica congelado e é redesenhado quando o zoom para, como no Maestri.
- **Foco:** abrir um terminal em tela cheia ou encaixá-lo numa borda da tela, com o canvas ao lado. O Maestri guarda uma borda de encaixe por workspace (`dockEdge`, inferido do modelo de dados).
- **Teclado:** o terminal em foco captura o teclado. Atalhos globais precisam de um modificador que não colida com o terminal, porque Ctrl-C, Ctrl-R e afins pertencem ao agente. Troca de workspace por atalho, no estilo do tmux.
- **Seleção:** retângulo de seleção para mover, agrupar ou salvar como modelo.
- **Ocultar:** borrar um nó com conteúdo sensível, útil ao gravar a tela. O Maestri tem algo assim (`isRedacted`, inferido).
- **Intervir:** interromper um agente, pausar todas as conversas de um workspace e retomar.

## Onde a nossa versão difere do Maestri

As diferenças tornam a conversa entre agentes mais visível e controlável, e quase todas pedem algo novo na interface.

| Tema | No Maestri | Na nossa versão | Impacto no design |
| --- | --- | --- | --- |
| Fim do turno | Detectado pela tela: libera de 5 a 6 s depois que ela para de mudar | Sinal nativo do agente (hooks do Claude Code, `notify` do Codex), quase imediato; a tela vira reserva | O estado "terminou" é confiável e pode ter uma transição marcada |
| Resposta | A tela inteira do destino, com moldura e barra de status | A última mensagem do agente, em texto limpo | A resposta pode aparecer formatada na conexão e no painel de atividade |
| Remetente | A mensagem chega sem dizer quem pediu | Envelope curto, como `[de: Revisor · pedido 7f3a]` | Mostrar a origem no destino e ligar o pedido à resposta |
| Pedidos longos | O agente que pede fica bloqueado até a resposta | Modo assíncrono: a resposta chega sozinha como nova mensagem | Caixa de entrada por terminal, com contador de pendentes |
| Laços entre agentes | Desfaz ciclos de espera, mas não limita conversas sem fim | Limite de saltos, limite de pedidos por minuto, botão de pausar | Aviso de laço, contador de mensagens, controle de pausa sempre à vista |
| Plataforma | App instalado: nativo no macOS, com Liquid Glass; Electron no Windows | Navegador em localhost; macOS, Linux e Windows | Convenções da web, sem imitar janelas de sistema |
| Segurança | App instalado, fora do alcance do navegador | Token de sessão e verificação de origem | Tela de pareamento na primeira abertura |
| Portais | Navegador embutido no app | Chromium no servidor, transmitido como imagem ao vivo | O portal parece um vídeo; precisa de estado de carregamento e de latência |
| Persistência | Agentes presos ao processo do app | Terminais num processo separado, que sobrevive a reinícios | Estado "reconectando" em que os agentes continuam trabalhando |
| Notas | Renomeadas sozinhas quando a primeira linha muda | Nome de arquivo estável | Mostrar título e nome de arquivo separados |

## Restrições técnicas

O canvas é React Flow (`@xyflow/react`) e cada terminal é um xterm.js com WebGL, num app React + Vite falando com o servidor por WebSocket; o design precisa caber no que essas peças fazem bem.

- **Terminais reais:** grade monoespaçada, 16 e 256 cores ANSI e truecolor. As TUIs dos agentes desenham as próprias molduras e barras. O tema do terminal (fundo, 16 cores, cursor, seleção) faz parte do design e precisa combinar com a interface ao redor.
- **Tamanho:** o terminal tem colunas e linhas; redimensionar o nó muda a grade e o agente redesenha a tela. O nó precisa de um tamanho mínimo útil, perto de 80 colunas por 24 linhas.
- **Muitos terminais:** vários terminais vivos ao mesmo tempo funcionam; dezenas, não. Abaixo de certo zoom, o nó vira miniatura ou cartão. Animações nas conexões devem ser leves (SVG ou CSS).
- **Teclado:** o navegador reserva atalhos como Cmd-W e Cmd-T, e o terminal em foco precisa receber o resto. O nó com o foco do teclado deve ser inconfundível.
- **Portais:** sites de terceiros não abrem dentro de um iframe, então o portal é uma imagem transmitida pelo servidor, com os cliques repassados a ele.
- **Tela:** desktop, a partir de cerca de 1280 px de largura. Celular fica fora do escopo.
- **Temas:** claro e escuro por tokens de cor; o tema dos terminais é escolhido à parte.
- **Acessibilidade:** estados nunca só por cor (cor + ícone + texto), contraste AA, navegação por teclado entre os nós.

## Direção visual

É uma ferramenta para sessões longas: calma, densa em informação, com a cor reservada para estados e para a identidade de cada agente.

**Princípios**

- **O terminal é o protagonista:** a interface ao redor recua, com cabeçalhos finos e pouco cromo, e o texto do terminal fica nítido.
- **Estado legível de longe:** com zoom de 25%, dá para saber quem trabalha, quem espera e quem terminou.
- **Conversa visível:** a coordenação aparece nas conexões, com mensagens em trânsito, pedidos pendentes e laços.
- **Cor com função:** uma família para papéis ou agentes, outra para estados; o resto em neutros.
- **Densidade de ferramenta de desenvolvedor:** tipografia compacta, sem cartões inflados nem sombras pesadas.
- **Identidade própria:** sem vidro da Apple e sem metáfora musical.

**O que evitar**

- Transparência ou desfoque sobre os terminais, que prejudica a leitura.
- Animação contínua em todos os nós, que distrai e pesa na GPU.
- Cor como único sinal de estado.
- Nomes, ícones e telas do Maestri.

**Decisões em aberto**

- [ ] Nome e identidade do produto
- [ ] Tema padrão: escuro, claro ou o do sistema
- [ ] Forma das conexões: fio com curva física (o Maestri guarda pontos de curva), Bézier simples ou linha reta
- [ ] Vocabulário: manter "andar", "papel" e "portal" ou trocar
- [ ] Fonte da interface e fonte padrão dos terminais
- [ ] Lugar do painel de atividade: lateral fixa, gaveta ou só na conexão
- [ ] Chat por terminal na primeira versão ou depois

## Fontes

- [Maestri, site oficial](https://www.themaestri.app/): proposta, recursos e estilo visual declarado
- [Maestri para Windows](https://www.themaestri.app/pt-br/windows): motor em Swift com interface em Electron
- [Maestri no Product Hunt](https://www.producthunt.com/products/maestri): autor e descrição do lançamento
- [Maestri na Setapp](https://setapp.com/apps/maestri): lista de recursos
- Investigação do Maestri 0.53.1, na pasta `docs/` do projeto: `01-investigacao-maestri.md` (o que foi observado e inferido), `02-arquitetura-maestri.md` (diagramas), `03-spec-agentes-conversam.md` (especificação da conversa entre agentes), `04-proposta-arquitetura.md` (nossa arquitetura e as diferenças)
