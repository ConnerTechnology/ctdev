# ctdev dashboard, first slice, part 1: the screens with fake data — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A React SPA at `web/` in this repo that shows the four first-slice screens (sign-in, machine list, one machine page with a live action stream and doctor results, users and roles) running entirely on a fake in-browser API, so Thomas can review the product before any service exists.

**Architecture:** `web/` is a Vite + React 19 + TypeScript app laid out the way the Foil It Up dashboard is (Tailwind 4, shadcn base components, TanStack Query, react-router, vitest + Testing Library), with one difference that matters: every screen reads from `src/fake/api.ts`, an in-memory implementation of the `Api` interface in `src/api/types.ts`. Part 2 of the first slice replaces `fake/` with the real client; nothing above it changes. The live action stream is real inside the fake: scripted lines arrive over time through a subscription, so the console behaves as it will in production.

**Tech Stack:** Node 24, Vite 8, React 19, TypeScript 5 strict, Tailwind CSS 4 (`@tailwindcss/vite`), shadcn (base-nova style, Base UI), `@tanstack/react-query` 5, `react-router-dom` 7, `lucide-react`, `vite-plugin-pwa`, vitest + `@testing-library/react` + `@testing-library/user-event`, ESLint (typescript-eslint, react-hooks, react-refresh) + Prettier.

**Spec:** `docs/superpowers/specs/2026-09-09-ctdev-platform-design.md` (decisions 1, 6, 7, 9, 10, 12, 13, 17, 20). The interview notes with Thomas's words: `docs/superpowers/plans/2026-09-09-ctdev-platform-interview-notes.md`.

## Global Constraints

- **Fake data only.** No network calls, no Google sign-in, no WebSocket. `src/fake/` is the only place fixtures live. Every screen must render with the fake API and nothing else.
- **Node `>=24.0.0`**, npm. `web/package.json` declares `"engines": { "node": ">=24.0.0" }`.
- **TypeScript strict** with `noUncheckedIndexedAccess` and `exactOptionalPropertyTypes` on, exactly as in `foil-it-up/apps/dashboard/tsconfig.json`.
- **Tests beside the code**, `*.test.tsx` / `*.test.ts`; run with `npm test` (vitest, jsdom). Each task reports line coverage for its files (`npm test -- --coverage`); dead tests are deleted, a green suite is not the bar.
- **Test data is synthetic and obviously fake:** machine names come from the real fleet shape (`ctpi01`, `thomas-desktop-linux`, `thomass-macbook-air`, `ai-node`) but every email is `@example.invalid`, every IP is `10.0.0.x`, and no real token or key appears anywhere.
- **Look: enterprise, Supabase as the reference, Conner Technology's own brand tokens.** The design plan below is the brief; do not restyle.
- **Copy** is sentence case, plain verbs, buttons say what they do ("Run doctor", "Invite user"). No ALL-CAPS labels, no tracked-out eyebrows, no "→" on links.
- **Accessibility floor:** every interactive element reachable by keyboard with a visible focus ring; every icon-only control has an accessible name; `prefers-reduced-motion` disables the console's auto-scroll animation and the busy pulse.
- **Commits:** one-line conventional messages, no body, no footer.
- **Do not touch** `ctdev/`, `CLAUDE.md`, `README.md` at the repo root, `CHANGELOG.md` or `VERSION`. The reviewer does docs and release.

## Design plan (from the frontend-design pass; the brief for every screen)

**Subject.** An internal operations console for the machines Conner Technology owns or looks after. Audience: Thomas today, staff and clients later. Primary job: see at a glance which machines need attention, then act on one and watch it happen.

**Color.** Conner Technology's brand from `marketing/src/styles/global.css`, set into a Supabase-like neutral frame:

| Token | Light | Dark | Use |
| --- | --- | --- | --- |
| `--background` | `#f7f7f9` | `#0b0d14` (brand ink) | page |
| `--card` | `#ffffff` | `#141724` | tiles, tables, panels |
| `--foreground` | `#0b0d14` | `#f2f1ed` (brand paper) | text |
| `--muted-foreground` | `#5b6178` | `#9aa1b8` | secondary text |
| `--border` | `#e3e5ec` | `#252a3c` | hairlines |
| `--primary` | `#4d68d6` (brand blue) | `#6e8bff` (brand blue on dark) | actions, focus, busy |
| `--healthy` | `#1f9d63` | `#3ccf8e` | status |
| `--attention` | `#f0a93b` (brand amber) | `#f0a93b` | status |
| `--danger` | `#d64545` | `#ff6b6b` | status, destructive |
| `--console` | `#0b0d14` | `#05060b` | the action stream pane, dark in both themes |

**Type.** Two families, both already the brand's: **Archivo** for page titles and machine names (weight 600, tight tracking), **Instrument Sans** for everything else, and **JetBrains Mono** only where the content is machine output: the console, commands, versions, agent IDs. Scale: 28 / 20 / 16 / 14 / 13 with line-height 1.4 on body text; tabular numerals on every number column.

**Layout.** A 56 px icon rail on the left (Machines, Users, Roles, Settings; tooltips give the names), a 48 px top bar with a breadcrumb (`Conner Technology / Machines / ctpi01`) and the signed-in user, and a content column of max 1280 px, left aligned, 24 px gutters. Tables are dense (40 px rows). Tiles sit in a 4-up grid that collapses to 2-up under 900 px and 1-up under 600 px, where the rail becomes a bottom bar.

```
┌──┬────────────────────────────────────────────────────────────┐
│▣ │ Conner Technology / Machines / ctpi01              Thomas ▾│
│  ├────────────────────────────────────────────────────────────┤
│▣ │ ctpi01                                  [Run doctor] [Update ▾]
│  │ home · linux/arm64 · agent 12.23.0 · enrolled 2026-09-09    │
│▣ │ ┌──────────┐┌──────────┐┌──────────┐┌──────────┐            │
│  │ │Status    ││Doctor    ││Updates   ││Drift     │            │
│▣ │ │● Healthy ││12 pass 1w││3 pending ││0 of 14   │            │
│  │ └──────────┘└──────────┘└──────────┘└──────────┘            │
│  │ ┌────────────────────────────────────────────────────────┐ │
│  │ │ $ ctdev doctor                    running · 00:04       │ │
│  │ │ ▸ dns.resolvers        pass  Pi-hole on 127.0.0.1       │ │
│  │ │ ▸ dns.dnssec           pass  bogus name SERVFAILs       │ │
│  │ │ ▸ disk.pressure        warn  / at 81%                   │ │
│  │ │ █                                                       │ │
│  │ └────────────────────────────────────────────────────────┘ │
│  │ Doctor                          Actions                     │
│  │ ...                             ...                         │
└──┴────────────────────────────────────────────────────────────┘
```

**Principles.** The console pane is the one bold element: full width, dark in both themes, monospaced, live. Everything else is quiet, hairline-bordered, and reads as a table or a tile. Status is always a colored dot plus a word, never color alone. Structure encodes information: a border means a boundary, a number means a count, nothing is decorative. One orchestrated motion: the console's caret blinks and new lines slide in by one line height; nothing else animates unprompted.

**Reviewed against the brief:** a Supabase-shaped console with Conner Technology's own blue, amber, ink and paper, Archivo titles, and the dark console pane as the memorable element. Not the SaaS-card kit (tiles have hairlines and no shadow, tables are tables), not cream-and-terracotta, not near-black with acid green (the dark theme is the brand ink, the healthy green is a status color used at dot size, never as the accent).

---

## File structure

```
web/
  package.json, package-lock.json
  index.html
  vite.config.ts                 Vite + React SWC + Tailwind + PWA + vitest config
  tsconfig.json, tsconfig.node.json
  eslint.config.mjs, .prettierrc
  components.json                shadcn config (base-nova, lucide, @/ aliases)
  public/favicon.svg, public/icon.svg
  src/
    main.tsx                     mounts App with providers
    App.tsx                      routes
    styles/index.css             tokens (light/dark), fonts, base
    lib/utils.ts                 cn()
    lib/format.ts                relativeTime, duration, formatCount
    api/types.ts                 domain types + the Api interface
    api/context.tsx              ApiProvider / useApi
    fake/fixtures.ts             deterministic data
    fake/actionScripts.ts        the lines each action kind streams
    fake/api.ts                  FakeApi implements Api
    session/SessionContext.tsx   who is signed in, can(permission, scope)
    components/ui/*              shadcn-generated (button, badge, input, table, dialog, checkbox, select, dropdown-menu, tooltip, skeleton, separator)
    components/shell/AppShell.tsx, Rail.tsx, TopBar.tsx
    components/StatusDot.tsx
    components/Sparkline.tsx
    components/Console.tsx
    pages/SignInPage.tsx
    pages/machines/MachinesPage.tsx
    pages/machines/MachinePage.tsx, MachineHeader.tsx, StatusTiles.tsx, ActionLog.tsx, DoctorSection.tsx
    pages/users/UsersPage.tsx, InviteUserDialog.tsx
    pages/roles/RolesPage.tsx, RoleEditor.tsx
    pages/NotFoundPage.tsx
    test/setup.ts, test/render.tsx
```

Each file has one responsibility. Pages compose; components render; `fake/` owns data; `api/types.ts` is the only shared vocabulary.

---

### Task 1: Scaffold `web/` and wire it into CI

**Files:**
- Create: `web/package.json`, `web/index.html`, `web/vite.config.ts`, `web/tsconfig.json`, `web/tsconfig.node.json`, `web/eslint.config.mjs`, `web/.prettierrc`, `web/components.json`, `web/public/favicon.svg`, `web/src/main.tsx`, `web/src/App.tsx`, `web/src/styles/index.css`, `web/src/lib/utils.ts`, `web/src/test/setup.ts`, `web/src/App.test.tsx`, `web/README.md`
- Modify: `.github/workflows/ci.yml` (add a `web-test` job after `go-test`; add it to `build-release`'s `needs`), `.gitignore` (add `web/node_modules/`, `web/dist/`, `web/coverage/`)

**Interfaces:**
- Produces: `cn(...inputs: ClassValue[]): string` in `src/lib/utils.ts`; the `@/` alias; `npm run dev|build|lint|typecheck|test|format`.

- [ ] **Step 1: Create the package**

`web/package.json`:

```json
{
  "name": "ctdev-dashboard",
  "private": true,
  "version": "0.0.0",
  "type": "module",
  "engines": { "node": ">=24.0.0" },
  "scripts": {
    "dev": "vite",
    "build": "tsc -b && vite build",
    "preview": "vite preview",
    "lint": "eslint .",
    "format": "prettier --write \"src/**/*.{ts,tsx,css}\"",
    "format:check": "prettier --check \"src/**/*.{ts,tsx,css}\"",
    "typecheck": "tsc --noEmit",
    "test": "vitest run",
    "test:watch": "vitest"
  },
  "dependencies": {
    "@base-ui/react": "^1.8.0",
    "@fontsource-variable/archivo": "^5.2.0",
    "@fontsource-variable/instrument-sans": "^5.2.0",
    "@fontsource-variable/jetbrains-mono": "^5.2.0",
    "@tanstack/react-query": "^5.102.8",
    "class-variance-authority": "^0.7.1",
    "clsx": "^2.1.1",
    "lucide-react": "^1.41.0",
    "react": "^19.2.8",
    "react-dom": "^19.2.8",
    "react-router-dom": "^7.18.3",
    "tailwind-merge": "^3.5.0"
  },
  "devDependencies": {
    "@eslint/js": "^9.39.0",
    "@tailwindcss/vite": "^4.3.3",
    "@testing-library/jest-dom": "^7.0.1",
    "@testing-library/react": "^16.3.3",
    "@testing-library/user-event": "^14.6.1",
    "@types/react": "^19.2.18",
    "@types/react-dom": "^19.2.7",
    "@vitejs/plugin-react-swc": "^4.3.3",
    "@vitest/coverage-v8": "^4.0.0",
    "eslint": "^9.39.0",
    "eslint-config-prettier": "^10.1.8",
    "eslint-plugin-react-hooks": "^7.1.1",
    "eslint-plugin-react-refresh": "^0.5.6",
    "globals": "^16.0.0",
    "jsdom": "^27.0.0",
    "prettier": "^3.6.0",
    "shadcn": "^4.21.0",
    "tailwindcss": "^4.1.18",
    "typescript": "^5.9.0",
    "typescript-eslint": "^8.46.0",
    "vite": "^8.2.2",
    "vite-plugin-pwa": "^1.3.0",
    "vitest": "^4.0.0"
  }
}
```

Version floors are the Foil It Up dashboard's as of 2026-09-09; `npm install` resolves the exact set into `package-lock.json`, which is committed.

- [ ] **Step 2: Configs**

`web/tsconfig.json` (copied from `foil-it-up/apps/dashboard/tsconfig.json`, `typeRoots` dropped):

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "useDefineForClassFields": true,
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "skipLibCheck": true,
    "moduleResolution": "bundler",
    "allowImportingTsExtensions": true,
    "isolatedModules": true,
    "moduleDetection": "force",
    "noEmit": true,
    "jsx": "react-jsx",
    "strict": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,
    "noUncheckedIndexedAccess": true,
    "exactOptionalPropertyTypes": true,
    "paths": { "@/*": ["./src/*"] },
    "resolveJsonModule": true,
    "types": ["vitest/globals", "@testing-library/jest-dom"]
  },
  "include": ["src"],
  "references": [{ "path": "./tsconfig.node.json" }]
}
```

`web/tsconfig.node.json`:

```json
{
  "compilerOptions": {
    "composite": true,
    "skipLibCheck": true,
    "module": "ESNext",
    "moduleResolution": "bundler",
    "allowSyntheticDefaultImports": true,
    "strict": true,
    "types": ["node"]
  },
  "include": ["vite.config.ts"]
}
```

`web/vite.config.ts`:

```ts
/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react-swc';
import tailwindcss from '@tailwindcss/vite';
import { VitePWA } from 'vite-plugin-pwa';
import { fileURLToPath, URL } from 'node:url';

