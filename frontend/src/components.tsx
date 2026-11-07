import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  ArrowRight,
  Check,
  CircleHelp,
  Copy,
  LoaderCircle,
  ShieldCheck,
  X,
} from "lucide-react";
import { api, startPayment, queryOrder, checkOrderPayment, type ShopOrder, type PaymentGateway } from "./api";
import { money } from "./data";
import { paymentLogo } from "./payments";
import QRCode from "qrcode";
import { useNavigate, useLocation } from "react-router-dom";
import { rememberPayment, paymentSession, openCashierWindow, closeCashierWindow, hasCashierWindow } from "./paymentSession";
import { useSite } from "./Site";

/** 订单金额是否超过该支付方式的单笔限额（如牛牛支付单笔 500 元）。 */
export const gatewayOverLimit = (g: PaymentGateway, amountCents: number) =>
  !!g.max_amount_cents && amountCents > g.max_amount_cents;
/** 优先保留偏好的支付方式，超额时改选第一个可用渠道；全部超额时返回空。 */
export function payableGateway(rows: PaymentGateway[], amountCents: number, preferred = "") {
  const usable = rows.filter((g) => !gatewayOverLimit(g, amountCents));
  return usable.find((g) => g.code === preferred)?.code || usable[0]?.code || "";
}
export function PaymentLimitNotice({ gateways, amountCents }: { gateways: PaymentGateway[]; amountCents: number }) {
  const { site } = useSite();
  const blocked = gateways.filter((g) => gatewayOverLimit(g, amountCents));
  if (!blocked.length) return null;
  const names = blocked.map((g) => `${g.label || g.name}单笔限额 ¥${money(g.max_amount_cents || 0)}`).join("、");
  const allBlocked = blocked.length === gateways.length;
  return (
    <p className="payment-limit-note" role="status">
      订单金额 ¥{money(amountCents)} 超过{names}，
      {allBlocked ? "请联系客服下单。" : "请选择其他支付方式，或联系客服下单。"}
      {site.customer_service && <span className="payment-limit-contact">客服：{site.customer_service}</span>}
    </p>
  );
}

export function PaymentLogo({
  code,
  logoUrl,
  size = 28,
}: {
  code: string;
  logoUrl?: string;
  size?: number;
}) {
  const [failedSource, setFailedSource] = useState("");
  const custom =
    logoUrl && /^\/uploads\/[A-Za-z0-9_-]+\.(png|jpg|jpeg|gif)$/.test(logoUrl)
      ? logoUrl
      : "";
  const source = custom && custom !== failedSource ? custom : paymentLogo(code);
  return (
    <img
      src={source}
      alt=""
      width={size}
      height={size}
      onError={() => {
        if (custom && source === custom) setFailedSource(custom);
      }}
    />
  );
}

export function Modal({
  title,
  onClose,
  children,
  wide = false,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  useEffect(() => {
    const dialog = ref.current;
    const opener =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    dialog?.showModal();
    const previous = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = previous;
      dialog?.close();
      if (opener?.isConnected) opener.focus({ preventScroll: true });
    };
  }, []);
  return (
    <dialog
      ref={ref}
      className={`modal ${wide ? "modal-wide" : ""}`}
      aria-label={title}
      onCancel={(event) => {
        event.preventDefault();
        onClose();
      }}
      onClick={(event) => {
        if (event.target === event.currentTarget) onClose();
      }}
    >
      <div className="modal-inner">
        <button
          className="icon-button modal-close"
          onClick={onClose}
          aria-label="关闭弹窗"
        >
          <X size={21} />
        </button>
        {children}
      </div>
    </dialog>
  );
}

export function ErrorMessage({ children }: { children: ReactNode }) {
  return (
    <p className="error-message" role="alert">
      <CircleHelp size={17} aria-hidden="true" />
      {children}
    </p>
  );
}

const statuses: Record<string, string> = {
  preview: "演示订单",
  pending: "等待支付",
  paid: "正在发货",
  completed: "美好已送达",
  closed: "订单已关闭",
  refunded: "已退款",
  refunding: "退款处理中",
};

