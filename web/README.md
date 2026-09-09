# ctdev dashboard

The web app for `dashboard.connertechnology.io`. Design and decisions:
`docs/superpowers/specs/2026-09-09-ctdev-platform-design.md`.

    npm install
    npm run dev        # http://localhost:8080
    npm test           # vitest
    npm run build      # dist/

Until the API exists every screen runs on `src/fake/`, an in-memory implementation of
`src/api/types.ts`. Swap `FakeApi` for the real client in `src/main.tsx`; nothing above it changes.