export default defineConfig({
  plugins: [
    react(),
    tailwindcss(),
    VitePWA({
      registerType: 'autoUpdate',
      includeAssets: ['favicon.svg', 'icon.svg'],
      manifest: {
        name: 'ctdev',
        short_name: 'ctdev',
        description: 'Conner Technology machine management',
        theme_color: '#0b0d14',
        background_color: '#f7f7f9',
        display: 'standalone',
        scope: '/',
        start_url: '/',
        icons: [{ src: 'icon.svg', sizes: 'any', type: 'image/svg+xml', purpose: 'any maskable' }],
      },
      workbox: {
        globPatterns: ['**/*.{js,css,html,ico,svg,woff,woff2}'],
        navigateFallback: 'index.html',
        navigateFallbackDenylist: [/^\/api\//],
      },
      devOptions: { enabled: false },
    }),
  ],
  resolve: { alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) } },
  server: { port: 8080, strictPort: true, host: true },
  build: { target: 'esnext', sourcemap: 'hidden' },
  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./src/test/setup.ts'],
    css: false,
    coverage: { provider: 'v8', include: ['src/**/*.{ts,tsx}'], exclude: ['src/**/*.test.*', 'src/components/ui/**', 'src/main.tsx'] },
  },
});
```

`web/eslint.config.mjs`:

```js
import eslint from '@eslint/js';
import tseslint from 'typescript-eslint';
import eslintConfigPrettier from 'eslint-config-prettier';
import reactHooks from 'eslint-plugin-react-hooks';
import reactRefresh from 'eslint-plugin-react-refresh';
import globals from 'globals';

export default tseslint.config(
  { ignores: ['dist/**', 'node_modules/**', 'coverage/**', '*.js', '*.mjs', 'dev-dist/**'] },
  eslint.configs.recommended,
  ...tseslint.configs.recommended,
  {
    files: ['**/*.{ts,tsx}'],
    languageOptions: {
      ecmaVersion: 'latest',
      sourceType: 'module',
      globals: { ...globals.browser, ...globals.es2022 },
      parserOptions: { ecmaFeatures: { jsx: true } },
    },
    plugins: { 'react-hooks': reactHooks, 'react-refresh': reactRefresh },
    rules: {
      ...reactHooks.configs.recommended.rules,
      'react-refresh/only-export-components': ['warn', { allowConstantExport: true }],
      '@typescript-eslint/no-unused-vars': ['error', { argsIgnorePattern: '^_' }],
    },
  },
  { files: ['src/components/ui/**'], rules: { 'react-refresh/only-export-components': 'off' } },
  eslintConfigPrettier,
);
```

`web/.prettierrc`:

```json
{ "singleQuote": true, "semi": true, "printWidth": 100, "trailingComma": "all" }
```

`web/components.json`:

```json
{
  "$schema": "https://ui.shadcn.com/schema.json",
  "style": "base-nova",
  "rsc": false,
  "tsx": true,
  "tailwind": { "config": "", "css": "src/styles/index.css", "baseColor": "neutral", "cssVariables": true, "prefix": "" },
  "iconLibrary": "lucide",
  "aliases": { "components": "@/components", "utils": "@/lib/utils", "ui": "@/components/ui", "lib": "@/lib", "hooks": "@/hooks" }
}
```

`web/index.html`:

```html
<!doctype html>
<html lang="en">
  <head>
    <meta charset="UTF-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1.0" />
    <meta name="color-scheme" content="light dark" />
    <link rel="icon" type="image/svg+xml" href="/favicon.svg" />
    <title>ctdev</title>
  </head>
  <body>
    <div id="root"></div>
    <script type="module" src="/src/main.tsx"></script>
  </body>
</html>
```

`web/public/favicon.svg` and `web/public/icon.svg` (same file; the brand mark is a later task, this is a placeholder that is clearly a placeholder):

```svg
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64"><rect width="64" height="64" rx="12" fill="#0b0d14"/><path d="M18 40 L32 20 L46 40" fill="none" stroke="#6e8bff" stroke-width="6" stroke-linecap="round" stroke-linejoin="round"/></svg>
```

- [ ] **Step 3: Base styles, utils, entry point, and the failing smoke test**

`web/src/styles/index.css` (tokens from the design plan; the shadcn variables map onto them):

```css
@import 'tailwindcss';
@import 'shadcn/tailwind.css';
@import '@fontsource-variable/archivo';
@import '@fontsource-variable/instrument-sans';
@import '@fontsource-variable/jetbrains-mono';

@custom-variant dark (&:is(.dark *));

:root {
  --background: #f7f7f9;
  --foreground: #0b0d14;
  --card: #ffffff;
  --card-foreground: #0b0d14;
  --popover: #ffffff;
  --popover-foreground: #0b0d14;
  --primary: #4d68d6;
  --primary-foreground: #ffffff;
  --secondary: #eef0f5;
  --secondary-foreground: #0b0d14;
  --muted: #eef0f5;
  --muted-foreground: #5b6178;
  --accent: #eef1fc;
  --accent-foreground: #0b0d14;
  --destructive: #d64545;
  --border: #e3e5ec;
  --input: #d5d8e2;
  --ring: #4d68d6;
  --radius: 0.375rem;
  --sidebar: #ffffff;
  --sidebar-foreground: #0b0d14;
  --sidebar-border: #e3e5ec;
  --healthy: #1f9d63;
  --attention: #f0a93b;
  --danger: #d64545;
  --console: #0b0d14;
  --console-foreground: #e6e8f0;
  --console-muted: #7d8399;
}

.dark {
  --background: #0b0d14;
  --foreground: #f2f1ed;
  --card: #141724;
  --card-foreground: #f2f1ed;
  --popover: #141724;
  --popover-foreground: #f2f1ed;
  --primary: #6e8bff;
  --primary-foreground: #0b0d14;
  --secondary: #1b1f30;
  --secondary-foreground: #f2f1ed;
  --muted: #1b1f30;
  --muted-foreground: #9aa1b8;
  --accent: #1b2140;
  --accent-foreground: #f2f1ed;
  --destructive: #ff6b6b;
  --border: #252a3c;
  --input: #2d3348;
  --ring: #6e8bff;
  --sidebar: #0f1220;
  --sidebar-foreground: #f2f1ed;
  --sidebar-border: #252a3c;
  --healthy: #3ccf8e;
  --attention: #f0a93b;
  --danger: #ff6b6b;
  --console: #05060b;
  --console-foreground: #e6e8f0;
  --console-muted: #7d8399;
}

@theme inline {
  --color-background: var(--background);
  --color-foreground: var(--foreground);
  --color-card: var(--card);
  --color-card-foreground: var(--card-foreground);
  --color-popover: var(--popover);
  --color-popover-foreground: var(--popover-foreground);
  --color-primary: var(--primary);
  --color-primary-foreground: var(--primary-foreground);
  --color-secondary: var(--secondary);
  --color-secondary-foreground: var(--secondary-foreground);
  --color-muted: var(--muted);
  --color-muted-foreground: var(--muted-foreground);
  --color-accent: var(--accent);
  --color-accent-foreground: var(--accent-foreground);
  --color-destructive: var(--destructive);
  --color-border: var(--border);
  --color-input: var(--input);
  --color-ring: var(--ring);
  --color-sidebar: var(--sidebar);
  --color-sidebar-foreground: var(--sidebar-foreground);
  --color-sidebar-border: var(--sidebar-border);
  --color-healthy: var(--healthy);
  --color-attention: var(--attention);
  --color-danger: var(--danger);
  --color-console: var(--console);
  --color-console-foreground: var(--console-foreground);
  --color-console-muted: var(--console-muted);
  --font-sans: 'Instrument Sans Variable', ui-sans-serif, system-ui, sans-serif;
  --font-display: 'Archivo Variable', ui-sans-serif, system-ui, sans-serif;
  --font-mono: 'JetBrains Mono Variable', ui-monospace, SFMono-Regular, monospace;
  --radius-sm: calc(var(--radius) - 2px);
  --radius-md: var(--radius);
  --radius-lg: calc(var(--radius) + 2px);
}

@layer base {
  * { @apply border-border outline-ring/50; }
  body { @apply bg-background text-foreground font-sans antialiased; font-variant-numeric: tabular-nums; }
  h1, h2 { @apply font-display tracking-tight; }
  @media (prefers-reduced-motion: reduce) {
    *, *::before, *::after { animation-duration: 0.01ms !important; transition-duration: 0.01ms !important; scroll-behavior: auto !important; }
  }
}
```

`web/src/lib/utils.ts`:

```ts
import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}
```

`web/src/test/setup.ts`:

```ts
import '@testing-library/jest-dom/vitest';
```

`web/src/App.tsx` (routes come in later tasks; for now one page so the app boots):

```tsx
export function App() {
  return <h1 className="p-6 text-2xl">ctdev</h1>;
}
```

`web/src/main.tsx`:

```tsx
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import '@/styles/index.css';
import { App } from '@/App';

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
```

`web/src/App.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { App } from '@/App';

test('the app renders its name', () => {
  render(<App />);
  expect(screen.getByRole('heading', { name: 'ctdev' })).toBeInTheDocument();
});
```

- [ ] **Step 4: Install and run**

```bash
cd web && npm install
npx shadcn@latest add button badge input table dialog checkbox select dropdown-menu tooltip skeleton separator
npm run lint && npm run typecheck && npm test && npm run build
```

Expected: shadcn writes `src/components/ui/*.tsx` (commit them; they are ours now). Lint, typecheck, one passing test, and a `dist/` build. If `npm install` reports a peer conflict on a floor above, lower that one floor to what resolves and note it in the commit; do not add `--legacy-peer-deps`.

- [ ] **Step 5: CI and gitignore**

Append to `.gitignore`:

```
# Dashboard build artifacts
web/node_modules/
web/dist/
web/dev-dist/
web/coverage/
```

In `.github/workflows/ci.yml`, after the `go-test` job:

```yaml
  web-test:
    name: Dashboard
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v6
      - name: Set up Node
        uses: actions/setup-node@v6
        with:
          node-version: 24
          cache: npm
          cache-dependency-path: web/package-lock.json
      - name: Install
        working-directory: web
        run: npm ci
      - name: Lint, typecheck, test, build
        working-directory: web
        run: |
          npm run lint
          npm run typecheck
          npm run format:check
          npm test
          npm run build
```

and add `web-test` to `build-release`'s `needs` list: `needs: [shellcheck, powershell-lint, go-test, web-test, test-ubuntu, test-macos]`.

`web/README.md`:

```markdown
# ctdev dashboard

The web app for `dashboard.connertechnology.io`. Design and decisions:
`docs/superpowers/specs/2026-09-09-ctdev-platform-design.md`.

    npm install
    npm run dev        # http://localhost:8080
    npm test           # vitest
    npm run build      # dist/

Until the API exists every screen runs on `src/fake/`, an in-memory implementation of
`src/api/types.ts`. Swap `FakeApi` for the real client in `src/main.tsx`; nothing above it changes.
```

- [ ] **Step 6: Commit**

```bash
git add web .github/workflows/ci.yml .gitignore
git commit -m "feat: scaffold the dashboard web app with vite, tailwind, shadcn and vitest"
```

---

### Task 2: App shell: rail, top bar, theme

**Files:**
- Create: `web/src/components/shell/AppShell.tsx`, `web/src/components/shell/Rail.tsx`, `web/src/components/shell/TopBar.tsx`, `web/src/components/shell/AppShell.test.tsx`, `web/src/pages/NotFoundPage.tsx`, `web/src/test/render.tsx`
- Modify: `web/src/App.tsx`, `web/src/main.tsx`

**Interfaces:**
- Consumes: `cn` from Task 1; shadcn `Tooltip`, `Button`, `DropdownMenu`.
- Produces: `<AppShell breadcrumb={[{label,href?}]} actions={ReactNode}>{children}</AppShell>`; `renderWithProviders(ui, { route })` test helper in `src/test/render.tsx`; `useTheme()` returning `{ theme: 'light'|'dark'|'system', setTheme }`.

- [ ] **Step 1: Write the failing test**

`web/src/components/shell/AppShell.test.tsx`:

```tsx
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { AppShell } from '@/components/shell/AppShell';

test('the rail names every section for the keyboard and screen readers', () => {
  renderWithProviders(<AppShell breadcrumb={[{ label: 'Machines' }]}>body</AppShell>);
  for (const name of ['Machines', 'Users', 'Roles', 'Settings']) {
    expect(screen.getByRole('link', { name })).toBeInTheDocument();
  }
});

test('the breadcrumb shows the trail with the organisation first', () => {
  renderWithProviders(
    <AppShell breadcrumb={[{ label: 'Machines', href: '/machines' }, { label: 'ctpi01' }]}>body</AppShell>,
  );
  const nav = screen.getByRole('navigation', { name: 'Breadcrumb' });
  expect(nav).toHaveTextContent('Conner Technology');
  expect(nav).toHaveTextContent('ctpi01');
});

test('the theme switch toggles the dark class on the root', async () => {
  renderWithProviders(<AppShell breadcrumb={[]}>body</AppShell>);
  await userEvent.click(screen.getByRole('button', { name: 'Switch to dark theme' }));
  expect(document.documentElement).toHaveClass('dark');
});
```

`web/src/test/render.tsx`:

```tsx
import { render, type RenderOptions } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactElement, ReactNode } from 'react';
import { ThemeProvider } from '@/components/shell/AppShell';

export function renderWithProviders(ui: ReactElement, { route = '/' }: { route?: string } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <QueryClientProvider client={client}>
        <ThemeProvider>
          <MemoryRouter initialEntries={[route]}>{children}</MemoryRouter>
        </ThemeProvider>
      </QueryClientProvider>
    );
  }
  return render(ui, { wrapper: Wrapper } satisfies RenderOptions);
}
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd web && npx vitest run src/components/shell`
Expected: FAIL, cannot resolve `@/components/shell/AppShell`.

- [ ] **Step 3: Implement**

`web/src/components/shell/AppShell.tsx`:

```tsx
import { createContext, useContext, useEffect, useState, type ReactNode } from 'react';
import { Rail } from './Rail';
import { TopBar, type Crumb } from './TopBar';

type Theme = 'light' | 'dark' | 'system';
const ThemeContext = createContext<{ theme: Theme; setTheme: (t: Theme) => void } | null>(null);

function prefersDark() {
  return typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches;
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
```

`web/src/components/shell/Rail.tsx`:

```tsx
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
      className="flex shrink-0 items-center gap-1 border-t border-sidebar-border bg-sidebar px-2 py-1 sm:w-14 sm:flex-col sm:border-t-0 sm:border-r sm:py-3"
    >
      <a href="/machines" className="mb-2 hidden h-8 w-8 items-center justify-center rounded bg-foreground font-display text-sm font-semibold text-background sm:flex" aria-label="ctdev home">
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
```

`web/src/components/shell/TopBar.tsx`:

```tsx
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
              {i > 0 && <span aria-hidden className="text-muted-foreground">/</span>}
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
          {dark ? <Sun className="h-4 w-4" aria-hidden /> : <Moon className="h-4 w-4" aria-hidden />}
        </Button>
      </div>
    </header>
  );
}
```

`web/src/pages/NotFoundPage.tsx`:

```tsx
import { Link } from 'react-router-dom';
import { AppShell } from '@/components/shell/AppShell';

export function NotFoundPage() {
  return (
    <AppShell breadcrumb={[{ label: 'Not found' }]}>
      <h1 className="text-2xl font-semibold">There is nothing at this address</h1>
      <p className="mt-2 text-muted-foreground">
        Check the link, or go back to <Link to="/machines" className="text-primary underline">machines</Link>.
      </p>
    </AppShell>
  );
}
```

`web/src/App.tsx`:

```tsx
import { Routes, Route, Navigate } from 'react-router-dom';
import { NotFoundPage } from '@/pages/NotFoundPage';

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/machines" replace />} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
```

`web/src/main.tsx` gains the providers:

```tsx
import { StrictMode } from 'react';
import { createRoot } from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import '@/styles/index.css';
import { App } from '@/App';
import { ThemeProvider } from '@/components/shell/AppShell';

const client = new QueryClient();

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <QueryClientProvider client={client}>
      <ThemeProvider>
        <BrowserRouter>
          <App />
        </BrowserRouter>
      </ThemeProvider>
    </QueryClientProvider>
  </StrictMode>,
);
```

Replace `web/src/App.test.tsx` with:

```tsx
import { screen } from '@testing-library/react';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

test('an unknown address shows the not-found page', () => {
  renderWithProviders(<App />, { route: '/nowhere' });
  expect(screen.getByRole('heading', { name: 'There is nothing at this address' })).toBeInTheDocument();
});
```

- [ ] **Step 4: Run the tests**

Run: `cd web && npm test && npm run lint && npm run typecheck`
Expected: PASS (4 tests). If the shadcn `Tooltip` API differs from the `render` prop shown (it is Base UI's `render` prop in base-nova), adapt to what `src/components/ui/tooltip.tsx` actually exports; the test is the contract.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat: add the dashboard shell with rail, breadcrumb and theme switch"
```

---

### Task 3: Domain types and the fake API

**Files:**
- Create: `web/src/api/types.ts`, `web/src/api/context.tsx`, `web/src/lib/format.ts`, `web/src/lib/format.test.ts`, `web/src/fake/fixtures.ts`, `web/src/fake/actionScripts.ts`, `web/src/fake/api.ts`, `web/src/fake/api.test.ts`
- Modify: `web/src/main.tsx`, `web/src/test/render.tsx`

**Interfaces:**
- Produces: every type below; `Api`; `ApiProvider`, `useApi()`; `FakeApi`; `relativeTime(iso, now?)`, `duration(startIso, endIso|null, now?)`, `formatCount(n, singular, plural?)`.

- [ ] **Step 1: Types**

`web/src/api/types.ts`:

```ts
export type Platform = 'linux' | 'macos' | 'windows';
export type MachineStatus = 'healthy' | 'attention' | 'offline';

export interface Group {
  id: string;
  name: string;
}

export interface Machine {
  id: string;
  name: string;
  groupId: string;
  platform: Platform;
  arch: 'arm64' | 'amd64';
  agentVersion: string;
  profile: string | null;
  /** components whose installed state differs from the profile */
  drift: number;
  componentCount: number;
  status: MachineStatus;
  /** ISO timestamp of the last heartbeat; older than 2 minutes means offline */
  lastHeartbeat: string;
  pendingUpdates: number;
  enrolledAt: string;
  /** last 24 CPU samples, percent */
  cpu: number[];
  /** the action currently running, if any */
  currentActionId: string | null;
}

