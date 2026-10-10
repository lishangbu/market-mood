import { usePollingResource } from "./usePollingResource";
import { time } from "./format";

type Task = {
  id: string;
  name: string;
  state: "starting" | "running" | "failed" | "stopped";
  started_at: string | null;
  last_checked_at: string | null;
  last_success_at: string | null;
  error: string | null;
};
const labels = {
  starting: "等待启动",
  running: "运行中",
  failed: "已异常停止",
  stopped: "已停止",
};

/** 独立轮询，行情接口失败时仍可定位采集任务；请求失败不能显示为运行正常。 */
export function TaskStatus() {
  const { data, error } = usePollingResource<{ status: string; tasks: Task[] }>(
    "/tasks",
  );
  const stopped =
    data?.tasks.filter((t) => t.state === "failed" || t.state === "stopped") ??
    [];
  const failing = data?.tasks.filter((t) => t.error) ?? [];
  return (
    <section aria-label="后台采集状态" style={{ overflowWrap: "anywhere" }}>
      {error ? (
        <p className="notice error" role="alert">
          无法确认后台采集状态：{error}
        </p>
      ) : stopped.length > 0 ? (
        <p className="notice error" role="alert">
          后台采集已停止：{stopped.map((t) => t.name).join("、")}
          。请检查服务日志并重启服务；页面保留的数据可能不再更新。
        </p>
      ) : failing.length > 0 ? (
        <p className="warning" role="status">
          部分后台任务失败，正在等待重试：
          {failing.map((t) => t.name).join("、")}
        </p>
      ) : null}
      <details>
        <summary>
          {error
            ? "后台状态待确认"
            : data?.status === "ok"
              ? "后台采集运行中"
              : "查看后台采集状态"}
        </summary>
        {!data && <p className="muted small">正在读取任务状态…</p>}
        {data?.tasks.map((task) => (
          <div key={task.id} className="source-panel">
            <strong>
              {task.name} · {labels[task.state]}
            </strong>
            <p>
              最近成功：
              {task.last_success_at ? time(task.last_success_at) : "尚未成功"}
            </p>
            <p>
              最近检查：
              {task.last_checked_at ? time(task.last_checked_at) : "尚未完成"}
            </p>
            {task.error && <p className="warning">{task.error}</p>}
          </div>
        ))}
      </details>
    </section>
  );
}
