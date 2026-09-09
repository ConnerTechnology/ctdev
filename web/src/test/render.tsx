import { render, type RenderOptions } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactElement, ReactNode } from 'react';
import { ThemeProvider } from '@/components/shell/AppShell';
import { ApiProvider } from '@/api/context';
import type { Api } from '@/api/types';
import { FakeApi } from '@/fake/api';

export function renderWithProviders(
  ui: ReactElement,
  { route = '/', api = new FakeApi() }: { route?: string; api?: Api } = {},
) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <ApiProvider api={api}>
          <ThemeProvider>
            <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>
          </ThemeProvider>
        </ApiProvider>
      </QueryClientProvider>
    );
  }
  return { ...render(ui, { wrapper: Wrapper } satisfies RenderOptions), api };
}
