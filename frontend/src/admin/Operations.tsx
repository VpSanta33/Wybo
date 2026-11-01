import { useState, type FormEvent, type ReactNode } from "react";
import {
  Plus,
  Download,
  RefreshCw,
  ShieldCheck,
  Database,
  Receipt,
  MessageCircle,
  PackageCheck,
} from "lucide-react";
import { useAccess } from "./access";
import { Modal } from "../components";
import {
  adminApi,
  useAdminQuery,
  yuan,
  toCents,
  exportCSV,
  type AdminProduct,
  type AgentRow,
  adminFetch,
} from "./api";
import {
  Banner,
  Busy,
  Empty,
  Field,
  PageHeading,
  QueryError,
  Status,
  shortDate,
} from "./ui";

type Props = { notify: (m: string) => void };
export function QueryPanel({
  loading,
  error,
  reload,
  children,
}: {
  loading: boolean;
  error: string;
  reload: () => void;
  children: ReactNode;
}) {
  return loading ? (
    <Busy />
  ) : error ? (
    <QueryError error={error} reload={reload} />
  ) : (
    <>{children}</>
  );
}
export function Metric({
  label,
  value,
  note,
}: {
  label: string;
  value: string;
  note?: string;
}) {
  return (
    <article className="a-metric">
      <div>
        <span>{label}</span>
      </div>
      <strong>{value}</strong>
      {note && <small>{note}</small>}
    </article>
  );
}
export function ActionDialog({
  title,
  description,
  fields,
  submit,
  onClose,
  label = "确认保存",
}: {
  title: string;
  description: string;
  fields: ReactNode;
  submit: () => Promise<void>;
  onClose: () => void;
  label?: string;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await submit();
      onClose();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title={title}
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <form className="a-editor" onSubmit={save}>
        <h2>{title}</h2>
        <p>{description}</p>
        {error && <Banner>{error}</Banner>}
        {fields}
        <div className="a-form-actions">
          <button
            type="button"
            className="a-button"
            disabled={busy}
            onClick={onClose}
          >
            取消
          </button>
          <button className="a-button primary" disabled={busy}>
            {busy ? "正在处理…" : label}
          </button>
        </div>
      </form>
    </Modal>
  );
}

