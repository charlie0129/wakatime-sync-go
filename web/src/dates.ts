// Dates are 'YYYY-MM-DD' strings, handled as UTC midnights so that no timezone shifts them.

const DAY_MS = 24 * 60 * 60 * 1000;

export function toDate(date: string): Date {
  return new Date(`${date}T00:00:00Z`);
}

export function fromDate(date: Date): string {
  return date.toISOString().slice(0, 10);
}

export function isDate(value: string): boolean {
  return /^\d{4}-\d{2}-\d{2}$/.test(value) && !Number.isNaN(toDate(value).getTime()) && fromDate(toDate(value)) === value;
}

export function addDays(date: string, days: number): string {
  return fromDate(new Date(toDate(date).getTime() + days * DAY_MS));
}

// countDays returns the number of days from start to end, both included.
export function countDays(start: string, end: string): number {
  return Math.round((toDate(end).getTime() - toDate(start).getTime()) / DAY_MS) + 1;
}

// weekday returns the day of the week, from 0 for Monday to 6 for Sunday.
export function weekday(date: string): number {
  return (toDate(date).getUTCDay() + 6) % 7;
}

export function startOfWeek(date: string): string {
  return addDays(date, -weekday(date));
}

export function eachDay(start: string, end: string): string[] {
  const days: string[] = [];
  for (let day = start; day <= end; day = addDays(day, 1)) {
    days.push(day);
  }
  return days;
}

// --- Date ranges ---

export type Preset =
  | 'yesterday'
  | '7d'
  | '30d'
  | '90d'
  | '12m'
  | 'this-week'
  | 'last-week'
  | 'this-month'
  | 'last-month'
  | 'this-year'
  | 'last-year';

// A date range is either a preset, which the server resolves in its timezone, or two dates.
export type Range = { preset: Preset } | { start: string; end: string };

export const PRESETS: Record<Preset, string> = {
  yesterday: 'Yesterday',
  '7d': 'Last 7 days',
  '30d': 'Last 30 days',
  '90d': 'Last 90 days',
  '12m': 'Last 12 months',
  'this-week': 'This week',
  'last-week': 'Last week',
  'this-month': 'This month',
  'last-month': 'Last month',
  'this-year': 'This year',
  'last-year': 'Last year',
};

// The presets offered next to the date range, with shorter names.
export const QUICK_PRESETS: Array<[Preset, string]> = [
  ['7d', '7 days'],
  ['30d', '30 days'],
  ['90d', '90 days'],
  ['12m', '12 months'],
];

export const DEFAULT_RANGE: Range = { preset: '7d' };

export function parseRange(search: string): Range {
  const params = new URLSearchParams(search);
  const preset = params.get('range');
  if (preset && preset in PRESETS) {
    return { preset: preset as Preset };
  }
  const start = params.get('start') ?? '';
  const end = params.get('end') ?? '';
  if (isDate(start) && isDate(end) && start <= end) {
    return { start, end };
  }
  return DEFAULT_RANGE;
}

// rangeQuery returns the query string of a range, for the page URL and the API.
export function rangeQuery(range: Range): string {
  if ('preset' in range) {
    return `range=${range.preset}`;
  }
  return `start=${range.start}&end=${range.end}`;
}
