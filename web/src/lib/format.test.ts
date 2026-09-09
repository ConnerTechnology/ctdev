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
