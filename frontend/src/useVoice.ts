import { useEffect, useRef, useState } from "react";
import type { Dashboard } from "./types";
export function useVoice(
  data: Dashboard | null,
  error: string,
  setToast: (text: string) => void,
) {
  const [voice, setVoice] = useState(
    () => localStorage.getItem("market-mood-voice") === "true",
  );
  const lastSpoken = useRef("");
  useEffect(() => {
    if (
      !voice ||
      !data ||
      !!error ||
      document.visibilityState !== "visible" ||
      !("speechSynthesis" in window)
    )
      return;
    const parts = new Intl.DateTimeFormat("en-GB", {
      timeZone: "Asia/Shanghai",
      hour: "2-digit",
      minute: "2-digit",
      hourCycle: "h23",
    }).formatToParts(new Date());
    const minute = Number(parts.find((p) => p.type === "minute")?.value);
    const hour = parts.find((p) => p.type === "hour")?.value;
    const key = `${data.day}-${hour}-${minute}`;
    if (
      data.session !== "交易中" ||
      minute % 10 !== 0 ||
      lastSpoken.current === key ||
      data.sealing?.rate == null ||
      data.sources.some((s) => s.stale || s.error)
    )
      return;
    lastSpoken.current = key;
    const u = new SpeechSynthesisUtterance(
      `当前主板封板率百分之${data.sealing.rate.toFixed(1)}`,
    );
    u.lang = "zh-CN";
    speechSynthesis.speak(u);
  }, [voice, data, error]);
  useEffect(() => {
    const cancelWhenHidden = () => {
      if (document.visibilityState !== "visible" && "speechSynthesis" in window)
        speechSynthesis.cancel();
    };
    document.addEventListener("visibilitychange", cancelWhenHidden);
    return () => {
      document.removeEventListener("visibilitychange", cancelWhenHidden);
      if ("speechSynthesis" in window) speechSynthesis.cancel();
    };
  }, []);
  function toggleVoice() {
    if (!("speechSynthesis" in window)) {
      setToast("当前浏览器不支持语音播报");
      return;
    }
    const enabled = !voice;
    setVoice(enabled);
    localStorage.setItem("market-mood-voice", String(enabled));
    if (enabled) {
      const u = new SpeechSynthesisUtterance(
        "已开启市场情绪播报，请保持页面在前台",
      );
      u.lang = "zh-CN";
      speechSynthesis.speak(u);
    } else speechSynthesis.cancel();
  }
  return { voice, toggleVoice };
}
