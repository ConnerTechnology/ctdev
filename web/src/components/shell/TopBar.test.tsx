import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

test('signing out from the top bar returns to the sign-in screen', async () => {
  renderWithProviders(<App />, { route: '/sign-in' });
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name: /Thomas Conner/ }));
  await screen.findByRole('heading', { name: 'Machines' });

  await userEvent.click(screen.getByRole('button', { name: 'Thomas Conner' }));
  await userEvent.click(await screen.findByRole('menuitem', { name: 'Sign out' }));

  expect(await screen.findByRole('button', { name: 'Continue with Google' })).toBeInTheDocument();
});
