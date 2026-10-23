import { useMemo, useState } from 'react';
import type { PointerEvent } from 'react';
import type { Activity, DayActivity, Duration } from './api';
import { Key, Section, Toggle, Tooltip, TooltipRows, TooltipTitle } from './components';
import { countDays, startOfWeek, weekday } from './dates';
import {
  formatClock, formatCompact, formatCost, formatDayLabel, formatDuration, formatHours, formatInteger,
  formatLongDate, formatShortDate,
} from './format';
import { useWidth } from './hooks';
import './Rhythm.css';

const DAY_SECONDS = 24 * 60 * 60;
// Longer ranges are drawn one week per row
const MAX_DAY_ROWS = 62;
// Weeks are drawn in cells of half an hour
const CELL_SECONDS = 30 * 60;
const MAX_HEIGHT = 420;

// A span of coding time in seconds since midnight, with ai set to 1 when AI tools were used.
type Span = [start: number, end: number, ai: number];

interface Cell {
  // days of the week with coding in the cell, and whether AI tools were used for most of it
  days: number;
  ai: boolean;
}

interface Row {
  key: string;
  label: string;
  // marker rows keep their label when rows are too small for all of them
  marker: boolean;
  spans: Span[];
  cells?: Cell[];
  totalSeconds: number;
  // range selected by clicking the row
  select?: [string, string];
  // details for tooltips and the table
  day?: DayActivity;
  week?: { start: string; days: number; coded: number; aiCodingSeconds: number };
  durations?: Duration[];
}

type Mode = 'lanes' | 'days' | 'weeks';

interface Hover {
  row: number;
  // time of day under the pointer, and the pointer position in the plot
  seconds: number;
  x: number;
  y: number;
}

interface RhythmProps {
  activity: Activity;
  days: DayActivity[];
  // durations of the day, when the range is a single day
  durations?: Duration[];
  onSelect: (start: string, end: string) => void;
}

