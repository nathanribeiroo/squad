// Mirrors of the Go server types (internal/store, internal/server, agentbus).

export type NodeKind = 'terminal' | 'note' | 'portal' | 'group' | 'drawing';
export type AgentType = 'claude' | 'codex' | 'devin' | 'gemini' | 'shell' | 'custom';
export type UIState = 'boot' | 'idle' | 'work' | 'wait' | 'ask' | 'done' | 'err' | 'off';

/** An answer the RESPONDER bar sends (internal/attention). */
export type AttentionChoice = 'allow' | 'always' | 'deny';

/** What a flight asks the user (activity "ask", PRIORIDADE). */
/** One of the agent's own options on a question or a plan. */
export interface AttentionOption {
  /** Its number: what picks it. */
  key: string;
  /** Its first line ("Azul", "Yes, manually approve edits"). */
  text: string;
  /** The line under it: its description. */
  detail?: string;
  /** The text field: the user writes their own answer. */
  input?: boolean;
}

export interface Attention {
  kind: 'permission' | 'question' | 'plan';
  /** Bash, Edit, WebFetch, mcp__… (from the agent's hook). */
  tool?: string;
  /** The agent's question on screen ("Do you want to proceed?"). */
  text?: string;
  /** What is at stake: the command, the file. */
  detail?: string;
  source: 'hook' | 'screen';
  since: string;
  /** The answers the interface may send; empty: open the terminal. */
  choices?: AttentionChoice[];
  /** The agent's own words for each answer: what "always" saves, and where. */
  labels?: Partial<Record<AttentionChoice, string>>;
  /** On a question or a plan: the agent's options the interface may pick. */
  options?: AttentionOption[];
  /** The plan to approve, whole (Claude Code sends it). */
  plan?: string;
}

export interface TerminalData {
  name: string;
  /** Short code shown with the name (PLN, IMP, IM2…), unique in the canvas. */
  callsign?: string;
  agent: AgentType | string;
  command?: string;
  shell?: string;
  cwd?: string;
  /** Id of a role in the library (RoleDef). */
  role?: string;
  /** Manager mode (torre new); unset follows the role. */
  manager?: boolean;
  cols?: number;
  rows?: number;
  /** The agent's id for the conversation the terminal continues (Torre keeps it). */
  conversation?: string;
  /** Nothing was said in it yet: there is nothing to continue. */
  empty?: boolean;
  /** The agent was in the middle of a turn. */
  busy?: boolean;
}

export interface ShellOption {
  path: string;
  label: string;
  default?: boolean;
}

export interface NoteData {
  title: string;
  /** "" while a draft. */
  file: string;
  locked?: boolean;
  /** Made in the UI and not named yet: the name the user confirms names the file. */
  draft?: boolean;
  /** Former names: agents that knew one still find the note. */
  aliases?: string[];
  /** The note at the base of its stack (its own id on the base), and its place in it. */
  stack?: string;
  stackPos?: number;
}

export interface PortalData {
  url: string;
  /** How agents refer to it (torre portal "Name" …). */
  name?: string;
}

/** A frame around nodes, for the user's eyes only (internal/store/visual.go). */
export interface GroupData {
  name: string;
  /** A color key: a role color (plan, impl…) or none. */
  color?: string;
  /** Shown as a card the node's size; members are the nodes it hides, openW × openH its frame. */
  collapsed?: boolean;
  members?: string[];
  openW?: number;
  openH?: number;
}

export type DrawingShape = 'pen' | 'arrow' | 'rect' | 'ellipse' | 'text';

/** A stroke, an arrow, a shape or a text on the canvas (internal/store/visual.go). */
export interface DrawingData {
  shape: DrawingShape;
  /** pen and arrow: 0–1 across the node's box (resizing scales the drawing). */
  points?: [number, number][];
  color?: string;
  text?: string;
}

export interface WNode {
  id: string;
  kind: NodeKind;
  x: number;
  y: number;
  w: number;
  h: number;
  terminal?: TerminalData;
  note?: NoteData;
  portal?: PortalData;
  group?: GroupData;
  drawing?: DrawingData;
  /** The terminal whose agent created this node (torre new, torre portal create). */
  createdBy?: string;
  /** The floor the node is on (a Floor id); absent on the ground floor. */
  floor?: string;
}

