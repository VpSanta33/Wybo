import { useId, type ReactNode } from "react";
import {
  CircleAlert,
  LoaderCircle,
  PackageOpen,
  RefreshCw,
} from "lucide-react";
export function Field({
  label,
  hint,
  children,
  className = "",
}: {
  label: string;
  hint?: string;
  children: ReactNode;
  className?: string;
}) {
  const id = useId();
  return (
    <div className={`a-field ${className}`}>
      <label id={id}>
        {label}
        {children}
      </label>
      {hint && <small>{hint}</small>}
    </div>
  );
}
export function Banner({
  children,
  success = false,
}: {
  children: ReactNode;
  success?: boolean;
}) {
  return (
    <div
      className={`a-banner ${success ? "is-success" : ""}`}
      role={success ? "status" : "alert"}
    >
      <CircleAlert size={17} aria-hidden="true" />
      <span>{children}</span>
    </div>
  );
}
export function Busy() {
  return (
    <div className="a-loading" role="status">
      <LoaderCircle size={24} className="spin" />
      <span>正在加载小铺数据…</span>
    </div>
  );
}
export function Empty({
  title = "暂时没有记录",
  text = "新的数据会在这里显示。",
  children,
}: {
  title?: string;
  text?: string;
  children?: ReactNode;
}) {
  return (
    <div className="a-empty">
      <span>
        <PackageOpen size={26} />
      </span>
      <h3>{title}</h3>
      <p>{text}</p>
      {children}
    </div>
  );
}
export function QueryError({
  error,
  reload,
}: {
  error: string;
  reload: () => void;
}) {
  return (
    <div className="a-query-error">
      <Banner>{error}</Banner>
      <button className="a-button" onClick={reload}>
        <RefreshCw size={15} />
        重新加载
      </button>
    </div>
  );
}
const statuses: Record<string, string> = {
  reserved: "已预占",
  replaced: "已替换",
  ordered: "待入库",
  received: "已入库",
  verified: "已验证",
  configured: "已配置",
  missing: "待配置",
  processing: "处理中",
  unknown: "待确认",
  manual: "需人工核查",
  error: "核查失败",
  unpaid: "未支付",
  created: "已创建",
  creating: "创建中",
  matched: "账实一致",
  mismatch: "信息不符",
  succeeded: "已退款",
  canceled: "已取消",
  open: "待处理",
  waiting: "等待回复",
  resolved: "已解决",
  refunding: "退款处理中",
  pending: "待付款",
  paid: "待发货",
  completed: "已完成",
  closed: "已关闭",
  refunded: "已退款",
  available: "可售",
  sold: "已售",
  disabled: "已停用",
  sent: "已发送",
  failed: "失败",
  sending: "发送中",
  cancelled: "已取消",
};
export function Status({ value, label }: { value: string; label?: string }) {
  return (
    <span className={`a-status a-status-${value}`}>
      <i />
      {label || statuses[value] || value}
    </span>
  );
}
export function PageHeading({
  eyebrow,
  title,
  description,
  children,
}: {
  eyebrow?: string;
  title: string;
  description: string;
  children?: ReactNode;
}) {
  return (
    <div className="a-page-heading">
      <div>
        {eyebrow && <span className="a-eyebrow">{eyebrow}</span>}
        <h1>{title}</h1>
        <p>{description}</p>
      </div>
      <div className="a-heading-actions">{children}</div>
    </div>
  );
}
export const shortDate = (value: string) =>
  new Date(value).toLocaleString("zh-CN", {
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
