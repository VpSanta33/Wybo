import {
  createContext,
  useCallback,
  useContext,
  useEffect,
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
const SiteContext = createContext({ site: defaults, refresh: async () => {} });
export const useSite = () => useContext(SiteContext);
export function SiteProvider({ children }: { children: ReactNode }) {
  const [site, setSite] = useState(defaults);
  const { pathname } = useLocation();
  const refresh = useCallback(async () => {
    try {
      setSite({ ...defaults, ...(await api<Record<string, string>>("/site")) });
    } catch {
      /* Keep the last loaded branding during a connection failure. */
    }
  }, []);
  useEffect(() => {
    void refresh();
    window.addEventListener("focus", refresh);
    return () => window.removeEventListener("focus", refresh);
  }, [refresh]);
  useEffect(() => {
    const consolePage = !["/", "/orders", "/guide"].includes(pathname);
    document.title =
      site.site_name +
      (pathname === "/agent"
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
  return (
    <SiteContext.Provider value={{ site, refresh }}>
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
