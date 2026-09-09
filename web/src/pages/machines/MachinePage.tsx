import { useState } from 'react';
import { useParams } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useApi } from '@/api/context';
import type { ActionKind } from '@/api/types';
import { useSession } from '@/session/SessionContext';
import { AppShell } from '@/components/shell/AppShell';
import { Console } from '@/components/Console';
import { useActionStream } from '@/hooks/useActionStream';
import { MachineHeader } from './MachineHeader';
import { StatusTiles } from './StatusTiles';
import { ActionLog } from './ActionLog';
import { DoctorSection } from './DoctorSection';

export function MachinePage() {
  const { id = '' } = useParams();
  const api = useApi();
  const { user } = useSession();
  const queryClient = useQueryClient();
  const [selectedRunId, setSelectedRunId] = useState<string | null>(null);

  const machine = useQuery({
    queryKey: ['machine', id],
    queryFn: () => api.getMachine(id),
    refetchInterval: 5_000,
  });
  const groups = useQuery({ queryKey: ['groups'], queryFn: () => api.listGroups() });
  const runs = useQuery({
    queryKey: ['actions', id],
    queryFn: () => api.listActions(id),
    refetchInterval: 5_000,
  });
  const doctor = useQuery({ queryKey: ['doctor', id], queryFn: () => api.listDoctorRuns(id) });
  const stream = useActionStream(selectedRunId);

  const run = useMutation({
    mutationFn: (kind: ActionKind) => api.runAction(id, kind, user?.email ?? 'unknown'),
    onSuccess: (started) => {
      setSelectedRunId(started.id);
      void queryClient.invalidateQueries({ queryKey: ['machine', id] });
      void queryClient.invalidateQueries({ queryKey: ['actions', id] });
    },
  });

  if (machine.isPending)
    return (
      <AppShell breadcrumb={[{ label: 'Machines', href: '/machines' }, { label: id }]}>
        Loading
      </AppShell>
    );
  if (!machine.data) {
    return (
      <AppShell breadcrumb={[{ label: 'Machines', href: '/machines' }, { label: id }]}>
        <h1 className="text-2xl font-semibold">No machine called {id}</h1>
      </AppShell>
    );
  }

  const m = machine.data;
  const groupName = groups.data?.find((g) => g.id === m.groupId)?.name ?? m.groupId;
  const selected = runs.data?.find((r) => r.id === selectedRunId) ?? null;
  const liveRun = stream.run ?? selected;

  return (
    <AppShell breadcrumb={[{ label: 'Machines', href: '/machines' }, { label: m.name }]}>
      <MachineHeader machine={m} group={groupName} onRun={(kind) => run.mutate(kind)} />
      {run.error && (
        <p role="alert" className="mb-4 text-sm text-danger">
          {run.error.message}
        </p>
      )}
      <StatusTiles machine={m} latestDoctor={doctor.data?.[0] ?? null} />
      <div className="mb-8">
        <Console run={liveRun} lines={stream.lines} />
      </div>
      <div className="grid gap-8 lg:grid-cols-2">
        <DoctorSection runs={doctor.data ?? []} />
        <ActionLog runs={runs.data ?? []} selectedId={selectedRunId} onSelect={setSelectedRunId} />
      </div>
    </AppShell>
  );
}
