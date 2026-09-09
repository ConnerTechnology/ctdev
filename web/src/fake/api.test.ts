import { FakeApi } from '@/fake/api';
import type { ActionLine, ActionRun } from '@/api/types';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

test('the fleet has four machines in two groups', async () => {
  const api = new FakeApi();
  expect(await api.listGroups()).toHaveLength(2);
  const machines = await api.listMachines();
  expect(machines.map((m) => m.name)).toEqual([
    'ai-node',
    'ctpi01',
    'thomas-desktop-linux',
    'thomass-macbook-air',
  ]);
});

test('running doctor streams lines over time and finishes with an exit code', async () => {
  const api = new FakeApi();
  const run = await api.runAction('ctpi01', 'doctor', 'thomas@example.invalid');
  expect(run.state).toBe('running');
  expect((await api.getMachine('ctpi01'))?.currentActionId).toBe(run.id);

  const lines: ActionLine[] = [];
  let done: ActionRun | null = null;
  api.subscribeAction(
    run.id,
    (l) => lines.push(l),
    (r) => (done = r),
  );
  expect(lines).toHaveLength(0);
  await vi.advanceTimersByTimeAsync(400);
  expect(lines.length).toBeGreaterThan(0);
  await vi.advanceTimersByTimeAsync(10_000);
  expect(done).not.toBeNull();
  expect(done!.state).toBe('succeeded');
  expect(done!.exitCode).toBe(0);
  expect((await api.getMachine('ctpi01'))?.currentActionId).toBeNull();
  expect((await api.listActions('ctpi01'))[0]?.id).toBe(run.id);
});

test('a late subscriber receives the lines it missed', async () => {
  const api = new FakeApi();
  const run = await api.runAction('ctpi01', 'status', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(800);
  const lines: ActionLine[] = [];
  api.subscribeAction(
    run.id,
    (l) => lines.push(l),
    () => {},
  );
  expect(lines.length).toBeGreaterThan(0);
  expect(lines.map((l) => l.seq)).toEqual([...lines.keys()]);
});

test('inviting a user adds them with the grant', async () => {
  const api = new FakeApi();
  const before = (await api.listUsers()).length;
  const user = await api.inviteUser({
    email: 'new@example.invalid',
    roleId: 'viewer',
    scope: { kind: 'group', groupId: 'home' },
  });
  expect(user.grants).toEqual([{ roleId: 'viewer', scope: { kind: 'group', groupId: 'home' } }]);
  expect(await api.listUsers()).toHaveLength(before + 1);
});

test('a built-in role cannot be saved, a copy can', async () => {
  const api = new FakeApi();
  const [owner] = await api.listRoles();
  await expect(api.saveRole({ ...owner!, name: 'x' })).rejects.toThrow('built-in');
  const copy = await api.saveRole({
    ...owner!,
    id: 'night-operator',
    name: 'Night operator',
    builtIn: false,
  });
  expect((await api.listRoles()).find((r) => r.id === 'night-operator')).toEqual(copy);
});

test('runAction rejects a second action while the machine is busy', async () => {
  const api = new FakeApi();
  await api.runAction('ctpi01', 'doctor', 'thomas@example.invalid');
  await expect(api.runAction('ctpi01', 'status', 'thomas@example.invalid')).rejects.toThrow(
    'is busy',
  );
});

test('a successful update clears pendingUpdates; a failed backup exits 1 and frees the machine', async () => {
  const api = new FakeApi();
  const update = await api.runAction('ctpi01', 'update', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(20_000);
  expect((await api.listActions('ctpi01')).find((r) => r.id === update.id)?.state).toBe(
    'succeeded',
  );
  expect((await api.getMachine('ctpi01'))?.pendingUpdates).toBe(0);

  const backup = await api.runAction('ctpi01', 'backup', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(20_000);
  const backupRun = (await api.listActions('ctpi01')).find((r) => r.id === backup.id);
  expect(backupRun?.state).toBe('failed');
  expect(backupRun?.exitCode).toBe(1);
  expect((await api.getMachine('ctpi01'))?.currentActionId).toBeNull();
});

test('subscribeAction on a finished run replays every line and calls onDone immediately', async () => {
  const api = new FakeApi();
  const run = await api.runAction('ctpi01', 'status', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(20_000);

  const lines: ActionLine[] = [];
  let done: ActionRun | null = null;
  api.subscribeAction(
    run.id,
    (l) => lines.push(l),
    (r) => (done = r),
  );
  expect(lines.length).toBeGreaterThan(0);
  expect(done).not.toBeNull();
  expect(done!.state).toBe('succeeded');
});

test('unsubscribing mid-stream stops further onLine calls', async () => {
  const api = new FakeApi();
  const run = await api.runAction('ctpi01', 'doctor', 'thomas@example.invalid');
  const lines: ActionLine[] = [];
  const unsubscribe = api.subscribeAction(
    run.id,
    (l) => lines.push(l),
    () => {},
  );
  await vi.advanceTimersByTimeAsync(400);
  const countAtUnsubscribe = lines.length;
  expect(countAtUnsubscribe).toBeGreaterThan(0);
  unsubscribe();
  await vi.advanceTimersByTimeAsync(20_000);
  expect(lines.length).toBe(countAtUnsubscribe);
});

test('inviteUser refuses an email that already exists', async () => {
  const api = new FakeApi();
  await expect(
    api.inviteUser({
      email: 'thomas@example.invalid',
      roleId: 'viewer',
      scope: { kind: 'organisation' },
    }),
  ).rejects.toThrow('already a user');
});

test('listDoctorRuns is newest first per machine, listPermissions has 14, listActions is scoped and newest first', async () => {
  const api = new FakeApi();
  expect((await api.listDoctorRuns('ctpi01')).map((r) => r.id)).toEqual([
    'dr-ctpi01-1',
    'dr-ctpi01-2',
  ]);
  expect(await api.listDoctorRuns('ai-node')).toEqual([]);
  expect(await api.listPermissions()).toHaveLength(14);

  const first = await api.runAction('ctpi01', 'status', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(20_000);
  const onOtherMachine = await api.runAction('ai-node', 'status', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(20_000);
  const second = await api.runAction('ctpi01', 'doctor', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(20_000);

  expect((await api.listActions('ai-node')).map((r) => r.id)).toEqual([onOtherMachine.id]);
  expect((await api.listActions('ctpi01')).map((r) => r.id)).toEqual([second.id, first.id]);
});
