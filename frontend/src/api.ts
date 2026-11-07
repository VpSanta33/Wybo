import type { Category, Product } from "./data";
import { demoCategories, demoProducts } from "./data";
import { pricePending } from "./productBrands";

const env = (import.meta as ImportMeta & { env: Record<string, string> }).env;
const API_BASE = env.VITE_API_BASE_URL || "/api/v1";

export async function api<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers: { "Content-Type": "application/json", ...options.headers },
    signal: options.signal ?? AbortSignal.timeout(10000),
  });
  if (!response.ok)
    throw new Error(
      response.status >= 500
        ? "小铺暂时连接不上，请稍后再试。"
        : "请求未能完成，请检查填写的信息。",
    );
  const body = await response.json();
  if (body.code !== 0)
    throw new Error(body.message || "请求失败，请稍后重试。");
  return body.data as T;
}

export interface Catalog {
  products: Product[];
  categories: Category[];
  demo: boolean;
  notice: string;
  site?: Record<string, string>;
}

type CatalogProduct = Omit<Product, "kind" | "art" | "subtitle" | "tags">;

async function loadAllProducts(signal?: AbortSignal) {
  const products = new Map<number, CatalogProduct>();
  const agent = encodeURIComponent(agentCode());
  for (let page = 1; ; page += 1) {
    const result = await api<{ list: CatalogProduct[]; total: number }>(
      `/products?page=${page}&size=100&agent=${agent}`,
      { signal },
    );
    const previousSize = products.size;
    for (const product of result.list) products.set(product.id, product);
    if (products.size >= result.total || result.list.length === 0) break;
    if (products.size === previousSize)
      throw new Error("商品列表已更新，请刷新页面重新加载。");
  }
  return [...products.values()];
}

export async function loadCatalog(signal?: AbortSignal): Promise<Catalog> {
  if (env.VITE_DEMO_MODE === "true")
    return {
      products: demoProducts,
      categories: demoCategories,
      demo: true,
      notice: "",
    };
  try {
    const [products, categories, announcements, site] = await Promise.all([
      loadAllProducts(signal),
      api<Category[]>("/categories", { signal }),
      api<{ title: string; content: string }[]>("/announcements", {
        signal,
      }).catch(() => []),
      api<Record<string, string>>("/site", { signal }).catch(
        () => ({}) as Record<string, string>,
      ),
    ]);
    const enriched = products.map((p) => {
      const name = p.name.toLowerCase();
      const art: Product["art"] = name.includes("claude")
        ? "sun"
        : name.includes("gemini")
          ? "orbit"
          : name.includes("youtube")
            ? "play"
            : name.includes("spotify")
              ? "music"
              : name.includes("notion")
                ? "book"
                : "spark";
      return {
        ...p,
        art,
        kind: "ai" as const,
        subtitle: p.description?.split("\n")[0] || "月兔为你挑选的数字好物",
        tags: /chatgpt\s*pro\s*(5|20|50)\s*x/i.test(p.name)
          ? ["Pro 系列", `${p.name.match(/(5|20|50)\s*x/i)?.[0]} 套餐`]
          : [
              p.delivery_mode === "manual" ? "手工发货" : "自动发货",
              "数字商品",
            ],
      };
    });
    return {
      products: enriched,
      categories: categories ?? [],
      demo: false,
      notice: site.announcement || announcements?.[0]?.title || "",
      site,
    };
  } catch (error) {
    if (signal?.aborted) throw error;
    return {
      products: [],
      categories: [],
      demo: false,
      notice:
        error instanceof Error
          ? error.message
          : "小铺暂时连接不上，请稍后再试。",
    };
  }
}

export function agentCode() {
  return (
    new URLSearchParams(window.location.search).get("agent") ||
    sessionStorage.getItem("wybo-agent") ||
    ""
  );
}
export interface ShopOrder {
  contact?: string;
  order_no: string;
  total_cents: number;
  status: string;
  created_at: string;
  expired_at?: string;
  items?: {
    product_name: string;
    quantity: number;
    delivery_mode?: "auto" | "manual";
  }[];
  after_sales?: {
    kind: string;
    status: string;
    child_order_no: string;
    amount_cents: number;
  }[];
  cards?: { code: string; secret?: string }[];
  redemptions?: {
    product_id: number;
    product_name: string;
    redeem_url: string;
    redeem_guide: string;
  }[];
}

export interface PaymentGateway {
  code: string;
  payment_type?: string;
  name: string;
  label: string;
  description: string;
  logo_url: string;
  crypto: boolean;
  /** 渠道单笔限额（分），缺省表示不限制 */
  max_amount_cents?: number;
}

interface RecentOrder {
  order_no: string;
  name: string;
  demo: boolean;
  created_at: string;
}
const RECENT_KEY = "wybo-recent-orders-v1";

export function recentOrders(): RecentOrder[] {
  try {
    return JSON.parse(localStorage.getItem(RECENT_KEY) || "[]");
  } catch {
    return [];
  }
}

export function rememberOrder(order: ShopOrder, name: string, demo: boolean) {
  try {
    const existing = recentOrders().filter(
      (o) => o.order_no !== order.order_no,
    );
    localStorage.setItem(
      RECENT_KEY,
      JSON.stringify(
        [
          {
            order_no: order.order_no,
            name,
            demo,
            created_at: order.created_at,
          },
          ...existing,
        ].slice(0, 10),
      ),
    );
  } catch {
    /* Browsers with storage disabled can still use manual order lookup. */
  }
}

