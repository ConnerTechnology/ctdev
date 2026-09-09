import { render, screen } from '@testing-library/react';
import { StatusTiles } from '@/pages/machines/StatusTiles';
import type { DoctorRun, Machine } from '@/api/types';

const machine: Machine = {
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
  lastHeartbeat: '2026-09-09T12:00:00Z',
  pendingUpdates: 0,
  enrolledAt: '2026-09-05T12:00:00Z',
  cpu: [10, 12, 11],
  currentActionId: null,
};

const doctorRun: DoctorRun = {
  id: 'dr-1',
  machineId: 'ai-node',
  at: '2026-09-09T11:00:00Z',
  verdicts: [],
  checks: [
    { id: 'dns.resolvers', name: 'DNS servers', outcome: 'pass', detail: 'ok' },
    { id: 'disk.pressure', name: 'Disk pressure', outcome: 'warn', detail: '/ at 81%' },
  ],
};

test('with no doctor run it says so', () => {
  render(<StatusTiles machine={machine} latestDoctor={null} />);
  expect(screen.getByText('Never run')).toBeInTheDocument();
  expect(screen.getByText('Run it from the header')).toBeInTheDocument();
});

test('a busy machine with a doctor run on record shows both', () => {
  render(
    <StatusTiles machine={{ ...machine, currentActionId: 'run-1' }} latestDoctor={doctorRun} />,
  );
  expect(screen.getByText('Running')).toBeInTheDocument();
  expect(screen.getByText('1 pass, 1 not')).toBeInTheDocument();
});
