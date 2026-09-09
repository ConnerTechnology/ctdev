import { render, screen } from '@testing-library/react';
import { Sparkline } from '@/components/Sparkline';

test('a sparkline is an image with a name and one point per value', () => {
  const { container } = render(<Sparkline values={[10, 20, 15]} label="CPU, last 24 samples" />);
  expect(screen.getByRole('img', { name: 'CPU, last 24 samples' })).toBeInTheDocument();
  expect(container.querySelector('polyline')?.getAttribute('points')?.split(' ')).toHaveLength(3);
});
