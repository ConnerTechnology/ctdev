import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { Rail } from './Rail';
import { TopBar, type Crumb } from './TopBar';

type Theme = 'light' | 'dark' | 'system';
const ThemeContext = createContext<{ theme: Theme; setTheme: (t: Theme) => void } | null>(null);

function prefersDark() {
  return (
    typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches
  );
}

export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setTheme] = useState<Theme>(() => {
    try {
      return (localStorage.getItem('ctdev.theme') as Theme | null) ?? 'system';
    } catch {
      return 'system';
    }
  });
  useEffect(() => {
    const dark = theme === 'dark' || (theme === 'system' && prefersDark());
    document.documentElement.classList.toggle('dark', dark);
    try {
      localStorage.setItem('ctdev.theme', theme);
    } catch {
      /* private mode: the choice just does not persist */
    }
  }, [theme]);
  return <ThemeContext.Provider value={{ theme, setTheme }}>{children}</ThemeContext.Provider>;
}

export function useTheme() {
  const ctx = useContext(ThemeContext);
  if (!ctx) throw new Error('useTheme needs ThemeProvider');
  return ctx;
}

export function AppShell({
  breadcrumb,
  actions,
  children,
}: {
  breadcrumb: Crumb[];
  actions?: ReactNode;
  children: ReactNode;
}) {
  return (
    <div className="flex min-h-svh flex-col sm:flex-row">
      <Rail />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar breadcrumb={breadcrumb} actions={actions} />
        <main className="mx-auto w-full max-w-[1280px] flex-1 px-6 py-6">{children}</main>
      </div>
    </div>
  );
}
