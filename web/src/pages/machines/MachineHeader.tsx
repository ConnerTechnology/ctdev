import type { ActionKind, Machine } from '@/api/types';
import { useSession } from '@/session/SessionContext';
import { Button } from '@/components/ui/button';
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu';
import { relativeTime } from '@/lib/format';

const actions: { kind: ActionKind; label: string; permission: string }[] = [
  { kind: 'update', label: 'Install updates', permission: 'ctdev:update.run' },
  { kind: 'status', label: 'Refresh status', permission: 'ctdev:status.run' },
  { kind: 'backup', label: 'Back up now', permission: 'ctdev:backup.run' },
];

export function MachineHeader({
  machine,
  group,
  onRun,
}: {
  machine: Machine;
  group: string;
  onRun: (kind: ActionKind) => void;
}) {
  const { can } = useSession();
  const scope = { kind: 'group', groupId: machine.groupId } as const;
  const busy = machine.currentActionId !== null;
  const allowed = actions.filter((a) => can(a.permission, scope));
  return (
    <div className="mb-6 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="text-[28px] font-semibold leading-tight">{machine.name}</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          {group} · {machine.platform}/{machine.arch} · agent {machine.agentVersion}
        </p>
        <p className="text-sm text-muted-foreground">
          Enrolled {relativeTime(machine.enrolledAt)}, last seen{' '}
          {relativeTime(machine.lastHeartbeat)}
        </p>
      </div>
      <div className="flex gap-2">
        {can('ctdev:doctor.run', scope) && (
          <Button variant="secondary" disabled={busy} onClick={() => onRun('doctor')}>
            Run doctor
          </Button>
        )}
        {allowed.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger render={<Button disabled={busy} />}>Actions</DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {allowed.map((a) => (
                <DropdownMenuItem key={a.kind} onClick={() => onRun(a.kind)}>
                  {a.label}
                </DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </div>
  );
}
