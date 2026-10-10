import { useState } from "react";
import type { Dashboard } from "./types";
export function Trend({ history }: { history: Dashboard[] }) {
  const [metric, setMetric] = useState<"score" | "rate">("score");
  const value = (d: Dashboard) =>
    metric === "score" ? d.score.total : d.sealing?.rate;
  const points = history.map((d) => {
    const observed = d.observed_at;
    if (observed === null) return { t: NaN, x: NaN, v: null };
    const t = new Date(observed);
    const minutes =
      Number(observed.slice(11, 13)) * 60 + Number(observed.slice(14, 16));
    return {
      t: t.getTime(),
      x: ((minutes - 570) / 330) * 320 + 10,
      v: value(d),
    };
  });
  const segments: string[] = [];
  let segment = "";
  points.forEach((p, i) => {
    if (p.v == null) {
      if (segment) segments.push(segment);
      segment = "";
      return;
    }
    if (i && p.t - points[i - 1].t > 90000) {
      if (segment) segments.push(segment);
      segment = "";
    }
    segment += `${segment ? "L" : "M"}${p.x},${125 - p.v * 1.1} `;
  });
  if (segment) segments.push(segment);
  return (
    <section className="card trend">
      <div className="section-title">
        <h3>盘中情绪轨迹</h3>
        <select
          aria-label="趋势指标"
          value={metric}
          onChange={(e) => setMetric(e.target.value as "score" | "rate")}
        >
          <option value="score">情绪评分</option>
          <option value="rate">封板率</option>
        </select>
      </div>
      <svg
        viewBox="0 0 340 150"
        role="img"
        aria-label={
          metric === "score"
            ? "当日情绪评分曲线，断开处为缺失"
            : "当日封板率曲线，断开处为缺失"
        }
      >
        {[0, 50, 100].map((v) => (
          <g key={v}>
            <line
              x1="10"
              x2="330"
              y1={125 - v * 1.1}
              y2={125 - v * 1.1}
              stroke="#303b4b"
              strokeDasharray="3 5"
            />
            <text x="10" y={120 - v * 1.1}>
              {v}
            </text>
          </g>
        ))}
        {segments.map((d, i) => (
          <path key={i} d={d} fill="none" stroke="#e8c77b" strokeWidth="2" />
        ))}
        {points
          .filter((p) => p.v != null)
          .map((p, i) => (
            <circle
              key={i}
              cx={p.x}
              cy={125 - p.v! * 1.1}
              r="2"
              fill="#e8c77b"
            />
          ))}
        <text x="10" y="145">
          09:30
        </text>
        <text x="150" y="145">
          午间休市
        </text>
        <text x="300" y="145">
          15:00
        </text>
      </svg>
      {points.every((p) => p.v == null) && (
        <p className="muted small">当日尚无有效指标，未绘制数值曲线。</p>
      )}
    </section>
  );
}
