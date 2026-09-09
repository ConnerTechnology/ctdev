import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

async function signInAs(name: RegExp) {
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name }));
}

test('lists every machine with status, group, heartbeat and pending updates', async () => {
  renderWithProviders(<App />, { route: '/machines' });
  await signInAs(/Thomas Conner/);
  const table = await screen.findByRole('table', { name: 'Machines' });
  const rows = within(table).getAllByRole('row').slice(1);
  expect(rows).toHaveLength(4);
  const pi = rows.find((r) => within(r).queryByText('ctpi01'))!;
  expect(within(pi).getByText('Needs attention')).toBeInTheDocument();
  expect(within(pi).getByText('Home')).toBeInTheDocument();
  expect(within(pi).getByText('3 updates')).toBeInTheDocument();
});

test('a viewer scoped to one group sees only that group', async () => {
  renderWithProviders(<App />, { route: '/machines' });
  await signInAs(/Family member/);
  const table = await screen.findByRole('table', { name: 'Machines' });
  expect(within(table).getAllByRole('row').slice(1)).toHaveLength(2);
  expect(within(table).queryByText('ai-node')).not.toBeInTheDocument();
});

test('filtering by status narrows the list', async () => {
  renderWithProviders(<App />, { route: '/machines' });
  await signInAs(/Thomas Conner/);
  await screen.findByRole('table', { name: 'Machines' });
  await userEvent.click(screen.getByRole('button', { name: 'Needs attention' }));
  expect(
    within(screen.getByRole('table', { name: 'Machines' }))
      .getAllByRole('row')
      .slice(1),
  ).toHaveLength(1);
});
