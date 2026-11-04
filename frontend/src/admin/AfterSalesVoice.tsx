import { useCallback, useEffect, useRef, useState } from "react";
import { Volume2, VolumeX } from "lucide-react";
import { Link } from "react-router-dom";
import { Modal } from "../components";
import { adminApi } from "./api";
import { Banner } from "./ui";

type Activity = { cursor: number; count: number };

export default function AfterSalesVoice({
  userId,
  base,
  notify,
}: {
  userId: number;
  base: string;
  notify: (message: string) => void;
}) {
  const preferenceKey = `wybo-after-sales-voice-v1:${userId}`;
  const supported =
    "speechSynthesis" in window && "SpeechSynthesisUtterance" in window;
  const [open, setOpen] = useState(false);
  const [enabled, setEnabled] = useState(false);
  const [remembered, setRemembered] = useState(() => {
    try {
      return localStorage.getItem(preferenceKey) === "on";
    } catch {
      return false;
    }
  });
  const [connectionError, setConnectionError] = useState("");
  const [speechError, setSpeechError] = useState("");
  const [lastNotice, setLastNotice] = useState("");
  const utterance = useRef<SpeechSynthesisUtterance | null>(null);
  const cursor = useRef<number>();

  const stopSpeech = useCallback(() => {
    const current = utterance.current;
    utterance.current = null;
    if (current) {
      current.onerror = null;
      current.onend = null;
      window.speechSynthesis.cancel();
    }
  }, []);

  const speak = useCallback((text: string) => {
    if (!supported) return;
    stopSpeech();
    setSpeechError("");
    const message = new SpeechSynthesisUtterance(text);
    message.lang = "zh-CN";
    const voices = window.speechSynthesis.getVoices();
    const chinese = voices.find((voice) => /^zh[-_]CN$/i.test(voice.lang)) ||
      voices.find((voice) => /^zh/i.test(voice.lang));
    if (chinese) message.voice = chinese;
    message.rate = 1;
    message.onend = () => {
      if (utterance.current === message) utterance.current = null;
    };
    message.onerror = (event) => {
      if (utterance.current !== message) return;
      utterance.current = null;
      if (event.error === "canceled" || event.error === "interrupted") return;
      setSpeechError("语音未能播放，请检查设备音量、中文语音及浏览器声音权限，再点击试听。");
    };
    utterance.current = message;
    try {
      window.speechSynthesis.speak(message);
    } catch {
      utterance.current = null;
      setSpeechError("当前浏览器无法播放语音，请使用支持语音播报的浏览器。");
    }
  }, [supported, stopSpeech]);

  function toggle() {
    const next = !enabled;
    setEnabled(next);
    setRemembered(next);
    setConnectionError("");
    setSpeechError("");
    try {
      localStorage.setItem(preferenceKey, next ? "on" : "off");
    } catch {
      // Playback still works when browser storage is unavailable.
    }
    if (next) speak("售后语音播报已开启。");
    else stopSpeech();
  }

  useEffect(() => {
    if (!enabled) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    let inFlight = false;
    async function poll() {
      if (inFlight || controller.signal.aborted) return;
      clearTimeout(timer);
      inFlight = true;
      try {
        const query = cursor.current === undefined ? "" : `?after=${cursor.current}`;
        const activity = await adminApi<Activity>(`/tickets/activity${query}`, {
          signal: AbortSignal.any([controller.signal, AbortSignal.timeout(10000)]),
        });
        if (controller.signal.aborted) return;
        cursor.current = activity.cursor;
        setConnectionError("");
        if (activity.count > 0) {
          const text = `您有 ${activity.count} 条新的售后消息，请及时处理。`;
          setLastNotice(text);
          notify(text);
          speak(text);
          window.dispatchEvent(new Event("wybo-ticket-activity"));
        }
      } catch (error) {
        if (!controller.signal.aborted) {
          setConnectionError(`售后提醒连接失败，将自动重试：${(error as Error).message}`);
        }
      } finally {
        inFlight = false;
        if (!controller.signal.aborted) timer = setTimeout(poll, 15000);
      }
    }
    function onVisible() {
      if (document.visibilityState === "visible") void poll();
    }
    cursor.current = undefined;
    void poll();
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("online", poll);
    return () => {
      controller.abort();
      clearTimeout(timer);
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("online", poll);
    };
  // Enabling starts a new listening session; route changes keep it running.
  }, [enabled, notify, speak]);

  useEffect(() => () => stopSpeech(), [stopSpeech]);

  const failed = !!connectionError || !!speechError;
  const status = !supported ? "不可用" : failed ? "需检查" : enabled ? "已开启" : remembered ? "待启用" : "已关闭";

  return (
    <>
      <button
        className={`a-voice-button ${enabled && !failed ? "is-on" : ""}`}
        aria-label={`售后语音播报：${status}`}
        title={`售后语音播报：${status}`}
        onClick={() => setOpen(true)}
      >
        {enabled ? <Volume2 size={17} aria-hidden="true" /> : <VolumeX size={17} aria-hidden="true" />}
        <span>售后播报</span>
        <i className={failed ? "has-error" : ""} aria-hidden="true" />
      </button>
      {open && (
        <Modal title="售后语音播报" onClose={() => setOpen(false)}>
          <div className="a-editor">
            <h2>售后语音播报</h2>
            <p>新工单和顾客追问到达时，用中文语音提醒你处理售后。</p>
            <p role="status">当前状态：{status}</p>
            <p className="a-hint">
              开启后每 15 秒检查一次，切换后台页面继续提醒；仅播报新消息数量，不朗读工单内容。
              请保持后台页面打开，重新打开或刷新后需点击启用。后台标签页可能受浏览器节能策略影响而延迟。
            </p>
            {!supported && <Banner>当前浏览器不支持语音播报。</Banner>}
            {connectionError && <Banner>{connectionError}</Banner>}
            {speechError && <Banner>{speechError}</Banner>}
            {lastNotice && <p role="status">最近提醒：{lastNotice}</p>}
            <div className="a-form-actions">
              <button
                className="a-button"
                disabled={!supported}
                onClick={() => speak("您有新的售后消息，请及时处理。")}
              >
                试听播报
              </button>
              <button
                className={`a-button ${enabled ? "" : "primary"}`}
                disabled={!supported}
                onClick={toggle}
              >
                {enabled ? "关闭播报" : "开启播报"}
              </button>
              <Link
                className="a-button"
                to={base + "/tickets"}
                onClick={() => setOpen(false)}
              >
                查看工单
              </Link>
            </div>
          </div>
        </Modal>
      )}
    </>
  );
}
