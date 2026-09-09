import { useEffect, useRef, useState } from 'react';
import type { ActionLine, ActionRun } from '@/api/types';
import { duration } from '@/lib/format';
import { cn } from '@/lib/utils';

export function Console({ run, lines }: { run: ActionRun | null; lines: ActionLine[] }) {
  const endRef = useRef<HTMLDivElement>(null);
  const [now, setNow] = useState(() => new Date());
  const [seenRunId, setSeenRunId] = useState(run?.id ?? null);

  // A run selected later than mount would otherwise show elapsed time frozen
  // at whatever `now` was seeded with, until the interval below ticks once.
  // Reset it as soon as the selected run changes (render-time adjustment,
  // same pattern as useActionStream's reset — not an effect).
  if (seenRunId !== (run?.id ?? null)) {
    setSeenRunId(run?.id ?? null);
    setNow(new Date());
  }

  useEffect(() => {
    if (run?.state !== 'running') return;
    const t = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(t);
  }, [run?.state]);

  useEffect(() => {
    endRef.current?.scrollIntoView?.({ block: 'end' });
  }, [lines.length]);

  const state =
    run?.state === 'running'
      ? 'Running'
      : run?.state === 'succeeded'
        ? `Succeeded, exit ${run.exitCode}`
        : run?.state === 'failed'
          ? `Failed, exit ${run.exitCode}`
          : null;

  return (
    <section className="overflow-hidden rounded-md border border-console bg-console font-mono text-[13px] text-console-foreground">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-2">
        <span>
          <span className="text-console-muted">$ </span>
          {run ? run.command : <span className="text-console-muted">ctdev</span>}
        </span>
        {run && (
          <span className="flex items-center gap-3 text-xs">
            <span
              className={cn(
                run.state === 'failed' && 'text-danger',
                run.state === 'succeeded' && 'text-healthy',
              )}
            >
              {state}
            </span>
            <span className="text-console-muted">
              {duration(run.startedAt, run.finishedAt, now)}
            </span>
          </span>
        )}
      </div>
      <div
        role="log"
        aria-live="polite"
        aria-label={`${run?.command ?? 'ctdev'} output`}
        className="max-h-[420px] min-h-[180px] overflow-y-auto px-4 py-3"
      >
        {!run && <p className="text-console-muted">Run an action to see its output here.</p>}
        {lines.map((l) => (
          <div
            key={l.seq}
            className={cn(
              'whitespace-pre-wrap motion-safe:animate-line-in',
              l.stream === 'stderr' && 'text-attention',
            )}
          >
            {l.text || ' '}
          </div>
        ))}
        {run?.state === 'running' && (
          <span
            aria-hidden
            className="inline-block h-4 w-2 bg-console-foreground motion-safe:animate-pulse"
          />
        )}
        <div ref={endRef} />
      </div>
    </section>
  );
}