// Rhythm is an actogram: one row per day (or week, or project for a single day), hours of the day
// across, and coding time drawn where it happened.
export default function Rhythm({ activity, days, durations, onSelect }: RhythmProps) {
  const [view, setView] = useState<'chart' | 'table'>('chart');
  const [hover, setHover] = useState<Hover | null>(null);
  const [ref, width] = useWidth<HTMLDivElement>();

  const mode: Mode = activity.days.length === 1 ? 'lanes' : activity.days.length <= MAX_DAY_ROWS ? 'days' : 'weeks';
  const rows = useMemo(() => {
    if (mode === 'lanes') return laneRows(activity.days[0].date, durations ?? [], activity.timezone);
    if (mode === 'days') return dayRows(activity, days);
    return weekRows(activity, days);
  }, [mode, activity, days, durations]);

  const labelWidth = mode === 'lanes' ? Math.min(180, Math.max(96, width * 0.22)) : 72;
  const valueWidth = 56;
  const plotWidth = Math.max(0, width - labelWidth - valueWidth);
  const rowHeight = mode === 'lanes' ? 24 : clamp(Math.floor(MAX_HEIGHT / Math.max(rows.length, 1)), mode === 'weeks' ? 6 : 8, 28);
  const showAllLabels = rowHeight >= 14;
  const hours = plotWidth < 420 ? [0, 6, 12, 18, 24] : [0, 3, 6, 9, 12, 15, 18, 21, 24];
  const total = rows.reduce((sum, row) => sum + row.totalSeconds, 0);

  const onPointerMove = (event: PointerEvent<HTMLDivElement>, row: number) => {
    const box = event.currentTarget.getBoundingClientRect();
    const plotX = event.clientX - box.left - labelWidth;
    if (plotX < 0 || plotX > plotWidth) {
      setHover(null);
      return;
    }
    setHover({ row, seconds: (plotX / plotWidth) * DAY_SECONDS, x: labelWidth + plotX, y: event.currentTarget.offsetTop });
  };

  const legend = (
    <>
      <Key tone="hand">By hand</Key>
      <Key tone="ai">With AI tools</Key>
      <Toggle label="View" value={view} options={[['chart', 'Chart'], ['table', 'Table']]} onChange={setView} />
    </>
  );
  const note = {
    lanes: 'One row per project. Point at a span for its details.',
    days: 'One row per day. Select a day to see its projects.',
    weeks: 'One row per week, in half hours. Darker cells were coded on more days of the week. Select a week to see its days.',
  }[mode];

  if (mode === 'lanes' && !durations) {
    return (
      <Section title="When you coded">
        <p className="empty-note">Loading the day…</p>
      </Section>
    );
  }
  if (total === 0 && rows.every((row) => row.spans.length === 0)) {
    return (
      <Section title="When you coded">
        <p className="empty-note">No coding time recorded in this period.</p>
      </Section>
    );
  }

  return (
    <Section title="When you coded" aside={legend} note={note}>
      {view === 'table' ? (
        <RhythmTable mode={mode} rows={rows} />
      ) : (
        <div className="rhythm" ref={ref} onPointerLeave={() => setHover(null)}>
          <div className="rhythm-axis" style={{ marginLeft: labelWidth, width: plotWidth }} aria-hidden="true">
            {hours.map((hour) => (
              <span key={hour} className="rhythm-hour" style={{ left: `${(hour / 24) * 100}%` }}>
                {hour}
              </span>
            ))}
            {hover && <span className="rhythm-clock num" style={{ left: hover.x - labelWidth }}>{formatClock(hover.seconds)}</span>}
          </div>

          <div className="rhythm-rows">
            {rows.map((row, i) => {
              const Label = row.select ? 'button' : 'span';
              return (
                <div
                  key={row.key}
                  className={`rhythm-row${row.select ? ' is-selectable' : ''}${hover?.row === i ? ' is-hovered' : ''}`}
                  style={{ height: rowHeight }}
                  onPointerMove={(event) => onPointerMove(event, i)}
                  onClick={row.select ? () => onSelect(...row.select!) : undefined}
                >
                  <Label
                    className={`rhythm-label${mode === 'lanes' ? ' code' : ''}`}
                    style={{ width: labelWidth }}
                    title={mode === 'lanes' ? row.label : undefined}
                    aria-label={row.select ? `Show ${rowTitle(mode, row)}` : undefined}
                  >
                    {(showAllLabels || row.marker) && row.label}
                  </Label>
                  <svg
                    className="rhythm-track"
                    style={{ width: plotWidth, height: rowHeight }}
                    viewBox={`0 0 ${DAY_SECONDS} 1`}
                    preserveAspectRatio="none"
                    aria-hidden="true"
                  >
                    {row.cells
                      ? row.cells.map((cell, c) =>
                          cell.days > 0 ? (
                            <rect
                              key={c}
                              className={cell.ai ? 'tone-ai' : 'tone-hand'}
                              x={c * CELL_SECONDS}
                              width={CELL_SECONDS - Math.min(CELL_SECONDS / 4, (2 * DAY_SECONDS) / Math.max(plotWidth, 1))}
                              y={0}
                              height={1}
                              opacity={cellOpacity(cell.days, row.week?.days ?? 7)}
                            />
                          ) : null,
                        )
                      : row.spans.map(([start, end, ai], s) => (
                          <rect
                            key={s}
                            className={ai ? 'tone-ai' : 'tone-hand'}
                            x={start}
                            width={Math.max(end - start, DAY_SECONDS / Math.max(plotWidth, 1))}
                            y={0.2}
                            height={0.6}
                          />
                        ))}
                  </svg>
                  <span className="rhythm-value num" style={{ width: valueWidth }}>
                    {rowHeight >= 12 && row.totalSeconds > 0 ? formatHours(row.totalSeconds) : ''}
                  </span>
                </div>
              );
            })}
            {hover && <span className="rhythm-crosshair" style={{ left: hover.x }} aria-hidden="true" />}
          </div>

          {hover && (
            <Tooltip x={hover.x} y={hover.y + 26} containerWidth={width}>
              <RhythmTooltip mode={mode} row={rows[hover.row]} seconds={hover.seconds} tolerance={(4 * DAY_SECONDS) / Math.max(plotWidth, 1)} />
            </Tooltip>
          )}
        </div>
      )}
    </Section>
  );
}

