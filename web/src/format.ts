import { toDate } from './dates';

const compactFormat = new Intl.NumberFormat('en', { notation: 'compact', maximumFractionDigits: 1 });
const integerFormat = new Intl.NumberFormat('en');
const centsFormat = new Intl.NumberFormat('en', { minimumFractionDigits: 2, maximumFractionDigits: 2 });

// formatDuration formats seconds as hours and minutes: 60 h 40 m, 45 m.
export function formatDuration(seconds: number): string {
  const minutes = Math.round(seconds / 60);
  if (seconds > 0 && minutes === 0) return '<1 m';
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  if (hours === 0) return `${rest} m`;
  if (rest === 0) return `${hours} h`;
  return `${hours} h ${rest} m`;
}

// formatHours formats seconds as hours with one decimal, for compact columns: 7.6 h.
export function formatHours(seconds: number): string {
  const hours = seconds / 3600;
  if (hours === 0) return '0 h';
  if (hours < 0.05) return '<0.1 h';
  if (hours >= 100) return `${Math.round(hours)} h`;
  return `${hours.toFixed(1)} h`;
}

// formatCompact formats large numbers: 1,284 as 1.3K, 12,900,000 as 12.9M.
export function formatCompact(value: number): string {
  return compactFormat.format(value);
}

export function formatInteger(value: number): string {
  return integerFormat.format(Math.round(value));
}

// formatCost formats an estimated cost in USD: $0.47, $313.66, $10,843.
export function formatCost(value: number): string {
  if (value === 0) return '$0';
  if (value < 0.01) return '<$0.01';
  if (value >= 1000) return `$${integerFormat.format(Math.round(value))}`;
  return `$${centsFormat.format(value)}`;
}

// formatCostCompact formats costs for headline figures and axes: $313.66, $46.5K.
export function formatCostCompact(value: number): string {
  return value >= 1000 ? `$${compactFormat.format(value)}` : formatCost(value);
}

// formatShare formats a fraction as a percentage: 97%.
export function formatShare(fraction: number): string {
  if (fraction > 0 && fraction < 0.01) return '<1%';
  return `${Math.round(fraction * 100)}%`;
}

// describeChange describes how a value changed since the previous period, or returns null when
// there is nothing to compare with: 12% more than before.
export function describeChange(current: number, previous: number): string | null {
  if (previous <= 0) return null;
  const percent = Math.round((Math.abs(current - previous) / previous) * 100);
  if (percent === 0) return 'Same as before';
  return `${percent}% ${current > previous ? 'more' : 'less'} than before`;
}

// formatClock formats a time of day given in seconds since midnight: 14:05.
export function formatClock(seconds: number): string {
  const minutes = Math.round(seconds / 60);
  return `${Math.floor(minutes / 60)}:${String(minutes % 60).padStart(2, '0')}`;
}

const weekdayFormat = new Intl.DateTimeFormat('en', { weekday: 'short', timeZone: 'UTC' });
const longDateFormat = new Intl.DateTimeFormat('en', { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric', timeZone: 'UTC' });
const shortDateFormat = new Intl.DateTimeFormat('en', { month: 'short', day: 'numeric', timeZone: 'UTC' });
const rangeFormat = new Intl.DateTimeFormat('en', { month: 'short', day: 'numeric', year: 'numeric', timeZone: 'UTC' });

// formatDayLabel formats a date for a row label: Thu 15.
export function formatDayLabel(date: string): string {
  return `${weekdayFormat.format(toDate(date))} ${Number(date.slice(8))}`;
}

// formatLongDate formats a date in full: Thursday, October 15, 2026.
export function formatLongDate(date: string): string {
  return longDateFormat.format(toDate(date));
}

// formatShortDate formats a date without its year: Oct 15.
export function formatShortDate(date: string): string {
  return shortDateFormat.format(toDate(date));
}

// formatRange formats a date range: Oct 15 – 21, 2026.
export function formatRange(start: string, end: string): string {
  if (start === end) return rangeFormat.format(toDate(start));
  return rangeFormat.formatRange(toDate(start), toDate(end));
}
