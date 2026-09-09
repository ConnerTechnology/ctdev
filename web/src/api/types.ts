export type Platform = 'linux' | 'macos' | 'windows';
export type MachineStatus = 'healthy' | 'attention' | 'offline';

export interface Group {
  id: string;
  name: string;
}

export interface Machine {
  id: string;
  name: string;
  groupId: string;
  platform: Platform;
  arch: 'arm64' | 'amd64';
  agentVersion: string;
  profile: string | null;
  /** components whose installed state differs from the profile */
  drift: number;
  componentCount: number;
  status: MachineStatus;
  /** ISO timestamp of the last heartbeat; older than 2 minutes means offline */
  lastHeartbeat: string;
  pendingUpdates: number;
  enrolledAt: string;
  /** last 24 CPU samples, percent */
  cpu: number[];
  /** the action currently running, if any */
  currentActionId: string | null;
}

export type ActionKind = 'doctor' | 'update' | 'status' | 'backup';
export type ActionState = 'running' | 'succeeded' | 'failed';

export interface ActionRun {
  id: string;
  machineId: string;
  kind: ActionKind;
  command: string;
  requestedBy: string;
  startedAt: string;
  finishedAt: string | null;
  state: ActionState;
  exitCode: number | null;
}

export interface ActionLine {
  runId: string;
  seq: number;
  at: string;
  stream: 'stdout' | 'stderr';
  text: string;
}

export type CheckOutcome = 'pass' | 'warn' | 'fail' | 'skipped';

export interface DoctorCheck {
  id: string;
  name: string;
  outcome: CheckOutcome;
  detail: string;
}

export interface DoctorRun {
  id: string;
  machineId: string;
  at: string;
  checks: DoctorCheck[];
  /** plain-English verdicts from the correlation engine */
  verdicts: string[];
}

export interface Permission {
  /** tool-prefixed, e.g. "ctdev:doctor.run" */
  id: string;
  area: 'machines' | 'actions' | 'users' | 'roles';
  label: string;
  destructive: boolean;
}

export interface Role {
  id: string;
  name: string;
  description: string;
  builtIn: boolean;
  permissionIds: string[];
}

export type Scope = { kind: 'organisation' } | { kind: 'group'; groupId: string };

export interface Grant {
  roleId: string;
  scope: Scope;
}

export interface User {
  id: string;
  email: string;
  name: string;
  /** a connertechnology.io Workspace account */
  staff: boolean;
  invitedAt: string;
  lastSignIn: string | null;
  grants: Grant[];
}

export interface InviteInput {
  email: string;
  roleId: string;
  scope: Scope;
}

export interface Api {
  listGroups(): Promise<Group[]>;
  listMachines(): Promise<Machine[]>;
  getMachine(id: string): Promise<Machine | null>;
  listActions(machineId: string): Promise<ActionRun[]>;
  runAction(machineId: string, kind: ActionKind, requestedBy: string): Promise<ActionRun>;
  /** delivers every line so far, then new ones; returns an unsubscribe */
  subscribeAction(
    runId: string,
    onLine: (line: ActionLine) => void,
    onDone: (run: ActionRun) => void,
  ): () => void;
  listDoctorRuns(machineId: string): Promise<DoctorRun[]>;
  listUsers(): Promise<User[]>;
  inviteUser(input: InviteInput): Promise<User>;
  listRoles(): Promise<Role[]>;
  listPermissions(): Promise<Permission[]>;
  saveRole(role: Role): Promise<Role>;
}
