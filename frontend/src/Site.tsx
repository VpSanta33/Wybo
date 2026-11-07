import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Link, useLocation } from "react-router-dom";
import { api } from "./api";
import { BunnyMark } from "./Illustrations";

const defaults: Record<string, string> = {
  site_name: "望月AI代充小铺",
  site_brand: "Wybo",
  site_logo: "",
  site_favicon: "",
  site_desc: "ChatGPT · Claude · Grok 会员充值，支付成功自动发放卡密",
  site_footer: "一点月光，刚好喜欢。",
};
const SiteContext = createContext({
  site: defaults,
  refresh: async () => {},
  loading: true,
  error: "",
});
export const useSite = () => useContext(SiteContext);
const FOCUS_REFRESH_MS = 60_000;
function sameSite(a: Record<string, string>, b: Record<string, string>) {
  const keys = Object.keys(a);
  return (
    keys.length === Object.keys(b).length && keys.every((k) => a[k] === b[k])
  );
}
export function SiteProvider({ children }: { children: ReactNode }) {
  const [site, setSite] = useState(defaults);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const { pathname } = useLocation();
  const loaded = useRef(false);
  const lastFetch = useRef(0);
  const refresh = useCallback(async () => {
    lastFetch.current = Date.now();
    try {
      const next = {
        ...defaults,
        ...(await api<Record<string, string>>("/site")),
      };
      loaded.current = true;
      // 内容未变化时保留原对象，避免所有 useSite 消费者无谓重渲染。
      setSite((prev) => (sameSite(prev, next) ? prev : next));
      setError("");
    } catch (e) {
      // 已经加载过站点信息时，后台刷新失败只保留旧数据，不把整个应用替换成错误页。
      if (loaded.current) return;
      setError(
        (import.meta as ImportMeta & { env: Record<string, string> }).env
          .VITE_DEMO_MODE === "true"
          ? ""
          : e instanceof Error
            ? e.message
            : "站点信息加载失败",
      );
    } finally {
      setLoading(false);
    }
  }, []);
  useEffect(() => {
    void refresh();
    // 切回窗口时最多每分钟刷新一次。
    const onFocus = () => {
      if (Date.now() - lastFetch.current >= FOCUS_REFRESH_MS) void refresh();
    };
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [refresh]);
  useEffect(() => {
    const consolePage = !["/", "/orders", "/guide"].includes(pathname);
    document.title =
      site.site_name +
      (pathname === "/apply"
        ? " · 分站申请"
        : site.is_station === "true" && consolePage
          ? " · 分站工作台"
          : pathname === "/agent"
            ? " · 代理工作台"
            : consolePage
              ? " · 商家工作台"
              : "");
    document
      .querySelector('meta[name="description"]')
      ?.setAttribute("content", site.site_desc);
    const icon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (icon) {
      icon.href = safeSiteImage(site.site_favicon) || "/favicon.svg";
      icon.removeAttribute("type");
    }
  }, [site, pathname]);
  const value = useMemo(
    () => ({ site, refresh, loading, error }),
    [site, refresh, loading, error],
  );
  return (
    <SiteContext.Provider value={value}>
      {children}
    </SiteContext.Provider>
  );
}
export function safeSiteImage(url?: string) {
  return url && /^\/uploads\/[A-Za-z0-9_-]+\.(png|jpg|jpeg|gif)$/.test(url)
    ? url
    : "";
}
export function SiteLogo({
  url,
  className = "",
}: {
  url?: string;
  className?: string;
}) {
  const [failed, setFailed] = useState("");
  const src = safeSiteImage(url);
  return src && src !== failed ? (
    <img
      className={`site-logo ${className}`}
      src={src}
      alt=""
      width={34}
      height={34}
      onError={() => setFailed(src)}
    />
  ) : (
    <BunnyMark />
  );
}
export function SiteBrand() {
  const { site } = useSite();
  return (
    <Link className="store-brand" to="/" aria-label={site.site_name + "首页"}>
      <SiteLogo url={site.site_logo} />
      <span title={site.site_brand}>
        {site.site_brand}
        <span className="brand-period">.</span>
      </span>
      <span className="brand-chinese" title={site.site_name}>
        {site.site_name}
      </span>
    </Link>
  );
}
