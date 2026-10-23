import { useMemo, useState } from 'react';
import type { ReactNode } from 'react';
import type { AIModelUsage, BreakdownItem, Dashboard, DayActivity } from './api';
import Columns from './Columns';
import type { Column, ColumnSeries } from './Columns';
import { BarList, Figures, Key, Section, Toggle } from './components';
import type { BarListItem, Figure } from './components';
import { startOfWeek } from './dates';
import {
  describeChange, formatCompact, formatCost, formatCostCompact, formatInteger, formatLongDate, formatShare, formatShortDate,
} from './format';
import './AISection.css';

// Longer ranges are charted one column per week
const MAX_DAY_COLUMNS = 62;

const LINE_SERIES: ColumnSeries[] = [
  { key: 'ai', name: 'lines by AI', tone: 'ai' },
  { key: 'human', name: 'lines by hand', tone: 'hand' },
];
const COST_SERIES: ColumnSeries[] = [{ key: 'cost', name: 'estimated cost', tone: 'ai' }];

export default function AISection({ dashboard }: { dashboard: Dashboard }) {
  const [metric, setMetric] = useState<'lines' | 'cost'>('lines');
  const [view, setView] = useState<'chart' | 'table'>('chart');
  const { ai, previous } = dashboard;

  const totalLines = ai.lines + ai.human_lines;
  const tokens = ai.input_tokens + ai.cached_input_tokens + ai.output_tokens;
  const figures: Figure[] = [
    {
      label: 'Lines changed by AI',
      value: totalLines > 0 ? formatShare(ai.lines / totalLines) : '—',
      note: totalLines > 0 ? `${formatCompact(ai.lines)} of ${formatCompact(totalLines)} lines` : 'No lines changed',
    },
    { label: 'Estimated cost', value: formatCostCompact(ai.cost), note: describeChange(ai.cost, previous.ai.cost) },
    {
      label: 'Tokens',
      value: formatCompact(tokens),
      note: `${formatCompact(ai.input_tokens)} input, ${formatCompact(ai.cached_input_tokens)} cached, ${formatCompact(ai.output_tokens)} output`,
    },
    { label: 'Sessions', value: formatInteger(ai.sessions), note: `${formatInteger(ai.prompts)} prompts` },
  ];

  const weekly = dashboard.days.length > MAX_DAY_COLUMNS;
  const columns = useMemo(() => toColumns(dashboard.days, weekly), [dashboard.days, weekly]);
  const modelCosts = (column: ModelCostColumn): Array<[ReactNode, ReactNode]> =>
    Object.entries(column.data ?? {})
      .sort(([, a], [, b]) => b - a)
      .slice(0, 4)
      .map(([model, cost]) => [formatCost(cost), model]);

  return (
    <Section title="AI coding" note="Costs are estimated by WakaTime from the tokens each model used.">
      <Figures items={figures} />

      <div className="ai-block">
        <div className="ai-block-head">
          <h3 className="ai-block-title">{weekly ? 'Per week' : 'Per day'}</h3>
          <div className="section-aside">
            {view === 'chart' && metric === 'lines' && (
              <>
                <Key tone="ai">By AI</Key>
                <Key tone="hand">By hand</Key>
              </>
            )}
            <Toggle label="Measure" value={metric} options={[['lines', 'Lines changed'], ['cost', 'Estimated cost']]} onChange={setMetric} />
            <Toggle label="View" value={view} options={[['chart', 'Chart'], ['table', 'Table']]} onChange={setView} />
          </div>
        </div>
        {view === 'chart' ? (
          metric === 'lines' ? (
            <Columns columns={columns} series={LINE_SERIES} format={formatInteger} formatTick={formatCompact} />
          ) : (
            <Columns columns={columns} series={COST_SERIES} format={formatCost} formatTick={formatCostCompact} details={modelCosts} />
          )
        ) : (
          <ColumnsTable columns={columns} weekly={weekly} />
        )}
      </div>

      <div className="ai-lists">
        <BarList title="By project" code tone="ai" items={breakdownItems(dashboard.projects)} />
        <BarList title="By editor" tone="ai" items={breakdownItems(dashboard.editors)} />
      </div>

      <ModelsTable dashboard={dashboard} />
    </Section>
  );
}

// A column of lines and costs, with the cost of each model
type ModelCostColumn = Column<Record<string, number>>;

// toColumns returns a column per day, or per week.
function toColumns(days: DayActivity[], weekly: boolean): ModelCostColumn[] {
  if (!weekly) {
    return days.map((day) => ({
      key: day.date,
      label: formatShortDate(day.date),
      title: formatLongDate(day.date),
      values: { ai: day.ai_lines, human: day.human_lines, cost: day.ai_cost },
      data: day.model_costs,
    }));
  }

  const weeks = new Map<string, { values: { ai: number; human: number; cost: number }; models: Record<string, number> }>();
  for (const day of days) {
    const start = startOfWeek(day.date);
    const week = weeks.get(start) ?? { values: { ai: 0, human: 0, cost: 0 }, models: {} };
    week.values.ai += day.ai_lines;
    week.values.human += day.human_lines;
    week.values.cost += day.ai_cost;
    for (const [model, cost] of Object.entries(day.model_costs)) {
      week.models[model] = (week.models[model] ?? 0) + cost;
    }
    weeks.set(start, week);
  }
  return [...weeks].map(([start, week]) => ({
    key: start,
    label: formatShortDate(start),
    title: `Week of ${formatShortDate(start)}`,
    values: week.values,
    data: week.models,
  }));
}

