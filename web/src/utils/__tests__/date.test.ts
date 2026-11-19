import { formatSecondsToHumanReadable, fromServerUtc } from '../date';

describe('formatSecondsToHumanReadable', () => {
  test('formats sub-minute durations in seconds', () => {
    expect(formatSecondsToHumanReadable(0)).toBe('0s');
    expect(formatSecondsToHumanReadable(30)).toBe('30s');
  });

  test('strips trailing zeros from fractional seconds', () => {
    expect(formatSecondsToHumanReadable(1.5)).toBe('1.5s');
  });

  test('does not emit a trailing space for whole minutes or hours', () => {
    expect(formatSecondsToHumanReadable(60)).toBe('1m');
    expect(formatSecondsToHumanReadable(3600)).toBe('1h');
    expect(formatSecondsToHumanReadable(7200)).toBe('2h');
  });

  test('does not emit a trailing space when seconds are zero', () => {
    expect(formatSecondsToHumanReadable(3660)).toBe('1h 1m');
    expect(formatSecondsToHumanReadable(5400)).toBe('1h 30m');
  });

  test('joins non-zero hour/minute/second parts with a single space', () => {
    expect(formatSecondsToHumanReadable(90)).toBe('1m 30s');
    expect(formatSecondsToHumanReadable(125)).toBe('2m 5s');
    expect(formatSecondsToHumanReadable(3661)).toBe('1h 1m 1s');
  });

  test('returns "0s" for negative or non-numeric input', () => {
    expect(formatSecondsToHumanReadable(-5)).toBe('0s');
    expect(formatSecondsToHumanReadable(NaN)).toBe('0s');
  });
});

describe('fromServerUtc', () => {
  test('marks zone-less API timestamps as UTC', () => {
    expect(fromServerUtc('2026-11-18 18:49:04')).toBe('2026-11-18T18:49:04Z');
    expect(fromServerUtc('2026-11-18T18:49:04.120')).toBe(
      '2026-11-18T18:49:04.120Z',
    );
  });

  test('leaves zoned strings, epochs and empty values alone', () => {
    expect(fromServerUtc('2026-11-18T18:49:04+08:00')).toBe(
      '2026-11-18T18:49:04+08:00',
    );
    expect(fromServerUtc('Wed, 18 Nov 2026 18:49:04 GMT')).toBe(
      'Wed, 18 Nov 2026 18:49:04 GMT',
    );
    expect(fromServerUtc(1795027744080)).toBe(1795027744080);
    expect(fromServerUtc(undefined)).toBeUndefined();
  });
});
