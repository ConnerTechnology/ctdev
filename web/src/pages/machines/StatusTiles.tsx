import type { DoctorRun, Machine } from '@/api/types';
import { StatusDot } from '@/components/StatusDot';
import { formatCount, relativeTime } from '@/lib/format';

export function StatusTiles({
  machine,
  latestDoctor,
}: {
  machine: Machine;
  latestDoctor: DoctorRun | null;
}) {
  const pass = latestDoctor?.checks.filter((c) => c.outcome === 'pass').length ?? 0;
  const notPass = latestDoctor ? latestDoctor.checks.length - pass : 0;
  const tiles = [
    {
      title: 'Status',
      value: machine.currentActionId ? (
        <StatusDot status="busy" label="Busy" />
      ) : (
        <StatusDot status={machine.status} />
      ),
      note: `Heartbeat ${relativeTime(machine.lastHeartbeat)}`,
    },
    {
      title: 'Doctor',
      value: latestDoctor ? `${pass} pass, ${notPass} not` : 'Never run',
      note: latestDoctor ? relativeTime(latestDoctor.at) : 'Run it from the header',
    },
    {
      title: 'Updates',
      value:
        machine.pendingUpdates > 0 ? formatCount(machine.pendingUpdates, 'update') : 'Up to date',
      note: machine.pendingUpdates > 0 ? 'Pending' : 'Nothing pending',
    },
    {
      title: 'Profile',
      value: `${machine.drift} of ${machine.componentCount} drifted`,
      note: machine.profile ?? 'No profile',
    },
  ];
  return (
    <ul aria-label="Status" className="mb-6 grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
      {tiles.map((t) => (
        <li key={t.title} className="rounded-md border bg-card px-4 py-3">
          <div className="text-xs text-muted-foreground">{t.title}</div>
          <div className="mt-1 text-base font-medium">{t.value}</div>
          <div className="mt-1 text-xs text-muted-foreground">{t.note}</div>
        </li>
      ))}
    </ul>
  );
}
