import { Routes, Route, Navigate } from 'react-router-dom';
import { RequireSession } from '@/session/SessionContext';
import { SignInPage } from '@/pages/SignInPage';
import { NotFoundPage } from '@/pages/NotFoundPage';
import { MachinesPage } from '@/pages/machines/MachinesPage';

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/machines" replace />} />
      <Route path="/sign-in" element={<SignInPage />} />
      <Route
        path="/machines"
        element={
          <RequireSession>
            <MachinesPage />
          </RequireSession>
        }
      />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
