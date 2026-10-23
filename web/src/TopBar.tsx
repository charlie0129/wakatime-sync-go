import { useEffect, useRef, useState } from 'react';
import type { FormEvent } from 'react';
import type { SyncStatus } from './api';
import { PRESETS, QUICK_PRESETS, isDate } from './dates';
import type { Preset, Range } from './dates';
import { formatRange } from './format';
import './TopBar.css';

interface TopBarProps {
  range: Range;
  // the dates of the range, once the server resolved them
  resolved: { start: string; end: string } | null;
  today: string | null;
  status: SyncStatus | undefined;
  onChange: (range: Range) => void;
}

export default function TopBar({ range, resolved, today, status, onChange }: TopBarProps) {
  const selected = 'preset' in range ? range.preset : null;
  return (
    <header className="topbar">
      <div className="topbar-inner">
        <a className="wordmark" href="?">
          <svg width="18" height="18" viewBox="0 0 32 32" aria-hidden="true">
            <rect className="tone-hand" x="2" y="5" width="9" height="5" />
            <rect className="tone-ai" x="13" y="5" width="17" height="5" />
            <rect className="tone-hand" x="6" y="13.5" width="14" height="5" />
            <rect className="tone-ai" x="22" y="13.5" width="6" height="5" />
            <rect className="tone-ai" x="2" y="22" width="12" height="5" />
            <rect className="tone-hand" x="17" y="22" width="9" height="5" />
          </svg>
          WakaTime
        </a>

        <nav className="range-controls" aria-label="Date range">
          <div className="segmented" role="group" aria-label="Recent days">
            {QUICK_PRESETS.map(([preset, name]) => (
              <button key={preset} className="segmented-option" aria-pressed={selected === preset} onClick={() => onChange({ preset })}>
                {name}
              </button>
            ))}
          </div>
          <RangeMenu range={range} resolved={resolved} today={today} onChange={onChange} />
        </nav>

        <SyncState status={status} />
      </div>
    </header>
  );
}

function RangeMenu({ range, resolved, today, onChange }: Omit<TopBarProps, 'status'>) {
  const [open, setOpen] = useState(false);
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [error, setError] = useState<string | null>(null);
  const button = useRef<HTMLButtonElement>(null);
  const panel = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: PointerEvent) => {
      const target = event.target as Node;
      if (!panel.current?.contains(target) && !button.current?.contains(target)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setOpen(false);
        button.current?.focus();
      }
    };
    document.addEventListener('pointerdown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    (panel.current?.querySelector<HTMLElement>('[aria-pressed="true"]') ?? panel.current?.querySelector<HTMLElement>('button'))?.focus();
    return () => {
      document.removeEventListener('pointerdown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [open]);

  const toggle = () => {
    if (!open) {
      setFrom(resolved?.start ?? '');
      setTo(resolved?.end ?? '');
      setError(null);
    }
    setOpen(!open);
  };

  const choose = (next: Range) => {
    setOpen(false);
    button.current?.focus();
    onChange(next);
  };

  const apply = (event: FormEvent) => {
    event.preventDefault();
    if (!isDate(from) || !isDate(to)) {
      setError('Enter a start and an end date.');
    } else if (from > to) {
      setError('The start date is after the end date.');
    } else {
      choose({ start: from, end: to });
    }
  };

  const label = resolved
    ? formatRange(resolved.start, resolved.end)
    : 'preset' in range
      ? PRESETS[range.preset]
      : formatRange(range.start, range.end);

  return (
    <div className="range-menu">
      <button ref={button} className="range-button" aria-expanded={open} aria-haspopup="dialog" onClick={toggle}>
        {label}
        <svg width="10" height="10" viewBox="0 0 10 10" aria-hidden="true">
          <path d="M1.5 3.5 5 7l3.5-3.5" fill="none" stroke="currentColor" strokeWidth="1.5" />
        </svg>
      </button>
      {open && (
        <div ref={panel} className="range-panel" role="dialog" aria-label="Choose dates">
          <ul className="range-presets">
            {(Object.keys(PRESETS) as Preset[]).map((preset) => {
              const selected = 'preset' in range && range.preset === preset;
              return (
                <li key={preset}>
                  <button className="range-preset" aria-pressed={selected} onClick={() => choose({ preset })}>
                    {PRESETS[preset]}
                    {selected && (
                      <svg width="12" height="12" viewBox="0 0 12 12" aria-hidden="true">
                        <path d="m2 6.5 2.5 2.5L10 3.5" fill="none" stroke="currentColor" strokeWidth="2" />
                      </svg>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
          <form className="range-custom" onSubmit={apply} noValidate>
            <label className="range-field">
              From
              <input type="date" value={from} max={today ?? undefined} onChange={(e) => setFrom(e.target.value)} />
            </label>
            <label className="range-field">
              To
              <input type="date" value={to} max={today ?? undefined} onChange={(e) => setTo(e.target.value)} />
            </label>
            {error && (
              <p className="range-error" role="alert">
                {error}
              </p>
            )}
            <button type="submit" className="range-apply">
              Show these days
            </button>
          </form>
        </div>
      )}
    </div>
  );
}

function SyncState({ status }: { status: SyncStatus | undefined }) {
  if (!status) return null;
  if (status.running && status.run) {
    return (
      <p className="sync-state" role="status">
        Syncing, {status.run.done} of {status.run.days} days done
      </p>
    );
  }

  const failed = status.failed_days.length;
  return (
    <p className="sync-state">
      {status.last_synced_day ? `Synced through ${formatRange(status.last_synced_day, status.last_synced_day)}` : 'Not synced yet'}
      {failed > 0 && (
        <span className="sync-failed" title={`Failed: ${status.failed_days.join(', ')}. The next sync tries again.`}>
          {failed === 1 ? '1 day failed to sync' : `${failed} days failed to sync`}
        </span>
      )}
    </p>
  );
}
