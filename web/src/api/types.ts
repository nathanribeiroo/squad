// Mirrors of the Go server types (internal/store, internal/server, agentbus).

export type NodeKind = 'terminal' | 'note' | 'portal';
export type AgentType = 'claude' | 'codex' | 'devin' | 'gemini' | 'shell' | 'custom';
export type UIState = 'boot' | 'idle' | 'work' | 'wait' | 'ask' | 'done' | 'err' | 'off';

/** An answer the RESPONDER bar sends (internal/attention). */
export type AttentionChoice = 'allow' | 'always' | 'deny';

/** What a flight asks the user (activity "ask", PRIORIDADE). */
export interface Attention {
  kind: 'permission' | 'question';
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
}

export interface PortalData {
  url: string;
  /** How agents refer to it (torre portal "Name" …). */
  name?: string;
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
}

/** An agent's last turn (internal/summary): its final message, and a summary on demand. */
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
}

export interface StateExtra {
  paused: boolean;
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