export type ActionKind = 'doctor' | 'update' | 'status' | 'backup';
export type ActionState = 'running' | 'succeeded' | 'failed';

export interface ActionRun {
  id: string;
  machineId: string;
  kind: ActionKind;
  command: string;
  requestedBy: string;
  startedAt: string;
  finishedAt: string | null;
  state: ActionState;
  exitCode: number | null;
}

export interface ActionLine {
  runId: string;
  seq: number;
  at: string;
  stream: 'stdout' | 'stderr';
  text: string;
}

export type CheckOutcome = 'pass' | 'warn' | 'fail' | 'skipped';

export interface DoctorCheck {
  id: string;
  name: string;
  outcome: CheckOutcome;
  detail: string;
}

export interface DoctorRun {
  id: string;
  machineId: string;
  at: string;
  checks: DoctorCheck[];
  /** plain-English verdicts from the correlation engine */
  verdicts: string[];
}

export interface Permission {
  /** tool-prefixed, e.g. "ctdev:doctor.run" */
  id: string;
  area: 'machines' | 'actions' | 'users' | 'roles';
  label: string;
  destructive: boolean;
}

export interface Role {
  id: string;
  name: string;
  description: string;
  builtIn: boolean;
  permissionIds: string[];
}

export type Scope = { kind: 'organisation' } | { kind: 'group'; groupId: string };

export interface Grant {
  roleId: string;
  scope: Scope;
}

export interface User {
  id: string;
  email: string;
  name: string;
  /** a connertechnology.io Workspace account */
  staff: boolean;
  invitedAt: string;
  lastSignIn: string | null;
  grants: Grant[];
}

export interface InviteInput {
  email: string;
  roleId: string;
  scope: Scope;
}

export interface Api {
  listGroups(): Promise<Group[]>;
  listMachines(): Promise<Machine[]>;
  getMachine(id: string): Promise<Machine | null>;
  listActions(machineId: string): Promise<ActionRun[]>;
  runAction(machineId: string, kind: ActionKind, requestedBy: string): Promise<ActionRun>;
  /** delivers every line so far, then new ones; returns an unsubscribe */
  subscribeAction(
    runId: string,
    onLine: (line: ActionLine) => void,
    onDone: (run: ActionRun) => void,
  ): () => void;
  listDoctorRuns(machineId: string): Promise<DoctorRun[]>;
  listUsers(): Promise<User[]>;
  inviteUser(input: InviteInput): Promise<User>;
  listRoles(): Promise<Role[]>;
  listPermissions(): Promise<Permission[]>;
  saveRole(role: Role): Promise<Role>;
}
```

`web/src/api/context.tsx`:

```tsx
import { createContext, useContext, type ReactNode } from 'react';
import type { Api } from './types';

const ApiContext = createContext<Api | null>(null);

export function ApiProvider({ api, children }: { api: Api; children: ReactNode }) {
  return <ApiContext.Provider value={api}>{children}</ApiContext.Provider>;
}

export function useApi(): Api {
  const api = useContext(ApiContext);
  if (!api) throw new Error('useApi needs ApiProvider');
  return api;
}
```

- [ ] **Step 2: Failing tests for format helpers and the fake API**

`web/src/lib/format.test.ts`:

```ts
import { relativeTime, duration, formatCount } from '@/lib/format';

const now = new Date('2026-09-09T12:00:00Z');

test('relativeTime reads like a person wrote it', () => {
  expect(relativeTime('2026-09-09T11:59:50Z', now)).toBe('just now');
  expect(relativeTime('2026-09-09T11:57:00Z', now)).toBe('3 minutes ago');
  expect(relativeTime('2026-09-09T09:00:00Z', now)).toBe('3 hours ago');
  expect(relativeTime('2026-09-07T12:00:00Z', now)).toBe('2 days ago');
});

test('duration counts up while running and settles when finished', () => {
  expect(duration('2026-09-09T11:59:56Z', null, now)).toBe('00:04');
  expect(duration('2026-09-09T11:58:00Z', '2026-09-09T11:59:05Z', now)).toBe('01:05');
});

test('formatCount pluralises', () => {
  expect(formatCount(1, 'update')).toBe('1 update');
  expect(formatCount(3, 'update')).toBe('3 updates');
  expect(formatCount(0, 'entry', 'entries')).toBe('0 entries');
});
```

`web/src/fake/api.test.ts`:

```ts
import { FakeApi } from '@/fake/api';
import type { ActionLine, ActionRun } from '@/api/types';

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

test('the fleet has four machines in two groups', async () => {
  const api = new FakeApi();
  expect(await api.listGroups()).toHaveLength(2);
  const machines = await api.listMachines();
  expect(machines.map((m) => m.name)).toEqual(['ai-node', 'ctpi01', 'thomas-desktop-linux', 'thomass-macbook-air']);
});

test('running doctor streams lines over time and finishes with an exit code', async () => {
  const api = new FakeApi();
  const run = await api.runAction('ctpi01', 'doctor', 'thomas@example.invalid');
  expect(run.state).toBe('running');
  expect((await api.getMachine('ctpi01'))?.currentActionId).toBe(run.id);

  const lines: ActionLine[] = [];
  let done: ActionRun | null = null;
  api.subscribeAction(run.id, (l) => lines.push(l), (r) => (done = r));
  expect(lines).toHaveLength(0);
  await vi.advanceTimersByTimeAsync(400);
  expect(lines.length).toBeGreaterThan(0);
  await vi.advanceTimersByTimeAsync(10_000);
  expect(done).not.toBeNull();
  expect(done!.state).toBe('succeeded');
  expect(done!.exitCode).toBe(0);
  expect((await api.getMachine('ctpi01'))?.currentActionId).toBeNull();
  expect((await api.listActions('ctpi01'))[0]?.id).toBe(run.id);
});

test('a late subscriber receives the lines it missed', async () => {
  const api = new FakeApi();
  const run = await api.runAction('ctpi01', 'status', 'thomas@example.invalid');
  await vi.advanceTimersByTimeAsync(800);
  const lines: ActionLine[] = [];
  api.subscribeAction(run.id, (l) => lines.push(l), () => {});
  expect(lines.length).toBeGreaterThan(0);
  expect(lines.map((l) => l.seq)).toEqual([...lines.keys()]);
});

test('inviting a user adds them with the grant', async () => {
  const api = new FakeApi();
  const before = (await api.listUsers()).length;
  const user = await api.inviteUser({ email: 'new@example.invalid', roleId: 'viewer', scope: { kind: 'group', groupId: 'home' } });
  expect(user.grants).toEqual([{ roleId: 'viewer', scope: { kind: 'group', groupId: 'home' } }]);
  expect(await api.listUsers()).toHaveLength(before + 1);
});

