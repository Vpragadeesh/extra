import { describe, expect, it } from 'vitest';
import { fmtBytes, fmtDuration, localDay, time } from './format';

describe('fmtDuration', () => {
  it('formats seconds under a minute', () => {
    expect(fmtDuration(0)).toBe('0s');
    expect(fmtDuration(59)).toBe('59s');
  });
  it('formats minutes under an hour', () => {
    expect(fmtDuration(60)).toBe('1m');
    expect(fmtDuration(125)).toBe('2m');
  });
  it('formats hours with remaining minutes', () => {
    expect(fmtDuration(3600)).toBe('1h 0m');
    expect(fmtDuration(3725)).toBe('1h 2m');
  });
  it('tolerates junk input', () => {
    expect(fmtDuration(NaN)).toBe('0s');
    expect(fmtDuration(undefined)).toBe('0s');
  });
});

describe('fmtBytes', () => {
  it('stays in bytes below 1 KiB', () => {
    expect(fmtBytes(0)).toBe('0 B');
    expect(fmtBytes(1023)).toBe('1023 B');
  });
  it('formats KiB/MiB/GiB', () => {
    expect(fmtBytes(1024)).toBe('1.0 KB');
    expect(fmtBytes(3 * 1048576)).toBe('3.0 MB');
    expect(fmtBytes(2 * 1073741824)).toBe('2.00 GB');
  });
});

describe('localDay', () => {
  it('returns a YYYY-MM-DD local date', () => {
    expect(localDay()).toMatch(/^\d{4}-\d{2}-\d{2}$/);
  });
});

describe('time', () => {
  it('renders a clock time for valid timestamps', () => {
    expect(time('2026-10-27T12:00:00')).toMatch(/^\d{1,2}:\d{2}\s?(?:am|pm)?$/i);
  });
  it('falls back to a dash for invalid timestamps', () => {
    expect(time('not-a-date')).toBe('—');
  });
});
