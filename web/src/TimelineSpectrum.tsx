import { useMemo, useRef, useState } from 'react';
import type { DurationData } from './api';
import { formatDuration, getColor } from './utils';
import './TimelineSpectrum.css';

interface TimelineSpectrumProps {
  data: DurationData[];
  date: string; // ISO date string like "2024-01-01"
  loading?: boolean;
}

interface ProjectTimeline {
  project: string;
  segments: Array<{
    startTime: number; // Hour of day (0-24)
    duration: number; // Duration in hours
    startTimestamp: number; // Original timestamp
    durationSeconds: number; // Original duration in seconds
  }>;
  totalSeconds: number;
  color: string;
}

function TimelineSpectrum({ data, date, loading }: TimelineSpectrumProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const [hoveredSegment, setHoveredSegment] = useState<{
    project: string;
    startTime: string;
    endTime: string;
    duration: number;
    x: number;
    y: number;
  } | null>(null);

  // Parse the date to get the start of day timestamp
  const dayStart = useMemo(() => {
    const d = new Date(date + 'T00:00:00');
    return d.getTime() / 1000;
  }, [date]);

  // Group data by project and calculate time positions
  const projectTimelines: ProjectTimeline[] = useMemo(() => {
    if (!data || data.length === 0) return [];

    // Group by project
    const grouped: Record<string, { segments: typeof data; totalSeconds: number }> = {};
    
    data.forEach((d) => {
      const project = d.project || 'Unknown';
      if (!grouped[project]) {
        grouped[project] = { segments: [], totalSeconds: 0 };
      }
      grouped[project].segments.push(d);
      grouped[project].totalSeconds += d.duration;
    });

    // Sort by total time (descending)
    const sorted = Object.entries(grouped)
      .sort(([, a], [, b]) => b.totalSeconds - a.totalSeconds);

    // Convert to timeline format
    return sorted.map(([project, { segments, totalSeconds }], index) => ({
      project,
      totalSeconds,
      color: getColor(index),
      segments: segments.map((seg) => {
        // Calculate hour of day (0-24)
        const startHour = (seg.time - dayStart) / 3600;
        const durationHours = seg.duration / 3600;
        
        return {
          startTime: Math.max(0, Math.min(24, startHour)),
          duration: Math.min(durationHours, 24 - startHour),
          startTimestamp: seg.time,
          durationSeconds: seg.duration,
        };
      }).filter((seg) => seg.startTime >= 0 && seg.startTime < 24),
    })).filter((p) => p.segments.length > 0);
  }, [data, dayStart]);

  // Format time from timestamp
  const formatTime = (timestamp: number): string => {
    const date = new Date(timestamp * 1000);
    return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  };

  // Handle segment hover
  const handleSegmentHover = (
    e: React.MouseEvent,
    project: string,
    startTimestamp: number,
    durationSeconds: number
  ) => {
    const rect = containerRef.current?.getBoundingClientRect();
    if (!rect) return;

    const endTimestamp = startTimestamp + durationSeconds;
    
    setHoveredSegment({
      project,
      startTime: formatTime(startTimestamp),
      endTime: formatTime(endTimestamp),
      duration: durationSeconds,
      x: e.clientX - rect.left,
      y: e.clientY - rect.top,
    });
  };

  const handleMouseLeave = () => {
    setHoveredSegment(null);
  };

  // Generate hour markers for x-axis
  const hourMarkers = useMemo(() => {
    const markers = [];
    for (let h = 0; h <= 24; h += 2) {
      markers.push({
        hour: h,
        label: h === 0 ? '0:00' : h === 24 ? '24:00' : `${h}:00`,
        position: (h / 24) * 100,
      });
    }
    return markers;
  }, []);

  if (loading) {
    return (
      <div className="timeline-spectrum-loading">
        <div className="spinner" />
        Loading...
      </div>
    );
  }

  if (projectTimelines.length === 0) {
    return (
      <div className="timeline-spectrum-empty">
        <p>No activity recorded for this day</p>
      </div>
    );
  }

  const totalSeconds = projectTimelines.reduce((sum, p) => sum + p.totalSeconds, 0);

  return (
    <div className="timeline-spectrum" ref={containerRef}>
      <div className="timeline-spectrum-header">
        <span>Total: {formatDuration(totalSeconds)}</span>
        <span className="timeline-spectrum-hint">
          Hover over segments to see details
        </span>
      </div>

      {/* X-axis (time) */}
      <div className="timeline-spectrum-axis">
        <div className="timeline-spectrum-axis-label" />
        <div className="timeline-spectrum-axis-track">
          {hourMarkers.map((marker) => (
            <div
              key={marker.hour}
              className="timeline-spectrum-axis-marker"
              style={{ left: `${marker.position}%` }}
            >
              <span className="timeline-spectrum-axis-text">{marker.label}</span>
            </div>
          ))}
        </div>
      </div>

      {/* Project rows */}
      <div className="timeline-spectrum-rows">
        {projectTimelines.map((timeline) => (
          <div key={timeline.project} className="timeline-spectrum-row">
            <div className="timeline-spectrum-project-label">
              <div
                className="timeline-spectrum-project-color"
                style={{ backgroundColor: timeline.color }}
              />
              <span className="timeline-spectrum-project-name" title={timeline.project}>
                {timeline.project}
              </span>
              <span className="timeline-spectrum-project-time">
                {formatDuration(timeline.totalSeconds)}
              </span>
            </div>
            <div className="timeline-spectrum-track">
              {/* Grid lines */}
              {hourMarkers.map((marker) => (
                <div
                  key={marker.hour}
                  className="timeline-spectrum-grid-line"
                  style={{ left: `${marker.position}%` }}
                />
              ))}
              
              {/* Segments */}
              {timeline.segments.map((segment, idx) => {
                const left = (segment.startTime / 24) * 100;
                const width = Math.max((segment.duration / 24) * 100, 0.3); // Min width for visibility
                
                return (
                  <div
                    key={idx}
                    className="timeline-spectrum-segment"
                    style={{
                      left: `${left}%`,
                      width: `${width}%`,
                      backgroundColor: timeline.color,
                    }}
                    onMouseEnter={(e) => handleSegmentHover(
                      e,
                      timeline.project,
                      segment.startTimestamp,
                      segment.durationSeconds
                    )}
                    onMouseMove={(e) => handleSegmentHover(
                      e,
                      timeline.project,
                      segment.startTimestamp,
                      segment.durationSeconds
                    )}
                    onMouseLeave={handleMouseLeave}
                  />
                );
              })}
            </div>
          </div>
        ))}
      </div>

      {/* Tooltip */}
      {hoveredSegment && (
        <div
          className="timeline-spectrum-tooltip"
          style={{
            left: hoveredSegment.x,
            top: hoveredSegment.y - 60,
          }}
        >
          <div className="timeline-spectrum-tooltip-project">
            {hoveredSegment.project}
          </div>
          <div className="timeline-spectrum-tooltip-time">
            {hoveredSegment.startTime} - {hoveredSegment.endTime}
          </div>
          <div className="timeline-spectrum-tooltip-duration">
            {formatDuration(hoveredSegment.duration)}
          </div>
        </div>
      )}
    </div>
  );
}

export default TimelineSpectrum;
