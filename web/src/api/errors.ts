// The API's errors in Portuguese, by code (internal/apierr). The server's
// message stays in English for agents and logs; an error without a known
// code shows that message.

type Params = Record<string, string>;

const MESSAGES: Record<string, (p: Params) => string> = {
  'dir.required': () => 'Escolha uma pasta.',
  'dir.not_found': (p) => `Pasta não encontrada: ${p.folder}`,

  'workspace.name_required': () => 'O workspace precisa de um nome.',
  'workspace.last': () => 'O último workspace não pode ser removido.',
  'workspace.has_copies': () =>
    'O workspace tem cópias do repositório (níveis de voo ou sessões numa cópia). Pouse ou descarte os níveis e remova essas sessões antes.',

  'session.name_required': () => 'A sessão precisa de um nome.',
  'session.name_too_long': (p) => `O nome da sessão tem no máximo ${p.max} caracteres.`,
  'session.name_line': () => 'O nome da sessão é uma linha só.',
  'session.already_open': () => 'Essa sessão já está aberta.',
  'session.remove_open': () => 'A sessão aberta não pode ser removida: abra outra antes.',
  'session.has_levels': () => 'A sessão tem níveis de voo: abra-a e pouse ou descarte os níveis antes.',

  'flight.name_required': () => 'O voo precisa de um nome.',
  'flight.name_taken': (p) => `Já existe um voo chamado "${p.name}".`,
  'flight.callsign_taken': (p) => `Outro voo já tem o indicativo ${p.callsign}.`,

  'notam.name_taken': (p) => `Já existe uma NOTAM chamada "${p.name}".`,
  'notam.name_alias': (p) => `"${p.name}" já é um nome da NOTAM "${p.note}".`,

  'plan.name_required': () => 'O plano precisa de um nome.',
  'plan.name_too_long': (p) => `O nome do plano tem no máximo ${p.max} caracteres.`,
  'plan.empty': () => 'Não há nada para guardar: selecione voos, NOTAMs ou portais.',
  'plan.too_big': (p) => `Um plano guarda no máximo ${p.max} itens.`,
  'plan.too_many_levels': (p) => `Um plano guarda no máximo ${p.max} níveis.`,
  'plan.note_too_long': (p) => `A NOTAM "${p.note}" é grande demais para o plano: salve sem o conteúdo das NOTAMs.`,
  'plan.bad_agent': (p) => `O plano tem um agente desconhecido: ${p.agent}.`,
  'plan.bad_item': () => 'O arquivo do plano está incompleto ou corrompido.',
  'plan.bad_file': () => 'Esse arquivo não é um plano de voo da Torre.',
  'plan.bad_scope': () => 'Escolha o que guardar: a seleção, o nível ou a sessão.',
  'plan.review': () => 'Revise os comandos do plano antes de decolar.',
  'plan.no_level': () => 'Esse nível não existe mais.',

  'role.name_required': () => 'O papel precisa de um nome.',
  'role.name_too_long': (p) => `O nome do papel tem no máximo ${p.max} caracteres.`,
  'role.prompt_too_long': (p) => `As instruções têm no máximo ${p.max} caracteres.`,
  'role.name_taken': (p) => `Já existe um papel chamado "${p.name}".`,
  'role.abbr_too_long': () => 'A sigla tem no máximo 3 letras.',
  'role.abbr_taken': (p) => `O papel "${p.role}" já usa a sigla ${p.abbr}.`,
  'role.in_use': (p) => `O papel está em uso por ${p.names}: dê outro papel a eles antes.`,
  'role.builtin': () => 'Papéis prontos não podem ser removidos (use "Restaurar padrão").',

  'level.name_required': () => 'O nível precisa de um nome.',
  'level.name_too_long': (p) => `O nome do nível tem no máximo ${p.max} caracteres.`,
  'level.name_ground': () => 'Esse nome é o da pista: escolha outro.',
  'level.name_letter': () => 'O nome do nível precisa de uma letra ou um número.',
  'level.name_taken': (p) => `Já existe um nível chamado "${p.name}".`,
  'level.drive_root': () => 'Os níveis ficam ao lado do repositório, que não pode estar na raiz de um disco.',
  'level.folder_exists': (p) => `A pasta ${p.folder} já existe (sobrou de um nível antigo?): apague-a ou escolha outro nome.`,
  'level.setup_running': () => 'O preparo do nível ainda está rodando: espere terminar (ou descarte o nível).',
  'level.off_branch': (p) => `A pasta do nível não está no branch ${p.branch}: volte para ele antes de pousar.`,
  'level.leaked': (p) =>
    `O branch do nível tem commits de arquivos copiados da pista, que podem ter segredos: ${p.files}. Tire-os do branch (git rm --cached e commit) antes de mesclar.`,

  'copy.none': () => 'Esta sessão trabalha na pasta do projeto: não tem cópia nem branch próprio para pousar.',
  'copy.drive_root': () => 'A cópia fica ao lado do repositório, que não pode estar na raiz de um disco.',
  'copy.branch_letter': () => 'O nome do branch precisa de uma letra ou um número.',
  'copy.folder_exists': (p) => `A pasta ${p.folder} já existe (sobrou de uma sessão antiga?): apague-a ou escolha outro branch.`,
  'copy.setup_running': () => 'O preparo da cópia ainda está rodando: espere terminar.',
  'copy.gone': (p) => `A cópia da sessão sumiu (${p.folder}).`,
  'copy.off_branch': (p) => `A cópia não está no branch ${p.branch}: volte para ele antes de pousar.`,
  'copy.leaked': (p) =>
    `O branch da sessão tem commits de arquivos copiados da pasta do projeto, que podem ter segredos: ${p.files}. Tire-os do branch (git rm --cached e commit) antes de mesclar.`,

  'git.missing': () => 'O git não está instalado (ou não está no PATH).',
  'git.not_repo': () => 'A pasta do workspace não está num repositório git (os níveis de voo são cópias de um).',
  'git.no_commit': () => 'O repositório ainda não tem commits: a cópia parte do último commit.',
  'git.bad_branch': (p) => `Nome de branch inválido: "${p.branch}".`,
  'git.branch_exists': (p) => `Já existe um branch "${p.branch}": escolha outro nome.`,
  'git.no_branch': (p) => `Não existe o branch "${p.branch}".`,
  'git.no_target': (p) => `Não existe o branch "${p.branch}" para mesclar.`,
  'git.merge_self': (p) => `${p.branch} não pode ser mesclado nele mesmo.`,
  'git.detached': () => 'Começou de um HEAD solto: diga em qual branch mesclar.',
  'git.in_progress': (p) => `Há um ${p.op} em andamento: termine ou aborte antes.`,
  'git.target_in_progress': (p) => `${p.folder} tem um ${p.op} em andamento: termine ou aborte antes.`,

  'request.gone': () => 'Esse pedido já terminou.',
  'attention.none': () => 'O voo não está perguntando nada agora.',
  'attention.changed': () => 'A pergunta na tela mudou: abra a tela do voo para responder.',
  'attention.no_keys': () => 'Essa resposta não pode ser enviada daqui para este agente: abra a tela do voo.',
  'attention.not_agent': () => 'Só os agentes respondem por aqui.',

  'limits.range': (p) => `Esse limite vai de ${p.min} a ${p.max}.`,
  'agents.bad_command': () => 'O comando é uma linha só, com até 300 caracteres.',

  'summary.no_turn': () => 'Este voo ainda não terminou um turno.',
  'summary.no_engine': () => 'Nenhuma CLI de agente para escrever resumos (claude, devin, codex ou gemini) foi achada, ou os resumos estão desligados.',
};

/** The Portuguese for an error body ({error, code, params}), else its message. */
export function errorMessage(body: Record<string, unknown>, fallback: string): string {
  const code = typeof body.code === 'string' ? body.code : '';
  const fn = MESSAGES[code];
  if (fn) return fn((body.params as Params | undefined) ?? {});
  return typeof body.error === 'string' ? body.error : fallback;
}
