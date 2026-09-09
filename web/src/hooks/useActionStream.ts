import { useEffect, useState } from 'react';
import { useApi } from '@/api/context';
import type { ActionLine, ActionRun } from '@/api/types';

interface StreamState {
  runId: string | null;
  lines: ActionLine[];
  run: ActionRun | null;
}

export function useActionStream(runId: string | null) {
  const api = useApi();
  const [state, setState] = useState<StreamState>({ runId, lines: [], run: null });

  // Reset when the selected run changes. This runs during render (React's
  // documented way to adjust state on a prop change) rather than in an effect,
  // so switching runs doesn't show a stale frame of the previous run's lines.
  if (state.runId !== runId) {
    setState({ runId, lines: [], run: null });
  }

  useEffect(() => {
    if (!runId) return;
    return api.subscribeAction(
      runId,
      (line) =>
        setState((prev) =>
          prev.runId === runId ? { ...prev, lines: [...prev.lines, line] } : prev,
        ),
      (finished) => setState((prev) => (prev.runId === runId ? { ...prev, run: finished } : prev)),
    );
  }, [api, runId]);

  return { lines: state.lines, run: state.run };
}
