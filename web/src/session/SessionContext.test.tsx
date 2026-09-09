import { renderHook, act, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { ApiProvider } from '@/api/context';
import { FakeApi } from '@/fake/api';
import { SessionProvider, useSession } from '@/session/SessionContext';

function wrapper({ children }: { children: ReactNode }) {
  return (
    <ApiProvider api={new FakeApi()}>
      <SessionProvider>{children}</SessionProvider>
    </ApiProvider>
  );
}

test('an invited email signs in and holds its grants', async () => {
  const { result } = renderHook(() => useSession(), { wrapper });
  await act(() => result.current.signIn('family-member@example.invalid'));
  await waitFor(() => expect(result.current.user?.name).toBe('Family member'));
  expect(result.current.can('ctdev:machine.view', { kind: 'group', groupId: 'home' })).toBe(true);
  expect(result.current.can('ctdev:machine.view', { kind: 'group', groupId: 'office' })).toBe(
    false,
  );
  expect(result.current.can('ctdev:doctor.run', { kind: 'group', groupId: 'home' })).toBe(false);
});

test('an organisation-wide owner can do everything everywhere', async () => {
  const { result } = renderHook(() => useSession(), { wrapper });
  await act(() => result.current.signIn('thomas@example.invalid'));
  await waitFor(() => expect(result.current.user).not.toBeNull());
  expect(result.current.can('ctdev:restore.inplace', { kind: 'group', groupId: 'office' })).toBe(
    true,
  );
  expect(result.current.can('ctdev:user.invite')).toBe(true);
});

test('an email nobody invited is refused', async () => {
  const { result } = renderHook(() => useSession(), { wrapper });
  await expect(act(() => result.current.signIn('stranger@example.invalid'))).rejects.toThrow(
    'not been invited',
  );
  expect(result.current.user).toBeNull();
});
