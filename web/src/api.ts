const API_BASE = import.meta.env.DEV ? 'http://localhost:3040' : '';

export interface DurationData {
  project: string;
  time: number;
  duration: number;
  entity?: string;
  language?: string;
  branch?: string;
  type?: string;
}

export interface DurationResponse {
  data: DurationData[];
  start: string;
  end: string;
  timezone: string;
}

export interface HeartbeatData {
  entity: string;
  type: string;
  category: string;
  time: number;
  project: string;
  branch: string;
  language: string;
  is_write: boolean;
  machine_name_id: string;
  lines: number;
  lineno: number;
  cursorpos: number;
}

export interface HeartbeatResponse {
  data: HeartbeatData[];
  start: string;
  end: string;
  timezone: string;
}

export interface SummaryItem {
  name: string;
  total_seconds: number;
  percent: number;
  digital: string;
  hours: number;
  minutes: number;
  seconds?: number;
  text: string;
}

export interface GrandTotal {
  total_seconds: number;
  digital: string;
  hours: number;
  minutes: number;
  text: string;
}

export interface SummaryRange {
  date: string;
  start: string;
  end: string;
  text: string;
  timezone: string;
}

export interface DaySummary {
  grand_total: GrandTotal;
  categories: SummaryItem[];
  languages: SummaryItem[];
  editors: SummaryItem[];
  operating_systems: SummaryItem[];
  projects: SummaryItem[];
  dependencies: SummaryItem[];
  machines: SummaryItem[];
  range: SummaryRange;
}

export interface CumulativeTotal {
  seconds: number;
  text: string;
  digital: string;
}

export interface DailyAverage {
  seconds: number;
  text: string;
  days_including_holidays: number;
}

export interface SummariesResponse {
  data: DaySummary[];
  cumulative_total: CumulativeTotal;
  daily_average: DailyAverage;
  start: string;
  end: string;
}

export interface ProjectData {
  id: string;
  name: string;
  repository: string;
  badge: string;
  color: string;
  has_public_url: boolean;
  last_heartbeat_at: string;
  first_heartbeat_at: string;
  created_at: string;
}

export interface ProjectsResponse {
  data: ProjectData[];
}

export interface DailyStat {
  date: string;
  total_seconds: number;
  text: string;
}

export interface DailyStatsResponse {
  data: DailyStat[];
}

export interface RangeStats {
  total_seconds: number;
  text: string;
  categories: SummaryItem[];
  languages: SummaryItem[];
  editors: SummaryItem[];
  operating_systems: SummaryItem[];
  projects: SummaryItem[];
  projects_daily: Array<{
    day: string;
    name: string;
    total_seconds: number;
  }>;
  start: string;
  end: string;
}

export interface SyncStatus {
  last_synced_day: string;
}

class ApiClient {
  private async fetch<T>(endpoint: string, params?: Record<string, string>): Promise<T> {
    const url = new URL(API_BASE + endpoint);
    if (params) {
      Object.entries(params).forEach(([key, value]) => {
        if (value) url.searchParams.set(key, value);
      });
    }

    const response = await fetch(url.toString());
    if (!response.ok) {
      throw new Error(`API error: ${response.status}`);
    }
    return response.json();
  }

  async getDurations(date: string, project?: string): Promise<DurationResponse> {
    return this.fetch('/api/v1/users/current/durations', { date, project: project || '' });
  }

  async getHeartbeats(date: string): Promise<HeartbeatResponse> {
    return this.fetch('/api/v1/users/current/heartbeats', { date });
  }

  async getSummaries(start: string, end: string): Promise<SummariesResponse> {
    return this.fetch('/api/v1/users/current/summaries', { start, end });
  }

  async getProjects(query?: string): Promise<ProjectsResponse> {
    return this.fetch('/api/v1/users/current/projects', { q: query || '' });
  }

  async getDailyStats(start: string, end: string): Promise<DailyStatsResponse> {
    return this.fetch('/api/v1/stats/daily', { start, end });
  }

  async getRangeStats(start: string, end: string): Promise<RangeStats> {
    return this.fetch('/api/v1/stats/range', { start, end });
  }

  async getSyncStatus(): Promise<SyncStatus> {
    return this.fetch('/api/v1/sync/status');
  }

  async triggerSync(days: number, apiKey: string): Promise<{ message: string }> {
    const url = new URL(API_BASE + '/api/v1/sync');
    url.searchParams.set('days', days.toString());
    url.searchParams.set('api_key', apiKey);

    const response = await fetch(url.toString(), { method: 'POST' });
    if (!response.ok) {
      throw new Error(`Sync failed: ${response.status}`);
    }
    return response.json();
  }
}

export const api = new ApiClient();
