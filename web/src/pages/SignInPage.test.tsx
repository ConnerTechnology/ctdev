import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

test('signing in with an invited Google account lands on machines', async () => {
  renderWithProviders(<App />, { route: '/sign-in' });
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name: /Thomas Conner/ }));
  expect(await screen.findByRole('heading', { name: 'Machines' })).toBeInTheDocument();
});

test('an account nobody invited sees why and who to ask', async () => {
  renderWithProviders(<App />, { route: '/sign-in' });
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name: /stranger/ }));
  expect(await screen.findByText(/has not been invited/)).toBeInTheDocument();
});

test('a signed-out visit to machines goes to sign-in', () => {
  renderWithProviders(<App />, { route: '/machines' });
  expect(screen.getByRole('button', { name: 'Continue with Google' })).toBeInTheDocument();
});

test('signing in from a redirected sign-in returns to the page that was guarded', async () => {
  renderWithProviders(<App />, { route: '/machines' });
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name: /Thomas Conner/ }));
  expect(await screen.findByRole('heading', { name: 'Machines' })).toBeInTheDocument();
  const nav = screen.getByRole('navigation', { name: 'Breadcrumb' });
  expect(within(nav).getByText('Machines')).toHaveAttribute('aria-current', 'page');
});
