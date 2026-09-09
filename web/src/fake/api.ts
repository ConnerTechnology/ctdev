import type {
  ActionKind,
  ActionLine,
  ActionRun,
  Api,
  InviteInput,
  Machine,
  Role,
  User,
} from '@/api/types';
import { doctorRuns, groups, machines, permissions, roles, users } from './fixtures';
import { scripts } from './actionScripts';

const clone = <T>(v: T): T => structuredClone(v);
// ms === 0 resolves on a microtask rather than a real/faked timer: reads should
// clear on the next tick regardless of whether the caller's test owns fake timers,
// while the action-script playback (always ms > 0) still rides the fake clock.
const wait = (ms: number) =>
  ms === 0 ? Promise.resolve() : new Promise<void>((r) => setTimeout(r, ms));

interface LiveRun {
  run: ActionRun;
  lines: ActionLine[];
  listeners: Set<{ onLine: (l: ActionLine) => void; onDone: (r: ActionRun) => void }>;
}

/**
 * In-memory Api. State lives for the page's lifetime; a reload resets it.
 * Reads resolve on the next tick so components exercise their loading states.
 */
export class FakeApi implements Api {
  private groups = clone(groups);
  private machines = clone(machines);
  private doctorRuns = clone(doctorRuns);
  private users = clone(users);
  private roles = clone(roles);
  private permissions = clone(permissions);
  private runs = new Map<string, LiveRun>();
  private seq = 0;

  async listGroups() {
    await wait(0);
    return clone(this.groups);
  }
  async listMachines() {
    await wait(0);
    return clone(this.machines);
  }
  async getMachine(id: string) {
    await wait(0);
    return clone(this.machines.find((m) => m.id === id) ?? null);
  }
  async listDoctorRuns(machineId: string) {
    await wait(0);
    return clone(
      this.doctorRuns
        .filter((d) => d.machineId === machineId)
        .sort((a, b) => b.at.localeCompare(a.at)),
    );
  }
  async listUsers() {
    await wait(0);
    return clone(this.users);
  }
  async listRoles() {
    await wait(0);
    return clone(this.roles);
  }
  async listPermissions() {
    await wait(0);
    return clone(this.permissions);
  }

  async listActions(machineId: string) {
    await wait(0);
    return [...this.runs.values()]
      .map((r) => clone(r.run))
      .filter((r) => r.machineId === machineId)
      .sort((a, b) => b.startedAt.localeCompare(a.startedAt));
  }

  async runAction(machineId: string, kind: ActionKind, requestedBy: string): Promise<ActionRun> {
    const machine = this.machines.find((m) => m.id === machineId);
    if (!machine) throw new Error(`no machine ${machineId}`);
    if (machine.currentActionId) throw new Error(`${machine.name} is busy`);
    const script = scripts[kind];
    const run: ActionRun = {
      id: `run-${++this.seq}`,
      machineId,
      kind,
      command: script.command,
      requestedBy,
      startedAt: new Date().toISOString(),
      finishedAt: null,
      state: 'running',
      exitCode: null,
    };
    const live: LiveRun = { run, lines: [], listeners: new Set() };
    this.runs.set(run.id, live);
    machine.currentActionId = run.id;
    void this.play(live, script.steps, script.exitCode, machine);
    return clone(run);
  }

  private async play(
    live: LiveRun,
    steps: (typeof scripts)[ActionKind]['steps'],
    exitCode: number,
    machine: Machine,
  ) {
    for (const [delay, stream, text] of steps) {
      await wait(delay);
      const line: ActionLine = {
        runId: live.run.id,
        seq: live.lines.length,
        at: new Date().toISOString(),
        stream,
        text,
      };
      live.lines.push(line);
      for (const l of live.listeners) l.onLine(clone(line));
    }
    live.run = {
      ...live.run,
      finishedAt: new Date().toISOString(),
      state: exitCode === 0 ? 'succeeded' : 'failed',
      exitCode,
    };
    machine.currentActionId = null;
    if (live.run.kind === 'update' && exitCode === 0) machine.pendingUpdates = 0;
    for (const l of live.listeners) l.onDone(clone(live.run));
  }

  subscribeAction(
    runId: string,
    onLine: (line: ActionLine) => void,
    onDone: (run: ActionRun) => void,
  ) {
    const live = this.runs.get(runId);
    if (!live) throw new Error(`no run ${runId}`);
    for (const line of live.lines) onLine(clone(line));
    if (live.run.state !== 'running') {
      onDone(clone(live.run));
      return () => {};
    }
    const listener = { onLine, onDone };
    live.listeners.add(listener);
    return () => {
      live.listeners.delete(listener);
    };
  }

  async inviteUser(input: InviteInput): Promise<User> {
    await wait(0);
    if (this.users.some((u) => u.email === input.email))
      throw new Error(`${input.email} is already a user`);
    const user: User = {
      id: `u-${this.users.length + 1}`,
      email: input.email,
      name: input.email.split('@')[0] ?? input.email,
      staff: input.email.endsWith('@connertechnology.io'),
      invitedAt: new Date().toISOString(),
      lastSignIn: null,
      grants: [{ roleId: input.roleId, scope: input.scope }],
    };
    this.users.push(user);
    return clone(user);
  }

  async saveRole(role: Role): Promise<Role> {
    await wait(0);
    const existing = this.roles.find((r) => r.id === role.id);
    if (existing?.builtIn) throw new Error(`${existing.name} is built-in; copy it to change it`);
    if (existing) Object.assign(existing, clone(role));
    else this.roles.push(clone({ ...role, builtIn: false }));
    return clone(role);
  }
}
