import { useState } from 'react';
import type { CheckOutcome, DoctorRun } from '@/api/types';
import { relativeTime } from '@/lib/format';
import { cn } from '@/lib/utils';

const outcome: Record<CheckOutcome, { label: string; dot: string }> = {
  pass: { label: 'Pass', dot: 'bg-healthy' },
  warn: { label: 'Warn', dot: 'bg-attention' },
  fail: { label: 'Fail', dot: 'bg-danger' },
  skipped: { label: 'Skipped', dot: 'bg-muted-foreground' },
};

function worst(run: DoctorRun): CheckOutcome {
  if (run.checks.some((c) => c.outcome === 'fail')) return 'fail';
  if (run.checks.some((c) => c.outcome === 'warn')) return 'warn';
  return 'pass';
}

export function DoctorSection({ runs }: { runs: DoctorRun[] }) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const run = runs.find((r) => r.id === selectedId) ?? runs[0] ?? null;

  return (
    <section>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">Doctor</h2>
        {runs.length > 1 && (
          <div className="flex items-center gap-1" role="group" aria-label="History">
            {runs.map((r) => (
              <button
                key={r.id}
                type="button"
                aria-label={`Run ${relativeTime(r.at)}, ${outcome[worst(r)].label}`}
                aria-pressed={r.id === run?.id}
                onClick={() => setSelectedId(r.id)}
                className={cn(
                  'h-4 w-4 rounded-full border-2 border-transparent p-[3px] focus-visible:outline-2 focus-visible:outline-ring',
                  r.id === run?.id && 'border-foreground',
                )}
              >
                <span className={cn('block h-full w-full rounded-full', outcome[worst(r)].dot)} />
              </button>
            ))}
          </div>
        )}
      </div>
      {!run ? (
        <p className="rounded-md border bg-card px-4 py-8 text-center text-sm text-muted-foreground">
          Doctor has not run on this machine. Run it from the header.
        </p>
      ) : (
        <div className="rounded-md border bg-card">
          <div className="border-b px-4 py-3 text-sm">
            <div className="text-muted-foreground">{relativeTime(run.at)}</div>
            {run.verdicts.length === 0 ? (
              <p className="mt-1">No verdicts. Everything passed.</p>
            ) : (
              run.verdicts.map((v) => (
                <p key={v} className="mt-1 font-medium">
                  {v}
                </p>
              ))
            )}
          </div>
          <ul aria-label="Checks" className="divide-y">
            {run.checks.map((c) => (
              <li key={c.id} className="flex items-center gap-3 px-4 py-2 text-sm">
                <span
                  aria-hidden
                  className={cn('h-2 w-2 shrink-0 rounded-full', outcome[c.outcome].dot)}
                />
                <span className="w-32 shrink-0">{c.name}</span>
                <span className="w-14 shrink-0 text-muted-foreground">
                  {outcome[c.outcome].label}
                </span>
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
                  {c.detail}
                </span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
