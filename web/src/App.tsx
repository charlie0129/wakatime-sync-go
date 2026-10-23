import { useCallback, useState } from 'react';
import { api } from './api';
import type { Dashboard } from './api';
import AISection from './AISection';
import Calendar from './Calendar';
import { BarList, Figures, Section } from './components';
import type { Figure } from './components';
import { countDays, rangeQuery } from './dates';
import type { Range } from './dates';
import { describeChange, formatDuration, formatRange, formatShare, formatShortDate } from './format';
import { useData, useRange, useTick } from './hooks';
import Rhythm from './Rhythm';
import TopBar from './TopBar';
import './App.css';

// How often to refresh the sync state, which also shows the progress of a sync
const STATUS_INTERVAL = 15_000;

export default function App() {
  const [range, setRange] = useRange();
  // The year the calendar shows, when chosen; otherwise that of the range
  const [calendarYear, setCalendarYear] = useState<number | null>(null);
  const key = rangeQuery(range);

  const loadDashboard = useCallback((signal: AbortSignal) => api.dashboard(range, signal), [range]);
  const loadActivity = useCallback((signal: AbortSignal) => api.activity(range, signal), [range]);
  const dashboard = useData(key, loadDashboard);
  const activity = useData(key, loadActivity);
  const dash = dashboard.data;

  // The projects of a single day are drawn from its durations
  const day = dash && !dashboard.loading && dash.start === dash.end ? dash.start : null;
  const loadDurations = useCallback((signal: AbortSignal) => api.durations(day!, signal), [day]);
  const durations = useData(day !== null ? `durations:${day}` : null, loadDurations);

  const tick = useTick(STATUS_INTERVAL);
  const loadStatus = useCallback((signal: AbortSignal) => api.syncStatus(signal), []);
  const status = useData(`status:${tick}`, loadStatus);

  const loadYears = useCallback((signal: AbortSignal) => api.years(signal), []);
  const years = useData('years', loadYears);
  const year = calendarYear ?? (dash ? Number(dash.end.slice(0, 4)) : null);
  const loadYear = useCallback((signal: AbortSignal) => api.year(year!, signal), [year]);
  const yearDays = useData(year !== null ? `year:${year}` : null, loadYear);

  const navigate = useCallback(
    (next: Range) => {
      setCalendarYear(null);
      setRange(next);
    },
    [setRange],
  );
  const selectDays = useCallback((start: string, end: string) => navigate({ start, end }), [navigate]);

  const loading = dashboard.loading || activity.loading;
  const error = dashboard.error ?? activity.error;
  const retry = () => {
    dashboard.retry();
    activity.retry();
  };

  const today = dash?.today ?? status.data?.today ?? null;
  const knownYears = years.data ?? [];
  const previousYear = year ? knownYears.filter((y) => y < year).sort((a, b) => b - a)[0] : undefined;
  const nextYear = year ? knownYears.filter((y) => y > year).sort((a, b) => a - b)[0] : undefined;

  return (
    <>
      <TopBar
        range={range}
        resolved={dash && !dashboard.loading ? { start: dash.start, end: dash.end } : null}
        today={today}
        status={status.data}
        onChange={navigate}
      />

      <main className="page" aria-busy={loading}>
        {error && (
          <div className="error-note" role="alert">
            <p>Couldn't load the dashboard. {error.message}</p>
            <button className="text-button" onClick={retry}>
              Try again
            </button>
          </div>
        )}

        {!dash ? (
          !error && <p className="page-loading">Loading…</p>
        ) : (
          <div className={`page-content${loading ? ' is-loading' : ''}`}>
            <header className="period">
              <h1 className="period-title">{formatRange(dash.start, dash.end)}</h1>
              <p className="period-note">{describePeriod(dash)}</p>
            </header>
            <Figures items={periodFigures(dash)} />

            {activity.data && (
              <Rhythm
                activity={activity.data}
                days={dash.days}
                durations={day && day === activity.data.days[0]?.date && !durations.loading ? durations.data : undefined}
                onSelect={selectDays}
              />
            )}

            <Breakdowns dash={dash} />

            {hasAIActivity(dash) && <AISection dashboard={dash} />}

            {year && (
              <Section
                title={year}
                note="Select a day, or drag across days, to show them above."
                aside={
                  <div className="year-nav">
                    <button className="year-button" disabled={previousYear === undefined} onClick={() => setCalendarYear(previousYear!)}>
                      Previous year
                    </button>
                    <button className="year-button" disabled={nextYear === undefined} onClick={() => setCalendarYear(nextYear!)}>
                      Next year
                    </button>
                  </div>
                }
              >
                <Calendar
                  year={year}
                  days={yearDays.data ?? []}
                  today={today ?? dash.end}
                  selection={{ start: dash.start, end: dash.end }}
                  onSelect={selectDays}
                />
              </Section>
            )}
          </div>
        )}
      </main>
    </>
  );
}

// describePeriod says what the changes in the figures compare with.
function describePeriod(dash: Dashboard): string {
  const days = countDays(dash.start, dash.end);
  const { start, end } = dash.previous;
  if (days === 1) {
    return `Compared with the day before, ${formatShortDate(start)}.`;
  }
  return `${days} days, compared with the ${days} days before: ${formatRange(start, end)}.`;
}

function periodFigures(dash: Dashboard): Figure[] {
  const days = countDays(dash.start, dash.end);
  const figures: Figure[] = [
    { label: 'Coding time', value: formatDuration(dash.total_seconds), note: describeChange(dash.total_seconds, dash.previous.total_seconds) },
    { label: 'Days coded', value: `${dash.active_days} of ${days}`, note: null },
    {
      label: 'Per day coded',
      value: dash.active_days > 0 ? formatDuration(dash.total_seconds / dash.active_days) : '—',
      note: null,
    },
  ];
  if (dash.ai.coding_seconds > 0) {
    figures.push({
      label: 'AI coding',
      value: formatShare(dash.ai.coding_seconds / Math.max(dash.total_seconds, 1)),
      note: `${formatDuration(dash.ai.coding_seconds)} of the coding time`,
    });
  }
  return figures;
}

function hasAIActivity(dash: Dashboard): boolean {
  const { ai } = dash;
  return ai.coding_seconds > 0 || ai.lines > 0 || ai.cost > 0 || ai.input_tokens + ai.cached_input_tokens + ai.output_tokens > 0;
}

function Breakdowns({ dash }: { dash: Dashboard }) {
  const lists: Array<[string, Dashboard['projects'], boolean]> = [
    ['Projects', dash.projects, true],
    ['Languages', dash.languages, false],
    ['Editors', dash.editors, false],
    ['Categories', dash.categories, false],
    ['Operating systems', dash.operating_systems, false],
    ['Machines', dash.machines, false],
  ];
  return (
    <Section title="Where the time went">
      <div className="breakdowns">
        {lists
          // A single item, such as the only machine, says nothing about where the time went
          .filter(([, items]) => items.length > 1)
          .map(([title, items, code]) => (
            <BarList
              key={title}
              title={title}
              code={code}
              total={dash.total_seconds}
              items={items.map((item) => ({ name: item.name, value: item.total_seconds, label: formatDuration(item.total_seconds) }))}
            />
          ))}
      </div>
    </Section>
  );
}
