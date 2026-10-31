"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { errorMessage } from "@/lib/api/client";

export function useCommercePage<T extends { id: string }>(
  load: (
    cursor: string | undefined,
    signal: AbortSignal,
  ) => Promise<{ items: T[]; next_cursor: string | null }>,
) {
  const [items, setItems] = useState<T[]>([]),
    [next, setNext] = useState<string | null>(null),
    [error, setError] = useState(""),
    [loading, setLoading] = useState(true);
  const request = useRef<AbortController | null>(null),
    busy = useRef(false),
    generation = useRef(0);
  const perform = useCallback(
    async (cursor?: string) => {
      if (cursor && busy.current) return;
      request.current?.abort();
      const controller = new AbortController();
      request.current = controller;
      const version = ++generation.current;
      busy.current = true;
      setLoading(true);
      setError("");
      if (!cursor) {
        setItems([]);
        setNext(null);
      }
      try {
        const page = await load(cursor, controller.signal);
        if (!controller.signal.aborted && generation.current === version) {
          setItems((prior) =>
            cursor
              ? [...prior, ...page.items.filter((p) => !prior.some((old) => old.id === p.id))]
              : page.items,
          );
          setNext(page.next_cursor || null);
        }
      } catch (cause) {
        if (!controller.signal.aborted && generation.current === version)
          setError(errorMessage(cause));
      } finally {
        if (generation.current === version && !controller.signal.aborted) {
          busy.current = false;
          setLoading(false);
        }
      }
    },
    [load],
  );
  useEffect(() => {
    void perform();
    return () => {
      generation.current++;
      request.current?.abort();
      busy.current = false;
    };
  }, [perform]);
  return {
    items,
    next,
    error,
    loading,
    refresh: () => void perform(),
    more: () => {
      if (next) void perform(next);
    },
  };
}
