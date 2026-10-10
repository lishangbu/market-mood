import { useEffect, useState } from "react";
type Source = "tdx" | "eastmoney" | "tencent" | "tonghuashun";
type Config = { quotes: Source[]; stock_quotes: Source[] };
const names: Record<Source, string> = {
  tdx: "通达信",
  eastmoney: "东方财富",
  tencent: "腾讯行情",
  tonghuashun: "同花顺",
};
export function SourceSettings() {
  const [config, setConfig] = useState<Config | null>(null),
    [message, setMessage] = useState(""),
    [busy, setBusy] = useState(false);
  useEffect(() => {
    fetch("/api/sources")
      .then((r) => {
        if (!r.ok) throw Error("读取来源配置失败");
        return r.json();
      })
      .then((c) =>
        setConfig({ quotes: c.quotes, stock_quotes: c.stock_quotes }),
      )
      .catch((e) => setMessage(e.message));
  }, []);
  async function save() {
    setBusy(true);
    setMessage("");
    try {
      const r = await fetch("/api/sources", {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(config),
      });
      if (!r.ok) throw Error((await r.json()).error ?? "保存失败");
      setMessage("已保存，后续采集使用新优先级；已缓存行情保留原来源。");
    } catch (e) {
      setMessage((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <details className="source-panel">
      <summary>数据源设置</summary>
      {config && (
        <>
          {(["quotes", "stock_quotes"] as const).map((key) => (
            <fieldset key={key}>
              <legend>
                {key === "quotes" ? "全市场行情" : "自选与股票详情"}
              </legend>
              <p className="small muted">
                按列表顺序尝试，失败或过期时切换下一来源。
              </p>
              {config[key].map((source, index) => (
                <div className="source-setting-row" key={source}>
                  <span>
                    {index + 1}. {names[source]}
                  </span>
                  <div>
                    <button
                      disabled={busy || index === 0}
                      aria-label={`${key} ${names[source]}上移`}
                      onClick={() =>
                        setConfig((c) => {
                          if (!c) return c;
                          const list = [...c[key]];
                          [list[index - 1], list[index]] = [
                            list[index],
                            list[index - 1],
                          ];
                          return { ...c, [key]: list };
                        })
                      }
                    >
                      上移
                    </button>
                    <button
                      disabled={busy || config[key].length === 1}
                      aria-label={`${key} 停用${names[source]}`}
                      onClick={() =>
                        setConfig(
                          (c) =>
                            c && {
                              ...c,
                              [key]: c[key].filter((s) => s !== source),
                            },
                        )
                      }
                    >
                      停用
                    </button>
                  </div>
                </div>
              ))}
              {(key === "quotes"
                ? ["tdx", "tencent", "eastmoney"]
                : ["tdx", "tencent", "eastmoney", "tonghuashun"]
              )
                .filter((s) => !config[key].includes(s as Source))
                .map((s) => (
                  <button
                    className="source-enable"
                    disabled={busy}
                    key={s}
                    onClick={() =>
                      setConfig(
                        (c) => c && { ...c, [key]: [...c[key], s as Source] },
                      )
                    }
                  >
                    启用{names[s as Source]}
                  </button>
                ))}
            </fieldset>
          ))}
          <p className="small muted">
            通达信全市场行情仍需验证，可手动启用；数据过期或不完整时会切换备用源。历史日线：通达信
            → 新浪 → 同花顺；两市分时：腾讯 →
            东方财富；当日封炸板池：东方财富；历史封炸板池：东方财富 →
            同花顺；基础资料：本地目录与交易所补充。
          </p>
          <button className="primary-button" disabled={busy} onClick={save}>
            {busy ? "保存中…" : "保存数据源配置"}
          </button>
        </>
      )}
      {message && (
        <p role="status" className="small">
          {message}
        </p>
      )}
    </details>
  );
}