test('a built-in role cannot be saved, a copy can', async () => {
  const api = new FakeApi();
  const [owner] = await api.listRoles();
  await expect(api.saveRole({ ...owner!, name: 'x' })).rejects.toThrow('built-in');
  const copy = await api.saveRole({ ...owner!, id: 'night-operator', name: 'Night operator', builtIn: false });
  expect((await api.listRoles()).find((r) => r.id === 'night-operator')).toEqual(copy);
});
```

- [ ] **Step 3: Run them to see them fail**

Run: `cd web && npx vitest run src/lib src/fake`
Expected: FAIL, modules not found.

- [ ] **Step 4: Implement**

`web/src/lib/format.ts`:

```ts
export function relativeTime(iso: string, now: Date = new Date()): string {
  const seconds = Math.max(0, Math.round((now.getTime() - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return 'just now';
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? '' : 's'} ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} hour${hours === 1 ? '' : 's'} ago`;
  const days = Math.round(hours / 24);
  return `${days} day${days === 1 ? '' : 's'} ago`;
}

export function duration(startIso: string, endIso: string | null, now: Date = new Date()): string {
  const end = endIso ? new Date(endIso) : now;
  const total = Math.max(0, Math.round((end.getTime() - new Date(startIso).getTime()) / 1000));
  const mm = String(Math.floor(total / 60)).padStart(2, '0');
  const ss = String(total % 60).padStart(2, '0');
  return `${mm}:${ss}`;
}

export function formatCount(n: number, singular: string, plural = `${singular}s`): string {
  return `${n} ${n === 1 ? singular : plural}`;
}
```

`web/src/fake/fixtures.ts` (all dates relative to a fixed `NOW` so the screens look alive; `NOW` is the moment the module loads):

```ts
import type { DoctorRun, Group, Machine, Permission, Role, User } from '@/api/types';

export const NOW = new Date();
const ago = (ms: number) => new Date(NOW.getTime() - ms).toISOString();
const min = 60_000;
const hour = 60 * min;
const day = 24 * hour;

export const groups: Group[] = [
  { id: 'home', name: 'Home' },
  { id: 'office', name: 'Office' },
];

const wave = (base: number, amp: number, n = 24) =>
  Array.from({ length: n }, (_, i) => Math.round(base + amp * Math.sin(i / 3) + ((i * 7) % 5)));

export const machines: Machine[] = [
  {
    id: 'ai-node', name: 'ai-node', groupId: 'office', platform: 'linux', arch: 'amd64',
    agentVersion: '12.23.0', profile: 'ai-node', drift: 0, componentCount: 11, status: 'healthy',
    lastHeartbeat: ago(20_000), pendingUpdates: 0, enrolledAt: ago(3 * day), cpu: wave(12, 6), currentActionId: null,
  },
  {
    id: 'ctpi01', name: 'ctpi01', groupId: 'home', platform: 'linux', arch: 'arm64',
    agentVersion: '12.23.0', profile: 'pihole-node', drift: 0, componentCount: 14, status: 'attention',
    lastHeartbeat: ago(8_000), pendingUpdates: 3, enrolledAt: ago(4 * day), cpu: wave(18, 9), currentActionId: null,
  },
  {
    id: 'thomas-desktop-linux', name: 'thomas-desktop-linux', groupId: 'home', platform: 'linux', arch: 'amd64',
    agentVersion: '12.22.0', profile: 'dev-workstation', drift: 2, componentCount: 31, status: 'healthy',
    lastHeartbeat: ago(41_000), pendingUpdates: 7, enrolledAt: ago(4 * day), cpu: wave(35, 20), currentActionId: null,
  },
  {
    id: 'thomass-macbook-air', name: 'thomass-macbook-air', groupId: 'office', platform: 'macos', arch: 'arm64',
    agentVersion: '12.22.0', profile: 'dev-workstation', drift: 1, componentCount: 27, status: 'offline',
    lastHeartbeat: ago(6 * hour), pendingUpdates: 2, enrolledAt: ago(2 * day), cpu: wave(8, 4), currentActionId: null,
  },
];

const check = (id: string, name: string, outcome: DoctorRun['checks'][number]['outcome'], detail: string) => ({ id, name, outcome, detail });

export const doctorRuns: DoctorRun[] = [
  {
    id: 'dr-ctpi01-1', machineId: 'ctpi01', at: ago(1 * day),
    verdicts: ['Disk is filling: / is at 81%. Run cleanup or grow the volume.'],
    checks: [
      check('dns.resolvers', 'DNS servers', 'pass', 'Pi-hole on 127.0.0.1, fallback 9.9.9.9'),
      check('dns.dnssec', 'DNSSEC', 'pass', 'dnssec-failed.org returns SERVFAIL'),
      check('dns.roots', 'Root reach', 'pass', 'a.root-servers.net answered authoritatively'),
      check('disk.pressure', 'Disk pressure', 'warn', '/ at 81%'),
      check('thermal', 'Temperature', 'pass', '52 °C'),
      check('docker.containers', 'Containers', 'pass', '6 running, 0 unhealthy'),
      check('backup.timer', 'Backup timer', 'pass', 'last snapshot 6 hours ago'),
      check('security.ssh', 'SSH hardening', 'pass', 'password auth off'),
    ],
  },
  {
    id: 'dr-ctpi01-2', machineId: 'ctpi01', at: ago(2 * day), verdicts: [],
    checks: [
      check('dns.resolvers', 'DNS servers', 'pass', 'Pi-hole on 127.0.0.1, fallback 9.9.9.9'),
      check('dns.dnssec', 'DNSSEC', 'pass', 'dnssec-failed.org returns SERVFAIL'),
      check('dns.roots', 'Root reach', 'pass', 'a.root-servers.net answered authoritatively'),
      check('disk.pressure', 'Disk pressure', 'pass', '/ at 74%'),
      check('thermal', 'Temperature', 'pass', '50 °C'),
      check('docker.containers', 'Containers', 'pass', '6 running, 0 unhealthy'),
      check('backup.timer', 'Backup timer', 'pass', 'last snapshot 5 hours ago'),
      check('security.ssh', 'SSH hardening', 'pass', 'password auth off'),
    ],
  },
  {
    id: 'dr-desktop-1', machineId: 'thomas-desktop-linux', at: ago(1 * day), verdicts: [],
    checks: [
      check('dns.resolvers', 'DNS servers', 'pass', 'ctpi01 via Tailscale'),
      check('disk.pressure', 'Disk pressure', 'pass', '/ at 43%'),
      check('memory.cgroup', 'Memory cgroup', 'pass', 'memory controller present'),
      check('security.ssh', 'SSH hardening', 'skipped', 'no SSH server'),
    ],
  },
];

export const permissions: Permission[] = [
  { id: 'ctdev:machine.view', area: 'machines', label: 'See machines, status, inventory and history', destructive: false },
  { id: 'ctdev:machine.enroll', area: 'machines', label: 'Add a machine to a group', destructive: false },
  { id: 'ctdev:machine.revoke', area: 'machines', label: 'Revoke a machine', destructive: true },
  { id: 'ctdev:doctor.run', area: 'actions', label: 'Run doctor', destructive: false },
  { id: 'ctdev:status.run', area: 'actions', label: 'Refresh status', destructive: false },
  { id: 'ctdev:update.run', area: 'actions', label: 'Install updates', destructive: false },
  { id: 'ctdev:backup.run', area: 'actions', label: 'Run a backup now', destructive: false },
  { id: 'ctdev:machine.uninstall', area: 'actions', label: 'Uninstall components', destructive: true },
  { id: 'ctdev:restore.inplace', area: 'actions', label: 'Restore in place', destructive: true },
  { id: 'ctdev:user.view', area: 'users', label: 'See users and their roles', destructive: false },
  { id: 'ctdev:user.invite', area: 'users', label: 'Invite users and grant roles you hold', destructive: false },
  { id: 'ctdev:user.remove', area: 'users', label: 'Remove a user', destructive: true },
  { id: 'ctdev:role.view', area: 'roles', label: 'See roles', destructive: false },
  { id: 'ctdev:role.edit', area: 'roles', label: 'Create and edit custom roles', destructive: false },
];

const ids = (...list: string[]) => list;
export const roles: Role[] = [
  { id: 'owner', name: 'Owner', description: 'Everything, including users, roles and destructive actions.', builtIn: true, permissionIds: permissions.map((p) => p.id) },
  { id: 'admin', name: 'Admin', description: 'Manage machines and grants in scope; no destructive actions.', builtIn: true,
    permissionIds: ids('ctdev:machine.view', 'ctdev:machine.enroll', 'ctdev:doctor.run', 'ctdev:status.run', 'ctdev:update.run', 'ctdev:backup.run', 'ctdev:user.view', 'ctdev:user.invite', 'ctdev:role.view') },
  { id: 'operator', name: 'Operator', description: 'Run the actions that change a machine.', builtIn: true,
    permissionIds: ids('ctdev:machine.view', 'ctdev:doctor.run', 'ctdev:status.run', 'ctdev:update.run', 'ctdev:backup.run') },
  { id: 'viewer', name: 'Viewer', description: 'Read only.', builtIn: true,
    permissionIds: ids('ctdev:machine.view', 'ctdev:user.view', 'ctdev:role.view') },
];

export const users: User[] = [
  { id: 'u-thomas', email: 'thomas@example.invalid', name: 'Thomas Conner', staff: true, invitedAt: ago(4 * day), lastSignIn: ago(10 * min), grants: [{ roleId: 'owner', scope: { kind: 'organisation' } }] },
  { id: 'u-family', email: 'family-member@example.invalid', name: 'Family member', staff: false, invitedAt: ago(2 * day), lastSignIn: ago(1 * day), grants: [{ roleId: 'viewer', scope: { kind: 'group', groupId: 'home' } }] },
  { id: 'u-contractor', email: 'contractor@example.invalid', name: 'Contractor', staff: false, invitedAt: ago(1 * day), lastSignIn: null, grants: [{ roleId: 'operator', scope: { kind: 'group', groupId: 'office' } }] },
];
```

`web/src/fake/actionScripts.ts` (what each kind prints; `[delayMs, stream, text]`):

```ts
import type { ActionKind } from '@/api/types';

type Step = [number, 'stdout' | 'stderr', string];

export const scripts: Record<ActionKind, { command: string; steps: Step[]; exitCode: number }> = {
  doctor: {
    command: 'ctdev doctor',
    exitCode: 0,
    steps: [
      [300, 'stdout', 'ctdev doctor 12.23.0 on ctpi01 (linux/arm64)'],
      [500, 'stdout', '  dns.resolvers      pass   Pi-hole on 127.0.0.1, fallback 9.9.9.9'],
      [400, 'stdout', '  dns.dnssec         pass   dnssec-failed.org returns SERVFAIL'],
      [900, 'stdout', '  dns.roots          pass   a.root-servers.net answered authoritatively'],
      [300, 'stdout', '  disk.pressure      warn   / at 81%'],
      [300, 'stdout', '  thermal            pass   52 °C'],
      [700, 'stdout', '  docker.containers  pass   6 running, 0 unhealthy'],
      [400, 'stdout', '  backup.timer       pass   last snapshot 6 hours ago'],
      [300, 'stdout', '  security.ssh       pass   password auth off'],
      [500, 'stdout', ''],
      [100, 'stdout', 'Disk is filling: / is at 81%. Run cleanup or grow the volume.'],
      [200, 'stdout', '8 checks: 7 pass, 1 warn'],
    ],
  },
  update: {
    command: 'ctdev update -y',
    exitCode: 0,
    steps: [
      [400, 'stdout', 'Checking apt, components and compose stacks...'],
      [1200, 'stdout', '  apt        3 packages'],
      [300, 'stdout', '  pihole     image update available (2026.09.1)'],
      [200, 'stdout', ''],
      [800, 'stdout', 'Installing 3 packages...'],
      [1500, 'stdout', '  libssl3 3.0.15 → 3.0.16'],
      [900, 'stdout', '  curl 8.5.0 → 8.5.1'],
      [700, 'stdout', '  ca-certificates 2026.08 → 2026.09'],
      [400, 'stdout', 'Pulling pihole/pihole:latest...'],
      [2500, 'stdout', '  pulled sha256:8b1f…3e2a'],
      [1200, 'stdout', '  pihole recreated'],
      [200, 'stdout', 'Done. No reboot required.'],
    ],
  },
  status: {
    command: 'ctdev status',
    exitCode: 0,
    steps: [
      [300, 'stdout', 'Nothing needs a reboot.'],
      [200, 'stdout', 'Disk: / at 81% (attention above 80%).'],
      [200, 'stdout', 'Containers: 6 running, 0 unhealthy.'],
      [200, 'stdout', 'Backups: last snapshot 6 hours ago.'],
      [200, 'stdout', 'Updates: 3 pending.'],
    ],
  },
  backup: {
    command: 'ctdev backup now',
    exitCode: 1,
    steps: [
      [400, 'stdout', 'Snapshotting 3 paths to b2:ctpi01...'],
      [1800, 'stdout', '  /home/ctadmin/pihole    12.4 MB'],
      [900, 'stdout', '  /etc                     1.1 MB'],
      [2200, 'stderr', 'Fatal: unable to open repository: connection reset by peer'],
      [100, 'stderr', 'restic exited 1'],
    ],
  },
};
```

`web/src/fake/api.ts`:

```ts
import type {
  ActionKind, ActionLine, ActionRun, Api, DoctorRun, Group, InviteInput, Machine, Permission, Role, User,
} from '@/api/types';
import { doctorRuns, groups, machines, permissions, roles, users } from './fixtures';
import { scripts } from './actionScripts';

const clone = <T>(v: T): T => structuredClone(v);
const wait = (ms: number) => new Promise<void>((r) => setTimeout(r, ms));

interface LiveRun {
  run: ActionRun;
  lines: ActionLine[];
  listeners: Set<{ onLine: (l: ActionLine) => void; onDone: (r: ActionRun) => void }>;
}

/**
 * In-memory Api. State lives for the page's lifetime; a reload resets it.
 * Reads resolve on the next tick so components exercise their loading states.
 */
export class FakeApi implements Api {
  private groups = clone(groups);
  private machines = clone(machines);
  private doctorRuns = clone(doctorRuns);
  private users = clone(users);
  private roles = clone(roles);
  private permissions = clone(permissions);
  private runs = new Map<string, LiveRun>();
  private seq = 0;

  async listGroups() { await wait(0); return clone(this.groups); }
  async listMachines() { await wait(0); return clone(this.machines); }
  async getMachine(id: string) { await wait(0); return clone(this.machines.find((m) => m.id === id) ?? null); }
  async listDoctorRuns(machineId: string) {
    await wait(0);
    return clone(this.doctorRuns.filter((d) => d.machineId === machineId).sort((a, b) => b.at.localeCompare(a.at)));
  }
  async listUsers() { await wait(0); return clone(this.users); }
  async listRoles() { await wait(0); return clone(this.roles); }
  async listPermissions() { await wait(0); return clone(this.permissions); }

  async listActions(machineId: string) {
    await wait(0);
    return [...this.runs.values()].map((r) => clone(r.run)).filter((r) => r.machineId === machineId).sort((a, b) => b.startedAt.localeCompare(a.startedAt));
  }

  async runAction(machineId: string, kind: ActionKind, requestedBy: string): Promise<ActionRun> {
    const machine = this.machines.find((m) => m.id === machineId);
    if (!machine) throw new Error(`no machine ${machineId}`);
    if (machine.currentActionId) throw new Error(`${machine.name} is busy`);
    const script = scripts[kind];
    const run: ActionRun = {
      id: `run-${++this.seq}`, machineId, kind, command: script.command, requestedBy,
      startedAt: new Date().toISOString(), finishedAt: null, state: 'running', exitCode: null,
    };
    const live: LiveRun = { run, lines: [], listeners: new Set() };
    this.runs.set(run.id, live);
    machine.currentActionId = run.id;
    void this.play(live, script.steps, script.exitCode, machine);
    return clone(run);
  }

  private async play(live: LiveRun, steps: (typeof scripts)[ActionKind]['steps'], exitCode: number, machine: Machine) {
    for (const [delay, stream, text] of steps) {
      await wait(delay);
      const line: ActionLine = { runId: live.run.id, seq: live.lines.length, at: new Date().toISOString(), stream, text };
      live.lines.push(line);
      for (const l of live.listeners) l.onLine(clone(line));
    }
    live.run = { ...live.run, finishedAt: new Date().toISOString(), state: exitCode === 0 ? 'succeeded' : 'failed', exitCode };
    machine.currentActionId = null;
    if (live.run.kind === 'update' && exitCode === 0) machine.pendingUpdates = 0;
    for (const l of live.listeners) l.onDone(clone(live.run));
  }

  subscribeAction(runId: string, onLine: (line: ActionLine) => void, onDone: (run: ActionRun) => void) {
    const live = this.runs.get(runId);
    if (!live) throw new Error(`no run ${runId}`);
    for (const line of live.lines) onLine(clone(line));
    if (live.run.state !== 'running') { onDone(clone(live.run)); return () => {}; }
    const listener = { onLine, onDone };
    live.listeners.add(listener);
    return () => { live.listeners.delete(listener); };
  }

  async inviteUser(input: InviteInput): Promise<User> {
    await wait(0);
    if (this.users.some((u) => u.email === input.email)) throw new Error(`${input.email} is already a user`);
    const user: User = {
      id: `u-${this.users.length + 1}`, email: input.email, name: input.email.split('@')[0] ?? input.email,
      staff: input.email.endsWith('@connertechnology.io'), invitedAt: new Date().toISOString(), lastSignIn: null,
      grants: [{ roleId: input.roleId, scope: input.scope }],
    };
    this.users.push(user);
    return clone(user);
  }

  async saveRole(role: Role): Promise<Role> {
    await wait(0);
    const existing = this.roles.find((r) => r.id === role.id);
    if (existing?.builtIn) throw new Error(`${existing.name} is built-in; copy it to change it`);
    if (existing) Object.assign(existing, clone(role));
    else this.roles.push(clone({ ...role, builtIn: false }));
    return clone(role);
  }
}
```

Wire the provider. In `web/src/main.tsx` wrap `<App />` with `<ApiProvider api={new FakeApi()}>` (import both). In `web/src/test/render.tsx` add an `api` option defaulting to `new FakeApi()` and wrap with `<ApiProvider api={api}>` inside the `QueryClientProvider`; return `{ ...render(...), api }` so tests can reach the instance.

- [ ] **Step 5: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS. Coverage for `src/fake/api.ts` and `src/lib/format.ts` should be above 90%; report it.

- [ ] **Step 6: Commit**

```bash
git add web
git commit -m "feat: add the dashboard domain types and an in-memory fake api with live action streams"
```

---

### Task 4: Session and the sign-in screen

**Files:**
- Create: `web/src/session/SessionContext.tsx`, `web/src/session/SessionContext.test.tsx`, `web/src/pages/SignInPage.tsx`, `web/src/pages/SignInPage.test.tsx`
- Modify: `web/src/App.tsx`, `web/src/main.tsx`, `web/src/test/render.tsx`, `web/src/components/shell/TopBar.tsx`

**Interfaces:**
- Consumes: `User`, `Role`, `Scope`, `Api` (Task 3).
- Produces: `SessionProvider`, `useSession()` → `{ user: User | null; roles: Role[]; signIn(email): Promise<void>; signOut(): void; can(permissionId: string, scope?: Scope): boolean }`; `<RequireSession>` route wrapper.

- [ ] **Step 1: Failing tests**

`web/src/session/SessionContext.test.tsx`:

```tsx
import { renderHook, act, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { ApiProvider } from '@/api/context';
import { FakeApi } from '@/fake/api';
import { SessionProvider, useSession } from '@/session/SessionContext';

function wrapper({ children }: { children: ReactNode }) {
  return <ApiProvider api={new FakeApi()}><SessionProvider>{children}</SessionProvider></ApiProvider>;
}

test('an invited email signs in and holds its grants', async () => {
  const { result } = renderHook(() => useSession(), { wrapper });
  await act(() => result.current.signIn('family-member@example.invalid'));
  await waitFor(() => expect(result.current.user?.name).toBe('Family member'));
  expect(result.current.can('ctdev:machine.view', { kind: 'group', groupId: 'home' })).toBe(true);
  expect(result.current.can('ctdev:machine.view', { kind: 'group', groupId: 'office' })).toBe(false);
  expect(result.current.can('ctdev:doctor.run', { kind: 'group', groupId: 'home' })).toBe(false);
});

test('an organisation-wide owner can do everything everywhere', async () => {
  const { result } = renderHook(() => useSession(), { wrapper });
  await act(() => result.current.signIn('thomas@example.invalid'));
  await waitFor(() => expect(result.current.user).not.toBeNull());
  expect(result.current.can('ctdev:restore.inplace', { kind: 'group', groupId: 'office' })).toBe(true);
  expect(result.current.can('ctdev:user.invite')).toBe(true);
});

test('an email nobody invited is refused', async () => {
  const { result } = renderHook(() => useSession(), { wrapper });
  await expect(act(() => result.current.signIn('stranger@example.invalid'))).rejects.toThrow('not been invited');
  expect(result.current.user).toBeNull();
});
```

`web/src/pages/SignInPage.test.tsx`:

```tsx
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

test('signing in with an invited Google account lands on machines', async () => {
  renderWithProviders(<App />, { route: '/sign-in' });
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name: /Thomas Conner/ }));
  expect(await screen.findByRole('heading', { name: 'Machines' })).toBeInTheDocument();
});

test('an account nobody invited sees why and who to ask', async () => {
  renderWithProviders(<App />, { route: '/sign-in' });
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name: /stranger/ }));
  expect(await screen.findByText(/has not been invited/)).toBeInTheDocument();
});

test('a signed-out visit to machines goes to sign-in', () => {
  renderWithProviders(<App />, { route: '/machines' });
  expect(screen.getByRole('button', { name: 'Continue with Google' })).toBeInTheDocument();
});
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd web && npx vitest run src/session src/pages/SignInPage`
Expected: FAIL, modules not found.

- [ ] **Step 3: Implement**

`web/src/session/SessionContext.tsx`:

```tsx
import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { useApi } from '@/api/context';
import type { Role, Scope, User } from '@/api/types';

interface Session {
  user: User | null;
  roles: Role[];
  signIn(email: string): Promise<void>;
  signOut(): void;
  can(permissionId: string, scope?: Scope): boolean;
}

const SessionContext = createContext<Session | null>(null);

export function SessionProvider({ children }: { children: ReactNode }) {
  const api = useApi();
  const [user, setUser] = useState<User | null>(null);
  const [roles, setRoles] = useState<Role[]>([]);

  useEffect(() => {
    void api.listRoles().then(setRoles);
  }, [api]);

  const signIn = useCallback(
    async (email: string) => {
      const users = await api.listUsers();
      const found = users.find((u) => u.email.toLowerCase() === email.toLowerCase());
      if (!found) throw new Error(`${email} has not been invited. Ask an admin to invite it.`);
      setUser(found);
    },
    [api],
  );

  const signOut = useCallback(() => setUser(null), []);

  const can = useCallback(
    (permissionId: string, scope?: Scope) => {
      if (!user) return false;
      return user.grants.some((g) => {
        const role = roles.find((r) => r.id === g.roleId);
        if (!role?.permissionIds.includes(permissionId)) return false;
        if (g.scope.kind === 'organisation') return true;
        if (!scope) return false;
        return scope.kind === 'group' && scope.groupId === g.scope.groupId;
      });
    },
    [user, roles],
  );

  const value = useMemo(() => ({ user, roles, signIn, signOut, can }), [user, roles, signIn, signOut, can]);
  return <SessionContext.Provider value={value}>{children}</SessionContext.Provider>;
}

export function useSession(): Session {
  const ctx = useContext(SessionContext);
  if (!ctx) throw new Error('useSession needs SessionProvider');
  return ctx;
}

export function RequireSession({ children }: { children: ReactNode }) {
  const { user } = useSession();
  const location = useLocation();
  if (!user) return <Navigate to="/sign-in" replace state={{ from: location.pathname }} />;
  return <>{children}</>;
}
```

`web/src/pages/SignInPage.tsx`. Until Google exists, "Continue with Google" opens a chooser of the fake accounts plus one uninvited address; the chooser is the stand-in for Google's own account picker and is removed in part 2.

```tsx
import { useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { Button } from '@/components/ui/button';
import { useSession } from '@/session/SessionContext';

const accounts = [
  { email: 'thomas@example.invalid', name: 'Thomas Conner' },
  { email: 'family-member@example.invalid', name: 'Family member' },
  { email: 'contractor@example.invalid', name: 'Contractor' },
  { email: 'stranger@example.invalid', name: 'stranger (not invited)' },
];

export function SignInPage() {
  const { signIn } = useSession();
  const navigate = useNavigate();
  const location = useLocation() as { state?: { from?: string } };
  const [choosing, setChoosing] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function choose(email: string) {
    setError(null);
    try {
      await signIn(email);
      navigate(location.state?.from ?? '/machines', { replace: true });
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }

  return (
    <main className="flex min-h-svh items-center justify-center bg-background px-6">
      <div className="w-full max-w-sm">
        <div className="mb-8 flex items-center gap-3">
          <span className="flex h-9 w-9 items-center justify-center rounded bg-foreground font-display text-base font-semibold text-background">ct</span>
          <div>
            <h1 className="text-xl font-semibold">ctdev</h1>
            <p className="text-sm text-muted-foreground">Conner Technology machines</p>
          </div>
        </div>
        {!choosing ? (
          <Button className="w-full" onClick={() => setChoosing(true)}>
            Continue with Google
          </Button>
        ) : (
          <div className="rounded-md border bg-card" role="group" aria-label="Choose an account">
            {accounts.map((a) => (
              <button
                key={a.email}
                type="button"
                onClick={() => choose(a.email)}
                className="flex w-full flex-col items-start border-b px-4 py-3 text-left last:border-b-0 hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring"
              >
                <span className="text-sm">{a.name}</span>
                <span className="font-mono text-xs text-muted-foreground">{a.email}</span>
              </button>
            ))}
          </div>
        )}
        {error && (
          <p role="alert" className="mt-4 rounded-md border border-danger/40 bg-card px-3 py-2 text-sm">
            {error}
          </p>
        )}
        <p className="mt-6 text-xs text-muted-foreground">
          Sign in with the Google account an admin invited. Conner Technology staff accounts are recognised automatically.
        </p>
      </div>
    </main>
  );
}
```

`web/src/App.tsx` (the machines heading the test expects comes from a placeholder page until Task 5 replaces it):

```tsx
import { Routes, Route, Navigate } from 'react-router-dom';
import { RequireSession } from '@/session/SessionContext';
import { SignInPage } from '@/pages/SignInPage';
import { NotFoundPage } from '@/pages/NotFoundPage';
import { AppShell } from '@/components/shell/AppShell';

function MachinesPlaceholder() {
  return <AppShell breadcrumb={[{ label: 'Machines' }]}><h1 className="text-2xl font-semibold">Machines</h1></AppShell>;
}

export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/machines" replace />} />
      <Route path="/sign-in" element={<SignInPage />} />
      <Route path="/machines" element={<RequireSession><MachinesPlaceholder /></RequireSession>} />
      <Route path="*" element={<NotFoundPage />} />
    </Routes>
  );
}
```

Wrap with `<SessionProvider>` inside `ApiProvider` in both `main.tsx` and `test/render.tsx`. In `TopBar.tsx`, replace the right-hand group's contents with the signed-in user's name and a "Sign out" item in a `DropdownMenu` (trigger label: the user's name; when there is no user render nothing), keeping the theme button.

- [ ] **Step 4: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat: add the session with scoped permission checks and the sign-in screen"
```

---

### Task 5: The machine list

**Files:**
- Create: `web/src/components/StatusDot.tsx`, `web/src/components/Sparkline.tsx`, `web/src/components/Sparkline.test.tsx`, `web/src/pages/machines/MachinesPage.tsx`, `web/src/pages/machines/MachinesPage.test.tsx`
- Modify: `web/src/App.tsx` (replace the placeholder)

**Interfaces:**
- Consumes: `useApi`, `useSession`, `Machine`, `Group`, `relativeTime`, `formatCount`.
- Produces: `<StatusDot status={MachineStatus | 'busy'} label?>`; `<Sparkline values={number[]} label={string}>`; route `/machines`.

- [ ] **Step 1: Failing tests**

`web/src/components/Sparkline.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { Sparkline } from '@/components/Sparkline';

test('a sparkline is an image with a name and one point per value', () => {
  const { container } = render(<Sparkline values={[10, 20, 15]} label="CPU, last 24 samples" />);
  expect(screen.getByRole('img', { name: 'CPU, last 24 samples' })).toBeInTheDocument();
  expect(container.querySelector('polyline')?.getAttribute('points')?.split(' ')).toHaveLength(3);
});
```

`web/src/pages/machines/MachinesPage.test.tsx`:

```tsx
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

async function signInAs(name: RegExp) {
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name }));
}

