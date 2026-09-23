"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { listServices, Service } from "@/lib/api/services";
import { errorMessage } from "@/lib/api/client";

export function useServices(slug: string) {
  const [services, setServices] = useState<Service[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const current = useRef<AbortController | null>(null);
  const reload = useCallback(async () => {
    current.current?.abort();
    const controller = new AbortController();
    current.current = controller;
    setLoading(true);
    setError("");
    setServices([]);
    try {
      const result = await listServices(slug, controller.signal);
      if (!controller.signal.aborted) setServices(result);
    } catch (error) {
      if (!controller.signal.aborted) setError(errorMessage(error));
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [slug]);
  useEffect(() => {
    void reload();
    return () => current.current?.abort();
  }, [reload]);
  return { services, loading, error, reload };
}
