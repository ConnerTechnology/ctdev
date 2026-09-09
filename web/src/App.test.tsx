import { screen } from '@testing-library/react';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

test('an unknown address shows the not-found page', () => {
  renderWithProviders(<App />, { route: '/nowhere' });
  expect(
    screen.getByRole('heading', { name: 'There is nothing at this address' }),
  ).toBeInTheDocument();
});
