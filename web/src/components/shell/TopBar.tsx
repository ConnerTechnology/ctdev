import { Link } from 'react-router-dom';
import { Moon, Sun } from 'lucide-react';
import type { ReactNode } from 'react';
import { Button } from '@/components/ui/button';
import { useTheme } from './AppShell';

export interface Crumb {
  label: string;
  href?: string;
}

export function TopBar({ breadcrumb, actions }: { breadcrumb: Crumb[]; actions?: ReactNode }) {
  const { theme, setTheme } = useTheme();
  const dark = theme === 'dark';
  const trail: Crumb[] = [{ label: 'Conner Technology', href: '/machines' }, ...breadcrumb];
  return (
    <header className="flex h-12 items-center justify-between border-b bg-card px-6">
      <nav aria-label="Breadcrumb">
        <ol className="flex items-center gap-2 text-sm">
          {trail.map((c, i) => (
            <li key={`${c.label}-${i}`} className="flex items-center gap-2">
              {i > 0 && (
                <span aria-hidden className="text-muted-foreground">
                  /
                </span>
              )}
              {c.href && i < trail.length - 1 ? (
                <Link to={c.href} className="text-muted-foreground hover:text-foreground">
                  {c.label}
                </Link>
              ) : (
                <span aria-current={i === trail.length - 1 ? 'page' : undefined}>{c.label}</span>
              )}
            </li>
          ))}
        </ol>
      </nav>
      <div className="flex items-center gap-2">
        {actions}
        <Button
          variant="ghost"
          size="icon"
          aria-label={dark ? 'Switch to light theme' : 'Switch to dark theme'}
          onClick={() => setTheme(dark ? 'light' : 'dark')}
        >
          {dark ? (
            <Sun className="h-4 w-4" aria-hidden />
          ) : (
            <Moon className="h-4 w-4" aria-hidden />
          )}
        </Button>
      </div>
    </header>
  );
}
