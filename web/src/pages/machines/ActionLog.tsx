import type { ActionRun } from '@/api/types';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { duration, relativeTime } from '@/lib/format';
import { cn } from '@/lib/utils';

export function ActionLog({
  runs,
  selectedId,
  onSelect,
}: {
  runs: ActionRun[];
  selectedId: string | null;
  onSelect: (id: string) => void;
}) {
  return (
    <section>
      <h2 className="mb-3 text-lg font-semibold">Actions</h2>
      <div className="rounded-md border bg-card">
        <Table aria-label="Actions">
          <TableHeader>
            <TableRow>
              <TableHead>Command</TableHead>
              <TableHead>Result</TableHead>
              <TableHead>Requested by</TableHead>
              <TableHead>Started</TableHead>
              <TableHead className="text-right">Took</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.length === 0 && (
              <TableRow>
                <TableCell colSpan={5} className="py-8 text-center text-muted-foreground">
                  Nothing has run on this machine yet.
                </TableCell>
              </TableRow>
            )}
            {runs.map((r) => (
              <TableRow
                key={r.id}
                className={cn('cursor-pointer', r.id === selectedId && 'bg-accent')}
                onClick={() => onSelect(r.id)}
              >
                <TableCell className="font-mono text-xs">{r.command}</TableCell>
                <TableCell
                  className={cn(
                    r.state === 'failed' && 'text-danger',
                    r.state === 'succeeded' && 'text-healthy',
                  )}
                >
                  {r.state === 'running'
                    ? 'Running…'
                    : r.state === 'succeeded'
                      ? 'Succeeded'
                      : `Failed, exit ${r.exitCode}`}
                </TableCell>
                <TableCell className="font-mono text-xs">{r.requestedBy}</TableCell>
                <TableCell>{relativeTime(r.startedAt)}</TableCell>
                <TableCell className="text-right font-mono text-xs">
                  {duration(r.startedAt, r.finishedAt)}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}