// RhythmTooltip describes the row under the pointer; for a project, the span within tolerance
// seconds of the pointer.
function RhythmTooltip({ mode, row, seconds, tolerance }: { mode: Mode; row: Row; seconds: number; tolerance: number }) {
  if (mode === 'lanes') {
    const index = nearestSpan(row.spans, seconds, tolerance);
    const duration = index >= 0 ? row.durations?.[index] : undefined;
    const span = row.spans[index];
    if (!duration || !span) {
      return (
        <>
          <TooltipTitle>
            <span className="code">{row.label}</span>
          </TooltipTitle>
          <TooltipRows rows={[[formatDuration(row.totalSeconds), 'coding this day']]} />
        </>
      );
    }
    const aiLines = duration.ai_additions + duration.ai_deletions;
    const humanLines = duration.human_additions + duration.human_deletions;
    const tokens = duration.ai_input_tokens + duration.ai_cached_input_tokens + duration.ai_output_tokens;
    const models = duration.ai_model_breakdown.map((m) => m.name);
    return (
      <>
        <TooltipTitle>
          <span className="code">{row.label}</span>
        </TooltipTitle>
        <TooltipRows
          rows={[
            [formatDuration(duration.duration), `from ${formatClock(span[0])} to ${formatClock(span[1])}`],
            (aiLines > 0 || humanLines > 0) && [formatInteger(aiLines), `lines by AI, ${formatInteger(humanLines)} by hand`],
            tokens > 0 && [formatCompact(tokens), 'tokens'],
            duration.ai_model_total_cost > 0 && [formatCost(duration.ai_model_total_cost), 'estimated cost'],
            models.length > 0 && [models.join(', '), models.length > 1 ? 'models' : 'model'],
          ]}
        />
      </>
    );
  }

  if (mode === 'weeks') {
    const week = row.week!;
    return (
      <>
        <TooltipTitle>Week of {formatShortDate(week.start)}</TooltipTitle>
        <TooltipRows
          rows={[
            [formatDuration(row.totalSeconds), 'coding'],
            [`${week.coded} of ${week.days}`, 'days coded'],
            week.aiCodingSeconds > 0 && [formatDuration(week.aiCodingSeconds), 'AI coding'],
          ]}
        />
      </>
    );
  }

  const day = row.day;
  const first = row.spans[0]?.[0];
  const last = row.spans[row.spans.length - 1]?.[1];
  return (
    <>
      <TooltipTitle>{formatLongDate(row.key)}</TooltipTitle>
      <TooltipRows
        rows={[
          [formatDuration(row.totalSeconds), 'coding'],
          !!day?.ai_coding_seconds && [formatDuration(day.ai_coding_seconds), 'AI coding'],
          first !== undefined && [`${formatClock(first)} to ${formatClock(last)}`, 'first and last activity'],
          !!day?.ai_cost && [formatCost(day.ai_cost), 'estimated AI cost'],
        ]}
      />
    </>
  );
}

