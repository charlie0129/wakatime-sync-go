import { useId, useState } from 'react';
import type { CSSProperties, ReactNode } from 'react';
import { formatShare } from './format';
import './components.css';

interface SectionProps {
  title: ReactNode;
  aside?: ReactNode;
  note?: ReactNode;
  children: ReactNode;
}

export function Section({ title, aside, note, children }: SectionProps) {
  const id = useId();
  return (
    <section className="section" aria-labelledby={id}>
      <div className="section-head">
        <h2 id={id} className="section-title">
          {title}
        </h2>
        {aside && <div className="section-aside">{aside}</div>}
      </div>
      {note && <p className="section-note">{note}</p>}
      {children}
    </section>
  );
}

// Key is the legend swatch of a kind of mark.
export function Key({ tone, children }: { tone: 'hand' | 'ai' | 'bar'; children: ReactNode }) {
  return (
    <span className="key">
      <span className={`key-swatch tone-${tone}`} aria-hidden="true" />
      {children}
    </span>
  );
}

// Toggle switches between the values of a small set, such as the chart and table views of data.
export function Toggle<T extends string>({ label, value, options, onChange }: {
  label: string;
  value: T;
  options: Array<[T, string]>;
  onChange: (value: T) => void;
}) {
  return (
    <div className="toggle" role="group" aria-label={label}>
      {options.map(([option, name]) => (
        <button key={option} className="toggle-option" aria-pressed={option === value} onClick={() => onChange(option)}>
          {name}
        </button>
      ))}
    </div>
  );
}

// Tooltip floats above a point of its positioned container, kept within the container's width.
export function Tooltip({ x, y, containerWidth, children }: {
  x: number;
  y: number;
  containerWidth: number;
  children: ReactNode;
}) {
  const style: CSSProperties = { left: Math.max(110, Math.min(x, containerWidth - 110)), top: y };
  return (
    <div className="tooltip" style={style} role="status">
      {children}
    </div>
  );
}

export function TooltipTitle({ children }: { children: ReactNode }) {
  return <div className="tooltip-title">{children}</div>;
}

// TooltipRows lists values with what they are, values first.
export function TooltipRows({ rows }: { rows: Array<[ReactNode, ReactNode] | null | false> }) {
  return (
    <dl className="tooltip-rows">
      {rows.map(
        (row, i) =>
          row && (
            <div key={i} className="tooltip-row">
              <dd className="num">{row[0]}</dd>
              <dt>{row[1]}</dt>
            </div>
          ),
      )}
    </dl>
  );
}

export interface Figure {
  label: string;
  value: string;
  note?: string | null;
}

// Figures are the headline numbers of a period.
export function Figures({ items }: { items: Figure[] }) {
  return (
    <dl className="figures">
      {items.map((figure) => (
        <div key={figure.label} className="figure">
          <dt className="figure-label">{figure.label}</dt>
          <dd className="figure-value">{figure.value}</dd>
          {figure.note && <dd className="figure-note">{figure.note}</dd>}
        </div>
      ))}
    </dl>
  );
}

export interface BarListItem {
  name: string;
  value: number;
  // label is the formatted value
  label: string;
  detail?: string;
}

interface BarListProps {
  title: string;
  items: BarListItem[];
  // total, when set, shows each item's share of it
  total?: number;
  code?: boolean;
  tone?: 'bar' | 'ai';
  limit?: number;
}

// BarList ranks items by value, with a bar for each.
export function BarList({ title, items, total, code = false, tone = 'bar', limit = 8 }: BarListProps) {
  const [expanded, setExpanded] = useState(false);
  const max = Math.max(0, ...items.map((item) => item.value));
  const shown = expanded ? items : items.slice(0, limit);

  return (
    <div className="bar-list">
      <h3 className="bar-list-title">{title}</h3>
      {items.length === 0 ? (
        <p className="empty-note">Nothing recorded.</p>
      ) : (
        <ol className="bar-list-items">
          {shown.map((item) => (
            <li key={item.name} className="bar-list-item">
              <span className={`bar-list-name${code ? ' code' : ''}`} title={item.name}>
                {item.name}
              </span>
              <span className="bar-list-value num">
                {item.label}
                {total ? <span className="bar-list-share">{formatShare(item.value / total)}</span> : null}
              </span>
              {item.detail && <span className="bar-list-detail">{item.detail}</span>}
              <span className="bar-list-track" aria-hidden="true">
                <span className={`bar-list-bar tone-${tone}`} style={{ width: `${max > 0 ? (item.value / max) * 100 : 0}%` }} />
              </span>
            </li>
          ))}
        </ol>
      )}
      {items.length > limit && (
        <button className="text-button" onClick={() => setExpanded(!expanded)}>
          {expanded ? 'Show fewer' : `Show all ${items.length}`}
        </button>
      )}
    </div>
  );
}
