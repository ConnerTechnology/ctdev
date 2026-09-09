import { render, screen } from '@testing-library/react';
import { Console } from '@/components/Console';
import type { ActionLine, ActionRun } from '@/api/types';

const run: ActionRun = {
  id: 'r1',
  machineId: 'ctpi01',
  kind: 'doctor',
  command: 'ctdev doctor',
  requestedBy: 'thomas@example.invalid',
  startedAt: '2026-09-09T12:00:00Z',
  finishedAt: null,
  state: 'running',
  exitCode: null,
};
const lines: ActionLine[] = [
  { runId: 'r1', seq: 0, at: '2026-09-09T12:00:01Z', stream: 'stdout', text: 'dns.resolvers pass' },
  {
    runId: 'r1',
    seq: 1,
    at: '2026-09-09T12:00:02Z',
    stream: 'stderr',
    text: 'warning: disk at 81%',
  },
];

test('shows the command, each line, and that it is running', () => {
  render(<Console run={run} lines={lines} />);
  const log = screen.getByRole('log', { name: 'ctdev doctor output' });
  expect(log).toHaveTextContent('dns.resolvers pass');
  expect(screen.getByText('warning: disk at 81%')).toHaveClass('text-attention');
  expect(screen.getByText('Running')).toBeInTheDocument();
});

test('a finished run shows its exit and duration', () => {
  render(
    <Console
      run={{ ...run, finishedAt: '2026-09-09T12:01:05Z', state: 'failed', exitCode: 1 }}
      lines={lines}
    />,
  );
  expect(screen.getByText('Failed, exit 1')).toBeInTheDocument();
  expect(screen.getByText('01:05')).toBeInTheDocument();
});

test('with no run it invites an action', () => {
  render(<Console run={null} lines={[]} />);
  expect(screen.getByText('Run an action to see its output here.')).toBeInTheDocument();
});
