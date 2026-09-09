import { Routes, Route, Navigate } from 'react-router-dom';
import { RequireSession } from '@/session/SessionContext';
import { SignInPage } from '@/pages/SignInPage';
import { NotFoundPage } from '@/pages/NotFoundPage';
import { AppShell } from '@/components/shell/AppShell';

function MachinesPlaceholder() {
  return (
    <AppShell breadcrumb={[{ label: 'Machines' }]}>
      <h1 className="text-2xl font-semibold">Machines</h1>
    </AppShell>
  );
}

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/machines" replace />} />
      <Route path="/sign-in" element={<SignInPage />} />
      <Route
        path="/machines"
        element={
          <RequireSession>
            <MachinesPlaceholder />
          </RequireSession>
        }
      />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
