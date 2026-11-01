import { useEffect, useRef, useState, type ReactNode } from "react";
import {
  ArrowRight,
  CircleHelp,
  Copy,
  LoaderCircle,
  ShieldCheck,
  X,
} from "lucide-react";
import { api, startPayment, type ShopOrder } from "./api";
import { money } from "./data";

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
}: {
  order: ShopOrder;
  onCopy: (value: string) => void;
}) {
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
  const [gateways, setGateways] = useState<
    { code: string; name: string }[] | null
  >(null);
  const [gateway, setGateway] = useState("");
  const [revision, setRevision] = useState(0);
  useEffect(() => {
    const ctrl = new AbortController();
    setError("");
    api<{ code: string; name: string }[]>("/gateways", { signal: ctrl.signal })
      .then((rows) => {
        setGateways(rows);
        setGateway(rows[0]?.code || "");
      })
      .catch((e) => {
        if (!ctrl.signal.aborted) setError(e.message);
      });
    return () => ctrl.abort();
  }, [revision]);
  async function pay() {
    setBusy(true);
    setError("");
    try {
      window.location.assign(
        await startPayment(order.order_no, gateway, order.contact || ""),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "支付暂不可用，请稍后重试。");
      setBusy(false);
    }
  }
  return (
    <>
      {gateways && gateways.length > 1 && (
        <div className="form-field">
          <label htmlFor={"payment-" + order.order_no}>支付方式</label>
          <select
            id={"payment-" + order.order_no}
            value={gateway}
            onChange={(e) => setGateway(e.target.value)}
            style={{
              width: "100%",
              padding: 12,
              border: "1px solid #dde4f0",
              borderRadius: 10,
              marginBottom: 12,
            }}
          >
            {gateways.map((g) => (
              <option key={g.code} value={g.code}>
                {g.name}
              </option>
            ))}
          </select>
        </div>
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
            ? "前往安全收银台"
            : "暂无可用支付方式"}
        <ArrowRight size={16} />
      </button>
    </>
  );
}