function ColumnsTable({ columns, weekly }: { columns: ModelCostColumn[]; weekly: boolean }) {
  return (
    <div className="table-scroll">
      <table className="data-table">
        <thead>
          <tr>
            <th scope="col">{weekly ? 'Week of' : 'Day'}</th>
            <th scope="col">Lines by AI</th>
            <th scope="col">Lines by hand</th>
            <th scope="col">Estimated cost</th>
          </tr>
        </thead>
        <tbody>
          {columns.map((column) => (
            <tr key={column.key}>
              <th scope="row">{weekly ? column.label : column.title}</th>
              <td className="num">{formatInteger(column.values.ai)}</td>
              <td className="num">{formatInteger(column.values.human)}</td>
              <td className="num">{formatCost(column.values.cost)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

// breakdownItems ranks projects or editors by estimated AI cost, or by lines changed by AI when
// nothing was paid for.
function breakdownItems(items: BreakdownItem[]): BarListItem[] {
  const byCost = items.some((item) => item.ai_cost > 0);
  return items
    .filter((item) => (byCost ? item.ai_cost : item.ai_lines) > 0)
    .sort((a, b) => (byCost ? b.ai_cost - a.ai_cost : b.ai_lines - a.ai_lines))
    .map((item) => ({
      name: item.name,
      value: byCost ? item.ai_cost : item.ai_lines,
      label: byCost ? formatCost(item.ai_cost) : `${formatCompact(item.ai_lines)} lines`,
      detail: byCost && item.ai_lines > 0 ? `${formatCompact(item.ai_lines)} lines by AI` : undefined,
    }));
}

interface ModelRow {
  name: string;
  lines?: number;
  input: number;
  cached: number;
  output: number;
  sessions?: number;
  cost?: number;
  version?: boolean;
}

// ModelsTable lists the usage of each AI model, with the versions reported by plugins under it.
function ModelsTable({ dashboard }: { dashboard: Dashboard }) {
  const rows = useMemo(() => {
    const families: AIModelUsage[] = [...dashboard.ai_models];
    for (const version of dashboard.ai_model_versions) {
      if (!families.some((family) => family.name === version.model)) {
        families.push({ name: version.model, lines: 0, input_tokens: 0, cached_input_tokens: 0, output_tokens: 0, cost: 0 });
      }
    }

    const result: ModelRow[] = [];
    for (const family of families) {
      const versions = dashboard.ai_model_versions.filter((v) => v.model === family.name);
      const known = dashboard.ai_models.includes(family);
      result.push({
        name: family.name,
        lines: known ? family.lines : undefined,
        input: known ? family.input_tokens : versions.reduce((sum, v) => sum + v.input_tokens, 0),
        cached: known ? family.cached_input_tokens : versions.reduce((sum, v) => sum + v.cached_input_tokens, 0),
        output: known ? family.output_tokens : versions.reduce((sum, v) => sum + v.output_tokens, 0),
        cost: known ? family.cost : undefined,
      });
      for (const v of versions) {
        const reasoning = !v.complexity ? '' : v.complexity === 'none' ? ', no reasoning' : `, ${v.complexity} reasoning`;
        result.push({
          name: `${v.version || 'Unknown version'}${reasoning}`,
          input: v.input_tokens,
          cached: v.cached_input_tokens,
          output: v.output_tokens,
          sessions: v.sessions,
          version: true,
        });
      }
    }
    return result;
  }, [dashboard]);

  if (rows.length === 0) return null;
  const cell = (value: number | undefined, format: (v: number) => string) => (value === undefined ? '—' : format(value));

  return (
    <div className="ai-block">
      <h3 className="ai-block-title">Models</h3>
      <div className="table-scroll">
        <table className="data-table">
          <thead>
            <tr>
              <th scope="col">Model</th>
              <th scope="col">Lines changed</th>
              <th scope="col">Input tokens</th>
              <th scope="col">Cached input</th>
              <th scope="col">Output tokens</th>
              <th scope="col">Sessions</th>
              <th scope="col">Estimated cost</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((row, i) => (
              <tr key={i} className={row.version ? 'ai-version' : undefined}>
                <th scope="row">{row.name}</th>
                <td className="num">{cell(row.lines, formatInteger)}</td>
                <td className="num">{formatCompact(row.input)}</td>
                <td className="num">{formatCompact(row.cached)}</td>
                <td className="num">{formatCompact(row.output)}</td>
                <td className="num">{cell(row.sessions, formatInteger)}</td>
                <td className="num">{cell(row.cost, formatCost)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <p className="ai-footnote">
        Versions come from the plugins that sent the data, and only have token counts: WakaTime counts lines and costs per model.
      </p>
    </div>
  );
}