function RhythmTable({ mode, rows }: { mode: Mode; rows: Row[] }) {
  const columns = {
    lanes: ['Project', 'Coding', 'With AI tools', 'First activity', 'Last activity'],
    days: ['Day', 'Coding', 'AI coding', 'First activity', 'Last activity'],
    weeks: ['Week of', 'Coding', 'Days coded', 'AI coding'],
  }[mode];

  return (
    <div className="table-scroll">
      <table className="data-table">
        <thead>
          <tr>
            {columns.map((column) => (
              <th key={column} scope="col">
                {column}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => {
            const first = row.spans[0]?.[0];
            const last = row.spans[row.spans.length - 1]?.[1];
            const clock = (seconds: number | undefined) => (seconds === undefined ? '' : formatClock(seconds));
            const cells =
              mode === 'weeks'
                ? [formatShortDate(row.week!.start), formatDuration(row.totalSeconds), `${row.week!.coded} of ${row.week!.days}`, formatDuration(row.week!.aiCodingSeconds)]
                : mode === 'days'
                  ? [formatLongDate(row.key), formatDuration(row.totalSeconds), formatDuration(row.day?.ai_coding_seconds ?? 0), clock(first), clock(last)]
                  : [row.label, formatDuration(row.totalSeconds), formatDuration(spanSeconds(row.spans, 1)), clock(first), clock(last)];
            return (
              <tr key={row.key}>
                <th scope="row" className={mode === 'lanes' ? 'code' : undefined}>
                  {cells[0]}
                </th>
                {cells.slice(1).map((cell, i) => (
                  <td key={i} className="num">
                    {cell}
                  </td>
                ))}
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

function rowTitle(mode: Mode, row: Row): string {
  return mode === 'weeks' ? `the week of ${formatShortDate(row.key)}` : formatLongDate(row.key);
}

function dayRows(activity: Activity, days: DayActivity[]): Row[] {
  const byDate = new Map(days.map((day) => [day.date, day]));
  return activity.days.map(({ date, spans }, i) => {
    const day = byDate.get(date);
    const firstOfMonth = date.endsWith('-01');
    return {
      key: date,
      label: i === 0 || firstOfMonth ? formatShortDate(date) : formatDayLabel(date),
      marker: i === 0 || firstOfMonth || weekday(date) === 0,
      spans,
      totalSeconds: day?.total_seconds ?? spanSeconds(spans),
      select: [date, date],
      day,
    };
  });
}

function weekRows(activity: Activity, days: DayActivity[]): Row[] {
  const byDate = new Map(days.map((day) => [day.date, day]));
  const weeks = new Map<string, Row & { week: NonNullable<Row['week']>; dates: string[]; tally: Array<{ days: number; ai: number; hand: number }> }>();

  for (const { date, spans } of activity.days) {
    const start = startOfWeek(date);
    let week = weeks.get(start);
    if (!week) {
      week = {
        key: start,
        label: formatShortDate(start),
        marker: weeks.size === 0 || Number(start.slice(8)) <= 7,
        spans: [],
        totalSeconds: 0,
        week: { start, days: 0, coded: 0, aiCodingSeconds: 0 },
        dates: [],
        tally: Array.from({ length: DAY_SECONDS / CELL_SECONDS }, () => ({ days: 0, ai: 0, hand: 0 })),
      };
      weeks.set(start, week);
    }

    const day = byDate.get(date);
    week.dates.push(date);
    week.week.days++;
    week.totalSeconds += day?.total_seconds ?? spanSeconds(spans);
    week.week.aiCodingSeconds += day?.ai_coding_seconds ?? 0;
    if (spans.length > 0) week.week.coded++;

    const touched = new Set<number>();
    for (const [start, end, ai] of spans) {
      for (let c = Math.floor(start / CELL_SECONDS); c * CELL_SECONDS < end; c++) {
        const overlap = Math.min(end, (c + 1) * CELL_SECONDS) - Math.max(start, c * CELL_SECONDS);
        if (overlap <= 0) continue;
        week.tally[c][ai ? 'ai' : 'hand'] += overlap;
        touched.add(c);
      }
    }
    for (const c of touched) week.tally[c].days++;
  }

  return [...weeks.values()].map((week) => ({
    ...week,
    cells: week.tally.map((t) => ({ days: t.days, ai: t.ai > t.hand })),
    spans: activity.days.filter((d) => week.dates.includes(d.date)).flatMap((d) => d.spans),
    select: [week.dates[0], week.dates[week.dates.length - 1]],
  }));
}

function laneRows(date: string, durations: Duration[], timeZone: string): Row[] {
  const toSeconds = secondsSinceMidnight(date, timeZone);
  const lanes = new Map<string, Row & { durations: Duration[] }>();
  for (const duration of [...durations].sort((a, b) => a.time - b.time)) {
    const start = Math.max(0, toSeconds(duration.time));
    const end = Math.min(DAY_SECONDS, toSeconds(duration.time) + duration.duration);
    if (end <= start) continue;

    const project = duration.project || 'Unknown project';
    let lane = lanes.get(project);
    if (!lane) {
      lane = { key: project, label: project, marker: true, spans: [], totalSeconds: 0, durations: [] };
      lanes.set(project, lane);
    }
    const ai = duration.ai_input_tokens + duration.ai_cached_input_tokens + duration.ai_output_tokens + duration.ai_additions + duration.ai_deletions > 0;
    lane.spans.push([start, end, ai ? 1 : 0]);
    lane.durations.push(duration);
    lane.totalSeconds += end - start;
  }
  return [...lanes.values()].sort((a, b) => b.totalSeconds - a.totalSeconds);
}

// secondsSinceMidnight returns a function converting UNIX timestamps to seconds since midnight of
// the date in the given timezone, falling back to the browser's for one it does not know.
function secondsSinceMidnight(date: string, timeZone: string): (timestamp: number) => number {
  let format: Intl.DateTimeFormat;
  try {
    format = new Intl.DateTimeFormat('en-US', { timeZone, hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' });
  } catch {
    format = new Intl.DateTimeFormat('en-US', { hourCycle: 'h23', year: 'numeric', month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' });
  }
  return (timestamp) => {
    const parts = Object.fromEntries(format.formatToParts(new Date(timestamp * 1000)).map((p) => [p.type, p.value]));
    const localDate = `${parts.year}-${parts.month}-${parts.day}`;
    const dayOffset = localDate === date ? 0 : localDate > date ? countDays(date, localDate) - 1 : 1 - countDays(localDate, date);
    return dayOffset * DAY_SECONDS + Number(parts.hour) * 3600 + Number(parts.minute) * 60 + Number(parts.second);
  };
}

// nearestSpan returns the index of the span closest to seconds, if within tolerance, or -1.
function nearestSpan(spans: Span[], seconds: number, tolerance: number): number {
  let nearest = -1;
  let nearestDistance = tolerance;
  spans.forEach(([start, end], i) => {
    const distance = seconds < start ? start - seconds : seconds > end ? seconds - end : 0;
    if (distance <= nearestDistance) {
      nearest = i;
      nearestDistance = distance;
    }
  });
  return nearest;
}

// spanSeconds returns the time covered by spans, only those of one kind when ai is given.
function spanSeconds(spans: Span[], ai?: number): number {
  return spans.reduce((sum, [start, end, kind]) => (ai === undefined || kind === ai ? sum + end - start : sum), 0);
}

// cellOpacity shades a cell of a week by the share of the week's days coded at that time.
function cellOpacity(coded: number, days: number): number {
  const share = coded / Math.max(days, 1);
  return share > 0.75 ? 1 : share > 0.5 ? 0.75 : share > 0.25 ? 0.5 : 0.3;
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value));
}
