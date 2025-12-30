import { useState, useEffect, useCallback, useMemo } from 'react';
import {
  BarChart, Bar, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  PieChart, Pie, Cell, AreaChart, Area
} from 'recharts';
import { Clock, Calendar, Code, Monitor, Layers, ChevronLeft, ChevronRight, Activity } from 'lucide-react';
import { api } from './api';
import type { SummariesResponse, RangeStats, DurationData, DurationResponse } from './api';
import { formatDate, formatDisplayDate, formatDuration, formatHours, getDateRange, getColor } from './utils';
import type { DateRange } from './utils';
import { subDays, addDays, parseISO } from 'date-fns';
import './App.css';

interface CustomTooltipProps {
  active?: boolean;
  payload?: Array<{ value: number; name: string; payload: Record<string, unknown> }>;
  label?: string;
}

function CustomTooltip({ active, payload, label }: CustomTooltipProps) {
  if (active && payload && payload.length) {
    return (
      <div className="custom-tooltip">
        <div className="tooltip-label">{label}</div>
        {payload.map((entry, index) => (
          <div key={index} className="tooltip-value">
            {entry.name}: {formatDuration(entry.value)}
          </div>
        ))}
      </div>
    );
  }
  return null;
}

interface PieLegendProps {
  data: Array<{ name: string; total_seconds: number; percent: number }>;
}

function PieLegend({ data }: PieLegendProps) {
  return (
    <div className="pie-legend">
      {data.slice(0, 8).map((item, index) => (
        <div key={item.name} className="pie-legend-item">
          <div className="pie-legend-left">
            <div className="pie-legend-color" style={{ backgroundColor: getColor(index) }} />
            <span className="pie-legend-name" title={item.name}>{item.name}</span>
          </div>
          <div className="pie-legend-right">
            <span className="pie-legend-time">{formatDuration(item.total_seconds)}</span>
            <span className="pie-legend-percent">{item.percent.toFixed(1)}%</span>
          </div>
        </div>
      ))}
    </div>
  );
}

// Shared tooltip styles for all charts
const tooltipStyle = {
  contentStyle: {
    backgroundColor: '#161b22',
    border: '1px solid #30363d',
    borderRadius: '8px',
    color: '#e6edf3',
  },
  labelStyle: {
    color: '#e6edf3',
  },
  itemStyle: {
    color: '#e6edf3',
  },
};

