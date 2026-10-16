// Mirrors of the Go server types (internal/store, internal/server, agentbus).

export type NodeKind = 'terminal' | 'note' | 'portal';
export type AgentType = 'claude' | 'codex' | 'devin' | 'gemini' | 'shell' | 'custom';
export type UIState = 'boot' | 'idle' | 'work' | 'wait' | 'done' | 'err' | 'off';

export interface TerminalData {
  name: string;
  agent: AgentType | string;
  command?: string;
  shell?: string;
  cwd?: string;
  /** Id of a role in the library (RoleDef). */
  role?: string;
  /** Manager mode (squad new); unset follows the role. */
  manager?: boolean;
  cols?: number;
  rows?: number;
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
  /** How agents refer to it (squad portal "Name" …). */
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
  /** The terminal whose agent created this node (squad new, squad portal create). */
  createdBy?: string;
  /** The floor the node is on (a Floor id); absent on the ground floor. */
  floor?: string;
}

/** An isolated copy of the repository (git worktree) with its own canvas. */
export interface Floor {
  id: string;
  name: string;
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
}

export interface TerminalRuntime {
  id: string;
  status: 'starting' | 'running' | 'exited' | 'stopped' | 'failed';
  exitCode: number;
  title?: string;
  error?: string;
  activity: UIState;
  activitySince: string;
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
  /** The instructions the agent receives. */
  prompt: string;
  /** --r-<color> / --rb-<color> */
  color: string;
  icon: string;
  /** Its terminals may create and connect terminals (squad new). */
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
}
