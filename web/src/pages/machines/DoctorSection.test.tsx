import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DoctorSection } from '@/pages/machines/DoctorSection';
import { doctorRuns } from '@/fake/fixtures';

const runs = doctorRuns.filter((r) => r.machineId === 'ctpi01');

test('shows the verdict, then every check of the latest run with its outcome', () => {
  render(<DoctorSection runs={runs} />);
  expect(
    screen.getByText('Disk is filling: / is at 81%. Run cleanup or grow the volume.'),
  ).toBeInTheDocument();
  const list = screen.getByRole('list', { name: 'Checks' });
  expect(within(list).getAllByRole('listitem')).toHaveLength(8);
  expect(within(list).getByText('Disk pressure').closest('li')).toHaveTextContent('Warn');
  expect(screen.getByText('1 day ago').closest('div')).toHaveTextContent('Warn');
  const historyButton = screen.getByRole('button', { name: /2 days ago/ });
  expect(historyButton.getAttribute('title')).toMatch(/Pass/);
});

test('history lets you open an earlier run', async () => {
  render(<DoctorSection runs={runs} />);
  await userEvent.click(screen.getByRole('button', { name: /2 days ago/ }));
  expect(screen.getByText('/ at 74%')).toBeInTheDocument();
  expect(screen.getByText('No verdicts. Everything passed.')).toBeInTheDocument();
});

test('with no runs it says how to get one', () => {
  render(<DoctorSection runs={[]} />);
  expect(
    screen.getByText('Doctor has not run on this machine. Run it from the header.'),
  ).toBeInTheDocument();
});