function App() {
  const [dateRange, setDateRange] = useState<DateRange>('last7days');
  const [customStart, setCustomStart] = useState<Date>(subDays(new Date(), 7));
  const [customEnd, setCustomEnd] = useState<Date>(subDays(new Date(), 1));
  const [summaries, setSummaries] = useState<SummariesResponse | null>(null);
  const [rangeStats, setRangeStats] = useState<RangeStats | null>(null);
  const [loading, setLoading] = useState(true);
  const [lastSynced, setLastSynced] = useState<string>('');

  // Duration view state
  const [durationDate, setDurationDate] = useState<Date>(subDays(new Date(), 1));
  const [durations, setDurations] = useState<DurationResponse | null>(null);
  const [durationLoading, setDurationLoading] = useState(false);

  // Memoize date range to prevent infinite re-renders
  const { start, end } = useMemo(
    () => getDateRange(dateRange, customStart, customEnd),
    [dateRange, customStart.getTime(), customEnd.getTime()]
  );

  // Use string representations for stable dependencies
  const startStr = formatDate(start);
  const endStr = formatDate(end);
  const durationDateStr = formatDate(durationDate);

  const fetchData = useCallback(async () => {
    setLoading(true);
    try {
      const [summariesData, rangeData, syncStatus] = await Promise.all([
        api.getSummaries(startStr, endStr),
        api.getRangeStats(startStr, endStr),
        api.getSyncStatus(),
      ]);
      setSummaries(summariesData);
      setRangeStats(rangeData);
      setLastSynced(syncStatus.last_synced_day);
    } catch (error) {
      console.error('Failed to fetch data:', error);
    } finally {
      setLoading(false);
    }
  }, [startStr, endStr]);

  const fetchDurations = useCallback(async () => {
    setDurationLoading(true);
    try {
      const data = await api.getDurations(durationDateStr);
      setDurations(data);
    } catch (error) {
      console.error('Failed to fetch durations:', error);
    } finally {
      setDurationLoading(false);
    }
  }, [durationDateStr]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  useEffect(() => {
    fetchDurations();
  }, [fetchDurations]);

  // Prepare chart data
  const dailyChartData = summaries?.data.map((day) => ({
    date: day.range.date,
    displayDate: formatDisplayDate(day.range.date).split(',')[0],
    hours: day.grand_total.total_seconds / 3600,
    seconds: day.grand_total.total_seconds,
  })) || [];

  // Group durations by project and calculate totals
  const durationsByProject = durations?.data.reduce((acc: Record<string, { project: string; totalSeconds: number; items: DurationData[] }>, d) => {
    const project = d.project || 'Unknown';
    if (!acc[project]) {
      acc[project] = { project, totalSeconds: 0, items: [] };
    }
    acc[project].totalSeconds += d.duration;
    acc[project].items.push(d);
    return acc;
  }, {}) || {};

  const durationBarData = Object.values(durationsByProject)
    .sort((a, b) => b.totalSeconds - a.totalSeconds)
    .map((p) => ({
      name: p.project,
      hours: p.totalSeconds / 3600,
      seconds: p.totalSeconds,
    }));

  const totalDurationSeconds = Object.values(durationsByProject).reduce((sum, p) => sum + p.totalSeconds, 0);

  const canGoNext = addDays(durationDate, 1) < new Date();
  const canGoPrev = true;

  return (
    <div className="app">
      <header className="header">
        <div className="header-left">
          <div className="logo">
            <Clock size={24} />
            WakaTime Stats
          </div>
        </div>
        <div className="header-right">
          <div className="sync-status">
            <div className="sync-status-dot" />
            Last synced: {lastSynced}
          </div>
        </div>
      </header>

      <main className="main-content">
        {/* Date Range Selector */}
        <div className="date-range-selector">
          <div className="range-buttons">
            {(['last7days', 'last14days', 'last30days', 'last3months', 'last6months', 'lastYear'] as DateRange[]).map((range) => (
              <button
                key={range}
                className={`range-button ${dateRange === range ? 'active' : ''}`}
                onClick={() => setDateRange(range)}
              >
                {range === 'last7days' ? '7 Days' :
                  range === 'last14days' ? '14 Days' :
                    range === 'last30days' ? '30 Days' :
                      range === 'last3months' ? '3 Months' :
                        range === 'last6months' ? '6 Months' : '1 Year'}
              </button>
            ))}
          </div>
          <div className="date-inputs">
            <input
              type="date"
              className="date-input"
              value={formatDate(customStart)}
              onChange={(e) => {
                setCustomStart(parseISO(e.target.value));
                setDateRange('custom');
              }}
            />
            <span style={{ color: 'var(--text-muted)' }}>to</span>
            <input
              type="date"
              className="date-input"
              value={formatDate(customEnd)}
              onChange={(e) => {
                setCustomEnd(parseISO(e.target.value));
                setDateRange('custom');
              }}
            />
          </div>
        </div>

        {loading ? (
          <div className="loading">
            <div className="spinner" />
            Loading...
          </div>
        ) : (
          <>
            {/* Summary Cards */}
            <div className="summary-cards">
              <div className="summary-card">
                <div className="summary-card-label">
                  <Clock size={16} />
                  Total Time
                </div>
                <div className="summary-card-value">
                  {summaries?.cumulative_total.text || '0 hrs'}
                </div>
                <div className="summary-card-secondary">
                  {formatHours(summaries?.cumulative_total.seconds || 0)} hours
                </div>
              </div>
              <div className="summary-card">
                <div className="summary-card-label">
                  <Calendar size={16} />
                  Daily Average
                </div>
                <div className="summary-card-value">
                  {summaries?.daily_average.text || '0 hrs'}
                </div>
                <div className="summary-card-secondary">
                  {summaries?.daily_average.days_including_holidays || 0} days in range
                </div>
              </div>
              <div className="summary-card">
                <div className="summary-card-label">
                  <Layers size={16} />
                  Top Project
                </div>
                <div className="summary-card-value">
                  {rangeStats?.projects[0]?.name || 'N/A'}
                </div>
                <div className="summary-card-secondary">
                  {rangeStats?.projects[0] ? formatDuration(rangeStats.projects[0].total_seconds) : ''}
                </div>
              </div>
              <div className="summary-card">
                <div className="summary-card-label">
                  <Code size={16} />
                  Top Language
                </div>
                <div className="summary-card-value">
                  {rangeStats?.languages[0]?.name || 'N/A'}
                </div>
                <div className="summary-card-secondary">
                  {rangeStats?.languages[0] ? formatDuration(rangeStats.languages[0].total_seconds) : ''}
                </div>
              </div>
            </div>

            {/* Daily Activity Chart */}
            <div className="chart-card chart-card-full" style={{ marginBottom: 24 }}>
              <div className="chart-header">
                <div className="chart-title">
                  <Activity size={18} />
                  Daily Activity
                </div>
              </div>
              <div className="chart-container">
                <ResponsiveContainer width="100%" height="100%">
                  <AreaChart data={dailyChartData}>
                    <defs>
                      <linearGradient id="colorHours" x1="0" y1="0" x2="0" y2="1">
                        <stop offset="5%" stopColor="#58a6ff" stopOpacity={0.3} />
                        <stop offset="95%" stopColor="#58a6ff" stopOpacity={0} />
                      </linearGradient>
                    </defs>
                    <CartesianGrid strokeDasharray="3 3" stroke="#30363d" />
                    <XAxis
                      dataKey="displayDate"
                      stroke="#8b949e"
                      fontSize={12}
                      tickLine={false}
                    />
                    <YAxis
                      stroke="#8b949e"
                      fontSize={12}
                      tickLine={false}
                      tickFormatter={(value) => `${(value / 3600).toFixed(1)}h`}
                    />
                    <Tooltip content={<CustomTooltip />} />
                    <Area
                      type="monotone"
                      dataKey="seconds"
                      stroke="#58a6ff"
                      strokeWidth={2}
                      fillOpacity={1}
                      fill="url(#colorHours)"
                      name="Time"
                    />
                  </AreaChart>
                </ResponsiveContainer>
              </div>
            </div>

            {/* Charts Grid */}
            <div className="charts-grid">
              {/* Languages Pie Chart */}
              <div className="chart-card">
                <div className="chart-header">
                  <div className="chart-title">
                    <Code size={18} />
                    Languages
                  </div>
                </div>
                <div className="chart-container-sm">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={(rangeStats?.languages.slice(0, 8) || []) as unknown as Array<Record<string, unknown>>}
                        dataKey="total_seconds"
                        nameKey="name"
                        cx="50%"
                        cy="50%"
                        innerRadius={50}
                        outerRadius={80}
                        paddingAngle={2}
                      >
                        {rangeStats?.languages.slice(0, 8).map((_, index) => (
                          <Cell key={`cell-${index}`} fill={getColor(index)} />
                        ))}
                      </Pie>
                      <Tooltip
                        formatter={(value) => formatDuration(Number(value) || 0)}
                        contentStyle={tooltipStyle.contentStyle}
                        labelStyle={tooltipStyle.labelStyle}
                        itemStyle={tooltipStyle.itemStyle}
                      />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
                <PieLegend data={rangeStats?.languages || []} />
              </div>

              {/* Editors Pie Chart */}
              <div className="chart-card">
                <div className="chart-header">
                  <div className="chart-title">
                    <Monitor size={18} />
                    Editors
                  </div>
                </div>
                <div className="chart-container-sm">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={(rangeStats?.editors.slice(0, 8) || []) as unknown as Array<Record<string, unknown>>}
                        dataKey="total_seconds"
                        nameKey="name"
                        cx="50%"
                        cy="50%"
                        innerRadius={50}
                        outerRadius={80}
                        paddingAngle={2}
                      >
                        {rangeStats?.editors.slice(0, 8).map((_, index) => (
                          <Cell key={`cell-${index}`} fill={getColor(index)} />
                        ))}
                      </Pie>
                      <Tooltip
                        formatter={(value) => formatDuration(Number(value) || 0)}
                        contentStyle={tooltipStyle.contentStyle}
                        labelStyle={tooltipStyle.labelStyle}
                        itemStyle={tooltipStyle.itemStyle}
                      />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
                <PieLegend data={rangeStats?.editors || []} />
              </div>

              {/* Projects Bar Chart */}
              <div className="chart-card">
                <div className="chart-header">
                  <div className="chart-title">
                    <Layers size={18} />
                    Projects
                  </div>
                </div>
                <div className="chart-container">
                  <ResponsiveContainer width="100%" height="100%">
                    <BarChart
                      data={rangeStats?.projects.slice(0, 10).map((p, i) => ({
                        ...p,
                        fill: getColor(i),
                      })) || []}
                      layout="vertical"
                    >
                      <CartesianGrid strokeDasharray="3 3" stroke="#30363d" horizontal={false} />
                      <XAxis
                        type="number"
                        stroke="#8b949e"
                        fontSize={12}
                        tickFormatter={(value) => `${(value / 3600).toFixed(0)}h`}
                      />
                      <YAxis
                        type="category"
                        dataKey="name"
                        stroke="#8b949e"
                        fontSize={12}
                        width={120}
                        tickLine={false}
                      />
                      <Tooltip
                        formatter={(value) => formatDuration(Number(value) || 0)}
                        contentStyle={tooltipStyle.contentStyle}
                        labelStyle={tooltipStyle.labelStyle}
                        itemStyle={tooltipStyle.itemStyle}
                      />
                      <Bar dataKey="total_seconds" name="Time" radius={[0, 4, 4, 0]}>
                        {rangeStats?.projects.slice(0, 10).map((_, index) => (
                          <Cell key={`cell-${index}`} fill={getColor(index)} />
                        ))}
                      </Bar>
                    </BarChart>
                  </ResponsiveContainer>
                </div>
              </div>

              {/* Operating Systems Pie Chart */}
              <div className="chart-card">
                <div className="chart-header">
                  <div className="chart-title">
                    <Monitor size={18} />
                    Operating Systems
                  </div>
                </div>
                <div className="chart-container-sm">
                  <ResponsiveContainer width="100%" height="100%">
                    <PieChart>
                      <Pie
                        data={(rangeStats?.operating_systems.slice(0, 8) || []) as unknown as Array<Record<string, unknown>>}
                        dataKey="total_seconds"
                        nameKey="name"
                        cx="50%"
                        cy="50%"
                        innerRadius={50}
                        outerRadius={80}
                        paddingAngle={2}
                      >
                        {rangeStats?.operating_systems.slice(0, 8).map((_, index) => (
                          <Cell key={`cell-${index}`} fill={getColor(index)} />
                        ))}
                      </Pie>
                      <Tooltip
                        formatter={(value) => formatDuration(Number(value) || 0)}
                        contentStyle={tooltipStyle.contentStyle}
                        labelStyle={tooltipStyle.labelStyle}
                        itemStyle={tooltipStyle.itemStyle}
                      />
                    </PieChart>
                  </ResponsiveContainer>
                </div>
                <PieLegend data={rangeStats?.operating_systems || []} />
              </div>
            </div>

            {/* Duration Timeline */}
            <div className="chart-card duration-timeline">
              <div className="timeline-header">
                <div className="chart-title">
                  <Clock size={18} />
                  Daily Durations
                </div>
                <div className="timeline-nav">
                  <button
                    className="nav-button"
                    onClick={() => setDurationDate(subDays(durationDate, 1))}
                    disabled={!canGoPrev}
                  >
                    <ChevronLeft size={16} />
                    Prev
                  </button>
                  <span className="timeline-date">{formatDisplayDate(durationDate)}</span>
                  <button
                    className="nav-button"
                    onClick={() => setDurationDate(addDays(durationDate, 1))}
                    disabled={!canGoNext}
                  >
                    Next
                    <ChevronRight size={16} />
                  </button>
                </div>
              </div>

              {durationLoading ? (
                <div className="loading">
                  <div className="spinner" />
                  Loading...
                </div>
              ) : durationBarData.length === 0 ? (
                <div className="empty-state">
                  <Clock className="empty-state-icon" />
                  <p>No activity recorded for this day</p>
                </div>
              ) : (
                <>
                  <div style={{ marginBottom: 16, color: 'var(--text-secondary)', fontSize: 14 }}>
                    Total: {formatDuration(totalDurationSeconds)}
                  </div>
                  <div className="chart-container">
                    <ResponsiveContainer width="100%" height="100%">
                      <BarChart data={durationBarData} layout="vertical">
                        <CartesianGrid strokeDasharray="3 3" stroke="#30363d" horizontal={false} />
                        <XAxis
                          type="number"
                          stroke="#8b949e"
                          fontSize={12}
                          tickFormatter={(value) => `${(value).toFixed(1)}h`}
                        />
                        <YAxis
                          type="category"
                          dataKey="name"
                          stroke="#8b949e"
                          fontSize={12}
                          width={150}
                          tickLine={false}
                        />
                        <Tooltip
                          formatter={(value) => formatDuration((Number(value) || 0) * 3600)}
                          contentStyle={tooltipStyle.contentStyle}
                          labelStyle={tooltipStyle.labelStyle}
                          itemStyle={tooltipStyle.itemStyle}
                        />
                        <Bar dataKey="hours" name="Time" radius={[0, 4, 4, 0]}>
                          {durationBarData.map((_, index) => (
                            <Cell key={`cell-${index}`} fill={getColor(index)} />
                          ))}
                        </Bar>
                      </BarChart>
                    </ResponsiveContainer>
                  </div>
                </>
              )}
            </div>
          </>
        )}
      </main>
    </div>
  );
}

export default App;
