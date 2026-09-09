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
    // Each subscription accumulates into its own local buffer, scoped to this
    // effect run, rather than appending onto whatever `state.lines` already
    // holds. A re-subscribe to the same runId (say, the Api instance changes)
    // would otherwise double up on the lines `subscribeAction` replays.
    let lines: ActionLine[] = [];
    let run: ActionRun | null = null;
    return api.subscribeAction(
      runId,
      (line) => {
        lines = [...lines, line];
        setState({ runId, lines, run });
      },
      (finished) => {
        run = finished;
        setState({ runId, lines, run });
      },
    );
  }, [api, runId]);

  return { lines: state.lines, run: state.run };
}
