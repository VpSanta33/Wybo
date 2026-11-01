import { useCallback, useEffect, useState } from "react";
export const TOKEN_KEY = "wybo-admin-session-v1";
const apiBase =
  (import.meta as ImportMeta & { env: Record<string, string> }).env
    .VITE_API_BASE_URL || "/api/v1";
export function adminBase() {
  return (
    "/" + (window.location.pathname.split("/").filter(Boolean)[0] || "admin")
  );
}
export function token() {
  return sessionStorage.getItem(TOKEN_KEY) || "";
}
export async function adminFetch(
  path: string,
  options: RequestInit = {},
): Promise<Response> {
  const send = (proof = "") =>
    fetch(apiBase + "/admin" + path, {
      ...options,
      signal: options.signal ?? AbortSignal.timeout(15000),
      headers: {
        "Content-Type": "application/json",
        "X-Admin-Path": adminBase(),
        Authorization: `Bearer ${token()}`,
        ...options.headers,
        ...(proof ? { "X-Admin-Confirm": proof } : {}),
      },
    });
  let response = await send();
  if (response.status === 428) {
    const action = new URL(apiBase + "/admin" + path, window.location.origin)
      .pathname;
    const proof = await new Promise<string>((resolve, reject) =>
      window.dispatchEvent(
        new CustomEvent("wybo-admin-confirm", {
          detail: { action, resolve, reject },
        }),
      ),
    );
    if (options.signal?.aborted) throw new Error("操作已取消");
    response = await send(proof);
  }
  return response;
}
export async function adminApi<T>(
  path: string,
  options: RequestInit = {},
): Promise<T> {
  const response = await adminFetch(path, options);
  let body;
  try {
    body = await response.json();
  } catch {
    throw new Error(
      response.status === 404
        ? "后台地址不存在或已修改"
        : "服务暂时不可用，请稍后重试",
    );
  }
  if (response.status === 401 || body.code === 401) {
    if (path !== "/login")
      window.dispatchEvent(new Event("wybo-admin-expired"));
    throw new Error(body.message || "登录已过期");
  }
  if (!response.ok || body.code !== 0)
    throw new Error(body.message || "操作未完成");
  return body.data as T;
}
export async function checkEntry(path: string) {
  const res = await fetch(apiBase + "/admin/entry", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ path }),
    signal: AbortSignal.timeout(10000),
  });
  if (res.status === 404) return false;
  if (!res.ok) throw new Error("管理服务暂时连接不上，请稍后重试");
  return true;
}
export function useAdminQuery<T>(path: string) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const reload = useCallback(() => setRevision((v) => v + 1), []);
  useEffect(() => {
    const ctrl = new AbortController();
    setLoading(true);
    setError("");
    adminApi<T>(path, { signal: ctrl.signal })
      .then(setData)
      .catch((e) => {
        if (!ctrl.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setLoading(false);
      });
    return () => ctrl.abort();
  }, [path, revision]);
  return { data, error, loading, reload };
}
export const yuan = (cents: number) =>
  new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "CNY",
    minimumFractionDigits: 2,
  }).format(cents / 100);
export const number = (v: number) => new Intl.NumberFormat("zh-CN").format(v);
export function toCents(value: string) {
  if (!/^\d+(\.\d{1,2})?$/.test(value))
    throw new Error("金额须为非负数，最多保留两位小数");
  const [a, b = ""] = value.split(".");
  const result = Number(a) * 100 + Number((b + "00").slice(0, 2));
  if (!Number.isSafeInteger(result) || result > 10000000000)
    throw new Error("金额超出可用范围");
  return result;
}
export function exportCSV(name: string, rows: (string | number)[][]) {
  const escape = (v: string | number) =>
    '"' +
    String(v)
      .replace(/^[=+@-]/, "'$&")
      .replaceAll('"', '""') +
    '"';
  const blob = new Blob(
    ["\uFEFF" + rows.map((row) => row.map(escape).join(",")).join("\r\n")],
    { type: "text/csv;charset=utf-8" },
  );
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
export interface AdminUser {
  permissions: string[];
  id: number;
  username: string;
  two_factor_enabled: boolean;
  admin_path: string;
  role: string;
}
export interface AdminProduct {
  id: number;
  category_id: number;
  name: string;
  description: string;
  cover: string;
  price_cents: number;
  cost_cents: number | null;
  original_price_cents: number;
  redeem_guide: string;
  stock: number;
  sales: number;
  enabled: boolean;
  sort: number;
}
export interface AdminCategory {
  id: number;
  name: string;
  sort: number;
  enabled: boolean;
}
export interface AdminOrder {
  delivered_refund?: boolean;
  net_profit_cents?: number | null;
  id: number;
  order_no: string;
  status: string;
  total_cents: number;
  cost_cents: number | null;
  fee_cents: number | null;
  fee_source: string;
  commission_cents: number;
  refunded_cents: number;
  cost_source: string;
  agent_name: string;
  agent_id: number | null;
  contact: string;
  email: string;
  pay_method: string;
  created_at: string;
  paid_at: string | null;
  items?: { product_name: string; quantity: number }[];
}
export interface DayStat {
  net_profit_cents: number;
  date: string;
  revenue_cents: number;
  cost_cents: number;
  profit_cents: number;
  paid_orders: number;
  pv: number;
  uv: number;
}
export interface DashboardData {
  stock_threshold: number;
  net_profit_cents: number;
  fee_cents: number;
  commission_cents: number;
  refund_cents: number;
  unknown_net_orders: number;
  estimated_orders: number;
  days: number;
  revenue_cents: number;
  cost_cents: number;
  profit_cents: number;
  paid_orders: number;
  untracked_orders: number;
  pv: number;
  uv: number;
  today: DayStat;
  daily: DayStat[];
  pending_orders: number;
  low_stock_products: number;
  product_count: number;
  failed_notifications: number;
  recent_orders: AdminOrder[];
  top_products: {
    id: number;
    name: string;
    quantity: number;
    revenue_cents: number;
    profit_cents: number;
  }[];
  updated_at: string;
}
export interface Gateway {
  id: number;
  name: string;
  code: string;
  enabled: boolean;
  config: string;
}
export interface Agent {
  commission_bps: number;
  settlement_days: number;
  id: number;
  name: string;
  code: string;
  contact: string;
  enabled: boolean;
}
export interface AgentRow {
  portal_enabled: boolean;
  balance: {
    available_cents: number;
    pending_cents: number;
    reserved_cents: number;
    paid_cents: number;
  };
  agent: Agent;
  prices: { product_id: number; price_cents: number }[];
  paid_orders: number;
  revenue_cents: number;
}
export interface Card {
  id: number;
  product_id: number;
  code: string;
  secret: string;
  status: string;
  batch_no: string;
  created_at: string;
}
export interface Notification {
  id: number;
  order_id: number;
  channel: string;
  status: string;
  attempts: number;
  last_error: string;
  created_at: string;
}

export function orderNet(o: AdminOrder) {
  if (o.net_profit_cents !== undefined) return o.net_profit_cents;
  if (
    (o.cost_cents == null && (o.status !== "refunded" || o.delivered_refund)) ||
    o.fee_cents == null ||
    !o.paid_at
  )
    return null;
  return (
    o.total_cents -
    (o.refunded_cents || 0) -
    (o.status === "refunded" && !o.delivered_refund ? 0 : o.cost_cents || 0) -
    o.fee_cents -
    (o.commission_cents || 0)
  );
}
