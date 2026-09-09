import { render, screen } from '@testing-library/react';
import { App } from '@/App';

test('the app renders its name', () => {
  render(<App />);
  expect(screen.getByRole('heading', { name: 'ctdev' })).toBeInTheDocument();
});
