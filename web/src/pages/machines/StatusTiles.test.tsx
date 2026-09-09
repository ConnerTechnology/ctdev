import { render, screen } from '@testing-library/react';
import { StatusTiles } from '@/pages/machines/StatusTiles';
import type { Machine } from '@/api/types';

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

test('with no doctor run it says so', () => {
  render(<StatusTiles machine={machine} latestDoctor={null} />);
  expect(screen.getByText('Never run')).toBeInTheDocument();
  expect(screen.getByText('Run it from the header')).toBeInTheDocument();
});

test('a healthy, up-to-date machine reads that way', () => {
  render(<StatusTiles machine={machine} latestDoctor={null} />);
  expect(screen.getByText('Healthy')).toBeInTheDocument();
  expect(screen.getByText('Up to date')).toBeInTheDocument();
  expect(screen.getByText('Nothing pending')).toBeInTheDocument();
});
