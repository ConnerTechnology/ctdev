import type { MachineStatus } from '@/api/types';
import { cn } from '@/lib/utils';

const copy: Record<MachineStatus | 'busy', { label: string; className: string }> = {
  healthy: { label: 'Healthy', className: 'bg-healthy' },
  attention: { label: 'Needs attention', className: 'bg-attention' },
  offline: { label: 'Offline', className: 'bg-muted-foreground' },
  busy: { label: 'Running', className: 'bg-primary motion-safe:animate-pulse' },
};

export function StatusDot({ status, label }: { status: MachineStatus | 'busy'; label?: string }) {
  const c = copy[status];
  return (
    <span className="inline-flex items-center gap-2 text-sm">
      <span aria-hidden className={cn('h-2 w-2 rounded-full', c.className)} />
      {label ?? c.label}
    </span>
  );
}
