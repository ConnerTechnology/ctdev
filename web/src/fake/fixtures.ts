import type { DoctorRun, Group, Machine, Permission, Role, User } from '@/api/types';

export const NOW = new Date();
const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString();
const min = 60_000;
const hour = 60 * min;
const day = 24 * hour;

export const groups: Group[] = [
  { id: 'home', name: 'Home' },
  { id: 'office', name: 'Office' },
];

const wave = (base: number, amp: number, n = 24) =>
  Array.from({ length: n }, (_, i) => Math.round(base + amp * Math.sin(i / 3) + ((i * 7) % 5)));

export const machines: Machine[] = [
  {
    id: 'ai-node',
    name: 'ai-node',
    groupId: 'office',
    platform: 'linux',
    arch: 'amd64',
    agentVersion: '12.23.0',
    profile: 'ai-node',
    drift: 0,
    componentCount: 11,
    status: 'healthy',
    lastHeartbeat: ago(20_000),
    pendingUpdates: 0,
    enrolledAt: ago(3 * day),
    cpu: wave(12, 6),
    currentActionId: null,
  },
  {
    id: 'ctpi01',
    name: 'ctpi01',
    groupId: 'home',
    platform: 'linux',
    arch: 'arm64',
    agentVersion: '12.23.0',
    profile: 'pihole-node',
    drift: 0,
    componentCount: 14,
    status: 'attention',
    lastHeartbeat: ago(8_000),
    pendingUpdates: 3,
    enrolledAt: ago(4 * day),
    cpu: wave(18, 9),
    currentActionId: null,
  },
  {
    id: 'thomas-desktop-linux',
    name: 'thomas-desktop-linux',
    groupId: 'home',
    platform: 'linux',
    arch: 'amd64',
    agentVersion: '12.22.0',
    profile: 'dev-workstation',
    drift: 2,
    componentCount: 31,
    status: 'healthy',
    lastHeartbeat: ago(41_000),
    pendingUpdates: 7,
    enrolledAt: ago(4 * day),
    cpu: wave(35, 20),
    currentActionId: null,
  },
  {
    id: 'thomass-macbook-air',
    name: 'thomass-macbook-air',
    groupId: 'office',
    platform: 'macos',
    arch: 'arm64',
    agentVersion: '12.22.0',
    profile: 'dev-workstation',
    drift: 1,
    componentCount: 27,
    status: 'offline',
    lastHeartbeat: ago(6 * hour),
    pendingUpdates: 2,
    enrolledAt: ago(2 * day),
    cpu: wave(8, 4),
    currentActionId: null,
  },
];

const check = (
  id: string,
  name: string,
  outcome: DoctorRun['checks'][number]['outcome'],
  detail: string,
) => ({ id, name, outcome, detail });

export const doctorRuns: DoctorRun[] = [
  {
    id: 'dr-ctpi01-1',
    machineId: 'ctpi01',
    at: ago(1 * day),
    verdicts: ['Disk is filling: / is at 81%. Run cleanup or grow the volume.'],
    checks: [
      check('dns.resolvers', 'DNS servers', 'pass', 'Pi-hole on 127.0.0.1, fallback 9.9.9.9'),
      check('dns.dnssec', 'DNSSEC', 'pass', 'dnssec-failed.org returns SERVFAIL'),
      check('dns.roots', 'Root reach', 'pass', 'a.root-servers.net answered authoritatively'),
      check('disk.pressure', 'Disk pressure', 'warn', '/ at 81%'),
      check('thermal', 'Temperature', 'pass', '52 °C'),
      check('docker.containers', 'Containers', 'pass', '6 running, 0 unhealthy'),
      check('backup.timer', 'Backup timer', 'pass', 'last snapshot 6 hours ago'),
      check('security.ssh', 'SSH hardening', 'pass', 'password auth off'),
    ],
  },
  {
    id: 'dr-ctpi01-2',
    machineId: 'ctpi01',
    at: ago(2 * day),
    verdicts: [],
    checks: [
      check('dns.resolvers', 'DNS servers', 'pass', 'Pi-hole on 127.0.0.1, fallback 9.9.9.9'),
      check('dns.dnssec', 'DNSSEC', 'pass', 'dnssec-failed.org returns SERVFAIL'),
      check('dns.roots', 'Root reach', 'pass', 'a.root-servers.net answered authoritatively'),
      check('disk.pressure', 'Disk pressure', 'pass', '/ at 74%'),
      check('thermal', 'Temperature', 'pass', '50 °C'),
      check('docker.containers', 'Containers', 'pass', '6 running, 0 unhealthy'),
      check('backup.timer', 'Backup timer', 'pass', 'last snapshot 5 hours ago'),
      check('security.ssh', 'SSH hardening', 'pass', 'password auth off'),
    ],
  },
  {
    id: 'dr-desktop-1',
    machineId: 'thomas-desktop-linux',
    at: ago(1 * day),
    verdicts: [],
    checks: [
      check('dns.resolvers', 'DNS servers', 'pass', 'ctpi01 via Tailscale'),
      check('disk.pressure', 'Disk pressure', 'pass', '/ at 43%'),
      check('memory.cgroup', 'Memory cgroup', 'pass', 'memory controller present'),
      check('security.ssh', 'SSH hardening', 'skipped', 'no SSH server'),
    ],
  },
];

