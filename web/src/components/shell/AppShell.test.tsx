import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { AppShell } from '@/components/shell/AppShell';

test('the rail names every section for the keyboard and screen readers', () => {
  renderWithProviders(<AppShell breadcrumb={[{ label: 'Machines' }]}>body</AppShell>);
  for (const name of ['Machines', 'Users', 'Roles', 'Settings']) {
    expect(screen.getByRole('link', { name })).toBeInTheDocument();
  }
});

test('the breadcrumb shows the trail with the organisation first', () => {
  renderWithProviders(
    <AppShell breadcrumb={[{ label: 'Machines', href: '/machines' }, { label: 'ctpi01' }]}>
      body
    </AppShell>,
  );
  const nav = screen.getByRole('navigation', { name: 'Breadcrumb' });
  expect(nav).toHaveTextContent('Conner Technology');
  expect(nav).toHaveTextContent('ctpi01');
});

test('the theme switch toggles the dark class on the root', async () => {
  renderWithProviders(<AppShell breadcrumb={[]}>body</AppShell>);
  await userEvent.click(screen.getByRole('button', { name: 'Switch to dark theme' }));
  expect(document.documentElement).toHaveClass('dark');
});

test('the rail orders after the content on small screens and before it from sm up', () => {
  renderWithProviders(<AppShell breadcrumb={[]}>body</AppShell>);
  const rail = screen.getByRole('navigation', { name: 'Sections' });
  expect(rail).toHaveClass('order-last');
  expect(rail).toHaveClass('sm:order-first');
});
