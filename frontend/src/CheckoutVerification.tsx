import { useEffect, useRef, useState } from "react";
import { api } from "./api";
import { RefreshCw, ShieldCheck } from "lucide-react";

type TurnstileAPI = {
  render(container: HTMLElement, options: Record<string, unknown>): string;
  remove(id: string): void;
};
declare global {
  interface Window {
    turnstile?: TurnstileAPI;
  }
}
let scriptPromise: Promise<TurnstileAPI> | null = null;
function loadTurnstile() {
  if (window.turnstile) return Promise.resolve(window.turnstile);
  if (scriptPromise) return scriptPromise;
  scriptPromise = new Promise<TurnstileAPI>((resolve, reject) => {
    const script = document.createElement("script");
    script.src =
      "https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit";
    script.async = true;
    const fail = () => {
      clearTimeout(timer);
      script.remove();
      scriptPromise = null;
      reject(new Error("人机验证暂时加载失败，请重试或检查网络。"));
    };
    const timer = window.setTimeout(fail, 15000);
    script.onerror = fail;
    script.onload = () => {
      if (!window.turnstile) {
        fail();
        return;
      }
      clearTimeout(timer);
      resolve(window.turnstile);
    };
    document.head.appendChild(script);
  });
  return scriptPromise;
}
export type VerificationState = { ready: boolean; token: string };
export default function CheckoutVerification({
  onChange,
}: {
  onChange: (state: VerificationState) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const callback = useRef(onChange);
  callback.current = onChange;
  const [status, setStatus] = useState("正在检查下单验证…");
  const [required, setRequired] = useState(true);
  const [error, setError] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    let active = true;
    let widget: string | undefined;
    let turnstile: TurnstileAPI | undefined;
    const ctrl = new AbortController();
    callback.current({ ready: false, token: "" });
    setError("");
    setRequired(true);
    setStatus("正在检查下单验证…");
    api<Record<string, string>>("/site", {
      signal: AbortSignal.any([ctrl.signal, AbortSignal.timeout(10000)]),
    })
      .then(async (site) => {
        if (!active) return;
        if (site.turnstile_enabled !== "true") {
          setRequired(false);
          callback.current({ ready: true, token: "" });
          return;
        }
        if (!site.turnstile_site_key)
          throw new Error("下单验证配置不完整，请联系店主。");
        setStatus("请完成人机验证");
        turnstile = await loadTurnstile();
        if (!active || !container.current) return;
        widget = turnstile.render(container.current, {
          sitekey: site.turnstile_site_key,
          action: "checkout",
          theme: "light",
          size: "compact",
          language: "zh-cn",
          "response-field": false,
          callback: (token: string) => {
            if (active) {
              setError("");
              setStatus("验证通过，可以提交订单");
              callback.current({ ready: true, token });
            }
          },
          "expired-callback": () => {
            if (active) {
              setStatus("验证已过期，正在重新验证…");
              callback.current({ ready: false, token: "" });
            }
          },
          "timeout-callback": () => {
            if (active) {
              setStatus("验证超时，请重试");
              setError("验证超时，请重新加载验证。");
              callback.current({ ready: false, token: "" });
            }
          },
          "error-callback": () => {
            if (active) {
              setError("验证暂时未通过，请重新加载验证。");
              callback.current({ ready: false, token: "" });
            }
            return true;
          },
        });
      })
      .catch((e) => {
        if (active && !ctrl.signal.aborted) {
          setError(
            e.name === "TimeoutError"
              ? "加载下单验证超时，请重新加载。"
              : e.message,
          );
          callback.current({ ready: false, token: "" });
        }
      });
    return () => {
      active = false;
      ctrl.abort();
      if (widget !== undefined) turnstile?.remove(widget);
    };
  }, [revision]);
  return (
    <section
      className="checkout-verification"
      hidden={!required}
      aria-label="下单人机验证"
    >
      <p className="verification-status" role="status">
        <ShieldCheck size={15} />
        {status}
      </p>
      <div ref={container} className="turnstile-container" />
      {error && (
        <p role="alert" className="verification-error">
          {error}
        </p>
      )}
      {error && (
        <button
          type="button"
          className="review-back"
          onClick={() => setRevision((v) => v + 1)}
        >
          <RefreshCw size={14} />
          重新加载验证
        </button>
      )}
    </section>
  );
}
