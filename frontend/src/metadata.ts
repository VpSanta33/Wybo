import { useEffect, useRef } from "react";
import { api, demoMode } from "./api";

interface PageMetadata {
  title: string;
  description: string;
  url?: string;
  type?: string;
  image?: string;
  indexable: boolean;
  structured?: Record<string, unknown>;
}

export function isProductPath(path: string) {
  return path === "/p" || path.startsWith("/p/");
}

function metaTag(attribute: "name" | "property", key: string, content?: string) {
  let tag = document.head.querySelector<HTMLMetaElement>(`meta[${attribute}="${key}"]`);
  if (!content) {
    tag?.remove();
    return;
  }
  if (!tag) {
    tag = document.createElement("meta");
    tag.setAttribute(attribute, key);
    document.head.append(tag);
  }
  tag.content = content;
}

function applyMetadata(meta: PageMetadata, name: string) {
  document.title = meta.title;
  metaTag("name", "description", meta.description);
  metaTag("name", "robots", meta.indexable ? "index, follow" : "noindex, nofollow");
  metaTag("property", "og:locale", meta.type && "zh_CN");
  metaTag("property", "og:site_name", meta.type && name);
  metaTag("property", "og:type", meta.type);
  metaTag("property", "og:title", meta.type && meta.title);
  metaTag("property", "og:description", meta.type && meta.description);
  metaTag("property", "og:url", meta.url);
  metaTag("property", "og:image", meta.image);
  let canonical = document.head.querySelector<HTMLLinkElement>('link[rel="canonical"]');
  if (meta.url) {
    if (!canonical) {
      canonical = document.createElement("link");
      canonical.rel = "canonical";
      document.head.append(canonical);
    }
    canonical.href = meta.url;
  } else canonical?.remove();

  document.getElementById("store-structured-data")?.remove();
  if (meta.structured) {
    const script = document.createElement("script");
    script.type = "application/ld+json";
    script.id = "store-structured-data";
    script.textContent = JSON.stringify(meta.structured);
    document.head.append(script);
  }
}

export function useDocumentMetadata(path: string, site: Record<string, string>, loading: boolean) {
  const previousSite = useRef<Record<string, string>>();
  useEffect(() => {
    if (loading) return;
    const initialPath = document.head.querySelector<HTMLMetaElement>('meta[name="storefront-path"]');
    const sameBrand = !previousSite.current || previousSite.current === site;
    previousSite.current = site;
    if (initialPath?.content === path && sameBrand) return;
    initialPath?.remove();
    const titles: Record<string, string> = { "/": "数字商品商店", "/guide": "购买指南", "/orders": "我的订单", "/apply": "分站申请", "/agent": "代理工作台", "/activate": "分站激活" };
    const product = isProductPath(path);
    const page = titles[path] ?? (product ? "商品详情" : site.is_station === "true" ? "分站工作台" : "商家工作台");
    applyMetadata({ title: page + " · " + site.site_name, description: site.site_desc, indexable: false }, site.site_name);
    if (!(product || path === "/" || path === "/guide") || demoMode) return;
    const controller = new AbortController();
    api<PageMetadata>("/page-meta?path=" + encodeURIComponent(path), { signal: controller.signal })
      .then((meta) => { if (!controller.signal.aborted) applyMetadata(meta, site.site_name); })
      .catch(() => {});
    return () => controller.abort();
  }, [loading, path, site]);
}
