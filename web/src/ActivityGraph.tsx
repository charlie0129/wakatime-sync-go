import { useState, useEffect, useMemo } from 'react';
import { Calendar, ChevronDown } from 'lucide-react';
import { api } from './api';
import type { YearlyActivityDay } from './api';
import { formatDuration } from './utils';
import {
  startOfYear, endOfYear, eachDayOfInterval, format, getDay,
  startOfWeek, addDays
} from 'date-fns';
import './ActivityGraph.css';

interface ActivityGraphProps {
  onYearChange?: (year: number) => void;
}

// Color levels for the heatmap (GitHub-style greens) - 9 levels for finer granularity
const LEVEL_COLORS = [
  '#161b22', // 0: no activity (dark background)
  '#0e4429', // 1: very low
  '#004d25', // 2: low
  '#006d32', // 3: low-medium
  '#1a7f37', // 4: medium-low
  '#26a641', // 5: medium
  '#2ea043', // 6: medium-high
  '#39d353', // 7: high
  '#4ae168', // 8: very high
];

function getActivityLevel(seconds: number, maxSeconds: number): number {
  if (seconds === 0) return 0;
  if (maxSeconds === 0) return 1;
  
  const ratio = seconds / maxSeconds;
  if (ratio <= 0.1) return 1;
  if (ratio <= 0.2) return 2;
  if (ratio <= 0.35) return 3;
  if (ratio <= 0.5) return 4;
  if (ratio <= 0.65) return 5;
  if (ratio <= 0.8) return 6;
  if (ratio <= 0.9) return 7;
  return 8;
}

interface TooltipData {
  date: string;
  totalSeconds: number;
  projects: Array<{ name: string; total_seconds: number }>;
  x: number;
  y: number;
}

