import { NavLink } from 'react-router-dom';
import { Server, Users, ShieldCheck, Settings } from 'lucide-react';
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip';
import { cn } from '@/lib/utils';

const items = [
  { to: '/machines', label: 'Machines', Icon: Server },
  { to: '/users', label: 'Users', Icon: Users },
  { to: '/roles', label: 'Roles', Icon: ShieldCheck },
  { to: '/settings', label: 'Settings', Icon: Settings },
];

export function Rail() {
  return (
    <nav
      aria-label="Sections"
      className="order-last flex shrink-0 items-center gap-1 border-t border-sidebar-border bg-sidebar px-2 py-1 sm:order-first sm:w-14 sm:flex-col sm:border-t-0 sm:border-r sm:py-3"
    >
      <a
        href="/machines"
        className="mb-2 hidden h-8 w-8 items-center justify-center rounded bg-foreground font-display text-sm font-semibold text-background sm:flex"
        aria-label="ctdev home"
      >
        ct
      </a>
      {items.map(({ to, label, Icon }) => (
        <Tooltip key={to}>
          <TooltipTrigger
            render={
              <NavLink
                to={to}
                aria-label={label}
                className={({ isActive }) =>
                  cn(
                    'flex h-10 w-10 items-center justify-center rounded-md text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring',
                    isActive && 'bg-accent text-foreground',
                  )
                }
              />
            }
          >
            <Icon className="h-5 w-5" aria-hidden />
          </TooltipTrigger>
          <TooltipContent side="right">{label}</TooltipContent>
        </Tooltip>
      ))}
    </nav>
  );
}
