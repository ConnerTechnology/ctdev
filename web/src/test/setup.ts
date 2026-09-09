import '@testing-library/jest-dom/vitest';
import { vi } from 'vitest';

// @testing-library/react's asyncWrapper drains the microtask queue after every
// act() by scheduling a timer and, under fake timers, immediately firing it —
// but it only does that when it finds a global `jest` (its fake-timer check is
// jest-specific). Vitest has none, so without this shim any userEvent
// interaction under vi.useFakeTimers() hangs forever waiting on that drain.
if (typeof (globalThis as { jest?: unknown }).jest === 'undefined') {
  (globalThis as { jest?: { advanceTimersByTime: (ms: number) => void } }).jest = {
    advanceTimersByTime: (ms: number) => vi.advanceTimersByTime(ms),
  };
}