test('lists every machine with status, group, heartbeat and pending updates', async () => {
  renderWithProviders(<App />, { route: '/machines' });
  await signInAs(/Thomas Conner/);
  const table = await screen.findByRole('table', { name: 'Machines' });
  const rows = within(table).getAllByRole('row').slice(1);
  expect(rows).toHaveLength(4);
  const pi = rows.find((r) => within(r).queryByText('ctpi01'))!;
  expect(within(pi).getByText('Needs attention')).toBeInTheDocument();
  expect(within(pi).getByText('Home')).toBeInTheDocument();
  expect(within(pi).getByText('3 updates')).toBeInTheDocument();
});

test('a viewer scoped to one group sees only that group', async () => {
  renderWithProviders(<App />, { route: '/machines' });
  await signInAs(/Family member/);
  const table = await screen.findByRole('table', { name: 'Machines' });
  expect(within(table).getAllByRole('row').slice(1)).toHaveLength(2);
  expect(within(table).queryByText('ai-node')).not.toBeInTheDocument();
});

test('filtering by status narrows the list', async () => {
  renderWithProviders(<App />, { route: '/machines' });
  await signInAs(/Thomas Conner/);
  await screen.findByRole('table', { name: 'Machines' });
  await userEvent.click(screen.getByRole('button', { name: 'Needs attention' }));
  expect(within(screen.getByRole('table', { name: 'Machines' })).getAllByRole('row').slice(1)).toHaveLength(1);
});
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd web && npx vitest run src/components/Sparkline src/pages/machines`
Expected: FAIL.

- [ ] **Step 3: Implement**

`web/src/components/StatusDot.tsx`:

```tsx
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
```

`web/src/components/Sparkline.tsx`:

```tsx
export function Sparkline({ values, label, width = 96, height = 24 }: { values: number[]; label: string; width?: number; height?: number }) {
  const max = Math.max(1, ...values);
  const step = values.length > 1 ? width / (values.length - 1) : 0;
  const points = values.map((v, i) => `${(i * step).toFixed(1)},${(height - (v / max) * (height - 2) - 1).toFixed(1)}`).join(' ');
  return (
    <svg role="img" aria-label={label} width={width} height={height} viewBox={`0 0 ${width} ${height}`} className="text-primary">
      <polyline points={points} fill="none" stroke="currentColor" strokeWidth="1.5" strokeLinejoin="round" />
    </svg>
  );
}
```

`web/src/pages/machines/MachinesPage.tsx`:

```tsx
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
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
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
  const machines = useQuery({ queryKey: ['machines'], queryFn: () => api.listMachines(), refetchInterval: 15_000 });
  const groups = useQuery({ queryKey: ['groups'], queryFn: () => api.listGroups() });

  const visible = useMemo(() => {
    const all = (machines.data ?? []).filter((m) => can('ctdev:machine.view', { kind: 'group', groupId: m.groupId }));
    return filter === 'all' ? all : all.filter((m) => m.status === filter);
  }, [machines.data, filter, can]);

  const groupName = (id: string) => groups.data?.find((g) => g.id === id)?.name ?? id;
  const counts = (machines.data ?? []).reduce<Record<string, number>>((acc, m) => ({ ...acc, [m.status]: (acc[m.status] ?? 0) + 1 }), {});

  return (
    <AppShell breadcrumb={[{ label: 'Machines' }]}>
      <div className="mb-6 flex flex-wrap items-end justify-between gap-4">
        <div>
          <h1 className="text-2xl font-semibold">Machines</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            {machines.data ? `${formatCount(machines.data.length, 'machine')}, ${counts['attention'] ?? 0} need attention` : 'Loading'}
          </p>
        </div>
        <div className="flex gap-1" role="group" aria-label="Filter by status">
          {filters.map((f) => (
            <Button key={f.value} size="sm" variant={filter === f.value ? 'secondary' : 'ghost'} aria-pressed={filter === f.value} onClick={() => setFilter(f.value)}>
              {f.label}
            </Button>
          ))}
        </div>
      </div>

      <div className="rounded-md border bg-card">
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
            {machines.isPending && [0, 1, 2].map((i) => (
              <TableRow key={i}><TableCell colSpan={7}><Skeleton className="h-5 w-full" /></TableCell></TableRow>
            ))}
            {visible.map((m) => <MachineRow key={m.id} machine={m} group={groupName(m.groupId)} />)}
            {machines.data && visible.length === 0 && (
              <TableRow><TableCell colSpan={7} className="py-10 text-center text-muted-foreground">No machines match this filter.</TableCell></TableRow>
            )}
          </TableBody>
        </Table>
      </div>
    </AppShell>
  );
}

