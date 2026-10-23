import { useEffect, useMemo, useRef, useState } from 'react';
import type { YearDay } from './api';
import { Tooltip, TooltipRows, TooltipTitle } from './components';
import { countDays, eachDay, startOfWeek, weekday } from './dates';
import { formatDuration, formatLongDate } from './format';
import { useWidth } from './hooks';
import './Calendar.css';

// Hours of coding that start levels 2 to 5; any time at all is level 1
const LEVEL_HOURS = [1, 3, 6, 9];
const LEVEL_NAMES = ['None', 'Under 1 h', '1 to 3 h', '3 to 6 h', '6 to 9 h', '9 h or more'];
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

const LABEL_WIDTH = 32;
const HEAD_HEIGHT = 18;
const GAP = 2;

function level(seconds: number): number {
  if (seconds <= 0) return 0;
  return 1 + LEVEL_HOURS.filter((hours) => seconds >= hours * 3600).length;
}

function ordered(a: string, b: string): { start: string; end: string } {
  return a <= b ? { start: a, end: b } : { start: b, end: a };
}

interface CalendarProps {
  year: number;
  days: YearDay[];
  today: string;
  selection: { start: string; end: string } | null;
  onSelect: (start: string, end: string) => void;
}

// Calendar shows the coding time of each day of a year, and selects days by clicking or dragging.
export default function Calendar({ year, days, today, selection, onSelect }: CalendarProps) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [drag, setDrag] = useState<{ from: string; to: string } | null>(null);
  const [hover, setHover] = useState<{ date: string; x: number; y: number } | null>(null);
  // On narrow screens the year scrolls sideways, starting with its last weeks
  const scroller = useRef<HTMLDivElement>(null);
  const drawn = width > 0;
  useEffect(() => {
    if (scroller.current) scroller.current.scrollLeft = scroller.current.scrollWidth;
  }, [year, drawn]);

  const byDate = useMemo(() => new Map(days.map((day) => [day.date, day])), [days]);
  const first = `${year}-01-01`;
  const last = `${year}-12-31`;
  const gridStart = startOfWeek(first);
  const weeks = Math.ceil(countDays(gridStart, last) / 7);
  const cell = Math.max(8, Math.min(16, Math.floor((width - LABEL_WIDTH) / weeks) - GAP));
  const step = cell + GAP;
  const position = (date: string) => ({
    x: LABEL_WIDTH + Math.floor((countDays(gridStart, date) - 1) / 7) * step,
    y: HEAD_HEIGHT + weekday(date) * step,
  });

  const shown = drag ? ordered(drag.from, drag.to) : selection;
  const dims = shown !== null && shown.start <= last && shown.end >= first;

  // A drag ends wherever the pointer is released
  useEffect(() => {
    if (!drag) return;
    const finish = () => {
      const { start, end } = ordered(drag.from, drag.to);
      setDrag(null);
      onSelect(start, end);
    };
    window.addEventListener('pointerup', finish);
    return () => window.removeEventListener('pointerup', finish);
  }, [drag, onSelect]);

  const hovered = hover && byDate.get(hover.date);

  return (
    <div className="calendar" ref={ref}>
      <div className="calendar-scroll" ref={scroller}>
        {drawn && (
          <svg
            className="calendar-grid"
            width={LABEL_WIDTH + weeks * step}
            height={HEAD_HEIGHT + 7 * step}
            onPointerLeave={() => setHover(null)}
            role="img"
            aria-label={`Coding time per day in ${year}`}
          >
            {MONTHS.map((month, m) => (
              <text key={month} className="calendar-month" x={position(`${year}-${String(m + 1).padStart(2, '0')}-01`).x} y={11}>
                {month}
              </text>
            ))}
            {['Mon', 'Wed', 'Fri'].map((name, i) => (
              <text key={name} className="calendar-weekday" x={0} y={HEAD_HEIGHT + i * 2 * step + cell - 2}>
                {name}
              </text>
            ))}

            {eachDay(first, last).map((date) => {
              const { x, y } = position(date);
              if (date > today) {
                return <rect key={date} className="calendar-day level-0 is-future" x={x} y={y} width={cell} height={cell} />;
              }
              const selected = !dims || (shown!.start <= date && date <= shown!.end);
              return (
                <rect
                  key={date}
                  className={`calendar-day level-${level(byDate.get(date)?.total_seconds ?? 0)}${selected ? '' : ' is-dimmed'}`}
                  x={x}
                  y={y}
                  width={cell}
                  height={cell}
                  onPointerDown={(event) => {
                    event.preventDefault();
                    setDrag({ from: date, to: date });
                  }}
                  onPointerEnter={() => {
                    setHover({ date, x: x + cell / 2 - (scroller.current?.scrollLeft ?? 0), y });
                    if (drag) setDrag({ ...drag, to: date });
                  }}
                />
              );
            })}
          </svg>
        )}
      </div>

      {hover && (
        <Tooltip x={hover.x} y={hover.y} containerWidth={width}>
          <TooltipTitle>{formatLongDate(hover.date)}</TooltipTitle>
          <TooltipRows
            rows={[
              [formatDuration(hovered?.total_seconds ?? 0), 'coding'],
              ...(hovered?.projects ?? []).slice(0, 3).map((p): [string, string] => [formatDuration(p.total_seconds), p.name]),
            ]}
          />
        </Tooltip>
      )}

      <div className="calendar-legend">
        {LEVEL_NAMES.map((name, i) => (
          <span key={name} className="calendar-legend-item">
            <svg width="10" height="10" aria-hidden="true">
              <rect className={`calendar-day level-${i}`} width="10" height="10" />
            </svg>
            {name}
          </span>
        ))}
      </div>
    </div>
  );
}
