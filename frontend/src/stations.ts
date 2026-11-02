export type Station = {
  id: number;
  agent_id: number;
  slug: string;
  status: "active" | "paused";
  site_name: string;
  site_brand: string;
  site_logo: string;
  site_favicon: string;
  site_desc: string;
  announcement: string;
  customer_service: string;
  default_redeem_url: string;
  redeem_guide: string;
  email: string;
};
export type StationProduct = {
  product_id: number;
  name: string;
  cover: string;
  stock: number;
  delivery_mode: string;
  product_enabled: boolean;
  wholesale_cents: number;
  retail_cents: number;
  authorized: boolean;
  listed: boolean;
};
export type StationOrder = {
  order_no: string;
  status: string;
  total_cents: number;
  wholesale_cents: number | null;
  profit_cents: number;
  refunded_cents: number;
  created_at: string;
  paid_at: string | null;
  contact: string;
  pay_method: string;
};
export type StationSettlement = {
  id: number;
  amount_cents: number;
  order_count: number;
  status: string;
  reference: string;
  created_at: string;
};
export type StationBalance = {
  available_cents: number;
  pending_cents: number;
  reserved_cents: number;
  paid_cents: number;
};
export type StationDashboard = {
  station: Station;
  url: string;
  settlement_days: number;
  two_factor_enabled: boolean;
  revenue_cents: number;
  profit_cents: number;
  refund_cents: number;
  paid_orders: number;
  pending_orders: number;
  pv: number;
  uv: number;
  traffic_days: number;
  balance: StationBalance;
  orders: StationOrder[];
  settlements: StationSettlement[];
};
export const STATION_TOKEN = "wybo-station-session-v1";
export async function stationApi<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const res = await fetch("/api/v1/station" + path, {
    ...options,
    signal: options.signal ?? AbortSignal.timeout(15000),
    headers: {
      ...(options.body instanceof FormData
        ? {}
        : { "Content-Type": "application/json" }),
      Authorization: "Bearer " + (sessionStorage.getItem(STATION_TOKEN) || ""),
      ...options.headers,
    },
  });
  let body;
  try {
    body = await res.json();
  } catch {
    throw new Error("分站服务暂时不可用，请重试");
  }
  if (res.status === 401) {
    sessionStorage.removeItem(STATION_TOKEN);
    window.dispatchEvent(new Event("wybo-station-expired"));
  }
  if (!res.ok || body.code !== 0) throw new Error(body.message || "操作未完成");
  return body.data;
}
export const emptyStation: Station = {
  id: 0,
  agent_id: 0,
  slug: "",
  status: "active",
  site_name: "",
  site_brand: "",
  site_logo: "",
  site_favicon: "",
  site_desc: "",
  announcement: "",
  customer_service: "",
  default_redeem_url: "",
  redeem_guide: "",
  email: "",
};
