import { useCallback, useEffect, useRef, useState } from "react";
import { api } from "./api";

/** 自动轮询不重入；手动刷新取代旧请求。取消和版本检查共同保护响应顺序。 */
export function usePollingResource<T>(
  path: string,
  interval = 10000,
  enabled = true,
) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const current = useRef<AbortController | null>(null);
  const version = useRef(0);
  const refresh = useCallback(
    async (replace = true) => {
      if (!enabled || (!replace && current.current)) return;
      current.current?.abort();
      const controller = new AbortController();
      current.current = controller;
      const requestVersion = ++version.current;
      try {
        const result = await api<T>(path, "GET", controller.signal);
        if (version.current === requestVersion) {
          setData(result);
          setError("");
        }
      } catch (e) {
        if (version.current === requestVersion && !controller.signal.aborted)
          setError((e as Error).message);
      } finally {
        if (version.current === requestVersion) {
          current.current = null;
          setLoading(false);
        }
      }
    },
    [path, enabled],
  );
  useEffect(() => {
    const cancel = () => {
      ++version.current;
      current.current?.abort();
      current.current = null;
    };
    const update = () => {
      if (document.visibilityState === "visible") void refresh(false);
    };
    const visibility = () => {
      if (document.visibilityState === "visible") update();
      else cancel();
    };
    update();
    const timer = interval > 0 ? setInterval(update, interval) : undefined;
    document.addEventListener("visibilitychange", visibility);
    return () => {
      clearInterval(timer);
      document.removeEventListener("visibilitychange", visibility);
      cancel();
    };
  }, [refresh, interval]);
  return { data, error, loading, refresh };
}
