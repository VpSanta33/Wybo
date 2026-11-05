import type { ShopOrder } from "./api";

type PaymentSession = { contact: string; gateway: string; expires: number };
const windows = new Map<string, Window>();
const memory = new Map<string, PaymentSession>();
const key = (orderNo: string) => `wybo-payment-session:${orderNo}`;

export function rememberPayment(order: ShopOrder, gateway: string) {
  const value = { contact: order.contact || "", gateway, expires: Date.now() + 2 * 60 * 60 * 1000 };
  memory.set(order.order_no, value);
  try { sessionStorage.setItem(key(order.order_no), JSON.stringify(value)); } catch { /* In-memory checkout still works. */ }
}
export function paymentSession(orderNo: string): PaymentSession | null {
  try {
    const value = memory.get(orderNo) || JSON.parse(sessionStorage.getItem(key(orderNo)) || "null");
    if (value && typeof value.contact === "string" && typeof value.gateway === "string" && typeof value.expires === "number" && value.expires > Date.now()) return value;
  } catch { /* Manual lookup remains available. */ }
  return null;
}
export function hasCashierWindow(orderNo: string) { return windows.has(orderNo); }
export function openCashierWindow(orderNo: string) {
  closeCashierWindow(orderNo);
  const target = window.open("about:blank", `wybo-cashier-${crypto.randomUUID()}`);
  if (!target) throw new Error("浏览器阻止了支付窗口，请允许本站打开弹窗后重试。");
  // Preserve the opener relation for this registered payment provider. Browsers
  // require that relation to let the shop close the cashier after a verified receipt.
  target.document.title = "正在打开收银台";
  const message = target.document.createElement("p");
  message.textContent = "正在打开安全收银台。请保留商城订单页，付款后将自动核对到账并显示商品。";
  target.document.body.append(message);
  windows.set(orderNo, target);
  return target;
}
export function closeCashierWindow(orderNo: string) {
  const target = windows.get(orderNo);
  windows.delete(orderNo);
  if (!target) return false;
  try { target.close(); } catch { /* COOP or mobile browsers may require manual closing. */ }
  return true;
}
