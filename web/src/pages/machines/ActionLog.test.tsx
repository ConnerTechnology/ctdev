import { render, screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { ActionLog } from '@/pages/machines/ActionLog';
import type { ActionRun } from '@/api/types';

const failedRun: ActionRun = {
  id: 'run-1',
  machineId: 'ctpi01',
  kind: 'backup',
  command: 'ctdev backup now',
  requestedBy: 'thomas@example.invalid',
  startedAt: '2026-09-09T12:00:00Z',
  finishedAt: '2026-09-09T12:00:05Z',
  state: 'failed',
  exitCode: 1,
};

test('with no runs it says nothing has run yet', () => {
  render(<ActionLog runs={[]} selectedId={null} onSelect={() => {}} />);
  expect(screen.getByText('Nothing has run on this machine yet.')).toBeInTheDocument();
});

test('a failed run shows its exit code and clicking its row selects it', async () => {
  const user = userEvent.setup();
  const onSelect = vi.fn();
  render(<ActionLog runs={[failedRun]} selectedId={null} onSelect={onSelect} />);
  expect(screen.getByText('Failed, exit 1')).toBeInTheDocument();
  await user.click(screen.getByText('ctdev backup now'));
  expect(onSelect).toHaveBeenCalledWith('run-1');
});