function MachineRow({ machine: m, group }: { machine: Machine; group: string }) {
  return (
    <TableRow className={cn(m.status === 'offline' && 'text-muted-foreground')}>
      <TableCell>
        <Link to={`/machines/${m.id}`} className="font-display font-semibold hover:underline">{m.name}</Link>
        <div className="font-mono text-xs text-muted-foreground">{m.platform}/{m.arch}{m.profile ? ` · ${m.profile}` : ''}</div>
      </TableCell>
      <TableCell>{m.currentActionId ? <StatusDot status="busy" /> : <StatusDot status={m.status} />}</TableCell>
      <TableCell>{group}</TableCell>
      <TableCell>{relativeTime(m.lastHeartbeat)}</TableCell>
      <TableCell className="text-right">{m.pendingUpdates > 0 ? formatCount(m.pendingUpdates, 'update') : <span className="text-muted-foreground">Up to date</span>}</TableCell>
      <TableCell><Sparkline values={m.cpu} label={`${m.name} CPU, last 24 samples`} /></TableCell>
      <TableCell className="font-mono text-xs">{m.agentVersion}</TableCell>
    </TableRow>
  );
}
```

In `App.tsx`, delete `MachinesPlaceholder` and route `/machines` to `<MachinesPage />` inside `RequireSession`.

- [ ] **Step 4: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS. Then `npm run dev`, open http://localhost:8080, sign in as Thomas, and check the list against the design plan: hairline table, tabular numbers, Archivo machine names, the offline row muted. Take a screenshot into `web/docs/` is not needed; describe what you saw in the commit message body is not allowed; just fix what is off.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat: add the machine list with status, group, heartbeat, updates and cpu sparkline"
```

---

### Task 6: The machine page with the live console

**Files:**
- Create: `web/src/components/Console.tsx`, `web/src/components/Console.test.tsx`, `web/src/hooks/useActionStream.ts`, `web/src/pages/machines/MachinePage.tsx`, `web/src/pages/machines/MachineHeader.tsx`, `web/src/pages/machines/StatusTiles.tsx`, `web/src/pages/machines/ActionLog.tsx`, `web/src/pages/machines/MachinePage.test.tsx`
- Modify: `web/src/App.tsx`

**Interfaces:**
- Consumes: `Api.runAction`, `Api.subscribeAction`, `Api.listActions`, `ActionRun`, `ActionLine`, `duration`, `StatusDot`.
- Produces: `useActionStream(runId | null)` → `{ lines: ActionLine[]; run: ActionRun | null }`; `<Console run lines>`; route `/machines/:id`.

- [ ] **Step 1: Failing tests**

`web/src/components/Console.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react';
import { Console } from '@/components/Console';
import type { ActionLine, ActionRun } from '@/api/types';

const run: ActionRun = { id: 'r1', machineId: 'ctpi01', kind: 'doctor', command: 'ctdev doctor', requestedBy: 'thomas@example.invalid', startedAt: '2026-09-09T12:00:00Z', finishedAt: null, state: 'running', exitCode: null };
const lines: ActionLine[] = [
  { runId: 'r1', seq: 0, at: '2026-09-09T12:00:01Z', stream: 'stdout', text: 'dns.resolvers pass' },
  { runId: 'r1', seq: 1, at: '2026-09-09T12:00:02Z', stream: 'stderr', text: 'warning: disk at 81%' },
];

test('shows the command, each line, and that it is running', () => {
  render(<Console run={run} lines={lines} />);
  const log = screen.getByRole('log', { name: 'ctdev doctor output' });
  expect(log).toHaveTextContent('dns.resolvers pass');
  expect(screen.getByText('warning: disk at 81%')).toHaveClass('text-attention');
  expect(screen.getByText('Running')).toBeInTheDocument();
});

test('a finished run shows its exit and duration', () => {
  render(<Console run={{ ...run, finishedAt: '2026-09-09T12:01:05Z', state: 'failed', exitCode: 1 }} lines={lines} />);
  expect(screen.getByText('Failed, exit 1')).toBeInTheDocument();
  expect(screen.getByText('01:05')).toBeInTheDocument();
});

test('with no run it invites an action', () => {
  render(<Console run={null} lines={[]} />);
  expect(screen.getByText('Run an action to see its output here.')).toBeInTheDocument();
});
```

`web/src/pages/machines/MachinePage.test.tsx`:

```tsx
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

async function signInAs(name: RegExp) {
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name }));
}

test('shows the machine header and status tiles', async () => {
  renderWithProviders(<App />, { route: '/machines/ctpi01' });
  await signInAs(/Thomas Conner/);
  expect(await screen.findByRole('heading', { name: 'ctpi01' })).toBeInTheDocument();
  expect(screen.getByText('Home · linux/arm64 · agent 12.23.0')).toBeInTheDocument();
  const tiles = screen.getByRole('list', { name: 'Status' });
  expect(within(tiles).getByText('Needs attention')).toBeInTheDocument();
  expect(within(tiles).getByText('3 updates')).toBeInTheDocument();
  expect(within(tiles).getByText('0 of 14 drifted')).toBeInTheDocument();
});

test('running doctor streams into the console and lands in the action log', async () => {
  const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime });
  vi.useFakeTimers();
  renderWithProviders(<App />, { route: '/machines/ctpi01' });
  await user.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await user.click(screen.getByRole('button', { name: /Thomas Conner/ }));
  await screen.findByRole('heading', { name: 'ctpi01' });
  await user.click(screen.getByRole('button', { name: 'Run doctor' }));
  expect(await screen.findByText('Running')).toBeInTheDocument();
  await vi.advanceTimersByTimeAsync(12_000);
  const log = screen.getByRole('log', { name: 'ctdev doctor output' });
  expect(log).toHaveTextContent('8 checks: 7 pass, 1 warn');
  expect(screen.getByText('Succeeded, exit 0')).toBeInTheDocument();
  expect(within(screen.getByRole('table', { name: 'Actions' })).getByText('ctdev doctor')).toBeInTheDocument();
  vi.useRealTimers();
});

test('a viewer cannot run actions', async () => {
  renderWithProviders(<App />, { route: '/machines/ctpi01' });
  await signInAs(/Family member/);
  await screen.findByRole('heading', { name: 'ctpi01' });
  expect(screen.queryByRole('button', { name: 'Run doctor' })).not.toBeInTheDocument();
});

test('an unknown machine says so', async () => {
  renderWithProviders(<App />, { route: '/machines/nope' });
  await signInAs(/Thomas Conner/);
  expect(await screen.findByText('No machine called nope')).toBeInTheDocument();
});
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd web && npx vitest run src/components/Console src/pages/machines/MachinePage`
Expected: FAIL.

- [ ] **Step 3: Implement**

`web/src/hooks/useActionStream.ts`:

```ts
import { useEffect, useState } from 'react';
import { useApi } from '@/api/context';
import type { ActionLine, ActionRun } from '@/api/types';

export function useActionStream(runId: string | null) {
  const api = useApi();
  const [lines, setLines] = useState<ActionLine[]>([]);
  const [run, setRun] = useState<ActionRun | null>(null);

  useEffect(() => {
    setLines([]);
    setRun(null);
    if (!runId) return;
    return api.subscribeAction(
      runId,
      (line) => setLines((prev) => [...prev, line]),
      (finished) => setRun(finished),
    );
  }, [api, runId]);

  return { lines, run };
}
```

`web/src/components/Console.tsx`:

```tsx
import { useEffect, useRef, useState } from 'react';
import type { ActionLine, ActionRun } from '@/api/types';
import { duration } from '@/lib/format';
import { cn } from '@/lib/utils';

export function Console({ run, lines }: { run: ActionRun | null; lines: ActionLine[] }) {
  const endRef = useRef<HTMLDivElement>(null);
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    if (run?.state !== 'running') return;
    const t = setInterval(() => setNow(new Date()), 1000);
    return () => clearInterval(t);
  }, [run?.state]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: 'end' });
  }, [lines.length]);

  const state =
    run?.state === 'running' ? 'Running'
    : run?.state === 'succeeded' ? `Succeeded, exit ${run.exitCode}`
    : run?.state === 'failed' ? `Failed, exit ${run.exitCode}`
    : null;

  return (
    <section className="overflow-hidden rounded-md border border-console bg-console font-mono text-[13px] text-console-foreground">
      <div className="flex items-center justify-between border-b border-white/10 px-4 py-2">
        <span>
          <span className="text-console-muted">$ </span>
          {run ? run.command : <span className="text-console-muted">ctdev</span>}
        </span>
        {run && (
          <span className="flex items-center gap-3 text-xs">
            <span className={cn(run.state === 'failed' && 'text-danger', run.state === 'succeeded' && 'text-healthy')}>{state}</span>
            <span className="text-console-muted">{duration(run.startedAt, run.finishedAt, now)}</span>
          </span>
        )}
      </div>
      <div role="log" aria-live="polite" aria-label={`${run?.command ?? 'ctdev'} output`} className="max-h-[420px] min-h-[180px] overflow-y-auto px-4 py-3">
        {!run && <p className="text-console-muted">Run an action to see its output here.</p>}
        {lines.map((l) => (
          <div key={l.seq} className={cn('whitespace-pre-wrap motion-safe:animate-in motion-safe:slide-in-from-bottom-1', l.stream === 'stderr' && 'text-attention')}>
            {l.text || ' '}
          </div>
        ))}
        {run?.state === 'running' && <span aria-hidden className="inline-block h-4 w-2 bg-console-foreground motion-safe:animate-pulse" />}
        <div ref={endRef} />
      </div>
    </section>
  );
}
```

`web/src/pages/machines/MachineHeader.tsx`:

```tsx
import type { ActionKind, Machine } from '@/api/types';
import { useSession } from '@/session/SessionContext';
import { Button } from '@/components/ui/button';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { relativeTime } from '@/lib/format';

const actions: { kind: ActionKind; label: string; permission: string }[] = [
  { kind: 'update', label: 'Install updates', permission: 'ctdev:update.run' },
  { kind: 'status', label: 'Refresh status', permission: 'ctdev:status.run' },
  { kind: 'backup', label: 'Back up now', permission: 'ctdev:backup.run' },
];

export function MachineHeader({ machine, group, onRun }: { machine: Machine; group: string; onRun: (kind: ActionKind) => void }) {
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
        <p className="text-sm text-muted-foreground">Enrolled {relativeTime(machine.enrolledAt)}, last seen {relativeTime(machine.lastHeartbeat)}</p>
      </div>
      <div className="flex gap-2">
        {can('ctdev:doctor.run', scope) && (
          <Button variant="secondary" disabled={busy} onClick={() => onRun('doctor')}>Run doctor</Button>
        )}
        {allowed.length > 0 && (
          <DropdownMenu>
            <DropdownMenuTrigger render={<Button disabled={busy} />}>Actions</DropdownMenuTrigger>
            <DropdownMenuContent align="end">
              {allowed.map((a) => (
                <DropdownMenuItem key={a.kind} onClick={() => onRun(a.kind)}>{a.label}</DropdownMenuItem>
              ))}
            </DropdownMenuContent>
          </DropdownMenu>
        )}
      </div>
    </div>
  );
}
```

`web/src/pages/machines/StatusTiles.tsx`:

```tsx
import type { DoctorRun, Machine } from '@/api/types';
import { StatusDot } from '@/components/StatusDot';
import { formatCount, relativeTime } from '@/lib/format';

export function StatusTiles({ machine, latestDoctor }: { machine: Machine; latestDoctor: DoctorRun | null }) {
  const pass = latestDoctor?.checks.filter((c) => c.outcome === 'pass').length ?? 0;
  const notPass = latestDoctor ? latestDoctor.checks.length - pass : 0;
  const tiles = [
    { title: 'Status', value: machine.currentActionId ? <StatusDot status="busy" /> : <StatusDot status={machine.status} />, note: `Heartbeat ${relativeTime(machine.lastHeartbeat)}` },
    { title: 'Doctor', value: latestDoctor ? `${pass} pass, ${notPass} not` : 'Never run', note: latestDoctor ? relativeTime(latestDoctor.at) : 'Run it from the header' },
    { title: 'Updates', value: machine.pendingUpdates > 0 ? formatCount(machine.pendingUpdates, 'update') : 'Up to date', note: machine.pendingUpdates > 0 ? 'Pending' : 'Nothing pending' },
    { title: 'Profile', value: `${machine.drift} of ${machine.componentCount} drifted`, note: machine.profile ?? 'No profile' },
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
```

