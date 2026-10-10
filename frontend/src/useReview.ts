import { useEffect, useRef, useState } from "react";
import type { Dashboard, HistoricalDay } from "./types";
import { api } from "./api";
import { usePollingResource } from "./usePollingResource";

/** 日期目录与补录进度独立刷新；切换日期会取消旧快照请求。 */
export function useReview(tab: string) {
  const dates = usePollingResource<string[]>("/history", 10000, tab === "复盘");
  const progress = usePollingResource<{
    updated_at: string;
    completed: number;
    running: boolean;
    error: string | null;
  }>("/backfill", 10000, tab === "复盘");
  const [day, setDay] = useState("");
  const [history, setHistory] = useState<Dashboard[]>([]);
  const [historical, setHistorical] = useState<HistoricalDay | null>(null);
  const [close, setClose] = useState<Dashboard | null>(null);
  const [error, setError] = useState("");
  const [historyLoading, setHistoryLoading] = useState(false);
  useEffect(() => {
    if (dates.data)
      setDay((old) =>
        dates.data!.includes(old) ? old : (dates.data![0] ?? ""),
      );
  }, [dates.data]);
  const loadedDay = useRef("");
  const revision = JSON.stringify(progress.data);
  useEffect(() => {
    if (!day || tab !== "复盘") return;
    const controller = new AbortController();
    const changingDay = loadedDay.current !== day;
    if (changingDay) {
      loadedDay.current = "";
      setHistoryLoading(true);
      setHistory([]);
      setHistorical(null);
      setClose(null);
    }
    setError("");
    api<{
      snapshots: Dashboard[];
      close: Dashboard | null;
      historical: HistoricalDay | null;
    }>(`/history/${day}`, "GET", controller.signal)
      .then((h) => {
        if (controller.signal.aborted) return;
        loadedDay.current = day;
        setHistory(h.snapshots);
        setClose(h.close);
        setHistorical(h.historical);
      })
      .catch((e) => {
        if (!controller.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!controller.signal.aborted) setHistoryLoading(false);
      });
    return () => controller.abort();
  }, [day, tab, revision]);
  return {
    days: dates.data ?? [],
    day,
    setDay,
    history,
    historical,
    backfill: progress.data,
    close,
    historyError: error || dates.error || progress.error,
    historyLoading,
  };
}
