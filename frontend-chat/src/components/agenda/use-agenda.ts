"use client";
import { useCallback, useEffect, useState } from "react";
import { listAgenda, type Agenda } from "@/lib/api/agenda";
import { ApiError, errorMessage } from "@/lib/api/client";

export function useAgenda(slug: string, pauseFocusRefresh = false) {
  const [month, setMonth] = useState<string>();
  const [after, setAfter] = useState<string>();
  const [revision, setRevision] = useState(0);
  const [data, setData] = useState<Agenda | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    const controller = new AbortController();
    setLoading(true);
    setError("");
    if (!after) setData(null);
    listAgenda(slug, month, after, controller.signal)
      .then((result) => {
        if (controller.signal.aborted) return;
        setData((previous) => {
          if (!after || !previous || previous.month !== result.month) return result;
          const items = new Map(previous.appointments.map((item) => [item.id, item]));
          result.appointments.forEach((item) => items.set(item.id, item));
          return { ...result, appointments: [...items.values()] };
        });
      })
      .catch((cause) => {
        if (controller.signal.aborted) return;
        if (cause instanceof ApiError && (cause.status === 401 || cause.status === 403))
          setData(null);
        setError(errorMessage(cause));
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [slug, month, after, revision]);

  const refresh = useCallback(() => {
    setAfter(undefined);
    setRevision((value) => value + 1);
  }, []);
  useEffect(() => {
    // Re-read committed bookings when returning from another tab, without retrying mutations.
    if (pauseFocusRefresh) return;
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, [refresh, pauseFocusRefresh]);

  function showMonth(value: string) {
    setMonth(value);
    setAfter(undefined);
    setData(null);
    setRevision((current) => current + 1);
  }
  function loadMore() {
    if (loading || !data?.next_cursor) return;
    setMonth(data.month);
    setAfter(data.next_cursor);
    setRevision((value) => value + 1);
  }
  return { data, loading, error, refresh, showMonth, loadMore };
}
