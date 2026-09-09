import { Link } from 'react-router-dom';
import { AppShell } from '@/components/shell/AppShell';

export function NotFoundPage() {
  return (
    <AppShell breadcrumb={[{ label: 'Not found' }]}>
      <h1 className="text-2xl font-semibold">There is nothing at this address</h1>
      <p className="mt-2 text-muted-foreground">
        Check the link, or go back to{' '}
        <Link to="/machines" className="text-primary underline">
          machines
        </Link>
        .
      </p>
    </AppShell>
  );
}
