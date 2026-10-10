"use client";
import { useCallback, useEffect, useRef, useState } from "react";
import { listServices, listAdminServices, Service } from "@/lib/api/services";
import { ApiError, errorMessage } from "@/lib/api/client";

export function useServices(slug: string, access: "public" | "admin" = "public") {
  const [services, setServices] = useState<Service[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [errorStatus, setErrorStatus] = useState(0);
  const current = useRef<AbortController | null>(null);
  const reload = useCallback(async () => {
    current.current?.abort();
    const controller = new AbortController();
    current.current = controller;
    setLoading(true);
    setError("");
    setErrorStatus(0);
    setServices([]);
    try {
      const list = access === "admin" ? listAdminServices : listServices;
      const result = await list(slug, controller.signal);
      if (!controller.signal.aborted) setServices(result);
    } catch (error) {
      if (!controller.signal.aborted) {
        setError(errorMessage(error));
        setErrorStatus(error instanceof ApiError ? error.status : 0);
      }
    } finally {
      if (!controller.signal.aborted) setLoading(false);
    }
  }, [slug, access]);
  useEffect(() => {
    void reload();
    return () => current.current?.abort();
  }, [reload]);
  return { services, loading, error, errorStatus, reload };
}
