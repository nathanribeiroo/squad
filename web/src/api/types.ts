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
  file: string;
  locked?: boolean;
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
