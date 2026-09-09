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