export function OrderSummary({
  order,
  onCopy,
  compact = false,
}: {
  order: ShopOrder;
  onCopy: (value: string) => void;
  compact?: boolean;
}) {
  if (compact) {
    return (
      <div className="compact-order-summary">
        <div className="order-overview">
          <strong
            className={`order-status status-${order.status}`}
            role="status"
          >
            {order.status === "completed"
              ? "支付成功 · 已发货"
              : order.status === "paid" &&
                  order.items?.some((item) => item.delivery_mode === "manual")
                ? "支付成功 · 等待手工发货"
                : statuses[order.status] || order.status}
          </strong>
          <strong className="order-price">¥{money(order.total_cents)}</strong>
        </div>
        {order.items?.map((item, i) => (
          <p className="order-product-line" key={i}>
            <strong>{item.product_name}</strong>
            <span>× {item.quantity}</span>
          </p>
        ))}
        <details className="order-metadata">
          <summary>
            订单详情<span>{order.order_no}</span>
          </summary>
          <div>
            <span>订单号</span>
            <code>{order.order_no}</code>
            <button
              className="icon-button"
              type="button"
              onClick={() => onCopy(order.order_no)}
              aria-label="复制订单号"
            >
              <Copy size={16} />
            </button>
          </div>
          <div>
            <span>下单时间</span>
            <time>
              {new Date(order.created_at).toLocaleString("zh-CN", {
                hour12: false,
              })}
            </time>
          </div>
        </details>
      </div>
    );
  }
  return (
    <div className="order-summary">
      <div>
        <span>订单号</span>
        <strong className="order-number">{order.order_no}</strong>
        <button
          className="icon-button"
          type="button"
          onClick={() => onCopy(order.order_no)}
          aria-label="复制订单号"
        >
          <Copy size={16} />
        </button>
      </div>
      <div>
        <span>订单状态</span>
        <strong className={`order-status status-${order.status}`}>
          {statuses[order.status] || order.status}
        </strong>
      </div>
      {order.items?.map((item, i) => (
        <div key={i}>
          <span>{item.product_name}</span>
          <strong>× {item.quantity}</strong>
        </div>
      ))}
      <div>
        <span>订单金额</span>
        <strong className="order-price">¥{money(order.total_cents)}</strong>
      </div>
      <div>
        <span>创建时间</span>
        <strong>
          {new Date(order.created_at).toLocaleString("zh-CN", {
            hour12: false,
          })}
        </strong>
      </div>
    </div>
  );
}

export function PaymentChoices({ gateways, value, onChange, disabled = false, name, checkout = false, amountCents = 0 }: {
  gateways: PaymentGateway[];
  value: string;
  onChange: (value: string) => void;
  disabled?: boolean;
  name: string;
  checkout?: boolean;
  amountCents?: number;
}) {
  return (
    <fieldset className={`payment-options ${checkout ? "checkout-payment-options" : ""}`} disabled={disabled}>
      <legend>支付方式</legend>
      <div className="payment-choices-grid">
        {gateways.map((g) => {
          const over = gatewayOverLimit(g, amountCents);
          return (
            <label className={`payment-choice ${value === g.code ? "is-selected" : ""} ${over ? "is-disabled" : ""}`} key={g.code}>
              <input type="radio" name={name} value={g.code} checked={value === g.code} disabled={over} onChange={() => onChange(g.code)} />
              <PaymentLogo code={g.payment_type && ["alipay", "usdt"].includes(g.payment_type) ? g.payment_type : g.code} logoUrl={g.logo_url} size={32} />
              <span>
                <strong>{g.label || g.name}</strong>
                <small>{over ? `单笔限额 ¥${money(g.max_amount_cents || 0)}，超出请联系客服下单` : g.description || g.name}</small>
              </span>
              <i className="payment-choice-check" aria-hidden="true">{value === g.code && <Check size={12} strokeWidth={3} />}</i>
            </label>
          );
        })}
      </div>
    </fieldset>
  );
}