/** An isolated copy of the repository (git worktree) with its own canvas. */
export interface Floor {
  id: string;
  name: string;
  /** The flight level: FL1, FL2…, in creation order, never given again. */
  number?: number;
  branch: string;
  /** Branch it started from (landing merges there by default). */
  base?: string;
  baseCommit: string;
  path: string;
  cwd: string;
  newBranch?: boolean;
  copied?: string[];
  status: 'preparing' | 'ready' | 'failed';
  error?: string;
  createdBy?: string;
  createdAt: string;
  /** From GET /api/floors and floors.updated. */
  terminals?: number;
  missing?: boolean;
  /** What its agents marked (torre floor mark), by kind. */
  checks?: Partial<Record<CheckKind, FloorCheck>>;
}

export type CheckKind = 'tests' | 'review' | 'security' | 'design';

/** A floor's tests or review, as an agent marked them on a commit. */
export interface FloorCheck {
  result: 'pass' | 'fail';
  note?: string;
  /** The floor's commit when marked: a later one makes it stale. */
  commit: string;
  by: string;
  byName: string;
  callsign?: string;
  at: string;
}

/** How new floors are seeded: untracked files to copy and a setup command. */
export interface FloorConfig {
  copy: string[];
  setup?: string;
}

/** A floor's (or the ground floor's) git state, for the selector. */
export interface FloorGit {
  branch: string;
  changes: number;
  ahead?: number;
  missing?: boolean;
  /** Floors: the commit their checks are compared with. */
  head?: string;
}

/** GET /api/floors/status. */
export interface FloorStatusResponse {
  repo: boolean;
  ground: FloorGit;
  floors: Record<string, FloorGit>;
}

/** GET /api/floors/{id}/changes: what landing would take. */
export interface FloorChanges {
  files: { path: string; status: string }[];
  ahead: number;
  behind: number;
  commits: { hash: string; subject: string }[];
  missing?: boolean;
  /** Files a merge into the base would conflict in; null when git can't tell. */
  conflicts?: string[] | null;
  /** Permission rules the agents saved here that the ground floor lacks. */
  rules: PermissionRule[];
}

/** A rule of an agent's permissions file (.claude/settings.local.json, .devin/config.local.json). */
export interface PermissionRule {
  file: string;
  kind: 'allow' | 'ask' | 'deny';
  rule: string;
}

/** An agent's last turn (internal/summary): its final message, and a summary on demand. */
/** A line of the Rádio, kept by the server (internal/radio): data the interface words. */
export interface RadioItem {
  key: string;
  time: string;
  outcome: 'pending' | 'replied' | 'cancelled' | 'delivered' | 'held' | 'note' | 'turn' | 'answered' | 'check';
  id?: string;
  from?: string;
  to?: string;
  fromName?: string;
  toName?: string;
  text?: string;
  elapsedMs?: number;
  source?: string;
  /** held: the general hold, the per-minute limit, or a loop (text: the loop). */
  hold?: 'paused' | 'rate' | 'loop';
  /** "user", or "external" (a note edited outside Torre). */
  by?: 'user' | 'external';
  images?: number;
  /** An ask the user answered: what it was, and the interface's answer. */
  kind?: 'permission' | 'question' | 'plan';
  tool?: string;
  detail?: string;
  choice?: AttentionChoice | 'option';
  /** On a question or a plan: what was picked ("Azul · Submit answers"), or the user's own text. */
  answer?: string;
  /** The reply's text is kept (GET /api/requests/{id}). */
  hasReply?: boolean;
  /** A pending request waiting for its target's turn to end. */
  queued?: boolean;
  /** check: what a flight marked on a floor (toName), and the result. */
  check?: CheckKind;
  result?: 'pass' | 'fail';
  number?: number;
}

export interface TurnCard {
  terminal: string;
  start?: string;
  at: string;
  /** "hook": the agent's final message; "screen": its screen's last lines. */
  source: 'hook' | 'screen';
  prompt?: string;
  message: string;
  summary?: { text: string; steps: string[]; by: string; at: string };
  summarizing?: boolean;
  error?: string;
}

/** Tokens a conversation used (internal/usage). */
export interface UsageTokens {
  /** Without the cache. */
  input: number;
  output: number;
  cacheRead: number;
  cacheWrite: number;
}

/** What one flight's conversation used so far (GET /api/usage, "usage.flight"). */
export interface UsageFlight {
  id: string;
  agent: string;
  tokens: UsageTokens;
  /** Claude Code's estimate at API prices (a subscription isn't charged it). */
  costUsd?: number;
  /** What Devin charged: Agent Compute Units, or credits. */
  acus?: number;
  credits?: number;
  context?: { used: number; size: number; percent: number };
  models?: string[];
  at: string;
}

