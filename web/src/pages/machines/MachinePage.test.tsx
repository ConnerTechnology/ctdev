import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

async function signInAs(name: RegExp) {
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name }));
}

test('shows the machine header and status tiles', async () => {
  renderWithProviders(<App />, { route: '/machines/ctpi01' });
  await signInAs(/Thomas Conner/);
  expect(await screen.findByRole('heading', { name: 'ctpi01' })).toBeInTheDocument();
  expect(screen.getByText('Home · linux/arm64 · agent 12.23.0')).toBeInTheDocument();
  const tiles = screen.getByRole('list', { name: 'Status' });
  expect(within(tiles).getByText('Needs attention')).toBeInTheDocument();
  expect(within(tiles).getByText('3 updates')).toBeInTheDocument();
  expect(within(tiles).getByText('0 of 14 drifted')).toBeInTheDocument();
});

test('running doctor streams into the console and lands in the action log', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  vi.useFakeTimers();
  renderWithProviders(<App />, { route: '/machines/ctpi01' });
  await user.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await user.click(screen.getByRole('button', { name: /Thomas Conner/ }));
  await screen.findByRole('heading', { name: 'ctpi01' });
  await user.click(screen.getByRole('button', { name: 'Run doctor' }));
  const tiles = screen.getByRole('list', { name: 'Status' });
  expect(await within(tiles).findByText('Running')).toBeInTheDocument();
  const consoleSection = screen
    .getByRole('log', { name: 'ctdev doctor output' })
    .closest('section');
  expect(consoleSection).not.toBeNull();
  expect(within(consoleSection as HTMLElement).getByText('Running')).toBeInTheDocument();
  expect(
    within(screen.getByRole('table', { name: 'Actions' })).getByText('Running'),
  ).toBeInTheDocument();
  await vi.advanceTimersByTimeAsync(12_000);
  const log = screen.getByRole('log', { name: 'ctdev doctor output' });
  expect(log).toHaveTextContent('8 checks: 7 pass, 1 warn');
  expect(screen.getByText('Succeeded, exit 0')).toBeInTheDocument();
  expect(
    within(screen.getByRole('table', { name: 'Actions' })).getByText('ctdev doctor'),
  ).toBeInTheDocument();
  vi.useRealTimers();
});

test('a viewer cannot run actions', async () => {
  renderWithProviders(<App />, { route: '/machines/ctpi01' });
  await signInAs(/Family member/);
  await screen.findByRole('heading', { name: 'ctpi01' });
  expect(screen.queryByRole('button', { name: 'Run doctor' })).not.toBeInTheDocument();
});

test('an unknown machine says so', async () => {
  renderWithProviders(<App />, { route: '/machines/nope' });
  await signInAs(/Thomas Conner/);
  expect(await screen.findByText('No machine called nope')).toBeInTheDocument();
});
