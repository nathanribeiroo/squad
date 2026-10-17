# Prompt inicial para o Claude Code

Copie tudo abaixo da linha e cole no Claude Code, aberto na raiz do repositório do Squad, com esta pasta (`torre-para-claude-code/`) copiada para dentro do repositório (por exemplo, em `docs/torre/`).

---

Você vai conduzir a mudança do **Squad** para **Torre**. O nome muda e a identidade também; a interface web é redesenhada por inteiro, e entram algumas funcionalidades novas, uma delas com mudança no backend Go. Todas as funcionalidades de hoje continuam. Nenhuma pode sumir.

## O que é o produto (em uma frase)

Uma ferramenta local (`squad serve`, binário Go em 127.0.0.1:7420, interface React embutida) em que agentes de código (Claude Code, Codex, Devin, Gemini) e shells rodam em terminais reais num canvas infinito e só conversam com quem estiver ligado a eles por um fio desenhado pela pessoa.

## A nova identidade: Torre

A metáfora é a torre de controle de um aeroporto. Cada agente é um **voo**; trabalhar é estar **no ar**; um agente que precisa de você pede **prioridade**; os fios são **frequências**; os andares (git worktrees) são **níveis de voo** (FL1, FL2…); o térreo é a **pista**; aterrissar é **pousar**, e a sua aprovação é a **autorização de pouso**. Visual: verde-fósforo de radar sobre preto, fichas de voo em papel creme, fontes **B612** e **B612 Mono** (feitas para cabines de avião), cantos retos, cor reservada para estados.

## Leia nesta ordem antes de escrever código

1. `docs/torre/README.md`: visão geral, como ler os arquivos de design, fidelidade, tokens e o plano por fases.
2. `docs/torre/RENOMEACAO.md`: tudo o que muda de nome (marca, binário, pastas, variáveis, MCP, skill, textos) e o vocabulário novo da interface, com a estratégia de compatibilidade.
3. `docs/torre/NOVIDADES-E-BACKEND.md`: o que é novo, o que exige backend e o contrato proposto (dados, eventos, rotas). **O principal é o estado PRIORIDADE ("o voo pede você").**
4. `docs/torre/TELAS.md`: as 22 telas, uma a uma, ligadas aos componentes de hoje (`src/...` da interface web).
5. `docs/torre/tokens/torre-tokens.css` e `docs/torre/tokens/xterm-themes-torre.ts`: os tokens novos.
6. Os mockups em `docs/torre/design/` (abra os `.dc.html` no navegador; precisam do `support.js` e do `TorreBar.dc.html` na mesma pasta). Eles são **referência visual em HTML**, não código para copiar.
7. O estado atual, para consulta: `docs/torre/referencia-squad-atual/FUNCIONALIDADES.md` (todas as funcionalidades de hoje, tela por tela), `REFERENCIA-TECNICA.md` (modelo de dados, eventos, API, CLI, MCP, limites), `capturas/` (44 telas reais) e `contexto/`.

## Regras

- **Recrie os mockups no código que já existe** (React 19 + Vite + TypeScript, `@xyflow/react`, `@xterm/xterm`, `zustand`, CSS puro com tokens). Não traga o HTML dos mockups para o app, nem os estilos inline deles.
- **Nada de funcionalidade a menos.** Se um mockup não mostra algo que existe em `FUNCIONALIDADES.md`, a funcionalidade continua, no estilo da Torre.
- **Onde o mockup e a renomeação discordarem, vale a renomeação.** Os mockups ainda mostram `squad ask`, `squad new`, `squad serve`, `~/.squad`, `.squad/floors` em alguns lugares: no produto final isso é `torre …`, `~/.torre`, `.torre/…` (veja `RENOMEACAO.md`).
- **Os dados dos mockups são ilustrativos** (workspace `relatorios-api`, pedido `7f3a` etc.). Use os dados reais da API. Onde o mockup mostra um dado que não existe (por exemplo, "✓ testes · ✓ revisão" na fila de pouso), siga o que `NOVIDADES-E-BACKEND.md` diz.
- **Regras de design que não mudam:** estado nunca só pela cor (cor + ícone + palavra); nada translúcido ou desfocado sobre os terminais; atalhos globais sempre com ⌥; âmbar só para espera; só pisca o que pede a pessoa (PRIORIDADE e cursor); respeitar `prefers-reduced-motion`; contraste AA; desktop a partir de 1280 px, fluido (os mockups são quadros fixos de 1440×900).
- **Português do Brasil** em toda a interface, inclusive nos erros que hoje chegam do servidor em inglês.
- Mantenha os identificadores internos do código (`floor`, `node`, `edge`, `role`, `manager`, `activity`) se trocá-los só aumentar o risco. O vocabulário novo é para o que a pessoa lê.

## Como trabalhar

1. Leia os documentos e o código, e me devolva **um plano por fases** (o `README.md` sugere seis: F0 a F5), com a lista de arquivos que cada fase toca, o que fica para depois e as dúvidas. **Espere eu aprovar antes de começar.**
2. Uma fase por branch e por PR, cada uma deixando o app funcionando. Comece pela F0 (renomeação e tokens, sem mudar comportamento).
3. Ao fim de cada fase: rode o build, os testes e o `scripts/check-icons.mjs` (há ícones novos; o subconjunto de Material Symbols precisa ser regerado com `scripts/subset-icons.py`), confira as telas contra os mockups e as capturas, e me mostre o que mudou.
4. Nas mudanças de backend, escreva testes para o estado novo, as rotas novas e a migração de `~/.squad` para `~/.torre`.
5. Quando algo dos documentos não fechar com o código real, pare e pergunte em vez de inventar.

Comece lendo e me trazendo o plano.