export const permissions: Permission[] = [
  {
    id: 'ctdev:machine.view',
    area: 'machines',
    label: 'See machines, status, inventory and history',
    destructive: false,
  },
  {
    id: 'ctdev:machine.enroll',
    area: 'machines',
    label: 'Add a machine to a group',
    destructive: false,
  },
  { id: 'ctdev:machine.revoke', area: 'machines', label: 'Revoke a machine', destructive: true },
  { id: 'ctdev:doctor.run', area: 'actions', label: 'Run doctor', destructive: false },
  { id: 'ctdev:status.run', area: 'actions', label: 'Refresh status', destructive: false },
  { id: 'ctdev:update.run', area: 'actions', label: 'Install updates', destructive: false },
  { id: 'ctdev:backup.run', area: 'actions', label: 'Run a backup now', destructive: false },
  {
    id: 'ctdev:machine.uninstall',
    area: 'actions',
    label: 'Uninstall components',
    destructive: true,
  },
  { id: 'ctdev:restore.inplace', area: 'actions', label: 'Restore in place', destructive: true },
  { id: 'ctdev:user.view', area: 'users', label: 'See users and their roles', destructive: false },
  {
    id: 'ctdev:user.invite',
    area: 'users',
    label: 'Invite users and grant roles you hold',
    destructive: false,
  },
  { id: 'ctdev:user.remove', area: 'users', label: 'Remove a user', destructive: true },
  { id: 'ctdev:role.view', area: 'roles', label: 'See roles', destructive: false },
  {
    id: 'ctdev:role.edit',
    area: 'roles',
    label: 'Create and edit custom roles',
    destructive: false,
  },
];

const ids = (...list: string[]) => list;
export const roles: Role[] = [
  {
    id: 'owner',
    name: 'Owner',
    description: 'Everything, including users, roles and destructive actions.',
    builtIn: true,
    permissionIds: permissions.map((p) => p.id),
  },
  {
    id: 'admin',
    name: 'Admin',
    description: 'Manage machines and grants in scope; no destructive actions.',
    builtIn: true,
    permissionIds: ids(
      'ctdev:machine.view',
      'ctdev:machine.enroll',
      'ctdev:doctor.run',
      'ctdev:status.run',
      'ctdev:update.run',
      'ctdev:backup.run',
      'ctdev:user.view',
      'ctdev:user.invite',
      'ctdev:role.view',
    ),
  },
  {
    id: 'operator',
    name: 'Operator',
    description: 'Run the actions that change a machine.',
    builtIn: true,
    permissionIds: ids(
      'ctdev:machine.view',
      'ctdev:doctor.run',
      'ctdev:status.run',
      'ctdev:update.run',
      'ctdev:backup.run',
    ),
  },
  {
    id: 'viewer',
    name: 'Viewer',
    description: 'Read only.',
    builtIn: true,
    permissionIds: ids('ctdev:machine.view', 'ctdev:user.view', 'ctdev:role.view'),
  },
];

export const users: User[] = [
  {
    id: 'u-thomas',
    email: 'thomas@example.invalid',
    name: 'Thomas Conner',
    staff: true,
    invitedAt: ago(4 * day),
    lastSignIn: ago(10 * min),
    grants: [{ roleId: 'owner', scope: { kind: 'organisation' } }],
  },
  {
    id: 'u-family',
    email: 'family-member@example.invalid',
    name: 'Family member',
    staff: false,
    invitedAt: ago(2 * day),
    lastSignIn: ago(1 * day),
    grants: [{ roleId: 'viewer', scope: { kind: 'group', groupId: 'home' } }],
  },
  {
    id: 'u-contractor',
    email: 'contractor@example.invalid',
    name: 'Contractor',
    staff: false,
    invitedAt: ago(1 * day),
    lastSignIn: null,
    grants: [{ roleId: 'operator', scope: { kind: 'group', groupId: 'office' } }],
  },
];
