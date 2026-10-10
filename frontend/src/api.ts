/** 共享本地API客户端：请求有超时，调用者可取消过期请求。 */
export async function api<T>(
  path: string,
  method = "GET",
  signal?: AbortSignal,
): Promise<T> {
  const timeout = new AbortController();
  const abort = () => timeout.abort(signal?.reason);
  if (signal?.aborted) abort();
  signal?.addEventListener("abort", abort, { once: true });
  const timer = setTimeout(
    () => timeout.abort(new Error("请求超时，请重试")),
    15000,
  );
  try {
    const response = await fetch(`/api${path}`, {
      method,
      signal: timeout.signal,
    });
    if (!response.ok) {
      const e = await response
        .json()
        .catch(() => ({ error: "服务暂时不可用" }));
      throw new Error(e.error ?? "请求失败");
    }
    return await response.json();
  } finally {
    clearTimeout(timer);
    signal?.removeEventListener("abort", abort);
  }
}
