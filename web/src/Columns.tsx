import { useState } from 'react';
import type { ReactNode } from 'react';
import { Tooltip, TooltipRows, TooltipTitle } from './components';
import { useWidth } from './hooks';
import './Columns.css';

export interface ColumnSeries {
  key: string;
  name: string;
  tone: 'hand' | 'ai' | 'bar';
}

// Column is a day or week, with a value per series, and any data the caller needs for tooltips.
export interface Column<T = unknown> {
  key: string;
  label: string;
  title: string;
  values: Record<string, number>;
  data?: T;
}

interface ColumnsProps<T> {
  columns: Column<T>[];
  // series are stacked from the bottom up
  series: ColumnSeries[];
  format: (value: number) => string;
  formatTick: (value: number) => string;
  // details returns more tooltip rows for a column
  details?: (column: Column<T>) => Array<[ReactNode, ReactNode]>;
}

const HEIGHT = 200;
const MARGIN = { top: 8, bottom: 24, left: 52 };

// Columns is a stacked column chart, one column per day or week.
export default function Columns<T>({ columns, series, format, formatTick, details }: ColumnsProps<T>) {
  const [ref, width] = useWidth<HTMLDivElement>();
  const [hover, setHover] = useState<number | null>(null);

  const plotWidth = Math.max(0, width - MARGIN.left);
  const plotHeight = HEIGHT - MARGIN.top - MARGIN.bottom;
  const totals = columns.map((column) => series.reduce((sum, s) => sum + (column.values[s.key] ?? 0), 0));
  const ticks = niceTicks(Math.max(0, ...totals));
  const top = ticks[ticks.length - 1] || 1;
  const band = plotWidth / Math.max(columns.length, 1);
  const barWidth = Math.max(1, Math.min(24, band * 0.7));
  const labelEvery = Math.max(1, Math.ceil(columns.length / Math.max(1, Math.floor(plotWidth / 64))));
  const y = (value: number) => MARGIN.top + plotHeight - (value / top) * plotHeight;

  return (
    <div className="columns" ref={ref} onPointerLeave={() => setHover(null)}>
      {width > 0 && (
        <svg width={width} height={HEIGHT} aria-hidden="true">
          {ticks.map((tick) => (
            <g key={tick}>
              <line className={tick === 0 ? 'columns-baseline' : 'columns-grid'} x1={MARGIN.left} x2={width} y1={y(tick)} y2={y(tick)} />
              <text className="columns-tick" x={MARGIN.left - 8} y={y(tick)} dy="0.32em" textAnchor="end">
                {formatTick(tick)}
              </text>
            </g>
          ))}

          {columns.map((column, i) => {
            const x = MARGIN.left + i * band + (band - barWidth) / 2;
            const shown = series.filter((s) => (column.values[s.key] ?? 0) > 0);
            let stacked = 0;
            return (
              <g key={column.key}>
                {hover === i && <rect className="columns-hover" x={MARGIN.left + i * band} y={MARGIN.top} width={band} height={plotHeight} />}
                {shown.map((s, j) => {
                  const value = column.values[s.key];
                  const bottom = y(stacked);
                  stacked += value;
                  const height = bottom - y(stacked);
                  // The top segment gets the rounded end; the others leave a gap above them
                  const isTop = j === shown.length - 1;
                  const gap = !isTop && height > 3 ? 2 : 0;
                  return (
                    <path key={s.key} className={`tone-${s.tone}`} d={columnPath(x, y(stacked) + gap, barWidth, height - gap, isTop && barWidth >= 6 ? 2 : 0)} />
                  );
                })}
                {i % labelEvery === 0 && (
                  <text className="columns-label" x={MARGIN.left + i * band + band / 2} y={HEIGHT - 6} textAnchor="middle">
                    {column.label}
                  </text>
                )}
                <rect className="columns-hit" x={MARGIN.left + i * band} y={MARGIN.top} width={band} height={plotHeight} onPointerEnter={() => setHover(i)} />
              </g>
            );
          })}
        </svg>
      )}

      {hover !== null && (
        <Tooltip x={MARGIN.left + hover * band + band / 2} y={y(totals[hover])} containerWidth={width}>
          <TooltipTitle>{columns[hover].title}</TooltipTitle>
          <TooltipRows
            rows={[
              ...[...series].reverse().map((s): [ReactNode, ReactNode] => [format(columns[hover].values[s.key] ?? 0), s.name]),
              ...(details?.(columns[hover]) ?? []),
            ]}
          />
        </Tooltip>
      )}
    </div>
  );
}

// niceTicks returns about four round ticks from 0 to at least max.
function niceTicks(max: number): number[] {
  if (max <= 0) return [0];
  const rough = max / 4;
  const power = 10 ** Math.floor(Math.log10(rough));
  const step = [1, 2, 2.5, 5, 10].map((m) => m * power).find((s) => s >= rough) ?? 10 * power;
  const ticks: number[] = [];
  for (let tick = 0; tick < max + step; tick += step) {
    ticks.push(tick);
  }
  return ticks;
}

// columnPath draws a column with its top corners rounded by radius.
function columnPath(x: number, y: number, width: number, height: number, radius: number): string {
  if (height <= 0) return '';
  const r = Math.min(radius, width / 2, height);
  return `M${x},${y + height}V${y + r}Q${x},${y} ${x + r},${y}H${x + width - r}Q${x + width},${y} ${x + width},${y + r}V${y + height}Z`;
}