`web/src/pages/machines/ActionLog.tsx`:

```tsx
import type { ActionRun } from '@/api/types';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { duration, relativeTime } from '@/lib/format';
import { cn } from '@/lib/utils';

export function ActionLog({ runs, selectedId, onSelect }: { runs: ActionRun[]; selectedId: string | null; onSelect: (id: string) => void }) {
  return (
    <section>
      <h2 className="mb-3 text-lg font-semibold">Actions</h2>
      <div className="rounded-md border bg-card">
        <Table aria-label="Actions">
          <TableHeader>
            <TableRow>
              <TableHead>Command</TableHead>
              <TableHead>Result</TableHead>
              <TableHead>Requested by</TableHead>
              <TableHead>Started</TableHead>
              <TableHead className="text-right">Took</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {runs.length === 0 && (
              <TableRow><TableCell colSpan={5} className="py-8 text-center text-muted-foreground">Nothing has run on this machine yet.</TableCell></TableRow>
            )}
            {runs.map((r) => (
              <TableRow key={r.id} className={cn('cursor-pointer', r.id === selectedId && 'bg-accent')} onClick={() => onSelect(r.id)}>
                <TableCell className="font-mono text-xs">{r.command}</TableCell>
                <TableCell className={cn(r.state === 'failed' && 'text-danger', r.state === 'succeeded' && 'text-healthy')}>
                  {r.state === 'running' ? 'Running' : r.state === 'succeeded' ? 'Succeeded' : `Failed, exit ${r.exitCode}`}
                </TableCell>
                <TableCell className="font-mono text-xs">{r.requestedBy}</TableCell>
                <TableCell>{relativeTime(r.startedAt)}</TableCell>
                <TableCell className="text-right font-mono text-xs">{duration(r.startedAt, r.finishedAt)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </section>
  );
}
```

`web/src/pages/machines/MachinePage.tsx`:

```tsx
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

  const machine = useQuery({ queryKey: ['machine', id], queryFn: () => api.getMachine(id), refetchInterval: 5_000 });
  const groups = useQuery({ queryKey: ['groups'], queryFn: () => api.listGroups() });
  const runs = useQuery({ queryKey: ['actions', id], queryFn: () => api.listActions(id), refetchInterval: 5_000 });
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

  if (machine.isPending) return <AppShell breadcrumb={[{ label: 'Machines', href: '/machines' }, { label: id }]}>Loading</AppShell>;
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
      {run.error && <p role="alert" className="mb-4 text-sm text-danger">{run.error.message}</p>}
      <StatusTiles machine={m} latestDoctor={doctor.data?.[0] ?? null} />
      <div className="mb-8"><Console run={liveRun} lines={stream.lines} /></div>
      <div className="grid gap-8 lg:grid-cols-2">
        <DoctorSection runs={doctor.data ?? []} />
        <ActionLog runs={runs.data ?? []} selectedId={selectedRunId} onSelect={setSelectedRunId} />
      </div>
    </AppShell>
  );
}
```

`DoctorSection` is Task 7; for this task create `web/src/pages/machines/DoctorSection.tsx` as the minimal version the page compiles against, replaced in Task 7:

```tsx
import type { DoctorRun } from '@/api/types';

export function DoctorSection({ runs }: { runs: DoctorRun[] }) {
  return (
    <section>
      <h2 className="mb-3 text-lg font-semibold">Doctor</h2>
      <p className="text-sm text-muted-foreground">{runs.length} runs recorded.</p>
    </section>
  );
}
```

Add the route in `App.tsx`: `<Route path="/machines/:id" element={<RequireSession><MachinePage /></RequireSession>} />`.

