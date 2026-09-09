import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { ApiProvider } from '@/api/context';
import { FakeApi } from '@/fake/api';
import { useActionStream } from '@/hooks/useActionStream';

// Regression coverage for the duplicate-lines bug: a subscription now
// accumulates into its own local buffer (see useActionStream.ts) instead of
// appending onto whatever state already held, so a run that FakeApi replays
// in full on subscribe (because it's already finished) never produces
// duplicate `seq` values. This exercises the replay path directly; the exact
// same-runId-re-subscribe path the fix also covers isn't independently
// reproducible here (the effect's deps are `[api, runId]`, and neither
// changes on a rerender with the same id), so it's covered by construction —
// the local accumulator makes every subscription self-contained regardless of
// why the effect reran.
test('a finished run replays without duplicate lines', async () => {
  vi.useFakeTimers();
  const api = new FakeApi();
  function wrapper({ children }: { children: ReactNode }) {
    return <ApiProvider api={api}>{children}</ApiProvider>;
  }

  const started = await api.runAction('ctpi01', 'status', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(2_000);

  const { result } = renderHook(() => useActionStream(started.id), { wrapper });
  await waitFor(() => expect(result.current.run?.state).toBe('succeeded'));

  const seqs = result.current.lines.map((l) => l.seq);
  expect(seqs.length).toBeGreaterThan(0);
  expect(new Set(seqs).size).toBe(seqs.length);

  vi.useRealTimers();
});