/** A plan limit: five_hour and seven_day (Claude, Codex), daily and weekly (Devin). */
export interface UsageWindow {
  kind: 'five_hour' | 'seven_day' | 'daily' | 'weekly';
  /** The share used, 0–100. */
  used: number;
  resetsAt?: string;
}

/** An agent account's plan limits, as last seen; the same in every workspace ("usage.account"). */
export interface UsageAccount {
  agent: string;
  windows?: UsageWindow[];
  acus?: { used: number; limit?: number };
  seen: string;
}

export type SummaryEngine = 'auto' | 'claude' | 'devin' | 'codex' | 'gemini' | 'off';

/** GET /api/summary/settings. */
export interface SummarySettings {
  cards: boolean;
  engine: SummaryEngine;
  engines: { name: string; path?: string; available: boolean }[];
}

/** POST /api/summary/workspace. */
export interface WorkspaceOverview {
  text: string;
  terminals: { name: string; status: string }[];
  attention: string[];
  by: string;
  at: string;
}

export interface LandResult {
  floor: string;
  branch: string;
  merged: boolean;
  into?: string;
  commit?: string;
  upToDate?: boolean;
  /** How many permission rules went to the ground floor. */
  permissions?: number;
}

export interface WEdge {
  id: string;
  a: string;
  b: string;
  ropePoints?: [number, number][];
  createdAt: string;
}

export interface Workspace {
  id: string;
  name: string;
  cwd: string;
  color?: string;
  nodes: WNode[];
  edges: WEdge[];
  floors?: Floor[];
  floorConfig?: FloorConfig;
  /** The last flight level given (FL1, FL2…); the next floor is one more. */
  floorSeq?: number;
  /** The session works in a copy of the repository, on a branch of its own
   * (internal/floors copy.go); its terminals and floors start there. */
  copy?: Floor;
}

export interface TerminalRuntime {
  id: string;
  status: 'starting' | 'running' | 'exited' | 'stopped' | 'failed';
  exitCode: number;
  title?: string;
  error?: string;
  activity: UIState;
  activitySince: string;
  /** Continued a conversation it was in the middle of (Torre closed or restarted). */
  interrupted?: boolean;
  /** What it asks the user (activity "ask"). */
  attention?: Attention;
  /** Its last turn was the user's and ended: it waits for them (shown while "done"). */
  yourTurn?: boolean;
}

export interface PendingRequest {
  id: string;
  from: string;
  to: string;
  fromName: string;
  toName: string;
  preview: string;
  hop: number;
  sentAt: string;
  /** It waits in the target's line for the target's turn to end. */
  queued?: boolean;
}

/** A flight of a loop's chain (agentbus.LoopStop). */
export interface LoopStop {
  id: string;
  name: string;
  callsign?: string;
}

/** Why conversations are on hold: the user's Espera geral, or a loop past the hop limit. */
export interface Pause {
  by: 'user' | 'loop';
  /** The loop: the first asker first, the refused request's target last. */
  chain?: LoopStop[];
  hops?: number;
  limit?: number;
  at: string;
}

export interface StateExtra {
  paused: boolean;
  pause?: Pause | null;
  limits: { maxHops: number; perMinute: number };
  rate: number;
  pending: PendingRequest[];
}

export interface StateResponse {
  workspace: Workspace;
  runtime: Record<string, TerminalRuntime>;
  extra?: StateExtra;
}

export interface ServerEvent<T = any> {
  type: string;
  time: string;
  data: T;
}

/** A role of the library (internal/roles), shared by every workspace. */
export interface RoleDef {
  id: string;
  name: string;
  /** Three letters (PLN, IMP…): the callsign of its flights. */
  abbr?: string;
  /** The instructions the agent receives. */
  prompt: string;
  /** When a leader should call a flight with this role (torre roles). */
  when?: string;
  /** --r-<color> / --rb-<color> */
  color: string;
  icon: string;
  /** Its terminals may create and connect terminals (torre new). */
  manager?: boolean;
  builtin?: boolean;
  /** A built-in role that differs from its default. */
  edited?: boolean;
}

