import { Routes, Route, Navigate } from 'react-router-dom';
import { NotFoundPage } from '@/pages/NotFoundPage';

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/machines" replace />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
