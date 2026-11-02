import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  ArrowRight,
  CircleHelp,
  Copy,
  LoaderCircle,
  ShieldCheck,
  X,
} from "lucide-react";
import { api, startPayment, type ShopOrder, type PaymentGateway } from "./api";
import { money } from "./data";
import { paymentLogo } from "./payments";
import QRCode from "qrcode";

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

export function PaymentAction({ order }: { order: ShopOrder }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [gateways, setGateways] = useState<PaymentGateway[] | null>(null);
  const [gateway, setGateway] = useState("");
  const [revision, setRevision] = useState(0);
  const [paymentQR, setPaymentQR] = useState("");
  const [qrError, setQRError] = useState(false);
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
        setGateway(rows[0]?.code || "");
      })
      .catch((e) => {
        if (!ctrl.signal.aborted) setError(e.message);
      });
    return () => ctrl.abort();
  }, [revision, order.order_no]);
  const selected = gateways?.find((g) => g.code === gateway);
  async function pay() {
    setBusy(true);
    setError("");
    try {
      const result = await startPayment(
        order.order_no,
        gateway,
        order.contact || "",
      );
      if (result.type === "qr") {
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
        const form = document.createElement("form");
        form.method = "POST";
        form.action = result.pay_url;
        form.acceptCharset = "UTF-8";
        form.hidden = true;
        for (const [name, value] of Object.entries(result.form || {})) {
          const input = document.createElement("input");
          input.type = "hidden";
          input.name = name;
          input.value = value;
          form.append(input);
        }
        document.body.append(form);
        try {
          HTMLFormElement.prototype.submit.call(form);
        } finally {
          form.remove();
        }
      } else {
        window.location.assign(result.pay_url);
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : "支付暂不可用，请稍后重试。");
      setBusy(false);
    }
  }
  return (
    <>
      {!!gateways?.length && (
        <fieldset className="payment-options" disabled={busy}>
          <legend>选择支付方式</legend>
          {gateways.map((g) => (
            <label
              className={`payment-choice ${gateway === g.code ? "is-selected" : ""}`}
              key={g.code}
            >
              <input
                type="radio"
                name={"payment-" + order.order_no}
                value={g.code}
                checked={gateway === g.code}
                onChange={() => {
                  setGateway(g.code);
                  setPaymentQR("");
                }}
              />
              <PaymentLogo code={g.code} logoUrl={g.logo_url} />
              <span>
                <strong>{g.label || g.name}</strong>
                <small>{g.description || g.name}</small>
              </span>
            </label>
          ))}
        </fieldset>
      )}
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
          <p>请核对收款金额。付款后点击下方“刷新订单”查看发货结果。</p>
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
            ? paymentQR
              ? "重新显示付款码"
              : "前往安全收银台"
            : "暂无可用支付方式"}
        <ArrowRight size={16} />
      </button>
    </>
  );
}
