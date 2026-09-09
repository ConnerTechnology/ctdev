import { useMemo, useState } from 'react';
import { Link } from 'react-router-dom';
import { useQuery } from '@tanstack/react-query';
import { useApi } from '@/api/context';
import type { Machine, MachineStatus } from '@/api/types';
import { useSession } from '@/session/SessionContext';
import { AppShell } from '@/components/shell/AppShell';
import { StatusDot } from '@/components/StatusDot';
import { Sparkline } from '@/components/Sparkline';
import { Button } from '@/components/ui/button';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table';
import { Skeleton } from '@/components/ui/skeleton';
import { relativeTime, formatCount } from '@/lib/format';
import { cn } from '@/lib/utils';

const filters: { value: MachineStatus | 'all'; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'attention', label: 'Needs attention' },
  { value: 'healthy', label: 'Healthy' },
  { value: 'offline', label: 'Offline' },
];

export function MachinesPage() {
  const api = useApi();
  const { can } = useSession();
  const [filter, setFilter] = useState<MachineStatus | 'all'>('all');
  const machines = useQuery({
    queryKey: ['machines'],
    queryFn: () => api.listMachines(),
    refetchInterval: 15_000,
  });
  const groups = useQuery({ queryKey: ['groups'], queryFn: () => api.listGroups() });

  const visible = useMemo(() => {
    const all = (machines.data ?? []).filter((m) =>
      can('ctdev:machine.view', { kind: 'group', groupId: m.groupId }),
    );
    return filter === 'all' ? all : all.filter((m) => m.status === filter);
  }, [machines.data, filter, can]);

  const groupName = (id: string) => groups.data?.find((g) => g.id === id)?.name ?? id;
  const counts = (machines.data ?? []).reduce<Record<string, number>>(
    (acc, m) => ({ ...acc, [m.status]: (acc[m.status] ?? 0) + 1 }),
    {},
  );

  return (
    <AppShell breadcrumb={[{ label: 'Machines' }]}>
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">Machines</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {machines.data
              ? `${formatCount(machines.data.length, 'machine')}, ${counts['attention'] ?? 0} need attention`
              : 'Loading'}
          </p>
        </div>
        <div className="flex gap-1" role="group" aria-label="Filter by status">
          {filters.map((f) => (
            <Button
              key={f.value}
              size="sm"
              variant={filter === f.value ? 'secondary' : 'ghost'}
              aria-pressed={filter === f.value}
              onClick={() => setFilter(f.value)}
            >
              {f.label}
            </Button>
          ))}
        </div>
      </div>

      <div className="rounded-md border bg-card">
        {machines.isPending ? (
          <div className="space-y-3 p-4" aria-label="Loading machines">
            {[0, 1, 2].map((i) => (
              <Skeleton key={i} className="h-5 w-full" />
            ))}
          </div>
        ) : (
          <Table aria-label="Machines">
            <TableHeader>
              <TableRow>
                <TableHead>Machine</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Group</TableHead>
                <TableHead>Last seen</TableHead>
                <TableHead className="text-right">Updates</TableHead>
                <TableHead>CPU</TableHead>
                <TableHead>Agent</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {visible.map((m) => (
                <MachineRow key={m.id} machine={m} group={groupName(m.groupId)} />
              ))}
              {visible.length === 0 && (
                <TableRow>
                  <TableCell colSpan={7} className="py-10 text-center text-muted-foreground">
                    No machines match this filter.
                  </TableCell>
                </TableRow>
              )}
            </TableBody>
          </Table>
        )}
      </div>
    </AppShell>
  );
}

function MachineRow({ machine: m, group }: { machine: Machine; group: string }) {
  return (
    <TableRow className={cn(m.status === 'offline' && 'text-muted-foreground')}>
      <TableCell>
        <Link to={`/machines/${m.id}`} className="font-display font-semibold hover:underline">
          {m.name}
        </Link>
        <div className="font-mono text-xs text-muted-foreground">
          {m.platform}/{m.arch}
          {m.profile ? ` · ${m.profile}` : ''}
        </div>
      </TableCell>
      <TableCell>
        {m.currentActionId ? <StatusDot status="busy" /> : <StatusDot status={m.status} />}
      </TableCell>
      <TableCell>{group}</TableCell>
      <TableCell>{relativeTime(m.lastHeartbeat)}</TableCell>
      <TableCell className="text-right">
        {m.pendingUpdates > 0 ? (
          formatCount(m.pendingUpdates, 'update')
        ) : (
          <span className="text-muted-foreground">Up to date</span>
        )}
      </TableCell>
      <TableCell>
        <Sparkline values={m.cpu} label={`${m.name} CPU, last 24 samples`} />
      </TableCell>
      <TableCell className="font-mono text-xs">{m.agentVersion}</TableCell>
    </TableRow>
  );
}