- [ ] **Step 4: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS. Then in `npm run dev`: run doctor on ctpi01 and watch lines arrive; run "Back up now" and see the stderr lines in amber and "Failed, exit 1"; confirm the buttons disable while busy and the list shows "Running" for the machine.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat: add the machine page with status tiles, a live action console and the action log"
```

---

### Task 7: Doctor results

**Files:**
- Create: `web/src/pages/machines/DoctorSection.test.tsx`
- Modify: `web/src/pages/machines/DoctorSection.tsx` (replace the stub)

**Interfaces:**
- Consumes: `DoctorRun`, `DoctorCheck`, `CheckOutcome`, `relativeTime`.
- Produces: `<DoctorSection runs={DoctorRun[]}>` with the latest run expanded and history as a row of dots.

- [ ] **Step 1: Failing test**

`web/src/pages/machines/DoctorSection.test.tsx`:

```tsx
import { render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { DoctorSection } from '@/pages/machines/DoctorSection';
import { doctorRuns } from '@/fake/fixtures';

const runs = doctorRuns.filter((r) => r.machineId === 'ctpi01');

test('shows the verdict, then every check of the latest run with its outcome', () => {
  render(<DoctorSection runs={runs} />);
  expect(screen.getByText('Disk is filling: / is at 81%. Run cleanup or grow the volume.')).toBeInTheDocument();
  const list = screen.getByRole('list', { name: 'Checks' });
  expect(within(list).getAllByRole('listitem')).toHaveLength(8);
  expect(within(list).getByText('Disk pressure').closest('li')).toHaveTextContent('Warn');
});

test('history lets you open an earlier run', async () => {
  render(<DoctorSection runs={runs} />);
  await userEvent.click(screen.getByRole('button', { name: /2 days ago/ }));
  expect(screen.getByText('/ at 74%')).toBeInTheDocument();
  expect(screen.getByText('No verdicts. Everything passed.')).toBeInTheDocument();
});

test('with no runs it says how to get one', () => {
  render(<DoctorSection runs={[]} />);
  expect(screen.getByText('Doctor has not run on this machine. Run it from the header.')).toBeInTheDocument();
});
```

- [ ] **Step 2: Run it to see it fail**

Run: `cd web && npx vitest run src/pages/machines/DoctorSection`
Expected: FAIL (the stub has no list).

- [ ] **Step 3: Implement**

`web/src/pages/machines/DoctorSection.tsx`:

```tsx
import { useState } from 'react';
import type { CheckOutcome, DoctorRun } from '@/api/types';
import { relativeTime } from '@/lib/format';
import { cn } from '@/lib/utils';

const outcome: Record<CheckOutcome, { label: string; dot: string }> = {
  pass: { label: 'Pass', dot: 'bg-healthy' },
  warn: { label: 'Warn', dot: 'bg-attention' },
  fail: { label: 'Fail', dot: 'bg-danger' },
  skipped: { label: 'Skipped', dot: 'bg-muted-foreground' },
};

function worst(run: DoctorRun): CheckOutcome {
  if (run.checks.some((c) => c.outcome === 'fail')) return 'fail';
  if (run.checks.some((c) => c.outcome === 'warn')) return 'warn';
  return 'pass';
}

export function DoctorSection({ runs }: { runs: DoctorRun[] }) {
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const run = runs.find((r) => r.id === selectedId) ?? runs[0] ?? null;

  return (
    <section>
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-lg font-semibold">Doctor</h2>
        {runs.length > 1 && (
          <div className="flex items-center gap-1" role="group" aria-label="History">
            {runs.map((r) => (
              <button
                key={r.id}
                type="button"
                aria-label={`Run ${relativeTime(r.at)}, ${outcome[worst(r)].label}`}
                aria-pressed={r.id === run?.id}
                onClick={() => setSelectedId(r.id)}
                className={cn('h-4 w-4 rounded-full border-2 border-transparent p-[3px] focus-visible:outline-2 focus-visible:outline-ring', r.id === run?.id && 'border-foreground')}
              >
                <span className={cn('block h-full w-full rounded-full', outcome[worst(r)].dot)} />
              </button>
            ))}
          </div>
        )}
      </div>
      {!run ? (
        <p className="rounded-md border bg-card px-4 py-8 text-center text-sm text-muted-foreground">Doctor has not run on this machine. Run it from the header.</p>
      ) : (
        <div className="rounded-md border bg-card">
          <div className="border-b px-4 py-3 text-sm">
            <div className="text-muted-foreground">{relativeTime(run.at)}</div>
            {run.verdicts.length === 0 ? (
              <p className="mt-1">No verdicts. Everything passed.</p>
            ) : (
              run.verdicts.map((v) => <p key={v} className="mt-1 font-medium">{v}</p>)
            )}
          </div>
          <ul aria-label="Checks" className="divide-y">
            {run.checks.map((c) => (
              <li key={c.id} className="flex items-center gap-3 px-4 py-2 text-sm">
                <span aria-hidden className={cn('h-2 w-2 shrink-0 rounded-full', outcome[c.outcome].dot)} />
                <span className="w-32 shrink-0">{c.name}</span>
                <span className="w-14 shrink-0 text-muted-foreground">{outcome[c.outcome].label}</span>
                <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">{c.detail}</span>
              </li>
            ))}
          </ul>
        </div>
      )}
    </section>
  );
}
```

- [ ] **Step 4: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS, including Task 6's page tests.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat: show doctor verdicts, checks and run history on the machine page"
```

---

### Task 8: Users and roles

**Files:**
- Create: `web/src/pages/users/UsersPage.tsx`, `web/src/pages/users/InviteUserDialog.tsx`, `web/src/pages/users/UsersPage.test.tsx`, `web/src/pages/roles/RolesPage.tsx`, `web/src/pages/roles/RoleEditor.tsx`, `web/src/pages/roles/RolesPage.test.tsx`
- Modify: `web/src/App.tsx`

**Interfaces:**
- Consumes: `Api.listUsers/inviteUser/listRoles/listPermissions/saveRole`, `useSession().can`, `Scope`.
- Produces: routes `/users`, `/roles`.

- [ ] **Step 1: Failing tests**

`web/src/pages/users/UsersPage.test.tsx`:

```tsx
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

async function signInAs(name: RegExp) {
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name }));
}

test('lists users with their role and scope', async () => {
  renderWithProviders(<App />, { route: '/users' });
  await signInAs(/Thomas Conner/);
  const table = await screen.findByRole('table', { name: 'Users' });
  const row = within(table).getByText('family-member@example.invalid').closest('tr')!;
  expect(row).toHaveTextContent('Viewer');
  expect(row).toHaveTextContent('Home');
  expect(row).toHaveTextContent('Never');
});

test('an owner invites a user with a role and a scope', async () => {
  renderWithProviders(<App />, { route: '/users' });
  await signInAs(/Thomas Conner/);
  await screen.findByRole('table', { name: 'Users' });
  await userEvent.click(screen.getByRole('button', { name: 'Invite user' }));
  const dialog = screen.getByRole('dialog', { name: 'Invite user' });
  await userEvent.type(within(dialog).getByLabelText('Email'), 'new@example.invalid');
  await userEvent.click(within(dialog).getByRole('button', { name: 'Send invite' }));
  expect(await screen.findByText('new@example.invalid')).toBeInTheDocument();
});

test('a viewer sees the list but no invite button', async () => {
  renderWithProviders(<App />, { route: '/users' });
  await signInAs(/Family member/);
  await screen.findByRole('table', { name: 'Users' });
  expect(screen.queryByRole('button', { name: 'Invite user' })).not.toBeInTheDocument();
});
```

`web/src/pages/roles/RolesPage.test.tsx`:

```tsx
import { screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

async function signInAs(name: RegExp) {
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name }));
}

test('the four built-in roles are listed and marked built-in', async () => {
  renderWithProviders(<App />, { route: '/roles' });
  await signInAs(/Thomas Conner/);
  const list = await screen.findByRole('list', { name: 'Roles' });
  expect(within(list).getAllByRole('listitem')).toHaveLength(4);
  expect(within(list).getAllByText('Built-in')).toHaveLength(4);
});

test('opening a built-in role shows its permissions read-only with a copy button', async () => {
  renderWithProviders(<App />, { route: '/roles' });
  await signInAs(/Thomas Conner/);
  await userEvent.click(await screen.findByRole('button', { name: 'Operator' }));
  const checks = screen.getAllByRole('checkbox');
  expect(checks.every((c) => c.hasAttribute('disabled'))).toBe(true);
  expect(screen.getByRole('checkbox', { name: 'Run doctor' })).toBeChecked();
  expect(screen.getByRole('checkbox', { name: 'Restore in place' })).not.toBeChecked();
  expect(screen.getByRole('button', { name: 'Copy to a new role' })).toBeInTheDocument();
});

test('copying a role and saving it adds a custom role', async () => {
  renderWithProviders(<App />, { route: '/roles' });
  await signInAs(/Thomas Conner/);
  await userEvent.click(await screen.findByRole('button', { name: 'Viewer' }));
  await userEvent.click(screen.getByRole('button', { name: 'Copy to a new role' }));
  await userEvent.clear(screen.getByLabelText('Name'));
  await userEvent.type(screen.getByLabelText('Name'), 'Auditor');
  await userEvent.click(screen.getByRole('checkbox', { name: 'Run doctor' }));
  await userEvent.click(screen.getByRole('button', { name: 'Save role' }));
  const list = screen.getByRole('list', { name: 'Roles' });
  expect(within(list).getByText('Auditor')).toBeInTheDocument();
  expect(within(list).getAllByText('Custom')).toHaveLength(1);
});
```

- [ ] **Step 2: Run them to see them fail**

Run: `cd web && npx vitest run src/pages/users src/pages/roles`
Expected: FAIL.

- [ ] **Step 3: Implement**

`web/src/pages/users/InviteUserDialog.tsx`:

```tsx
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useApi } from '@/api/context';
import type { Scope } from '@/api/types';
import { useSession } from '@/session/SessionContext';
import { Button } from '@/components/ui/button';
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';

export function InviteUserDialog() {
  const api = useApi();
  const { user, roles } = useSession();
  const queryClient = useQueryClient();
  const groups = useQuery({ queryKey: ['groups'], queryFn: () => api.listGroups() });
  const [open, setOpen] = useState(false);
  const [email, setEmail] = useState('');
  const [roleId, setRoleId] = useState('viewer');
  const [scope, setScope] = useState<string>('organisation');

  // A user may grant only roles they hold; an owner holds them all.
  const heldRoleIds = new Set(user?.grants.map((g) => g.roleId));
  const grantable = roles.filter((r) => heldRoleIds.has('owner') || heldRoleIds.has(r.id));

  const invite = useMutation({
    mutationFn: () => {
      const s: Scope = scope === 'organisation' ? { kind: 'organisation' } : { kind: 'group', groupId: scope };
      return api.inviteUser({ email: email.trim(), roleId, scope: s });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['users'] });
      setOpen(false);
      setEmail('');
    },
  });

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger render={<Button />}>Invite user</DialogTrigger>
      <DialogContent>
        <DialogHeader><DialogTitle>Invite user</DialogTitle></DialogHeader>
        <form
          className="space-y-4"
          onSubmit={(e) => { e.preventDefault(); invite.mutate(); }}
        >
          <label className="block text-sm">
            <span className="mb-1 block">Email</span>
            <Input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} placeholder="name@example.com" />
            <span className="mt-1 block text-xs text-muted-foreground">They sign in with the Google account for this address.</span>
          </label>
          <label className="block text-sm">
            <span className="mb-1 block">Role</span>
            <select className="w-full rounded-md border bg-card px-3 py-2" value={roleId} onChange={(e) => setRoleId(e.target.value)}>
              {grantable.map((r) => <option key={r.id} value={r.id}>{r.name}</option>)}
            </select>
          </label>
          <label className="block text-sm">
            <span className="mb-1 block">Where</span>
            <select className="w-full rounded-md border bg-card px-3 py-2" value={scope} onChange={(e) => setScope(e.target.value)}>
              <option value="organisation">Whole organisation</option>
              {groups.data?.map((g) => <option key={g.id} value={g.id}>{g.name} only</option>)}
            </select>
          </label>
          {invite.error && <p role="alert" className="text-sm text-danger">{invite.error.message}</p>}
          <DialogFooter>
            <Button type="submit" disabled={invite.isPending}>Send invite</Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
```

`web/src/pages/users/UsersPage.tsx`:

```tsx
import { useQuery } from '@tanstack/react-query';
import { useApi } from '@/api/context';
import type { Scope } from '@/api/types';
import { useSession } from '@/session/SessionContext';
import { AppShell } from '@/components/shell/AppShell';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { relativeTime } from '@/lib/format';
import { InviteUserDialog } from './InviteUserDialog';

export function UsersPage() {
  const api = useApi();
  const { can, roles } = useSession();
  const users = useQuery({ queryKey: ['users'], queryFn: () => api.listUsers() });
  const groups = useQuery({ queryKey: ['groups'], queryFn: () => api.listGroups() });

  const scopeName = (s: Scope) => (s.kind === 'organisation' ? 'Whole organisation' : groups.data?.find((g) => g.id === s.groupId)?.name ?? s.groupId);
  const roleName = (id: string) => roles.find((r) => r.id === id)?.name ?? id;

  return (
    <AppShell breadcrumb={[{ label: 'Users' }]} actions={can('ctdev:user.invite') ? <InviteUserDialog /> : null}>
      <h1 className="mb-1 text-2xl font-semibold">Users</h1>
      <p className="mb-6 text-sm text-muted-foreground">Everyone who can sign in, and what they may do where.</p>
      <div className="rounded-md border bg-card">
        <Table aria-label="Users">
          <TableHeader>
            <TableRow>
              <TableHead>User</TableHead>
              <TableHead>Role</TableHead>
              <TableHead>Where</TableHead>
              <TableHead>Last sign-in</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.data?.map((u) => (
              <TableRow key={u.id}>
                <TableCell>
                  <div>{u.name}{u.staff && <span className="ml-2 rounded border px-1.5 py-0.5 text-xs text-muted-foreground">Staff</span>}</div>
                  <div className="font-mono text-xs text-muted-foreground">{u.email}</div>
                </TableCell>
                <TableCell>{u.grants.map((g) => roleName(g.roleId)).join(', ')}</TableCell>
                <TableCell>{u.grants.map((g) => scopeName(g.scope)).join(', ')}</TableCell>
                <TableCell>{u.lastSignIn ? relativeTime(u.lastSignIn) : 'Never'}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </div>
    </AppShell>
  );
}
```

`web/src/pages/roles/RoleEditor.tsx`:

```tsx
import { useState } from 'react';
import type { Permission, Role } from '@/api/types';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import { Input } from '@/components/ui/input';

const areas: { key: Permission['area']; title: string }[] = [
  { key: 'machines', title: 'Machines' },
  { key: 'actions', title: 'Actions' },
  { key: 'users', title: 'Users' },
  { key: 'roles', title: 'Roles' },
];

export function RoleEditor({
  role, permissions, onCopy, onSave, saving,
}: { role: Role; permissions: Permission[]; onCopy: () => void; onSave: (r: Role) => void; saving: boolean }) {
  const [draft, setDraft] = useState(role);
  const readOnly = role.builtIn;
  const toggle = (id: string, on: boolean) =>
    setDraft((d) => ({ ...d, permissionIds: on ? [...d.permissionIds, id] : d.permissionIds.filter((p) => p !== id) }));

  return (
    <form className="rounded-md border bg-card" onSubmit={(e) => { e.preventDefault(); onSave(draft); }}>
      <div className="flex items-start justify-between gap-4 border-b px-4 py-3">
        <div className="flex-1">
          {readOnly ? (
            <h2 className="text-lg font-semibold">{role.name}</h2>
          ) : (
            <label className="block text-sm"><span className="mb-1 block">Name</span><Input value={draft.name} onChange={(e) => setDraft({ ...draft, name: e.target.value })} required /></label>
          )}
          <p className="mt-1 text-sm text-muted-foreground">{readOnly ? `${role.description} Built-in roles cannot change; copy one to start from it.` : 'Tick what this role may do.'}</p>
        </div>
        {readOnly ? (
          <Button type="button" variant="secondary" onClick={onCopy}>Copy to a new role</Button>
        ) : (
          <Button type="submit" disabled={saving}>Save role</Button>
        )}
      </div>
      {areas.map((a) => (
        <fieldset key={a.key} className="border-b px-4 py-3 last:border-b-0">
          <legend className="mb-2 text-sm font-medium">{a.title}</legend>
          <ul className="space-y-2">
            {permissions.filter((p) => p.area === a.key).map((p) => (
              <li key={p.id} className="flex items-center gap-3 text-sm">
                <Checkbox id={p.id} disabled={readOnly} checked={draft.permissionIds.includes(p.id)} onCheckedChange={(v) => toggle(p.id, v === true)} />
                <label htmlFor={p.id} className="flex-1">{p.label}</label>
                {p.destructive && <span className="rounded border border-danger/40 px-1.5 py-0.5 text-xs text-danger">Destructive</span>}
                <span className="font-mono text-xs text-muted-foreground">{p.id}</span>
              </li>
            ))}
          </ul>
        </fieldset>
      ))}
    </form>
  );
}
```

`web/src/pages/roles/RolesPage.tsx`:

```tsx
import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useApi } from '@/api/context';
import type { Role } from '@/api/types';
import { AppShell } from '@/components/shell/AppShell';
import { cn } from '@/lib/utils';
import { RoleEditor } from './RoleEditor';

export function RolesPage() {
  const api = useApi();
  const queryClient = useQueryClient();
  const roles = useQuery({ queryKey: ['roles'], queryFn: () => api.listRoles() });
  const permissions = useQuery({ queryKey: ['permissions'], queryFn: () => api.listPermissions() });
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [draft, setDraft] = useState<Role | null>(null);

  const save = useMutation({
    mutationFn: (r: Role) => api.saveRole(r),
    onSuccess: (saved) => {
      void queryClient.invalidateQueries({ queryKey: ['roles'] });
      setDraft(null);
      setSelectedId(saved.id);
    },
  });

  const selected = draft ?? roles.data?.find((r) => r.id === selectedId) ?? null;

  function copy(from: Role) {
    const id = `${from.id}-copy-${Date.now()}`;
    setDraft({ ...from, id, name: `${from.name} copy`, description: '', builtIn: false });
  }

  return (
    <AppShell breadcrumb={[{ label: 'Roles' }]}>
      <h1 className="mb-1 text-2xl font-semibold">Roles</h1>
      <p className="mb-6 text-sm text-muted-foreground">A role is a set of permissions. Grant one to a user for the whole organisation or one group.</p>
      <div className="grid gap-6 lg:grid-cols-[260px_1fr]">
        <ul aria-label="Roles" className="divide-y rounded-md border bg-card">
          {roles.data?.map((r) => (
            <li key={r.id}>
              <button
                type="button"
                onClick={() => { setDraft(null); setSelectedId(r.id); }}
                aria-pressed={r.id === selected?.id}
                className={cn('flex w-full items-center justify-between px-4 py-3 text-left text-sm hover:bg-accent focus-visible:outline-2 focus-visible:outline-ring', r.id === selected?.id && 'bg-accent')}
              >
                <span>{r.name}</span>
                <span className="text-xs text-muted-foreground">{r.builtIn ? 'Built-in' : 'Custom'}</span>
              </button>
            </li>
          ))}
        </ul>
        {selected && permissions.data ? (
          <RoleEditor
            key={selected.id}
            role={selected}
            permissions={permissions.data}
            onCopy={() => copy(selected)}
            onSave={(r) => save.mutate(r)}
            saving={save.isPending}
          />
        ) : (
          <p className="rounded-md border bg-card px-4 py-8 text-center text-sm text-muted-foreground">Pick a role to see what it allows.</p>
        )}
      </div>
    </AppShell>
  );
}
```

Routes in `App.tsx`: `/users` → `<UsersPage />`, `/roles` → `<RolesPage />`, both inside `RequireSession`. `/settings` can route to `NotFoundPage` for now.

- [ ] **Step 4: Run the tests**

Run: `cd web && npm test && npm run typecheck && npm run lint`
Expected: PASS. If the shadcn `Checkbox` uses `onCheckedChange(checked: boolean)` rather than a tri-state value, adjust `toggle`'s call accordingly; the test is the contract.

- [ ] **Step 5: Commit**

```bash
git add web
git commit -m "feat: add the users list with invites and the roles page with a permission checklist"
```

---

### Task 9: Responsive and accessibility pass, Cloudflare Pages, and the review build

**Files:**
- Create: `web/public/_redirects`, `web/src/a11y.test.tsx`
- Modify: `web/README.md`, any component the checks below flag

**Interfaces:** none new.

- [ ] **Step 1: Failing accessibility test across every page**

Install `vitest-axe` as a dev dependency (`npm i -D vitest-axe axe-core`). `web/src/a11y.test.tsx`:

```tsx
import { screen } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from 'vitest-axe';
import { renderWithProviders } from '@/test/render';
import { App } from '@/App';

async function signInAs(name: RegExp) {
  await userEvent.click(screen.getByRole('button', { name: 'Continue with Google' }));
  await userEvent.click(screen.getByRole('button', { name }));
}

for (const route of ['/machines', '/machines/ctpi01', '/users', '/roles']) {
  test(`${route} has no axe violations`, async () => {
    const { container } = renderWithProviders(<App />, { route });
    await signInAs(/Thomas Conner/);
    await screen.findByRole('navigation', { name: 'Breadcrumb' });
    expect(await axe(container)).toHaveNoViolations();
  });
}

test('/sign-in has no axe violations', async () => {
  const { container } = renderWithProviders(<App />, { route: '/sign-in' });
  expect(await axe(container)).toHaveNoViolations();
});
```

Add `import 'vitest-axe/extend-expect';` to `src/test/setup.ts`.

- [ ] **Step 2: Run it and fix what it finds**

Run: `cd web && npx vitest run src/a11y`
Expected: any violation is a real defect: fix the component (a missing name, a contrast failure on `text-muted-foreground` over `bg-accent`, a table without a caption). Do not disable rules.

- [ ] **Step 3: Responsive check by hand**

In `npm run dev`, at 1280, 900 and 375 px wide: the rail moves to the bottom under 640 px, tiles collapse 4 → 2 → 1, the console never causes horizontal page scroll (it scrolls inside), the machine table scrolls inside its card. Fix with Tailwind breakpoints only; no new components.

- [ ] **Step 4: Cloudflare Pages**

`web/public/_redirects` so deep links resolve on Pages:

```
/*    /index.html   200
```

Append to `web/README.md`:

```markdown
## Deploying

Cloudflare Pages, project `ctdev-dashboard`, production domain `dashboard.connertechnology.io`.
Build command `npm run build`, build output `web/dist`, root directory `web`, Node 24.
`public/_redirects` sends every path to `index.html` for the router.
```

- [ ] **Step 5: Coverage and the final run**

Run: `cd web && npm run lint && npm run typecheck && npm run format:check && npm test -- --coverage && npm run build`
Expected: all green. Record the coverage table for `src/` (excluding `components/ui`) in the final report; anything under 80% for a file this plan created gets a test or a justification.

- [ ] **Step 6: Commit**

```bash
git add web
git commit -m "feat: make the dashboard responsive and accessible and ready for cloudflare pages"
```

---

## Self-review

**Spec coverage (decision 17, part 1):** sign-in (Task 4), machine list with status (Task 5), one machine page with a live action stream and recent doctor results (Tasks 6, 7), users and roles (Task 8), Supabase-referenced enterprise look with Conner Technology's tokens (design plan, Task 1), reviewed by Thomas before any service exists (Task 9's build). Decision 7's rules appear where the UI can show them: built-in roles copy-only, a user grants only roles they hold. Decision 9's "destructive needs its own permission" appears as the Destructive tag and the absence of destructive actions from the operator preset. Not in this plan, by design: Google itself, enrolment, the WebSocket, the schema (part 2).

**Placeholders:** none. Every code step is complete. The two "if the shadcn API differs" notes give the rule for adapting (the test is the contract), not an instruction to figure it out.

**Type consistency:** `Api` methods and every domain type are defined once in Task 3 and used by name in Tasks 4 to 8; `useActionStream` returns `{ lines, run }` and `Console` takes `{ run, lines }`; `StatusDot` accepts `MachineStatus | 'busy'` everywhere it is used; `Scope` is the same union in session, invite and fixtures.
