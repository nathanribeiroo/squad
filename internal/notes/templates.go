package notes

import (
	"slices"
	"strings"
	"time"
)

// Templates give a new note the sections of a document the roles write
// (torre note create "PRD: X" --template prd): the roles' instructions say
// what goes in each, the template keeps them in order. Their text is in
// Portuguese, like the roles.
var templateNames = []string{"prd", "spec", "adr", "tasks", "task", "design", "memory"}

// templateHelp describes each template in a few words, for torre note help.
var templateHelp = map[string]string{
	"prd":    "a PRD: the what and the why (Produto)",
	"spec":   "a technical spec (Especificador)",
	"adr":    "an architecture decision record, dated today",
	"tasks":  "the base of a stack of tasks: the progress table",
	"task":   "one task of the stack",
	"design": "a UX/UI design (Designer)",
	"memory": "the shared memory of a piece of work (Planejador)",
}

// template returns the content of a template, or false.
func template(name string, now time.Time) (string, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !slices.Contains(templateNames, name) {
		return "", false
	}
	t := templates[name]
	return strings.ReplaceAll(t, "{date}", now.Format("2006-01-02")), true
}

var templates = map[string]string{
	"prd": `## Visão geral
_O problema, para quem e por que vale resolver._

## Objetivos e métricas
_O que muda para quem usa e como medir._

## Histórias de usuário
- Como <quem>, quero <o quê>, para <por quê>.

## Funcionalidades
_Por prioridade: o que cada uma faz e por que importa._

## Experiência do usuário
_Os fluxos principais, passo a passo._

## Critérios de aceite
- Dado <situação>, quando <ação>, então <resultado>.

## Restrições
_Integrações, desempenho percebido, privacidade. Nada de implementação._

## Fora do escopo

## Fases
### Primeira versão

### Depois

## Riscos
_De produto: adoção, prazo, dependências._

## Decisões
_As ADRs, uma linha cada: "ADR-001: …"._

## Perguntas em aberto
`,
	"spec": `## Resumo
_A abordagem e o principal custo da escolha._

## Componentes
_Cada parte, o que faz e o que muda nela._

## Contratos
_Tipos, assinaturas, rotas, comandos, formato dos dados, migrações._

## Impacto
| Parte | Muda como | Risco | O que fazer |
|---|---|---|---|

## Testes
_Unitários e de integração: o que cobrir._

## Ordem de construção
1. <passo>, sem dependências
2. <passo>, depende de 1

## Riscos
_Técnicos, e como reduzir._

## Decisões
_As ADRs, uma linha cada: "ADR-002: …"._
`,
	"adr": `**Status:** aceita
**Data:** {date}

## Contexto
_O problema e as restrições que levaram à decisão._

## Decisão

## Alternativas descartadas
- **<alternativa>:** prós, contras e por que não.

## Consequências
- **Boas:**
- **Ruins:**
- **Riscos:**
`,
	"tasks": `| # | Tarefa | Papel | Complexidade | Depende de | Andamento |
|---|---|---|---|---|---|
| 01 | <título> | <papel> | <baixa, média, alta, crítica> | — | a fazer |

_Andamento: a fazer, fazendo, feita. Cada tarefa é uma NOTAM nesta pilha._
`,
	"task": `**Papel sugerido:** <Implementador, Front-end, Documentador ou Depurador>
**Complexidade:** <baixa, média, alta ou crítica>
**Depende de:** <Tarefa 01, … ou nenhuma>
**Andamento:** a fazer

## Objetivo
_O que a tarefa entrega e por quê, em duas ou três frases._

## Requisitos
- DEVE …
- PODE …

## Subtarefas
- [ ] …

## Arquivos
- **Muda:** ` + "`caminho`" + ` (por quê)
- **Afetados:** ` + "`caminho`" + ` (por quê)

## ADRs
_As que a tarefa segue._

## Testes
- [ ] <entrada ou situação> → <resultado esperado>

## Pronto quando
- Os testes da tarefa e a verificação do projeto passam.
`,
	"design": `## Fluxo
_Passo a passo, do começo ao fim._

## Telas
_Cada tela e o que ela mostra._

## Estados
- **Vazio:**
- **Carregando:**
- **Erro:**
- **Sucesso:**

## Textos
_Rótulos, mensagens e confirmações, exatamente como aparecem._

## Acessibilidade
_Teclado, foco, leitor de tela, contraste, telas pequenas._

## Componentes e tokens
_O que se reaproveita do projeto e o que é novo, com o motivo._
`,
	"memory": `_Só o que outro voo vai precisar, que vale além de uma tarefa e que não está óbvio no código nem nas outras NOTAMs._

## Estado atual

## Decisões

## Aprendizados

## Riscos em aberto

## Passagens de serviço
_Quem saiu, o que fez e o que ficou pendente._
`,
}