/** A workspace as the selector shows it (GET /api/workspaces, /ws/workspaces). */
export interface WorkspaceSummary {
  id: string;
  name: string;
  cwd: string;
  color?: string;
  createdAt: string;
  terminals: number;
  /** Agents working or waiting for a reply. */
  active: number;
  /** Requests waiting for a reply. */
  pending: number;
  /** Flights asking the user (PRIORIDADE). */
  attention?: number;
  /** The open session (internal/hub sessions.go). */
  session: Session;
  /** How many sessions the workspace has. */
  sessions: number;
  /** Its agents run (active), are starting (loading) or are stopped
   * (inactive): a workspace off screen stops them (internal/hub sleep.go). */
  state?: WorkspaceState;
  /** While loading: how many of the agents it is starting are up. */
  loading?: { ready: number; total: number };
  /** Agents stopped in the middle of a turn, until they continue. */
  interrupted?: number;
  /** Its agents never stop off screen. */
  keepActive?: boolean;
}

export type WorkspaceState = 'active' | 'loading' | 'inactive';

/** When workspaces off screen stop their agents (Configurações → Workspaces). */
export interface WorkspaceSettings {
  /** Never: every workspace starts its agents with Torre. */
  keepAll: boolean;
  /** How long one off screen, with nothing going on, keeps them. */
  idleMinutes: number;
}

/** A session: a whole canvas of a workspace; only the open one runs. */
export interface Session {
  id: string;
  name: string;
  /** Named by the user (or by the agent); otherwise "Nova sessão · 17/10 14:32". */
  named?: boolean;
  createdAt: string;
  openedAt: string;
}

/** A session as the sessions panel shows it. */
export interface SessionSummary extends Session {
  current: boolean;
  terminals: number;
  notes: number;
  portals: number;
  floors: number;
  /** Agent types of its terminals (claude, devin…). */
  agents: string[];
  /** A session working in a copy: its branch and the copy's folder. */
  branch?: string;
  copy?: string;
}

/** GET /api/workspaces/{id}/sessions, newest first; also the "sessions" event. */
export interface SessionList {
  workspace: string;
  current: string;
  sessions: SessionSummary[];
}

/** GET /api/agents: an agent found (or not) on this computer. */
export interface AgentInfo {
  agent: 'claude' | 'codex' | 'devin' | 'gemini';
  found: boolean;
  version?: string;
  path?: string;
  defaultCommand: string;
  /** The user changed the command. */
  custom: boolean;
  installHint?: string;
}

export interface AgentsDetection {
  agents: AgentInfo[];
  browser: { found: boolean; path?: string };
}

/** A workspace's conversation limits (GET/PUT /w/<id>/api/limits). */
export interface Limits {
  maxHops: number;
  perMinute: number;
  maxTeam: number;
  portalsPerAgent: number;
}

export interface LimitsView {
  limits: Limits;
  defaults: Limits;
}

/** GET /api/dirs/info: a folder, for the first open (step 2, Pasta). */
export interface DirInfo {
  path: string;
  exists: boolean;
  git: boolean;
  branch?: string;
  commits: boolean;
  changes: number;
  envFiles: string[];
  /** The workspaces' folders. */
  recent?: string[];
}

/** A flight plan (internal/plans): a saved team, positions relative to the plan. */
export interface PlanBox {
  x: number;
  y: number;
  w: number;
  h: number;
}
export interface PlanFlight extends PlanBox {
  key: string;
  name: string;
  callsign?: string;
  agent: string;
  role?: string;
  manager?: boolean;
  /** Its own command line (runs as written: reviewed before it flies). */
  command?: string;
  cwd?: string;
  level?: string;
}
export interface PlanNote extends PlanBox {
  key: string;
  title: string;
  content?: string;
  level?: string;
}
export interface PlanPortal extends PlanBox {
  key: string;
  name: string;
  url?: string;
  level?: string;
}
export interface PlanGroup extends PlanBox {
  key: string;
  name: string;
  color?: string;
  level?: string;
}
export interface PlanDrawing extends PlanBox, DrawingData {
  key: string;
  level?: string;
}
export interface Plan {
  id: string;
  name: string;
  createdAt: string;
  /** Came from a file. */
  imported?: boolean;
  flights: PlanFlight[];
  notes: PlanNote[];
  portals: PlanPortal[];
  links: [string, string][];
  levels: { key: string; name: string; branch?: string }[];
  groups: PlanGroup[];
  drawings: PlanDrawing[];
}
/** What putting a plan in the air made. */
export interface PlanResult {
  nodes: string[];
  floors: string[];
  warnings: string[];
}
