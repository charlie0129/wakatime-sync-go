import { format, subDays, parseISO, startOfWeek, startOfMonth, endOfMonth } from 'date-fns';

export function formatDate(date: Date | string): string {
  const d = typeof date === 'string' ? parseISO(date) : date;
  return format(d, 'yyyy-MM-dd');
}

export function formatDisplayDate(date: Date | string): string {
  const d = typeof date === 'string' ? parseISO(date) : date;
  return format(d, 'MMM d, yyyy');
}

export function formatShortDate(date: Date | string): string {
  const d = typeof date === 'string' ? parseISO(date) : date;
  return format(d, 'MMM d');
}

export function formatDuration(seconds: number): string {
  if (seconds < 60) {
    return `${Math.round(seconds)} secs`;
  }
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  
  if (hours > 0) {
    return `${hours} hr${hours !== 1 ? 's' : ''} ${minutes} min${minutes !== 1 ? 's' : ''}`;
  }
  return `${minutes} min${minutes !== 1 ? 's' : ''}`;
}

export function formatDurationShort(seconds: number): string {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  return `${hours}:${minutes.toString().padStart(2, '0')}`;
}

export function formatHours(seconds: number): string {
  const hours = seconds / 3600;
  return hours.toFixed(1);
}

export type DateRange = 'today' | 'yesterday' | 'last7days' | 'last14days' | 'last30days' | 'last3months' | 'last6months' | 'lastYear' | 'thisWeek' | 'lastWeek' | 'thisMonth' | 'lastMonth' | 'custom';

export interface DateRangeValue {
  start: Date;
  end: Date;
  label: string;
}

export function getDateRange(range: DateRange, customStart?: Date, customEnd?: Date): DateRangeValue {
  const today = new Date();
  const yesterday = subDays(today, 1);

  switch (range) {
    case 'today':
      return { start: today, end: today, label: 'Today' };
    case 'yesterday':
      return { start: yesterday, end: yesterday, label: 'Yesterday' };
    case 'last7days':
      return { start: subDays(yesterday, 6), end: yesterday, label: 'Last 7 Days' };
    case 'last14days':
      return { start: subDays(yesterday, 13), end: yesterday, label: 'Last 14 Days' };
    case 'last30days':
      return { start: subDays(yesterday, 29), end: yesterday, label: 'Last 30 Days' };
    case 'last3months':
      return { start: subDays(yesterday, 89), end: yesterday, label: 'Last 3 Months' };
    case 'last6months':
      return { start: subDays(yesterday, 179), end: yesterday, label: 'Last 6 Months' };
    case 'lastYear':
      return { start: subDays(yesterday, 364), end: yesterday, label: 'Last Year' };
    case 'thisWeek':
      return { start: startOfWeek(today, { weekStartsOn: 1 }), end: today, label: 'This Week' };
    case 'lastWeek':
      const lastWeekEnd = subDays(startOfWeek(today, { weekStartsOn: 1 }), 1);
      return { start: startOfWeek(lastWeekEnd, { weekStartsOn: 1 }), end: lastWeekEnd, label: 'Last Week' };
    case 'thisMonth':
      return { start: startOfMonth(today), end: today, label: 'This Month' };
    case 'lastMonth':
      const lastMonthEnd = subDays(startOfMonth(today), 1);
      return { start: startOfMonth(lastMonthEnd), end: endOfMonth(lastMonthEnd), label: 'Last Month' };
    case 'custom':
      return {
        start: customStart || subDays(yesterday, 6),
        end: customEnd || yesterday,
        label: 'Custom Range'
      };
    default:
      return { start: subDays(yesterday, 6), end: yesterday, label: 'Last 7 Days' };
  }
}

// Chart colors palette
export const COLORS = [
  '#58a6ff', // blue
  '#3fb950', // green
  '#a371f7', // purple
  '#f78166', // orange
  '#d29922', // yellow
  '#f85149', // red
  '#79c0ff', // light blue
  '#7ee787', // light green
  '#d2a8ff', // light purple
  '#ffa657', // light orange
  '#e3b341', // light yellow
  '#ff7b72', // light red
  '#56d4dd', // cyan
  '#bc8cff', // violet
  '#ffc8be', // salmon
];

export function getColor(index: number): string {
  return COLORS[index % COLORS.length];
}

export function getProjectColor(projectName: string): string {
  // Generate consistent color based on project name
  let hash = 0;
  for (let i = 0; i < projectName.length; i++) {
    hash = projectName.charCodeAt(i) + ((hash << 5) - hash);
  }
  return COLORS[Math.abs(hash) % COLORS.length];
}
