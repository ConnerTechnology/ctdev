import { useEffect, type ReactNode } from 'react';
import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { ApiProvider } from '@/api/context';
import { FakeApi } from '@/fake/api';
import { SessionProvider, useSession } from '@/session/SessionContext';
import { MachineHeader } from '@/pages/machines/MachineHeader';
import type { Machine } from '@/api/types';

const busyMachine: Machine = {
  id: 'ctpi01',
  name: 'ctpi01',
  groupId: 'home',
  platform: 'linux',
  arch: 'arm64',
  agentVersion: '12.23.0',
  profile: 'pihole-node',
  drift: 0,
  componentCount: 14,
  status: 'attention',
  lastHeartbeat: '2026-09-09T12:00:00Z',
  pendingUpdates: 3,
  enrolledAt: '2026-09-05T12:00:00Z',
  cpu: [10, 12, 11],
  currentActionId: 'run-1',
};

// Signs in as the given email before rendering its children, so a component
// that reads useSession() sees a real, granted user rather than the signed-out
// default — mirrors the SessionProvider tests' pattern but keeps everything in
// one render tree so MachineHeader shares the same context instance.
function SignedInAs({ email, children }: { email: string; children: ReactNode }) {
  const { signIn, user } = useSession();
  useEffect(() => {
    void signIn(email);
  }, [signIn, email]);
  return user ? <>{children}</> : null;
}

function renderAsOwner(machine: Machine) {
  return render(
    <ApiProvider api={new FakeApi()}>
      <SessionProvider>
        <MemoryRouter>
          <SignedInAs email="thomas@example.invalid">
            <MachineHeader machine={machine} group="Home" onRun={() => {}} />
          </SignedInAs>
        </MemoryRouter>
      </SessionProvider>
    </ApiProvider>,
  );
}

test('both action buttons disable while the machine is busy', async () => {
  renderAsOwner(busyMachine);
  expect(await screen.findByRole('button', { name: 'Run doctor' })).toBeDisabled();
  expect(screen.getByRole('button', { name: 'Actions' })).toBeDisabled();
});
