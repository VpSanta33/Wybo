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
import { api, demoMode } from "./api";
import { useDocumentMetadata } from "./metadata";

const defaults: Record<string, string> = {
  site_name: "青屿集",
  site_brand: "Qingyu",
  site_logo: "",
  site_favicon: "",
  site_desc: "数字商品、会员服务与兑换码，支持在线下单和订单查询。",
  site_footer: "",
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
        demoMode
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
  useDocumentMetadata(pathname, site, loading);
  useEffect(() => {
    if (loading) return;
    const icon = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (icon) {
      icon.href = safeSiteImage(site.site_favicon) || "/favicon.svg";
      icon.removeAttribute("type");
    }
  }, [site, loading]);
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
    <img className={`site-logo ${className}`} src="/logo-icon.svg" width={34} height={34} alt="" />
  );
}
export function SiteBrand() {
  const { site } = useSite();
  return (
    <Link className="store-brand" to="/" aria-label={site.site_name + "首页"}>
      <SiteLogo url={site.site_logo} />
      <span className="store-wordmark">
        <strong title={site.site_name}>{site.site_name}</strong>
        {site.site_brand && site.site_brand !== site.site_name && <small>{site.site_brand}</small>}
      </span>
    </Link>
  );
}