export function PaymentAction({ order, initialGateway = "" }: { order: ShopOrder; initialGateway?: string }) {
  const navigate = useNavigate();
  const location = useLocation();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [gateways, setGateways] = useState<PaymentGateway[] | null>(null);
  const [gateway, setGateway] = useState("");
  const [revision, setRevision] = useState(0);
  const [paymentQR, setPaymentQR] = useState("");
  const [qrError, setQRError] = useState(false);
  useEffect(() => {
    if (!paymentQR || location.pathname === "/orders") return;
    const controller = new AbortController();
    let fetching = false;
    const refresh = async () => {
      if (fetching || document.visibilityState !== "visible") return;
      fetching = true;
      try {
        await checkOrderPayment(order.order_no, order.contact || "", controller.signal).catch(() => {});
        const current = await queryOrder(order.order_no, order.contact || "", controller.signal);
        if (!controller.signal.aborted && ["paid", "completed", "refunded"].includes(current.status)) {
          navigate(`/orders?order_no=${encodeURIComponent(order.order_no)}`, { state: { paymentContact: order.contact } });
        }
      } catch { /* Keep displaying the code until the server confirms payment. */ }
      finally { fetching = false; }
    };
    const timer = window.setInterval(refresh, 6000);
    return () => { controller.abort(); window.clearInterval(timer); };
  }, [paymentQR, order.order_no, order.contact, location.pathname, navigate]);
  useEffect(() => {
    const restore = (e: PageTransitionEvent) => { if (e.persisted) setBusy(false); };
    window.addEventListener("pageshow", restore);
    return () => window.removeEventListener("pageshow", restore);
  }, []);
  useEffect(() => {
    const ctrl = new AbortController();
    setError("");
    setGateways(null);
    setGateway("");
    setPaymentQR("");
    setQRError(false);
    api<PaymentGateway[]>("/gateways", { signal: ctrl.signal })
      .then((rows) => {
        setGateways(rows);
        const preferred = initialGateway || paymentSession(order.order_no)?.gateway;
        setGateway(payableGateway(rows, order.total_cents, preferred));
      })
      .catch((e) => {
        if (!ctrl.signal.aborted) setError(e.message);
      });
    return () => ctrl.abort();
  }, [revision, order.order_no, order.total_cents, initialGateway]);
  const selected = gateways?.find((g) => g.code === gateway);
  async function pay() {
    if (busy) return;
    setBusy(true);
    setError("");
    let cashier: Window | null = null;
    try {
      rememberPayment(order, gateway);
      if (["niuniupay", "epay"].includes(gateway)) cashier = openCashierWindow(order.order_no);
      const result = await startPayment(
        order.order_no,
        gateway,
        order.contact || "",
      );
      if (result.type === "qr") {
        if (cashier) closeCashierWindow(order.order_no);
        setQRError(false);
        setPaymentQR(
          result.qr
            ? await QRCode.toDataURL(result.qr, {
                width: 280,
                margin: 2,
                errorCorrectionLevel: "M",
              })
            : result.qr_image || "",
        );
        setBusy(false);
      } else if (result.type === "form") {
        const paymentDocument = cashier?.document || document;
        const form = paymentDocument.createElement("form");
        form.method = "POST";
        form.action = result.pay_url;
        form.acceptCharset = "UTF-8";
        form.hidden = true;
        for (const [name, value] of Object.entries(result.form || {})) {
          const input = paymentDocument.createElement("input");
          input.type = "hidden";
          input.name = name;
          input.value = value;
          form.append(input);
        }
        paymentDocument.body.append(form);
        try {
          HTMLFormElement.prototype.submit.call(form);
        } finally {
          form.remove();
        }
        if (cashier) {
          navigate(`/orders?order_no=${encodeURIComponent(order.order_no)}`, { state: { paymentContact: order.contact } });
          setBusy(false);
        }
      } else {
        if (cashier) {
          cashier.location.replace(result.pay_url);
          navigate(`/orders?order_no=${encodeURIComponent(order.order_no)}`, { state: { paymentContact: order.contact } });
          setBusy(false);
        } else window.location.assign(result.pay_url);
      }
    } catch (e) {
      if (cashier) closeCashierWindow(order.order_no);
      setError(e instanceof Error ? e.message : "支付暂不可用，请稍后重试。");
      setBusy(false);
    }
  }
  return (
    <>
      {hasCashierWindow(order.order_no) && <p className="soft-note" role="status">收银台已在独立窗口打开，本站正在核对到账。确认付款后会显示商品并尝试关闭支付窗口；若支付页未自动关闭，请关闭该页返回本订单。</p>}
      {!!gateways?.length && (
        <PaymentChoices gateways={gateways || []} value={gateway} disabled={busy} amountCents={order.total_cents} name={"payment-" + order.order_no} onChange={(code) => { setGateway(code); setPaymentQR(""); }} />
      )}
      {!!gateways?.length && <PaymentLimitNotice gateways={gateways} amountCents={order.total_cents} />}
      {selected?.crypto && (
        <p className="payment-crypto-note">
          请使用 USDT 钱包，确认收银台显示的网络和金额后再转账。
        </p>
      )}
      {paymentQR && (
        <section className="payment-qr-panel" aria-label="扫码支付">
          <h3>使用{selected?.label || "支付应用"}扫码付款</h3>
          <strong>¥{money(order.total_cents)}</strong>
          {qrError ? (
            <p role="alert">二维码图片加载失败，请重新获取或联系店主。</p>
          ) : (
            <img
              src={paymentQR}
              alt="当前订单付款二维码"
              width={240}
              height={240}
              referrerPolicy="no-referrer"
              onError={() => setQRError(true)}
            />
          )}
          <p>请核对收款金额。本站会自动确认到账，付款后显示订单与商品。</p>
        </section>
      )}
      {error && (
        <ErrorMessage>
          {error}
          <button
            type="button"
            className="cancel-order"
            onClick={() => setRevision((v) => v + 1)}
          >
            重新加载支付方式
          </button>
        </ErrorMessage>
      )}
      {gateways?.length === 0 && (
        <p className="soft-note">
          小铺暂未启用支付渠道，请联系店主或稍后再试。
        </p>
      )}
      <button
        type="button"
        className="button button-primary full-width"
        onClick={pay}
        disabled={busy || !gateway}
      >
        {busy ? (
          <LoaderCircle className="spin" size={17} />
        ) : (
          <ShieldCheck size={17} />
        )}
        {!gateways
          ? "正在加载支付方式…"
          : gateways.length
            ? !gateway
              ? "请联系客服下单"
              : paymentQR
              ? "重新显示付款码"
              : ["niuniupay", "epay"].includes(gateway)
                ? hasCashierWindow(order.order_no) ? "重新打开付款窗口" : "打开收银台（新窗口）"
                : "前往安全收银台"
            : "暂无可用支付方式"}
        <ArrowRight size={16} />
      </button>
    </>
  );
}
