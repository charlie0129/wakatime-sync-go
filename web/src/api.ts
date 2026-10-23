import { rangeQuery } from './dates';
import type { Range } from './dates';

// --- Dashboard ---

export interface DayActivity {
  date: string;
  total_seconds: number;
  ai_coding_seconds: number;
  ai_lines: number;
  human_lines: number;
  ai_tokens: number;
  ai_cost: number;
  model_costs: Record<string, number>;
}

// Time spent in a project, editor, language, etc.; AI metrics are only set for projects and editors.
export interface BreakdownItem {
  name: string;
  total_seconds: number;
  ai_lines: number;
  human_lines: number;
  ai_tokens: number;
  ai_cost: number;
}

export interface AISummary {
  coding_seconds: number;
  active_days: number;
  lines: number;
  human_lines: number;
  input_tokens: number;
  cached_input_tokens: number;
  output_tokens: number;
  cost: number;
  sessions: number;
  prompts: number;
  prompt_length_avg: number;
}

export interface AIModelUsage {
  name: string;
  lines: number;
  input_tokens: number;
  cached_input_tokens: number;
  output_tokens: number;
  cost: number;
}

// Token usage of an AI model version, from the plugins that sent heartbeats.
export interface AIModelVersionUsage {
  model: string;
  version: string;
  complexity: string;
  input_tokens: number;
  cached_input_tokens: number;
  output_tokens: number;
  sessions: number;
  heartbeats: number;
}

export interface Dashboard {
  range: string;
  start: string;
  end: string;
  today: string;
  total_seconds: number;
  active_days: number;
  days: DayActivity[];
  // The period of as many days before the range
  previous: {
    start: string;
    end: string;
    total_seconds: number;
    active_days: number;
    ai: AISummary;
  };
  categories: BreakdownItem[];
  languages: BreakdownItem[];
  editors: BreakdownItem[];
  operating_systems: BreakdownItem[];
  projects: BreakdownItem[];
  machines: BreakdownItem[];
  ai: AISummary;
  ai_models: AIModelUsage[];
  ai_model_versions: AIModelVersionUsage[];
}

// When coding happened on each day: spans of [start, end, ai] seconds since midnight, where ai is 1
// when AI coding tools were used during the span.
export interface Activity {
  start: string;
  end: string;
  timezone: string;
  days: Array<{ date: string; spans: Array<[number, number, number]> }>;
}

// --- Day details ---

export interface Duration {
  project: string;
  time: number;
  duration: number;
  ai_additions: number;
  ai_deletions: number;
  human_additions: number;
  human_deletions: number;
  ai_input_tokens: number;
  ai_cached_input_tokens: number;
  ai_output_tokens: number;
  ai_model_total_cost: number;
  ai_model_breakdown: Array<{ name: string; lines: number; cost: number }>;
}

// --- Calendar ---

export interface YearDay {
  date: string;
  total_seconds: number;
  projects?: Array<{ name: string; total_seconds: number }>;
}

// --- Sync ---

export interface SyncRun {
  started_at: string;
  finished_at?: string;
  days: number;
  done: number;
  failed: number;
}

export interface SyncStatus {
  last_synced_day: string;
  failed_days: string[];
  running: boolean;
  run: SyncRun | null;
  today: string;
  timezone: string;
}

async function get<T>(path: string, signal: AbortSignal): Promise<T> {
  const response = await fetch(`/api/v1/${path}`, { signal });
  if (!response.ok) {
    const body = await response.json().catch(() => null);
    throw new Error(body?.error ?? `The server responded with ${response.status}.`);
  }
  return response.json();
}

export const api = {
  dashboard: (range: Range, signal: AbortSignal) => get<Dashboard>(`dashboard?${rangeQuery(range)}`, signal),
  activity: (range: Range, signal: AbortSignal) => get<Activity>(`activity?${rangeQuery(range)}`, signal),
  durations: (date: string, signal: AbortSignal) =>
    get<{ data: Duration[] }>(`users/current/durations?date=${date}`, signal).then((r) => r.data),
  years: (signal: AbortSignal) => get<{ years: number[] }>('stats/years', signal).then((r) => r.years ?? []),
  year: (year: number, signal: AbortSignal) =>
    get<{ data: YearDay[] | null }>(`stats/yearly?year=${year}`, signal).then((r) => r.data ?? []),
  syncStatus: (signal: AbortSignal) => get<SyncStatus>('sync/status', signal),
};