// Preview records are stored only in this tab, never in the production API.
const previewOrders = new Map<string, { contact: string; order: ShopOrder }>();

export async function createOrder(
  product: Product,
  quantity: number,
  contact: string,
  coupon: string,
  demo: boolean,
  turnstileToken = "",
): Promise<ShopOrder> {
  if (pricePending(product))
    throw new Error("这份套餐尚未定价，暂时无法下单。");
  let order: ShopOrder;
  if (demo) {
    order = {
      order_no: `DEMO-${crypto.randomUUID().slice(0, 8).toUpperCase()}`,
      total_cents: product.price_cents * quantity,
      status: "preview",
      created_at: new Date().toISOString(),
      items: [
        {
          product_name: product.name,
          quantity,
          delivery_mode: product.delivery_mode || "auto",
        },
      ],
    };
    previewOrders.set(order.order_no, { contact, order });
  } else {
    const response = await api<{
      order_no: string;
      total_cents: number;
      expired_at: string;
    }>("/orders", {
      method: "POST",
      body: JSON.stringify({
        product_id: product.id,
        quantity,
        contact,
        email: contact.includes("@") ? contact : "",
        coupon_code: coupon,
        agent_code: agentCode(),
        turnstile_token: turnstileToken,
        ...attribution(),
      }),
    });
    order = {
      ...response,
      status: "pending",
      created_at: new Date().toISOString(),
      items: [
        {
          product_name: product.name,
          quantity,
          delivery_mode: product.delivery_mode || "auto",
        },
      ],
    };
  }
  order.contact = contact;
  rememberOrder(order, product.name, demo);
  return order;
}

export async function queryOrder(
  orderNo: string,
  contact: string,
  signal?: AbortSignal,
): Promise<ShopOrder> {
  if (orderNo.startsWith("DEMO-")) {
    const record = previewOrders.get(orderNo);
    if (!record)
      throw new Error("演示订单仅在当前标签页内有效，刷新后可重新创建体验。");
    if (record.contact !== contact)
      throw new Error("联系方式与下单时不一致，请核对后重试。");
    return record.order;
  }
  const order = await api<ShopOrder>(
    `/orders/${encodeURIComponent(orderNo)}?contact=${encodeURIComponent(contact)}`,
    { signal },
  );
  return { ...order, contact };
}

export async function checkOrderPayment(orderNo: string, contact: string, signal?: AbortSignal) {
  return api<{ checked: boolean }>(`/orders/${encodeURIComponent(orderNo)}/check-payment`, {
    method: "POST", body: JSON.stringify({ contact }), signal,
  });
}

export type PaymentCheckout = {
  pay_url: string;
  type: "url" | "form" | "qr";
  form?: Record<string, string>;
  qr?: string;
  qr_image?: string;
};

export async function startPayment(
  orderNo: string,
  gateway: string,
  contact: string,
): Promise<PaymentCheckout> {
  const result = await api<PaymentCheckout>(
    `/orders/${encodeURIComponent(orderNo)}/pay`,
    {
      method: "POST",
      body: JSON.stringify({ gateway, contact }),
      signal: AbortSignal.timeout(25000),
    },
  );
  if (
    result.type === "qr" &&
    (result.qr || /^https?:\/\//i.test(result.qr_image || ""))
  )
    return result;
  if (gateway === "niuniupay" && result.type === "url" && result.pay_url) {
    try {
      const target = new URL(result.pay_url);
      if (target.protocol === "weixin:" && target.hostname === "dl" && target.pathname === "/business/" && !target.username && !target.password && !target.port && !target.hash && target.searchParams.get("ticket")) return result;
    } catch { /* Continue to the ordinary cashier URL validation. */ }
  }
  if (
    !result.pay_url ||
    !/^https?:\/\//i.test(result.pay_url) ||
    !["url", "form"].includes(result.type)
  )
    throw new Error("支付渠道暂未提供有效收银台地址，请稍后重试或联系店主。");
  if (
    result.type === "form" &&
    (!result.form ||
      !Object.keys(result.form).length ||
      Object.values(result.form).some((v) => typeof v !== "string"))
  )
    throw new Error("支付参数不完整，请稍后重试或联系店主。");
  return result;
}

export function attribution() {
  if (navigator.doNotTrack === "1")
    return { session_id: "", source: "direct", campaign: "" };
  try {
    let id = sessionStorage.getItem("wybo-visitor");
    if (!id) {
      id = crypto.randomUUID();
      sessionStorage.setItem("wybo-visitor", id);
    }
    let saved = sessionStorage.getItem("wybo-attribution");
    if (!saved) {
      const p = new URLSearchParams(window.location.search);
      saved = JSON.stringify({
        source:
          p.get("utm_source") ||
          (document.referrer ? new URL(document.referrer).hostname : "direct"),
        campaign: p.get("utm_campaign") || "",
      });
      sessionStorage.setItem("wybo-attribution", saved);
    }
    return { session_id: id, ...JSON.parse(saved) };
  } catch {
    return { session_id: "", source: "direct", campaign: "" };
  }
}
export function track(kind: "visit" | "select", product_id = 0) {
  const data = attribution();
  if (!data.session_id) return;
  void api("/events", {
    method: "POST",
    body: JSON.stringify({ ...data, kind, product_id }),
  }).catch(() => {});
}