type ReportOrder = {
  order_no: string;
  status: string;
  revenue_cents: number;
  cost_cents: number | null;
  fee_cents: number | null;
  commission_cents: number;
  refund_cents: number;
  net_profit_cents: number | null;
  fee_source: string;
  cost_source: string;
};
type Report = {
  month: string;
  revenue_cents: number;
  cost_cents: number;
  fee_cents: number;
  commission_cents: number;
  refund_cents: number;
  net_profit_cents: number;
  unknown_orders: number;
  estimated_orders: number;
  orders: ReportOrder[];
  basis: string;
};
export function Reports() {
  const [month, setMonth] = useState(() =>
    new Intl.DateTimeFormat("sv-SE", {
      timeZone: "Asia/Shanghai",
      year: "numeric",
      month: "2-digit",
    }).format(new Date()),
  );
  const q = useAdminQuery<Report>(`/reports?month=${month}`);
  const d = q.data;
  return (
    <>
      <PageHeading
        title="利润与月报"
        description="每一笔收入、成本和支出，都有清楚的来处。"
      >
        <input
          type="month"
          aria-label="报表月份"
          value={month}
          onChange={(e) => {
            if (e.target.value) setMonth(e.target.value);
          }}
        />
        <button
          className="a-button"
          disabled={!d || q.loading}
          onClick={() =>
            d &&
            exportCSV(`Wybo-${month}-财务.csv`, [
              [
                "订单",
                "状态",
                "实收（分）",
                "成本（分）",
                "手续费（分）",
                "佣金（分）",
                "退款（分）",
                "经营净利润（分）",
                "成本依据",
                "手续费依据",
              ],
              ...d.orders.map((o) => [
                o.order_no,
                o.status,
                o.revenue_cents,
                o.cost_cents ?? "未知",
                o.fee_cents ?? "未知",
                o.commission_cents,
                o.refund_cents,
                o.net_profit_cents ?? "待补全",
                o.cost_source,
                o.fee_source,
              ]),
            ])
          }
        >
          <Download size={15} />
          导出月报
        </button>
      </PageHeading>
      <QueryPanel {...q}>
        {d && (
          <>
            {(d.unknown_orders > 0 || d.estimated_orders > 0) && (
              <Banner>
                {d.unknown_orders} 笔订单资料待补全，未计入净利润；
                {d.estimated_orders}{" "}
                笔使用商品成本或配置费率估算，可在订单详情补录实际手续费。
              </Banner>
            )}
            <div className="a-metrics">
              <Metric
                label="经营净利润"
                value={yuan(d.net_profit_cents)}
                note="已扣成本、手续费及佣金"
              />
              <Metric
                label="成交实收"
                value={yuan(d.revenue_cents)}
                note={`退款 ${yuan(d.refund_cents)}`}
              />
              <Metric label="商品成本" value={yuan(d.cost_cents)} />
              <Metric
                label="手续费 / 佣金"
                value={yuan(d.fee_cents)}
                note={`代理佣金 ${yuan(d.commission_cents)}`}
              />
            </div>
            <section className="a-panel">
              <div className="a-panel-heading">
                <div>
                  <h2>月度订单明细</h2>
                  <p>{d.basis}</p>
                </div>
              </div>
              {d.orders.length ? (
                <div className="a-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>订单</th>
                        <th>实收 / 退款</th>
                        <th>成本</th>
                        <th>手续费</th>
                        <th>佣金</th>
                        <th>净利润</th>
                      </tr>
                    </thead>
                    <tbody>
                      {d.orders.map((o) => (
                        <tr key={o.order_no}>
                          <td>
                            <strong>{o.order_no}</strong>
                            <Status value={o.status} />
                          </td>
                          <td>
                            {yuan(o.revenue_cents)}
                            <small>退款 {yuan(o.refund_cents)}</small>
                          </td>
                          <td>
                            {o.cost_cents == null
                              ? "待补全"
                              : yuan(o.cost_cents)}
                            <small>
                              {o.cost_source === "batch"
                                ? "批次成本"
                                : o.cost_source === "not_delivered"
                                  ? "未发货退款，无商品成本"
                                  : "商品成本快照"}
                            </small>
                          </td>
                          <td>
                            {o.fee_cents == null ? "待补全" : yuan(o.fee_cents)}
                            <small>
                              {o.fee_source === "actual"
                                ? "已核对"
                                : "按费率估算"}
                            </small>
                          </td>
                          <td>{yuan(o.commission_cents)}</td>
                          <td className="a-profit">
                            {o.net_profit_cents == null
                              ? "待补全"
                              : yuan(o.net_profit_cents)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <Empty title="本月还没有成交" />
              )}
            </section>
            <p className="a-hint">
              经营净利润尚未扣除税费、服务器及人工等经营费用。退款回溯原订单支付月份；手续费以渠道账单为准。
            </p>
          </>
        )}
      </QueryPanel>
    </>
  );
}

type Attempt = {
  id: number;
  order_no: string;
  gateway: string;
  provider_id: string;
  status: string;
  message: string;
  last_checked_at: string | null;
  created_at: string;
};
type Issue = {
  id: number;
  order_no: string;
  gateway: string;
  kind: string;
  status: string;
  message: string;
  reference: string;
};
type Refund = {
  id: number;
  order_no: string;
  status: string;
  amount_cents: number;
  reference: string;
  message: string;
  mode: string;
};
export function Reconciliation({ notify }: Props) {
  const q = useAdminQuery<{
    attempts: Attempt[];
    issues: Issue[];
    refunds: Refund[];
  }>("/payment-operations");
  const [busy, setBusy] = useState(0),
    [error, setError] = useState(""),
    [tab, setTab] = useState("attempts"),
    [issue, setIssue] = useState<Issue | null>(null),
    [note, setNote] = useState("");
  async function check(id: number) {
    setBusy(id);
    setError("");
    try {
      const msg = await adminApi<string>(`/reconcile/${id}`, {
        method: "POST",
      });
      notify(msg);
      q.reload();
    } catch (e) {
      setError((e as Error).message);
      q.reload();
    } finally {
      setBusy(0);
    }
  }
  const d = q.data;
  return (
    <>
      <PageHeading
        title="支付对账"
        description="核对渠道到账，及时发现漏单与重复付款。"
      >
        <button className="a-button" onClick={q.reload}>
          <RefreshCw size={15} />
          刷新记录
        </button>
      </PageHeading>
      {error && <Banner>{error}</Banner>}
      <div className="a-inline-note">
        <ShieldCheck size={19} />
        <p>
          Stripe、易支付、DujiaoPay 支持主动查询与漏单补发。BEpusdt
          使用已验签回调，异常时请核对渠道账单。未发货退款在订单详情处理；已发货退款请在售后处理页登记。
        </p>
      </div>
      <div className="a-tabs" role="group" aria-label="对账记录类型">
        {[
          ["attempts", "支付申请"],
          ["issues", "异常记录"],
          ["refunds", "退款记录"],
        ].map(([k, v]) => (
          <button
            key={k}
            aria-pressed={tab === k}
            className={tab === k ? "active" : ""}
            onClick={() => setTab(k)}
          >
            {v}
          </button>
        ))}
      </div>
      <section className="a-panel">
        <QueryPanel {...q}>
          {d &&
            (tab === "attempts" ? (
              d.attempts.length ? (
                <div className="a-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>订单 / 渠道</th>
                        <th>支付申请状态</th>
                        <th>最近核查</th>
                        <th>核查结果</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {d.attempts.map((a) => (
                        <tr key={a.id}>
                          <td>
                            <strong>{a.order_no}</strong>
                            <small>
                              {a.gateway} · {shortDate(a.created_at)}
                            </small>
                          </td>
                          <td>
                            <Status value={a.status} />
                          </td>
                          <td>
                            {a.last_checked_at
                              ? shortDate(a.last_checked_at)
                              : "尚未核查"}
                          </td>
                          <td className="a-cell-wrap">
                            {a.message || "等待渠道确认"}
                          </td>
                          <td>
                            <button
                              className="a-table-button"
                              disabled={!!busy}
                              onClick={() => check(a.id)}
                            >
                              {busy === a.id ? "核查中…" : "核对渠道"}
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <Empty title="还没有支付申请" />
              )
            ) : tab === "issues" ? (
              d.issues.length ? (
                <div className="a-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>订单 / 渠道</th>
                        <th>异常类型</th>
                        <th>处理状态</th>
                        <th>说明</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {d.issues.map((i) => (
                        <tr key={i.id}>
                          <td>
                            {i.order_no}
                            <small>{i.gateway}</small>
                          </td>
                          <td>
                            {{
                              duplicate: "重复收款",
                              mismatch: "金额不符",
                              callback_error: "回调异常",
                              recovered: "漏单已补",
                            }[i.kind] || i.kind}
                          </td>
                          <td>
                            <Status value={i.status} />
                          </td>
                          <td className="a-cell-wrap">{i.message}</td>
                          <td>
                            {i.status === "open" && (
                              <button
                                className="a-table-button"
                                onClick={() => {
                                  setIssue(i);
                                  setNote("");
                                }}
                              >
                                登记处理
                              </button>
                            )}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <Empty title="暂时没有支付异常" />
              )
            ) : d.refunds.length ? (
              <div className="a-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>订单</th>
                      <th>退款金额</th>
                      <th>状态</th>
                      <th>渠道凭证</th>
                      <th>说明</th>
                    </tr>
                  </thead>
                  <tbody>
                    {d.refunds.map((r) => (
                      <tr key={r.id}>
                        <td>
                          {r.order_no}
                          <small>
                            {r.mode === "manual"
                              ? "渠道退款登记"
                              : "Stripe API"}
                          </small>
                        </td>
                        <td>{yuan(r.amount_cents)}</td>
                        <td>
                          <Status value={r.status} />
                        </td>
                        <td>{r.reference || "等待渠道结果"}</td>
                        <td className="a-cell-wrap">{r.message}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <Empty title="还没有退款记录" />
            ))}
        </QueryPanel>
      </section>
      {issue && (
        <ActionDialog
          title="登记异常处理"
          description={`订单 ${issue.order_no}。请先核对渠道账单，处理说明会保存在异常记录中。`}
          onClose={() => setIssue(null)}
          fields={
            <Field label="处理说明">
              <textarea
                required
                minLength={4}
                maxLength={500}
                value={note}
                onChange={(e) => setNote(e.target.value)}
                rows={4}
              />
            </Field>
          }
          submit={async () => {
            await adminApi(`/payment-issues/${issue.id}/resolve`, {
              method: "POST",
              body: JSON.stringify({ note }),
            });
            q.reload();
            notify("处理记录已保存");
          }}
        />
      )}
    </>
  );
}

export function OrderFinance({
  order,
  reload,
}: {
  order: {
    order_no: string;
    status: string;
    fee_cents: number | null;
    fee_source: string;
    commission_cents: number;
    total_cents: number;
    pay_method: string;
    paid_at: string | null;
  };
  reload: () => void;
}) {
  const [fee, setFee] = useState(""),
    [mode, setMode] = useState(""),
    [reason, setReason] = useState(""),
    [reference, setReference] = useState(""),
    [confirm, setConfirm] = useState(false),
    [editing, setEditing] = useState(false);
  if (!order.paid_at) return null;
  return (
    <>
      <h3>手续费与佣金</h3>
      <div className="a-detail-line">
        <span>
          手续费 {order.fee_cents == null ? "待补全" : yuan(order.fee_cents)} ·{" "}
          {order.fee_source === "actual" ? "已核对" : "费率估算"}
        </span>
        <button
          className="a-table-button"
          onClick={() => {
            setFee(
              order.fee_cents == null ? "" : String(order.fee_cents / 100),
            );
            setEditing(true);
          }}
        >
          补录实际费用
        </button>
      </div>
      <div className="a-detail-line">
        <span>代理佣金</span>
        <strong>{yuan(order.commission_cents || 0)}</strong>
      </div>
      {["paid", "refunding"].includes(order.status) && (
        <div className="a-inline-note">
          <Receipt size={19} />
          <div>
            <p>未发货订单可全额退款 {yuan(order.total_cents)}。</p>
            <button
              className="a-table-button"
              onClick={() => {
                setMode(order.pay_method === "stripe" ? "stripe" : "manual");
                setConfirm(false);
                setReason("");
                setReference("");
              }}
            >
              {order.status === "refunding" ? "查询或继续退款" : "处理退款"}
            </button>
          </div>
        </div>
      )}
      {editing && (
        <ActionDialog
          title="补录实际手续费"
          description="请以渠道账单为准，费用为 0 时请明确填写 0。"
          onClose={() => setEditing(false)}
          fields={
            <Field label="实际手续费（元）">
              <input
                required
                type="number"
                min="0"
                step="0.01"
                value={fee}
                onChange={(e) => setFee(e.target.value)}
              />
            </Field>
          }
          submit={async () => {
            await adminApi(`/orders/${order.order_no}/fee`, {
              method: "PUT",
              body: JSON.stringify({ fee_cents: toCents(fee) }),
            });
            reload();
          }}
        />
      )}
      {!!mode && (
        <ActionDialog
          title="订单全额退款"
          description={`订单 ${order.order_no} · ${yuan(order.total_cents)}。退款期间会暂停发货。`}
          label={mode === "stripe" ? "确认提交 / 查询退款" : "确认登记已退款"}
          onClose={() => setMode("")}
          fields={
            <>
              <Field label="退款方式">
                <select
                  value={mode}
                  onChange={(e) => {
                    setMode(e.target.value);
                    setConfirm(false);
                  }}
                >
                  {order.pay_method === "stripe" && (
                    <option value="stripe">Stripe 原路全额退款</option>
                  )}
                  <option value="manual">已在渠道退款，登记凭证</option>
                </select>
              </Field>
              <Field label="退款原因">
                <textarea
                  required
                  minLength={2}
                  maxLength={500}
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                  rows={3}
                />
              </Field>
              {mode === "manual" && (
                <Field label="渠道退款流水 / 凭证">
                  <input
                    required
                    minLength={4}
                    maxLength={200}
                    value={reference}
                    onChange={(e) => setReference(e.target.value)}
                  />
                </Field>
              )}
              <label className="a-check">
                <input
                  required
                  type="checkbox"
                  checked={confirm}
                  onChange={(e) => setConfirm(e.target.checked)}
                />
                {mode === "stripe"
                  ? "我确认提交全额退款；已有申请将查询或继续原申请"
                  : "我确认已在渠道完成全额退款，并核对上述凭证"}
              </label>
            </>
          }
          submit={async () => {
            await adminApi(`/orders/${order.order_no}/refund`, {
              method: "POST",
              body: JSON.stringify({ mode, reason, reference, confirm }),
            });
            reload();
          }}
        />
      )}
    </>
  );
}

type Coupon = {
  id: number;
  code: string;
  type: string;
  value: number;
  min_amount_cents: number;
  start_at: string | null;
  end_at: string | null;
  total: number;
  used: number;
  enabled: boolean;
  product_ids: number[] | null;
};
export function Coupons({ notify }: Props) {
  const q = useAdminQuery<Coupon[]>("/coupons"),
    p = useAdminQuery<{ list: AdminProduct[] }>("/products?size=100");
  const [edit, setEdit] = useState<Coupon | null | undefined>();
  return (
    <>
      <PageHeading
        title="优惠与营销"
        description="为顾客准备一点小惊喜，也让每次优惠有据可查。"
      >
        <button className="a-button primary" onClick={() => setEdit(null)}>
          <Plus size={16} />
          创建优惠券
        </button>
      </PageHeading>
      <section className="a-panel">
        <QueryPanel {...q}>
          {q.data?.length ? (
            <div className="a-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>优惠码</th>
                    <th>优惠内容</th>
                    <th>适用商品</th>
                    <th>有效期</th>
                    <th>使用次数</th>
                    <th>状态</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.map((c) => (
                    <tr key={c.id}>
                      <td>
                        <strong>{c.code}</strong>
                      </td>
                      <td>
                        {c.type === "fixed"
                          ? `减 ${yuan(c.value)}`
                          : `支付 ${c.value}%`}
                        <small>满 {yuan(c.min_amount_cents)} 可用</small>
                      </td>
                      <td>
                        {c.product_ids?.length
                          ? `${c.product_ids.length} 件指定商品`
                          : "全部商品"}
                      </td>
                      <td>
                        {c.start_at ? shortDate(c.start_at) : "即刻生效"}
                        <small>
                          至 {c.end_at ? shortDate(c.end_at) : "长期有效"}
                        </small>
                      </td>
                      <td>
                        {c.used} / {c.total || "不限"}
                      </td>
                      <td>
                        <Status
                          value={c.enabled ? "available" : "disabled"}
                          label={c.enabled ? "启用" : "停用"}
                        />
                      </td>
                      <td>
                        <button
                          className="a-table-button"
                          onClick={() => setEdit(c)}
                        >
                          编辑
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title="准备第一份优惠"
              text="支持满减、折扣、限时与指定商品，也可以批量生成不同优惠码。"
            />
          )}
        </QueryPanel>
      </section>
      {edit !== undefined && (
        <CouponEditor
          row={edit}
          products={p.data?.list || []}
          onClose={() => setEdit(undefined)}
          onSaved={() => {
            q.reload();
            notify("优惠券已保存");
          }}
        />
      )}
    </>
  );
}
function localDate(v: string | null | undefined) {
  if (!v) return "";
  const d = new Date(v);
  return new Date(d.getTime() - d.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
}
function CouponEditor({
  row,
  products,
  onClose,
  onSaved,
}: {
  row: Coupon | null;
  products: AdminProduct[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [code, setCode] = useState(row?.code || ""),
    [type, setType] = useState(row?.type || "fixed"),
    [value, setValue] = useState(
      row ? String(row.type === "fixed" ? row.value / 100 : row.value) : "",
    ),
    [min, setMin] = useState(String((row?.min_amount_cents || 0) / 100)),
    [start, setStart] = useState(localDate(row?.start_at)),
    [end, setEnd] = useState(localDate(row?.end_at)),
    [total, setTotal] = useState(row?.total || 0),
    [enabled, setEnabled] = useState(row?.enabled ?? true),
    [ids, setIds] = useState<number[]>(row?.product_ids || []),
    [count, setCount] = useState(1),
    [prefix, setPrefix] = useState("MOON");
  return (
    <ActionDialog
      title={row ? "编辑优惠券" : "创建优惠券"}
      description="折扣填写实际支付百分比，例如 90 代表九折。次数在创建订单时占用。"
      onClose={onClose}
      fields={
        <>
          {!row && (
            <div className="a-form-grid">
              <Field label="生成数量">
                <input
                  type="number"
                  min="1"
                  max="200"
                  required
                  value={count}
                  onChange={(e) => setCount(Number(e.target.value))}
                />
              </Field>
              {count > 1 ? (
                <Field label="优惠码前缀">
                  <input
                    maxLength={10}
                    pattern="[A-Za-z0-9_-]*"
                    value={prefix}
                    onChange={(e) => setPrefix(e.target.value)}
                  />
                </Field>
              ) : (
                <Field label="优惠码">
                  <input
                    required
                    minLength={3}
                    maxLength={32}
                    pattern="[A-Za-z0-9_-]{3,32}"
                    value={code}
                    onChange={(e) => setCode(e.target.value)}
                  />
                </Field>
              )}
            </div>
          )}
          <div className="a-form-grid">
            <Field label="优惠类型">
              <select
                value={type}
                onChange={(e) => {
                  setType(e.target.value);
                  setValue("");
                }}
              >
                <option value="fixed">固定减免（元）</option>
                <option value="percent">折扣（支付百分比）</option>
              </select>
            </Field>
            <Field
              label={type === "fixed" ? "减免金额（元）" : "支付百分比（1–99）"}
            >
              <input
                required
                type="number"
                min={type === "fixed" ? "0.01" : "1"}
                max={type === "percent" ? "99" : undefined}
                step={type === "fixed" ? "0.01" : "1"}
                value={value}
                onChange={(e) => setValue(e.target.value)}
              />
            </Field>
            <Field label="最低订单金额（元）">
              <input
                type="number"
                required
                min="0"
                step="0.01"
                value={min}
                onChange={(e) => setMin(e.target.value)}
              />
            </Field>
            <Field label="每个优惠码可用次数" hint="0 表示不限次数">
              <input
                type="number"
                required
                min="0"
                step="1"
                value={total}
                onChange={(e) => setTotal(Number(e.target.value))}
              />
            </Field>
            <Field label="开始时间（本地时间）">
              <input
                type="datetime-local"
                value={start}
                onChange={(e) => setStart(e.target.value)}
              />
            </Field>
            <Field label="结束时间（本地时间）">
              <input
                type="datetime-local"
                value={end}
                onChange={(e) => setEnd(e.target.value)}
              />
            </Field>
          </div>
          <fieldset className="a-choice-list">
            <legend>
              适用商品 <small>不勾选则全部可用</small>
            </legend>
            {products.map((p) => (
              <label className="a-check" key={p.id}>
                <input
                  type="checkbox"
                  checked={ids.includes(p.id)}
                  onChange={(e) =>
                    setIds(
                      e.target.checked
                        ? [...ids, p.id]
                        : ids.filter((id) => id !== p.id),
                    )
                  }
                />
                {p.name}
              </label>
            ))}
          </fieldset>
          <label className="a-check">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
            />
            启用优惠券
          </label>
        </>
      }
      submit={async () => {
        const template = {
          code,
          type,
          value: type === "fixed" ? toCents(value) : Number(value),
          min_amount_cents: toCents(min),
          start_at: start ? new Date(start).toISOString() : null,
          end_at: end ? new Date(end).toISOString() : null,
          total,
          enabled,
          product_ids: ids,
        };
        if (!row && count > 1) {
          const coupons = await adminApi<Coupon[]>("/coupons/batch", {
            method: "POST",
            body: JSON.stringify({ template, count, prefix }),
          });
          exportCSV("Wybo-批量优惠码.csv", [
            ["优惠码"],
            ...coupons.map((c) => [c.code]),
          ]);
        } else {
          await adminApi(row ? `/coupons/${row.id}` : "/coupons", {
            method: row ? "PUT" : "POST",
            body: JSON.stringify(template),
          });
        }
        onSaved();
      }}
    />
  );
}

type Settlement = {
  id: number;
  agent_id: number;
  amount_cents: number;
  order_count: number;
  status: string;
  reference: string;
  paid_at: string | null;
  created_at: string;
};
export function Settlements({ notify }: Props) {
  const { can } = useAccess();
  const [cancelTarget, setCancelTarget] = useState<Settlement | null>(null);
  const q = useAdminQuery<Settlement[]>("/settlements"),
    a = useAdminQuery<AgentRow[]>("/agents");
  const [target, setTarget] = useState<Settlement | null>(null),
    [reference, setReference] = useState(""),
    [confirm, setConfirm] = useState(false),
    [error, setError] = useState(""),
    [busy, setBusy] = useState(0),
    [passwordAgent, setPasswordAgent] = useState<AgentRow | null>(null),
    [password, setPassword] = useState("");
  async function create(id: number) {
    setBusy(id);
    setError("");
    try {
      await adminApi(`/agents/${id}/settlements`, { method: "POST" });
      notify("结算单已创建，佣金已锁定");
      q.reload();
      a.reload();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(0);
    }
  }
  return (
    <>
      <PageHeading
        title="代理结算"
        description="按等待期结算佣金，记录每一次伙伴收益。"
      >
        <a className="a-button" href="/agent" target="_blank" rel="noreferrer">
          打开代理工作台
        </a>
      </PageHeading>
      {error && <Banner>{error}</Banner>}
      <div className="a-inline-note">
        <Receipt size={20} />
        <p>
          仅已发货且超过结算等待期的订单可生成结算单。同一佣金只结算一次。请在外部实际转账后，登记真实打款流水。
        </p>
      </div>
      <section className="a-panel">
        <div className="a-panel-heading">
          <h2>伙伴佣金账户</h2>
        </div>
        <QueryPanel {...a}>
          {a.data?.length ? (
            <div className="a-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>代理</th>
                    <th>佣金 / 等待期</th>
                    <th>可结算</th>
                    <th>等待期内</th>
                    <th>待打款 / 已打款</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {a.data.map((r) => (
                    <tr key={r.agent.id}>
                      <td>
                        <strong>{r.agent.name}</strong>
                        <small>
                          {r.portal_enabled
                            ? "代理工作台已开启"
                            : "尚未设置代理密码"}
                        </small>
                      </td>
                      <td>
                        {r.agent.commission_bps / 100}%
                        <small>{r.agent.settlement_days} 天</small>
                      </td>
                      <td>{yuan(r.balance?.available_cents || 0)}</td>
                      <td>{yuan(r.balance?.pending_cents || 0)}</td>
                      <td>
                        {yuan(r.balance?.reserved_cents || 0)}
                        <small>已付 {yuan(r.balance?.paid_cents || 0)}</small>
                      </td>
                      <td>
                        <button
                          className="a-table-button"
                          disabled={!!busy || !r.balance?.available_cents}
                          onClick={() => create(r.agent.id)}
                        >
                          {busy === r.agent.id ? "生成中…" : "生成结算单"}
                        </button>
                        {can("agents.write") && (
                          <button
                            className="a-table-button"
                            onClick={() => {
                              setPasswordAgent(r);
                              setPassword("");
                            }}
                          >
                            设置代理密码
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title="还没有合作代理"
              text="先在分销代理中添加伙伴、配置佣金比例。"
            />
          )}
        </QueryPanel>
      </section>
      <section className="a-panel a-section-gap">
        <div className="a-panel-heading">
          <h2>结算与打款记录</h2>
        </div>
        <QueryPanel {...q}>
          {q.data?.length ? (
            <div className="a-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>结算单 / 代理</th>
                    <th>佣金金额</th>
                    <th>订单数</th>
                    <th>状态</th>
                    <th>打款凭证</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.map((s) => (
                    <tr key={s.id}>
                      <td>
                        #{s.id} ·{" "}
                        {a.data?.find((r) => r.agent.id === s.agent_id)?.agent
                          .name || `代理 #${s.agent_id}`}
                        <small>{shortDate(s.created_at)}</small>
                      </td>
                      <td>{yuan(s.amount_cents)}</td>
                      <td>{s.order_count} 笔</td>
                      <td>
                        <Status
                          value={s.status}
                          label={
                            s.status === "paid"
                              ? "已打款"
                              : s.status === "canceled"
                                ? "已取消"
                                : "待打款"
                          }
                        />
                      </td>
                      <td>{s.reference || "尚未打款"}</td>
                      <td>
                        {s.status === "pending" && (
                          <>
                            <button
                              className="a-table-button"
                              onClick={() => {
                                setTarget(s);
                                setReference("");
                                setConfirm(false);
                              }}
                            >
                              登记打款
                            </button>
                            <button
                              className="a-table-button"
                              onClick={() => setCancelTarget(s)}
                            >
                              取消结算
                            </button>
                          </>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty title="暂无结算记录" />
          )}
        </QueryPanel>
      </section>
      {cancelTarget && (
        <ActionDialog
          title="取消待打款结算"
          description="仅在尚未向代理打款时取消；取消后订单佣金会释放，可在处理售后后重新结算。"
          onClose={() => setCancelTarget(null)}
          fields={
            <p>
              结算单 #{cancelTarget.id} · {yuan(cancelTarget.amount_cents)}
            </p>
          }
          submit={async () => {
            await adminApi(`/settlements/${cancelTarget.id}/cancel`, {
              method: "POST",
            });
            q.reload();
            a.reload();
            notify("待打款结算已取消");
          }}
        />
      )}
      {target && (
        <ActionDialog
          title="登记已打款"
          description={`结算单 #${target.id}，金额 ${yuan(target.amount_cents)}。此操作登记支付结果，不会从网站自动转账。`}
          onClose={() => setTarget(null)}
          fields={
            <>
              <Field label="实际转账流水 / 凭证">
                <input
                  required
                  minLength={4}
                  maxLength={200}
                  value={reference}
                  onChange={(e) => setReference(e.target.value)}
                />
              </Field>
              <label className="a-check">
                <input
                  type="checkbox"
                  required
                  checked={confirm}
                  onChange={(e) => setConfirm(e.target.checked)}
                />
                我已完成转账，并核对收款方与金额
              </label>
            </>
          }
          submit={async () => {
            await adminApi(`/settlements/${target.id}/paid`, {
              method: "POST",
              body: JSON.stringify({ reference, confirm }),
            });
            q.reload();
            a.reload();
            notify("打款凭证已登记");
          }}
        />
      )}
      {passwordAgent && (
        <ActionDialog
          title="设置代理工作台密码"
          description={`代理 ${passwordAgent.agent.name} 使用代理码 ${passwordAgent.agent.code} 登录 /agent。改密会使旧登录失效。`}
          onClose={() => setPasswordAgent(null)}
          fields={
            <Field
              label="新的代理密码"
              hint="10–72 字节，请通过可信方式交给代理"
            >
              <input
                type="password"
                required
                minLength={10}
                maxLength={72}
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
          }
          submit={async () => {
            await adminApi(`/agents/${passwordAgent.agent.id}/password`, {
              method: "PUT",
              body: JSON.stringify({ password }),
            });
            a.reload();
            notify("代理密码已更新");
          }}
        />
      )}
    </>
  );
}

type StockAlert = {
  id: number;
  product_name: string;
  product_id: number;
  stock: number;
  threshold: number;
  status: string;
  notified: boolean;
  last_error: string;
};
type Batch = {
  id: number;
  batch_no: string;
  product_id: number;
  quantity: number;
  unit_cost_cents: number | null;
  note: string;
  created_at: string;
};
export function StockOperations({ notify }: Props) {
  const q = useAdminQuery<StockAlert[]>("/stock-alerts"),
    b = useAdminQuery<Batch[]>("/batches"),
    p = useAdminQuery<{ list: AdminProduct[] }>("/products?size=100");
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <>
      <PageHeading
        title="库存预警与批次"
        description="成本随批次记录，补货后自动照顾等待中的订单。"
      >
        <button
          className="a-button"
          disabled={busy}
          onClick={async () => {
            setBusy(true);
            setError("");
            try {
              const r = await adminApi<{ delivered: number }>("/auto-deliver", {
                method: "POST",
              });
              notify(`本次完成 ${r.delivered} 笔补发`);
              q.reload();
            } catch (e) {
              setError((e as Error).message);
            } finally {
              setBusy(false);
            }
          }}
        >
          <PackageCheck size={16} />
          {busy ? "检查中…" : "检查待补发订单"}
        </button>
      </PageHeading>
      {error && <Banner>{error}</Banner>}
      <section className="a-panel">
        <div className="a-panel-heading">
          <h2>库存预警</h2>
          <p>每分钟检查；补货恢复后自动解除，下次不足时重新提醒。</p>
        </div>
        <QueryPanel {...q}>
          {q.data?.length ? (
            <div className="a-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>商品</th>
                    <th>可售库存 / 预警线</th>
                    <th>状态</th>
                    <th>邮件提醒</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.map((a) => (
                    <tr key={a.id}>
                      <td>{a.product_name}</td>
                      <td>
                        {a.stock} / {a.threshold}
                      </td>
                      <td>
                        <Status
                          value={a.status}
                          label={a.status === "open" ? "库存偏低" : "已恢复"}
                        />
                      </td>
                      <td className="a-cell-wrap">
                        {a.notified
                          ? "已通知"
                          : a.last_error || "未发送（请在运维设置启用）"}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty title="库存状态良好" />
          )}
        </QueryPanel>
      </section>
      <section className="a-panel a-section-gap">
        <div className="a-panel-heading">
          <h2>采购批次</h2>
          <p>
            在「卡密库存 →
            批量导入」中填写批次单张成本。历史未记录成本的批次不会凭空估算。
          </p>
        </div>
        <QueryPanel {...b}>
          {b.data?.length ? (
            <div className="a-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>批次 / 时间</th>
                    <th>商品</th>
                    <th>数量</th>
                    <th>单张成本</th>
                    <th>备注</th>
                  </tr>
                </thead>
                <tbody>
                  {b.data.map((x) => (
                    <tr key={x.id}>
                      <td>
                        {x.batch_no}
                        <small>{shortDate(x.created_at)}</small>
                      </td>
                      <td>
                        {p.data?.list.find((p) => p.id === x.product_id)
                          ?.name || `商品 #${x.product_id}`}
                      </td>
                      <td>{x.quantity}</td>
                      <td>
                        {x.unit_cost_cents == null
                          ? "未记录"
                          : yuan(x.unit_cost_cents)}
                      </td>
                      <td className="a-cell-wrap">{x.note || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title="还没有采购批次"
              text="下一次导入卡密时可同时记录成本。"
            />
          )}
        </QueryPanel>
      </section>
    </>
  );
}

type Backup = {
  id: number;
  filename: string;
  sha256: string;
  size: number;
  kind: string;
  created_at: string;
};
type Audit = {
  id: number;
  actor_id: number;
  action: string;
  target: string;
  result: number;
  created_at: string;
};
export function Maintenance({ notify }: Props) {
  const [tab, setTab] = useState("backup"),
    [page, setPage] = useState(1),
    [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [restore, setRestore] = useState<{
      command: string;
      instruction: string;
      sha256: string;
    } | null>(null);
  const b = useAdminQuery<Backup[]>("/backups"),
    a = useAdminQuery<{ list: Audit[]; total: number }>(
      `/audit-logs?page=${page}`,
    ),
    s = useAdminQuery<Record<string, string>>("/settings");
  const [hours, setHours] = useState<string | null>(null),
    [threshold, setThreshold] = useState<string | null>(null),
    [email, setEmail] = useState<boolean | null>(null),
    [reconcile, setReconcile] = useState<boolean | null>(null);
  async function run(fn: () => Promise<void>) {
    setBusy(true);
    setError("");
    try {
      await fn();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function download(id: number) {
    const res = await adminFetch(`/backups/${id}/download`);
    if (!res.ok || res.headers.get("Content-Type")?.includes("json"))
      throw new Error("备份下载失败，请重新登录或核查备份");
    const blob = await res.blob(),
      url = URL.createObjectURL(blob),
      link = document.createElement("a");
    link.href = url;
    link.download = b.data?.find((x) => x.id === id)?.filename || "wybo.db";
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  }
  return (
    <>
      <PageHeading
        title="日志与运维"
        description="记录关键操作，给小铺的数据留一份安心。"
      />
      <div className="a-tabs" role="group" aria-label="运维功能">
        {[
          ["backup", "数据备份"],
          ["audit", "操作日志"],
          ["settings", "自动任务设置"],
        ].map(([k, v]) => (
          <button
            key={k}
            aria-pressed={tab === k}
            className={tab === k ? "active" : ""}
            onClick={() => setTab(k)}
          >
            {v}
          </button>
        ))}
      </div>
      {error && <Banner>{error}</Banner>}
      {tab === "backup" && (
        <>
          <div className="a-inline-note">
            <Database size={19} />
            <p>
              备份包含卡密与加密配置，请妥善保管。在线备份使用 SQLite
              一致性快照；恢复前必须停服，工具会自动保留恢复前快照。
            </p>
          </div>
          <section className="a-panel">
            <div className="a-panel-heading">
              <h2>数据库快照</h2>
              <button
                className="a-button primary"
                disabled={busy}
                onClick={() =>
                  run(async () => {
                    await adminApi("/backups", {
                      method: "POST",
                      signal: AbortSignal.timeout(60000),
                    });
                    b.reload();
                    notify("一致性备份已创建");
                  })
                }
              >
                <Plus size={15} />
                {busy ? "处理中…" : "立即备份"}
              </button>
            </div>
            <QueryPanel {...b}>
              {b.data?.length ? (
                <div className="a-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>备份 / 时间</th>
                        <th>大小</th>
                        <th>来源</th>
                        <th>操作</th>
                      </tr>
                    </thead>
                    <tbody>
                      {b.data.map((x) => (
                        <tr key={x.id}>
                          <td className="a-cell-wrap">
                            <strong>{x.filename}</strong>
                            <small>{shortDate(x.created_at)}</small>
                          </td>
                          <td>{(x.size / 1024 / 1024).toFixed(2)} MB</td>
                          <td>
                            {x.kind === "scheduled" ? "自动备份" : "手动备份"}
                          </td>
                          <td>
                            <button
                              className="a-table-button"
                              disabled={busy}
                              onClick={() => run(() => download(x.id))}
                            >
                              下载
                            </button>
                            <button
                              className="a-table-button"
                              disabled={busy}
                              onClick={() =>
                                run(async () =>
                                  setRestore(
                                    await adminApi(
                                      `/backups/${x.id}/prepare-restore`,
                                      { method: "POST" },
                                    ),
                                  ),
                                )
                              }
                            >
                              恢复指引
                            </button>
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <Empty
                  title="创建第一份备份"
                  text="也可以在自动任务设置中启用定时备份。"
                />
              )}
            </QueryPanel>
          </section>
        </>
      )}
      {tab === "audit" && (
        <section className="a-panel">
          <div className="a-panel-heading">
            <h2>操作日志</h2>
            <p>记录操作人、接口、对象与结果，不记录密码、卡密或请求正文。</p>
          </div>
          <QueryPanel {...a}>
            {a.data?.list.length ? (
              <div className="a-table-wrap">
                <table>
                  <thead>
                    <tr>
                      <th>时间</th>
                      <th>操作人</th>
                      <th>操作</th>
                      <th>对象</th>
                      <th>HTTP 结果</th>
                    </tr>
                  </thead>
                  <tbody>
                    {a.data.list.map((x) => (
                      <tr key={x.id}>
                        <td>{shortDate(x.created_at)}</td>
                        <td>
                          {x.actor_id ? `管理员 #${x.actor_id}` : "系统任务"}
                        </td>
                        <td>
                          <code>{x.action}</code>
                        </td>
                        <td className="a-cell-wrap">{x.target || "—"}</td>
                        <td>{x.result}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            ) : (
              <Empty title="还没有操作日志" />
            )}
          </QueryPanel>
          <div className="a-table-footer">
            <span>共 {a.data?.total || 0} 条</span>
            <div className="a-pagination">
              <button
                className="a-button"
                disabled={page <= 1}
                onClick={() => setPage(page - 1)}
              >
                上一页
              </button>
              <button
                className="a-button"
                disabled={page * 30 >= (a.data?.total || 0)}
                onClick={() => setPage(page + 1)}
              >
                下一页
              </button>
            </div>
          </div>
        </section>
      )}
      {tab === "settings" && (
        <section className="a-panel a-ops-form">
          <QueryPanel {...s}>
            {s.data && (
              <form
                onSubmit={(e) => {
                  e.preventDefault();
                  run(async () => {
                    await adminApi("/settings", {
                      method: "PUT",
                      body: JSON.stringify({
                        backup_interval_hours:
                          hours ?? s.data!.backup_interval_hours ?? "0",
                        stock_threshold:
                          threshold ?? s.data!.stock_threshold ?? "5",
                        stock_alert_email: String(
                          email ?? s.data!.stock_alert_email === "true",
                        ),
                        reconcile_enabled: String(
                          reconcile ?? s.data!.reconcile_enabled === "true",
                        ),
                      }),
                    });
                    s.reload();
                    notify("自动任务设置已保存");
                  });
                }}
              >
                <h2>自动任务设置</h2>
                <div className="a-form-grid">
                  <Field
                    label="自动备份间隔（小时）"
                    hint="0 关闭；1–720 小时。服务运行时生效，备份保存在服务器。"
                  >
                    <input
                      required
                      type="number"
                      min="0"
                      max="720"
                      value={hours ?? s.data.backup_interval_hours ?? "0"}
                      onChange={(e) => setHours(e.target.value)}
                    />
                  </Field>
                  <Field label="库存预警线（张）">
                    <input
                      required
                      type="number"
                      min="0"
                      max="10000"
                      value={threshold ?? s.data.stock_threshold ?? "5"}
                      onChange={(e) => setThreshold(e.target.value)}
                    />
                  </Field>
                </div>
                <label className="a-check">
                  <input
                    type="checkbox"
                    checked={email ?? s.data.stock_alert_email === "true"}
                    onChange={(e) => setEmail(e.target.checked)}
                  />
                  低库存邮件提醒（需配置 Resend 与商家接收邮箱）
                </label>
                <label className="a-check">
                  <input
                    type="checkbox"
                    checked={reconcile ?? s.data.reconcile_enabled === "true"}
                    onChange={(e) => setReconcile(e.target.checked)}
                  />
                  自动核对近 72 小时的支付申请（每 5 分钟重查）
                </label>
                <div className="a-form-actions">
                  <button className="a-button primary" disabled={busy}>
                    {busy ? "保存中…" : "保存自动任务"}
                  </button>
                </div>
              </form>
            )}
          </QueryPanel>
        </section>
      )}
      {restore && (
        <Modal title="数据库恢复指引" onClose={() => setRestore(null)}>
          <div className="a-editor">
            <h2>恢复数据库</h2>
            <p>{restore.instruction}</p>
            <h3>在 backend 目录运行</h3>
            <pre className="a-secret">{restore.command}</pre>
            <h3>校验值 SHA256</h3>
            <code className="a-break">{restore.sha256}</code>
            <Banner>
              请同时保留当前
              JWT_SECRET。恢复会回退订单、卡密和后台设置，请在维护期间操作。
            </Banner>
            <button className="a-button" onClick={() => setRestore(null)}>
              知道了
            </button>
          </div>
        </Modal>
      )}
    </>
  );
}

type Ticket = {
  id: number;
  number: string;
  order_no: string;
  subject: string;
  status: string;
  created_at: string;
  updated_at: string;
};
type Message = {
  id: number;
  author: string;
  content: string;
  internal: boolean;
  created_at: string;
};
export function Tickets({ notify }: Props) {
  const q = useAdminQuery<Ticket[]>("/tickets");
  const [number, setNumber] = useState("");
  return (
    <>
      <PageHeading
        title="售后工单"
        description="把每一次求助接住，让服务有始有终。"
      >
        <button className="a-button" onClick={q.reload}>
          <RefreshCw size={15} />
          刷新工单
        </button>
      </PageHeading>
      <section className="a-panel">
        <QueryPanel {...q}>
          {q.data?.length ? (
            <div className="a-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>工单 / 主题</th>
                    <th>关联订单</th>
                    <th>状态</th>
                    <th>最近更新</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.map((t) => (
                    <tr key={t.id}>
                      <td>
                        <strong>{t.subject}</strong>
                        <small>{t.number}</small>
                      </td>
                      <td>{t.order_no}</td>
                      <td>
                        <Status value={t.status} />
                      </td>
                      <td>{shortDate(t.updated_at)}</td>
                      <td>
                        <button
                          className="a-table-button"
                          onClick={() => setNumber(t.number)}
                        >
                          <MessageCircle size={14} />
                          处理
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title="暂时没有售后工单"
              text="顾客凭订单号和查询凭证，在「我的订单」中提交问题。"
            />
          )}
        </QueryPanel>
      </section>
      {number && (
        <TicketDialog
          number={number}
          onClose={() => setNumber("")}
          onSaved={() => {
            q.reload();
            notify("工单回复已保存");
          }}
        />
      )}
    </>
  );
}
function TicketDialog({
  number,
  onClose,
  onSaved,
}: {
  number: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const q = useAdminQuery<{ ticket: Ticket; messages: Message[] }>(
      `/tickets/${number}`,
    ),
    [content, setContent] = useState(""),
    [status, setStatus] = useState("waiting"),
    [internal, setInternal] = useState(false),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  return (
    <Modal
      title="处理售后工单"
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <div className="a-editor">
        <h2>{q.data?.ticket.subject || "售后工单"}</h2>
        <p>
          {number} · 订单 {q.data?.ticket.order_no}
        </p>
        <QueryPanel {...q}>
          {q.data && (
            <>
              <div className="a-ticket-thread">
                {q.data.messages.map((m) => (
                  <article
                    className={`a-ticket-message ${m.internal ? "is-internal" : ""}`}
                    key={m.id}
                  >
                    <div>
                      <strong>
                        {m.author === "customer" ? "顾客" : "商家"}
                        {m.internal ? " · 内部备注" : ""}
                      </strong>
                      <small>{shortDate(m.created_at)}</small>
                    </div>
                    <p>{m.content}</p>
                  </article>
                ))}
              </div>
              <form
                onSubmit={async (e) => {
                  e.preventDefault();
                  setBusy(true);
                  setError("");
                  try {
                    await adminApi(`/tickets/${number}/replies`, {
                      method: "POST",
                      body: JSON.stringify({ content, status, internal }),
                    });
                    setContent("");
                    q.reload();
                    onSaved();
                  } catch (e) {
                    setError((e as Error).message);
                  } finally {
                    setBusy(false);
                  }
                }}
              >
                {error && <Banner>{error}</Banner>}
                <Field label={internal ? "内部备注（顾客不可见）" : "回复顾客"}>
                  <textarea
                    required
                    minLength={2}
                    maxLength={4000}
                    rows={4}
                    value={content}
                    onChange={(e) => setContent(e.target.value)}
                  />
                </Field>
                <div className="a-form-grid">
                  <Field label="回复后状态">
                    <select
                      value={status}
                      onChange={(e) => setStatus(e.target.value)}
                    >
                      <option value="waiting">等待顾客回复</option>
                      <option value="open">处理中</option>
                      <option value="resolved">已解决</option>
                    </select>
                  </Field>
                  <label className="a-check">
                    <input
                      type="checkbox"
                      checked={internal}
                      onChange={(e) => setInternal(e.target.checked)}
                    />
                    仅内部可见
                  </label>
                </div>
                <div className="a-form-actions">
                  <button className="a-button primary" disabled={busy}>
                    {busy ? "保存中…" : "保存回复"}
                  </button>
                </div>
              </form>
            </>
          )}
        </QueryPanel>
      </div>
    </Modal>
  );
}