export default function ActivityGraph({ onYearChange }: ActivityGraphProps) {
  const [availableYears, setAvailableYears] = useState<number[]>([]);
  const [selectedYear, setSelectedYear] = useState<number>(new Date().getFullYear());
  const [yearlyData, setYearlyData] = useState<YearlyActivityDay[]>([]);
  const [loading, setLoading] = useState(true);
  const [tooltip, setTooltip] = useState<TooltipData | null>(null);
  const [dropdownOpen, setDropdownOpen] = useState(false);

  // Fetch available years on mount
  useEffect(() => {
    async function fetchYears() {
      try {
        const response = await api.getAvailableYears();
        const years = response.years || [];
        setAvailableYears(years);
        if (years.length > 0 && !years.includes(selectedYear)) {
          setSelectedYear(years[0]);
        }
      } catch (error) {
        console.error('Failed to fetch available years:', error);
      }
    }
    fetchYears();
  }, []);

  // Fetch yearly activity when year changes
  useEffect(() => {
    async function fetchYearlyActivity() {
      setLoading(true);
      try {
        const response = await api.getYearlyActivity(selectedYear);
        setYearlyData(response.data || []);
        onYearChange?.(selectedYear);
      } catch (error) {
        console.error('Failed to fetch yearly activity:', error);
        setYearlyData([]);
      } finally {
        setLoading(false);
      }
    }
    fetchYearlyActivity();
  }, [selectedYear, onYearChange]);

  // Build the calendar grid data
  const { weeks, monthLabels, maxSeconds, totalSeconds, activeDays } = useMemo(() => {
    const yearStart = startOfYear(new Date(selectedYear, 0, 1));
    const yearEnd = endOfYear(new Date(selectedYear, 0, 1));
    const today = new Date();
    
    // Create a map of date -> activity data
    const dataMap = new Map<string, YearlyActivityDay>();
    let maxSecs = 0;
    let totalSecs = 0;
    let activeDaysCount = 0;
    
    for (const day of yearlyData) {
      dataMap.set(day.date, day);
      if (day.total_seconds > maxSecs) {
        maxSecs = day.total_seconds;
      }
      totalSecs += day.total_seconds;
      if (day.total_seconds > 0) {
        activeDaysCount++;
      }
    }

    // Generate all days of the year
    const allDays = eachDayOfInterval({ start: yearStart, end: yearEnd });
    
    // Group by weeks (starting from Sunday, like GitHub)
    const weeksData: Array<Array<{ date: Date; data?: YearlyActivityDay; isFuture: boolean }>> = [];
    
    // Find the first Sunday on or before the year start
    const firstSunday = startOfWeek(yearStart, { weekStartsOn: 0 });
    
    let currentWeek: Array<{ date: Date; data?: YearlyActivityDay; isFuture: boolean }> = [];
    let currentDate = firstSunday;
    
    // Fill in days before year start as empty
    while (currentDate < yearStart) {
      currentWeek.push({ date: currentDate, isFuture: true });
      currentDate = addDays(currentDate, 1);
    }
    
    // Fill in all days of the year
    for (const day of allDays) {
      const dayOfWeek = getDay(day);
      const dateStr = format(day, 'yyyy-MM-dd');
      const isFuture = day > today;
      
      currentWeek.push({
        date: day,
        data: dataMap.get(dateStr),
        isFuture,
      });
      
      if (dayOfWeek === 6) {
        // Saturday, end of week
        weeksData.push(currentWeek);
        currentWeek = [];
      }
    }
    
    // Add remaining days
    if (currentWeek.length > 0) {
      weeksData.push(currentWeek);
    }

    // Generate month labels
    const months: Array<{ name: string; weekIndex: number }> = [];
    let lastMonth = -1;
    
    weeksData.forEach((week, weekIndex) => {
      for (const day of week) {
        const month = day.date.getMonth();
        const year = day.date.getFullYear();
        if (year === selectedYear && month !== lastMonth) {
          lastMonth = month;
          months.push({
            name: format(day.date, 'MMM'),
            weekIndex,
          });
          break;
        }
      }
    });

    return {
      weeks: weeksData,
      monthLabels: months,
      maxSeconds: maxSecs,
      totalSeconds: totalSecs,
      activeDays: activeDaysCount,
    };
  }, [selectedYear, yearlyData]);

  const handleMouseEnter = (
    e: React.MouseEvent,
    date: Date,
    data?: YearlyActivityDay
  ) => {
    const rect = (e.target as HTMLElement).getBoundingClientRect();
    const containerRect = (e.target as HTMLElement).closest('.activity-graph-card')?.getBoundingClientRect();
    
    if (!containerRect) return;
    
    setTooltip({
      date: format(date, 'EEEE, MMMM d, yyyy'),
      totalSeconds: data?.total_seconds || 0,
      projects: data?.projects || [],
      x: rect.left - containerRect.left + rect.width / 2,
      y: rect.bottom - containerRect.top + 8,
    });
  };

  const handleMouseLeave = () => {
    setTooltip(null);
  };

  const handleYearSelect = (year: number) => {
    setSelectedYear(year);
    setDropdownOpen(false);
  };

  return (
    <div className="chart-card activity-graph-card">
      <div className="chart-header">
        <div className="chart-title">
          <Calendar size={18} />
          Activity Graph
        </div>
        <div className="year-selector">
          <button
            className="year-dropdown-button"
            onClick={() => setDropdownOpen(!dropdownOpen)}
          >
            {selectedYear}
            <ChevronDown size={16} />
          </button>
          {dropdownOpen && (
            <div className="year-dropdown">
              {availableYears.map((year) => (
                <button
                  key={year}
                  className={`year-option ${year === selectedYear ? 'active' : ''}`}
                  onClick={() => handleYearSelect(year)}
                >
                  {year}
                </button>
              ))}
              {availableYears.length === 0 && (
                <div className="year-option disabled">No data available</div>
              )}
            </div>
          )}
        </div>
      </div>

      {loading ? (
        <div className="loading">
          <div className="spinner" />
          Loading...
        </div>
      ) : (
        <>
          <div className="activity-stats">
            <span>{formatDuration(totalSeconds)} total</span>
            <span className="separator">•</span>
            <span>{activeDays} active days</span>
          </div>
          
          <div className="activity-graph">
            <div className="activity-graph-inner">
              <div className="graph-container">
                {/* Day of week labels */}
                <div className="day-labels">
                  <span className="month-spacer"></span>
                  <span></span>
                  <span>Mon</span>
                  <span></span>
                  <span>Wed</span>
                  <span></span>
                  <span>Fri</span>
                  <span></span>
                </div>
                
                {/* Weeks grid with month labels above */}
                <div className="weeks-container">
                  {/* Month labels */}
                  <div className="month-labels">
                    {monthLabels.map((month, i) => {
                      // Calculate the left position based on week index
                      // Each week column is 14px wide + 4px gap = 18px
                      const leftPos = month.weekIndex * 18;
                      return (
                        <div
                          key={i}
                          className="month-label"
                          style={{ left: leftPos }}
                        >
                          {month.name}
                        </div>
                      );
                    })}
                  </div>
                  
                  {/* Weeks grid */}
                  <div className="weeks-grid">
                  {weeks.map((week, weekIndex) => (
                  <div key={weekIndex} className="week-column">
                    {week.map((day, dayIndex) => {
                      const level = day.isFuture || day.date.getFullYear() !== selectedYear
                        ? -1
                        : getActivityLevel(day.data?.total_seconds || 0, maxSeconds);
                      
                      return (
                        <div
                          key={dayIndex}
                          className={`day-cell ${level === -1 ? 'empty' : ''}`}
                          style={{
                            backgroundColor: level >= 0 ? LEVEL_COLORS[level] : 'transparent',
                          }}
                          onMouseEnter={(e) => {
                            if (level >= 0) {
                              handleMouseEnter(e, day.date, day.data);
                            }
                          }}
                          onMouseLeave={handleMouseLeave}
                        />
                      );
                    })}
                  </div>
                ))}
                  </div>
                </div>
              </div>

              {/* Legend */}
              <div className="activity-legend">
                <span className="legend-label">Less</span>
                {LEVEL_COLORS.map((color, i) => (
                  <div
                    key={i}
                    className="legend-cell"
                    style={{ backgroundColor: color }}
                  />
                ))}
                <span className="legend-label">More</span>
              </div>
            </div>
          </div>

          {/* Tooltip - positioned relative to card */}
          {tooltip && (
            <div
              className="activity-tooltip"
              style={{
                left: tooltip.x,
                top: tooltip.y,
              }}
            >
              <div className="tooltip-date">{tooltip.date}</div>
              <div className="tooltip-total">
                {tooltip.totalSeconds > 0
                  ? formatDuration(tooltip.totalSeconds)
                  : 'No activity'}
              </div>
              {tooltip.projects.length > 0 && (
                <div className="tooltip-projects">
                  {tooltip.projects.slice(0, 5).map((project, i) => (
                    <div key={i} className="tooltip-project">
                      <span className="project-name">{project.name}</span>
                      <span className="project-time">
                        {formatDuration(project.total_seconds)}
                      </span>
                    </div>
                  ))}
                  {tooltip.projects.length > 5 && (
                    <div className="tooltip-more">
                      +{tooltip.projects.length - 5} more projects
                    </div>
                  )}
                </div>
              )}
            </div>
          )}
        </>
      )}
    </div>
  );
}
