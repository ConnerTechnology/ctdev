import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import '@/styles/index.css';
import { App } from '@/App';
import { ThemeProvider } from '@/components/shell/AppShell';
import { ApiProvider } from '@/api/context';
import { FakeApi } from '@/fake/api';

const client = new QueryClient();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <ApiProvider api={new FakeApi()}>
        <ThemeProvider>
          <BrowserRouter>
            <App />
          </BrowserRouter>
        </ThemeProvider>
      </ApiProvider>
    </QueryClientProvider>
  </StrictMode>,
);
