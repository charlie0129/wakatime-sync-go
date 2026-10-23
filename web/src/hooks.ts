import { useCallback, useEffect, useState } from 'react';
import { parseRange, rangeQuery } from './dates';
import type { Range } from './dates';

interface DataState<T> {
  key: string;
  data?: T;
  error?: Error;
}

// useData loads data for a key, with a load function that must only change with the key. While the
// data of a new key loads, it keeps returning the previous data, so that views dim instead of
// flashing, and loads that are no longer wanted are cancelled. A null key loads nothing.
export function useData<T>(key: string | null, load: (signal: AbortSignal) => Promise<T>) {
  const [attempt, setAttempt] = useState(0);
  const [state, setState] = useState<DataState<T> | null>(null);
  const currentKey = key === null ? null : `${key}#${attempt}`;

  useEffect(() => {
    if (currentKey === null) return;
    const controller = new AbortController();
    load(controller.signal).then(
      (data) => setState({ key: currentKey, data }),
      (error: Error) => {
        if (!controller.signal.aborted) {
          setState((previous) => ({ key: currentKey, data: previous?.data, error }));
        }
      },
    );
    return () => controller.abort();
  }, [currentKey, load]);

  const retry = useCallback(() => setAttempt((a) => a + 1), []);
  return {
    data: state?.data,
    error: state?.key === currentKey ? state.error : undefined,
    loading: currentKey !== null && state?.key !== currentKey,
    retry,
  };
}

// useRange returns the date range of the page, kept in its URL so that reloading, bookmarks, and
// the back button work, and a function to change it.
export function useRange(): [Range, (range: Range) => void] {
  const [range, setRange] = useState(() => parseRange(window.location.search));

  useEffect(() => {
    const onPopState = () => setRange(parseRange(window.location.search));
    window.addEventListener('popstate', onPopState);
    return () => window.removeEventListener('popstate', onPopState);
  }, []);

  const navigate = useCallback((next: Range) => {
    const search = `?${rangeQuery(next)}`;
    if (search !== window.location.search) {
      window.history.pushState(null, '', search);
    }
    setRange(next);
  }, []);

  return [range, navigate];
}

// useWidth returns a ref to attach to an element, and the element's width as it changes.
export function useWidth<T extends HTMLElement>(): [(element: T | null) => (() => void) | undefined, number] {
  const [width, setWidth] = useState(0);
  const ref = useCallback((element: T | null) => {
    if (!element) return undefined;
    const observer = new ResizeObserver(([entry]) => setWidth(entry.contentRect.width));
    observer.observe(element);
    return () => observer.disconnect();
  }, []);
  return [ref, width];
}

// useTick returns a number that increases every interval milliseconds.
export function useTick(interval: number): number {
  const [tick, setTick] = useState(0);
  useEffect(() => {
    const id = window.setInterval(() => setTick((t) => t + 1), interval);
    return () => window.clearInterval(id);
  }, [interval]);
  return tick;
}
