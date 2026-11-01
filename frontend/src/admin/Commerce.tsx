import { useState, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Copy,
  Eye,
  EyeOff,
  PackageCheck,
  Pencil,
  Plus,
  Upload,
  Users,
} from "lucide-react";
import { Modal } from "../components";
import {
  adminApi,
  orderNet,
  useAdminQuery,
  yuan,
  toCents,
  type AdminOrder,
  type AdminProduct,
  type Card,
  type AgentRow,
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
import { useAccess } from "./access";
import { OrderFinance } from "./Operations";
export function Pagination({
  page,
  total,
  onChange,
}: {
  page: number;
  total: number;
  onChange: (p: number) => void;
}) {
  return (
    <div className="a-table-footer">
      <span>
        共 {total} 条 · 第 {page} / {Math.max(1, Math.ceil(total / 20))} 页
      </span>
      <div className="a-pagination">
        <button
          className="a-button"
          disabled={page <= 1}
          onClick={() => onChange(page - 1)}
        >
          上一页
        </button>
        <button
          className="a-button"
          disabled={page * 20 >= total}
          onClick={() => onChange(page + 1)}
        >
          下一页
        </button>
      </div>
    </div>
  );
}
export function Orders({ notify }: { notify: (m: string) => void }) {
  const { can } = useAccess();
  const [params, setParams] = useSearchParams();
  const status = params.get("status") || "";
  const [page, setPage] = useState(1);
  const [detail, setDetail] = useState<string | null>(null);
  const q = useAdminQuery<{ list: AdminOrder[]; total: number }>(
    `/orders?status=${status}&page=${page}&size=20`,
  );
  return (
    <>
      <PageHeading
        title="订单管理"
        description="从一份期待到一笔成交，每一步都有迹可循。"
      />
      <section className="a-panel">
        <div className="a-list-toolbar">
          <h2>
            全部订单 <span className="a-count">{q.data?.total || 0}</span>
          </h2>
          <select
            aria-label="订单状态"
            value={status}
            onChange={(e) => {
              setPage(1);
              setParams(e.target.value ? { status: e.target.value } : {});
            }}
          >
            <option value="">所有状态</option>
            {Object.entries({
              pending: "待付款",
              paid: "待发货",
              completed: "已完成",
              closed: "已关闭",
              refunded: "已退款",
              refunding: "退款处理中",
            }).map(([k, v]) => (
              <option value={k} key={k}>
                {v}
              </option>
            ))}
          </select>
        </div>
        {q.loading ? (
          <Busy />
        ) : q.error ? (
          <QueryError error={q.error} reload={q.reload} />
        ) : q.data?.list.length ? (
          <div className="a-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>订单 / 时间</th>
                  <th>联系方式</th>
                  <th>{can("finance") ? "订单金额 / 成本" : "订单金额"}</th>
                  {can("finance") && <th>经营净利润</th>}
                  <th>归属</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {q.data.list.map((o) => (
                  <tr key={o.id}>
                    <td>
                      <strong>{o.order_no}</strong>
                      <small>{shortDate(o.created_at)}</small>
                    </td>
                    <td>{o.contact}</td>
                    <td>
                      <strong>{yuan(o.total_cents)}</strong>
                      {can("finance") && (
                        <small>
                          成本{" "}
                          {o.cost_cents == null ? "未记录" : yuan(o.cost_cents)}
                        </small>
                      )}
                    </td>
                    {can("finance") && (
                      <td className="a-profit">
                        {orderNet(o) == null
                          ? o.paid_at
                            ? "待补全"
                            : "—"
                          : yuan(orderNet(o)!)}
                      </td>
                    )}
                    <td>{o.agent_name || "直营"}</td>
                    <td>
                      <Status value={o.status} />
                    </td>
                    <td>
                      <button
                        className="a-table-button"
                        onClick={() => setDetail(o.order_no)}
                      >
                        详情
                      </button>
                      {o.status === "paid" && can("inventory") && (
                        <button
                          className="a-table-button"
                          onClick={() => setDetail(o.order_no)}
                        >
                          <PackageCheck size={14} />
                          发货处理
                        </button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty title="还没有这类订单" text="新订单会自动出现在这里。" />
        )}
        <Pagination total={q.data?.total || 0} page={page} onChange={setPage} />
      </section>
      {can("finance") && (
        <p className="a-hint">
          净利润 = 实收 − 退款 − 成本 − 手续费 −
          佣金。成本和手续费资料缺失时暂不计算。
        </p>
      )}
      {detail && (
        <OrderDetail
          orderNo={detail}
          notify={notify}
          onClose={() => {
            setDetail(null);
            q.reload();
          }}
        />
      )}
    </>
  );
}
function OrderDetail({
  orderNo,
  onClose,
  notify,
}: {
  orderNo: string;
  onClose: () => void;
  notify: (message: string) => void;
}) {
  const q = useAdminQuery<{
    order: AdminOrder;
    items: {
      product_name: string;
      quantity: number;
      unit_price_cents: number;
      delivery_mode?: "auto" | "manual";
    }[];
    cards: Card[];
  }>(`/orders/${orderNo}`);
  const { can } = useAccess();
  const [secrets, setSecrets] = useState<Record<number, Card>>({});
  const [revealing, setRevealing] = useState(false),
    [revealError, setRevealError] = useState("");
  const show = Object.keys(secrets).length > 0;
  const manual = q.data?.items.some((item) => item.delivery_mode === "manual");
  const quantity =
    q.data?.items.reduce((sum, item) => sum + item.quantity, 0) || 0;
  const [delivery, setDelivery] = useState("");
  const [sending, setSending] = useState(false);
  const [deliveryError, setDeliveryError] = useState("");
  const [confirmDelivery, setConfirmDelivery] = useState(false);
  async function fulfill(event: FormEvent) {
    event.preventDefault();
    if (sending) return;
    setDeliveryError("");
    const lines = delivery
      .split(/\r?\n/)
      .map((line) => line.trim())
      .filter(Boolean)
      .map((line) => {
        const split = line.indexOf("----");
        return split < 0
          ? { code: line, secret: "" }
          : {
              code: line.slice(0, split).trim(),
              secret: line.slice(split + 4).trim(),
            };
      });
    if (manual && lines.length !== quantity) {
      setDeliveryError(`此订单需要 ${quantity} 份发货内容，请每份填写一行。`);
      return;
    }
    setSending(true);
    try {
      await adminApi(`/orders/${orderNo}/${manual ? "fulfill" : "deliver"}`, {
        method: "POST",
        ...(manual ? { body: JSON.stringify({ lines }) } : {}),
      });
      setDelivery("");
      setConfirmDelivery(false);
      q.reload();
      notify("发货完成，已加入发货通知队列");
    } catch (e) {
      setDeliveryError((e as Error).message);
    } finally {
      setSending(false);
    }
  }
  async function reveal() {
    if (show) {
      setSecrets({});
      return;
    }
    setRevealing(true);
    setRevealError("");
    try {
      const found: Record<number, Card> = {};
      for (const c of q.data?.cards || []) {
        found[c.id] = await adminApi<Card>(`/cards/${c.id}/reveal`, {
          method: "POST",
        });
      }
      setSecrets(found);
    } catch (e) {
      setRevealError((e as Error).message);
    } finally {
      setRevealing(false);
    }
  }
  return (
    <Modal
      title="订单详情"
      onClose={() => {
        if (!sending) onClose();
      }}
    >
      <div className="a-editor">
        <span className="a-eyebrow">ORDER DETAIL</span>
        <h2>订单详情</h2>
        <p>{orderNo}</p>
        {q.loading ? (
          <Busy />
        ) : q.error ? (
          <QueryError error={q.error} reload={q.reload} />
        ) : (
          q.data && (
            <>
              <div className="a-detail-grid">
                <span>
                  订单状态
                  <Status value={q.data.order.status} />
                </span>
                <span>
                  订单金额<strong>{yuan(q.data.order.total_cents)}</strong>
                </span>
                {can("finance") && (
                  <span>
                    采购成本
                    <strong>
                      {q.data.order.cost_cents == null
                        ? "未记录"
                        : yuan(q.data.order.cost_cents)}
                    </strong>
                  </span>
                )}
                <span>
                  支付渠道<strong>{q.data.order.pay_method || "待支付"}</strong>
                </span>
                <span>
                  联系方式<strong>{q.data.order.contact}</strong>
                </span>
                <span>
                  下单时间<strong>{shortDate(q.data.order.created_at)}</strong>
                </span>
              </div>
              {can("finance") && (
                <OrderFinance order={q.data.order} reload={q.reload} />
              )}
              <h3>商品明细</h3>
              {q.data.items.map((x, i) => (
                <div className="a-detail-line" key={i}>
                  <strong>{x.product_name}</strong>
                  <span>
                    ×{x.quantity} · {yuan(x.unit_price_cents)} ·{" "}
                    {x.delivery_mode === "manual" ? "手工发货" : "自动发货"}
                  </span>
                </div>
              ))}
              {q.data.order.status === "paid" && can("inventory") && (
                <form className="a-manual-delivery" onSubmit={fulfill}>
                  <h3>{manual ? "手工发货" : "库存补发"}</h3>
                  {manual ? (
                    <Field
                      label="发货内容"
                      hint={`共 ${quantity} 份，每份一行；可填写卡密、账号或处理结果，账号密码使用“账号----密码”。每份内容和密码各不超过 255 字节。`}
                    >
                      <textarea
                        required
                        rows={4}
                        maxLength={51000}
                        value={delivery}
                        disabled={sending}
                        autoComplete="off"
                        spellCheck={false}
                        onChange={(e) => {
                          setDelivery(e.target.value);
                          setConfirmDelivery(false);
                        }}
                        placeholder="每份商品的发货内容，填写后顾客可在订单中查看"
                      />
                    </Field>
                  ) : (
                    <p className="a-hint">
                      从可售库存重新分配卡密，库存不足时请先补货。
                    </p>
                  )}
                  {deliveryError && <Banner>{deliveryError}</Banner>}
                  <label className="a-check">
                    <input
                      type="checkbox"
                      required
                      checked={confirmDelivery}
                      disabled={sending}
                      onChange={(e) => setConfirmDelivery(e.target.checked)}
                    />
                    我已核对订单与发货内容，确认发送给顾客
                  </label>
                  <button
                    className="a-button primary"
                    disabled={sending || !confirmDelivery}
                  >
                    <PackageCheck size={16} />
                    {sending
                      ? "发货中…"
                      : manual
                        ? "确认手工发货"
                        : "确认库存补发"}
                  </button>
                </form>
              )}
              {q.data.cards.length > 0 && (
                <>
                  <div className="a-list-toolbar">
                    <h3>{manual ? "已发货内容" : "已发卡密"}</h3>
                    {can("cards.reveal") && (
                      <button
                        className="a-button"
                        disabled={revealing}
                        onClick={() => void reveal()}
                      >
                        {show ? <EyeOff size={15} /> : <Eye size={15} />}{" "}
                        {revealing ? "验证中…" : show ? "隐藏" : "查看"}
                      </button>
                    )}
                  </div>
                  {revealError && <Banner>{revealError}</Banner>}
                  {q.data.cards.map((c) => (
                    <pre className="a-secret" key={c.id}>
                      {show
                        ? secrets[c.id]?.code +
                          (secrets[c.id]?.secret
                            ? "\n" + secrets[c.id].secret
                            : "")
                        : "••••••••••••••••"}
                    </pre>
                  ))}
                </>
              )}
            </>
          )
        )}
      </div>
    </Modal>
  );
}
export function Inventory({ notify }: { notify: (m: string) => void }) {
  const [params, setParams] = useSearchParams();
  const product = params.get("product") || "";
  const [status, setStatus] = useState("");
  const [page, setPage] = useState(1);
  const [secrets, setSecrets] = useState<Record<number, Card>>({});
  const { can } = useAccess();
  const show = Object.keys(secrets).map(Number);
  async function reveal(c: Card) {
    if (secrets[c.id]) {
      setSecrets((v) => {
        const n = { ...v };
        delete n[c.id];
        return n;
      });
      return;
    }
    setBusy(c.id);
    setError("");
    try {
      const raw = await adminApi<Card>(`/cards/${c.id}/reveal`, {
        method: "POST",
      });
      setSecrets((v) => ({ ...v, [c.id]: raw }));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(0);
    }
  }
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(0);
  const p = useAdminQuery<{ list: AdminProduct[] }>("/products?size=100");
  const q = useAdminQuery<{ list: Card[]; total: number }>(
    `/cards?product_id=${product}&status=${status}&page=${page}&size=20`,
  );
  async function disable(id: number) {
    setBusy(id);
    setError("");
    try {
      await adminApi(`/cards/${id}/disable`, { method: "POST" });
      q.reload();
      notify("卡密已停用，库存已更新");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(0);
    }
  }
  return (
    <>
      <PageHeading
        title="卡密库存"
        description="把每一份待发货的美好，整理妥当。"
      >
        <button className="a-button primary" onClick={() => setImporting(true)}>
          <Upload size={16} />
          批量导入
        </button>
      </PageHeading>
      {error && <Banner>{error}</Banner>}
      <section className="a-panel">
        <div className="a-list-toolbar">
          <h2>
            库存记录 <span className="a-count">{q.data?.total || 0}</span>
          </h2>
          <div className="a-filters">
            <select
              aria-label="筛选商品"
              value={product}
              onChange={(e) => {
                setPage(1);
                setParams(e.target.value ? { product: e.target.value } : {});
              }}
            >
              <option value="">全部商品</option>
              {p.data?.list.map((x) => (
                <option key={x.id} value={x.id}>
                  {x.name}
                </option>
              ))}
            </select>
            <select
              aria-label="库存状态"
              value={status}
              onChange={(e) => {
                setStatus(e.target.value);
                setPage(1);
              }}
            >
              <option value="">所有状态</option>
              <option value="available">可售</option>
              <option value="reserved">已预占</option>
              <option value="replaced">已替换</option>
              <option value="refunded">售后退款</option>
              <option value="sold">已售</option>
              <option value="disabled">已停用</option>
            </select>
          </div>
        </div>
        {q.loading ? (
          <Busy />
        ) : q.error ? (
          <QueryError error={q.error} reload={q.reload} />
        ) : q.data?.list.length ? (
          <div className="a-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>商品</th>
                  <th>卡密内容</th>
                  <th>批次 / 导入时间</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {q.data.list.map((c) => (
                  <tr key={c.id}>
                    <td>
                      {p.data?.list.find((x) => x.id === c.product_id)?.name ||
                        `商品 #${c.product_id}`}
                    </td>
                    <td>
                      <code className="a-card-code">
                        {show.includes(c.id)
                          ? secrets[c.id]?.code +
                            (secrets[c.id]?.secret
                              ? " / " + secrets[c.id].secret
                              : "")
                          : "••••••••••••••"}
                      </code>
                    </td>
                    <td>
                      {c.batch_no}
                      <small>{shortDate(c.created_at)}</small>
                    </td>
                    <td>
                      <Status value={c.status} />
                    </td>
                    <td>
                      <button
                        className="a-icon-button"
                        aria-label={
                          show.includes(c.id) ? "隐藏卡密" : "显示卡密"
                        }
                        disabled={!!busy || !can("cards.reveal")}
                        onClick={() => void reveal(c)}
                      >
                        {show.includes(c.id) ? (
                          <EyeOff size={16} />
                        ) : (
                          <Eye size={16} />
                        )}
                      </button>
                      {c.status === "available" && (
                        <button
                          className="a-table-button"
                          disabled={busy === c.id}
                          onClick={() => disable(c.id)}
                        >
                          停用
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
            title="还没有库存记录"
            text="导入卡密后，商品可售库存会自动更新。"
          />
        )}
        <Pagination page={page} total={q.data?.total || 0} onChange={setPage} />
      </section>
      {importing && (
        <ImportCards
          products={p.data?.list || []}
          initialProduct={product}
          onClose={() => setImporting(false)}
          onSaved={(m) => {
            setImporting(false);
            q.reload();
            notify(m);
          }}
        />
      )}
    </>
  );
}
function ImportCards({
  products,
  initialProduct,
  onClose,
  onSaved,
}: {
  products: AdminProduct[];
  initialProduct: string;
  onClose: () => void;
  onSaved: (m: string) => void;
}) {
  const [product, setProduct] = useState(
    initialProduct || String(products[0]?.id || ""),
  );
  const [content, setContent] = useState("");
  const [unitCost, setUnitCost] = useState("");
  const [note, setNote] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const r = await adminApi<{
        imported: number;
        duplicated: number;
        auto_delivered: number;
      }>("/cards/import", {
        method: "POST",
        body: JSON.stringify({
          product_id: Number(product),
          content,
          unit_cost_cents: unitCost === "" ? null : toCents(unitCost),
          note,
        }),
      });
      onSaved(
        `已导入 ${r.imported} 张，跳过重复 ${r.duplicated} 张，自动补发 ${r.auto_delivered} 笔订单`,
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal
      title="批量导入卡密"
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <form className="a-editor" onSubmit={save}>
        <span className="a-eyebrow">RESTOCK A LITTLE HAPPINESS</span>
        <h2>为小铺补点货</h2>
        <p>同一商品的重复卡密会自动跳过。</p>
        {error && <Banner>{error}</Banner>}
        <Field label="所属商品">
          <select
            required
            value={product}
            onChange={(e) => setProduct(e.target.value)}
          >
            {products.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </Field>
        <div className="a-form-grid">
          <Field
            label="本批次单张成本（元）"
            hint="填写后按实际发出的卡密核算成本；留空沿用订单中的商品成本快照。"
          >
            <input
              type="number"
              min="0"
              step="0.01"
              value={unitCost}
              onChange={(e) => setUnitCost(e.target.value)}
              placeholder="例如 105.00"
            />
          </Field>
          <Field label="采购备注">
            <input
              maxLength={500}
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder="供货来源、采购备注等"
            />
          </Field>
        </div>
        <Field
          label="卡密内容"
          hint="每行一张；账号与密码用英文逗号分隔，例如：账号,密码"
        >
          <textarea
            required
            rows={10}
            maxLength={500000}
            value={content}
            onChange={(e) => setContent(e.target.value)}
            placeholder={"兑换码\n账号,密码"}
          />
        </Field>
        <div className="a-form-actions">
          <button
            type="button"
            className="a-button"
            disabled={busy}
            onClick={onClose}
          >
            取消
          </button>
          <button className="a-button primary" disabled={busy || !product}>
            <Upload size={16} />
            {busy ? "正在导入…" : "确认导入"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
export function Agents({ notify }: { notify: (m: string) => void }) {
  const { can } = useAccess();
  const q = useAdminQuery<AgentRow[]>("/agents");
  const p = useAdminQuery<{ list: AdminProduct[] }>("/products?size=100");
  const [edit, setEdit] = useState<AgentRow | null | undefined>();
  async function copy(code: string) {
    try {
      await navigator.clipboard.writeText(
        `${window.location.origin}/?agent=${encodeURIComponent(code)}`,
      );
      notify("代理专属链接已复制");
    } catch {
      notify("复制失败，请手动复制代理码");
    }
  }
  return (
    <>
      <PageHeading
        title="分销代理"
        description="和伙伴一起，把好东西带给更多人。"
      >
        {can("agents.write") && (
          <button className="a-button primary" onClick={() => setEdit(null)}>
            <Plus size={16} />
            新增代理
          </button>
        )}
      </PageHeading>
      <div className="a-inline-note">
        <Users size={18} />
        <p>
          支持专属售价、佣金比例与订单归属；在「代理结算」中设置工作台密码、结算佣金。
        </p>
      </div>
      <section className="a-panel">
        {q.loading ? (
          <Busy />
        ) : q.error ? (
          <QueryError error={q.error} reload={q.reload} />
        ) : q.data?.length ? (
          <div className="a-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>代理</th>
                  <th>代理码</th>
                  <th>专属定价</th>
                  <th>成交订单</th>
                  <th>成交金额</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {q.data.map((r) => (
                  <tr key={r.agent.id}>
                    <td>
                      <strong>{r.agent.name}</strong>
                      <small>{r.agent.contact || "未填写联系方式"}</small>
                    </td>
                    <td>
                      <code>{r.agent.code}</code>
                    </td>
                    <td>{r.prices?.length || 0} 件商品</td>
                    <td>{r.paid_orders} 笔</td>
                    <td>{yuan(r.revenue_cents)}</td>
                    <td>
                      <Status
                        value={r.agent.enabled ? "completed" : "disabled"}
                        label={r.agent.enabled ? "合作中" : "已停用"}
                      />
                    </td>
                    <td>
                      <button
                        className="a-table-button"
                        onClick={() => copy(r.agent.code)}
                      >
                        <Copy size={14} />
                        链接
                      </button>
                      {can("agents.write") && (
                        <button
                          className="a-table-button"
                          onClick={() => setEdit(r)}
                        >
                          <Pencil size={14} />
                          编辑
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
            title="找到小铺的同行者"
            text="添加代理，设置专属价格，自动记录带来的订单。"
          />
        )}
      </section>
      {edit !== undefined && (
        <AgentEditor
          row={edit}
          products={p.data?.list || []}
          onClose={() => setEdit(undefined)}
          onSaved={() => {
            setEdit(undefined);
            q.reload();
            notify("代理资料和专属价格已保存");
          }}
        />
      )}
    </>
  );
}
function AgentEditor({
  row,
  products,
  onClose,
  onSaved,
}: {
  row: AgentRow | null;
  products: AdminProduct[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(row?.agent.name || "");
  const [code, setCode] = useState(row?.agent.code || "");
  const [contact, setContact] = useState(row?.agent.contact || "");
  const [commission, setCommission] = useState(
    String((row?.agent.commission_bps || 0) / 100),
  );
  const [settlementDays, setSettlementDays] = useState(
    row?.agent.settlement_days ?? 7,
  );
  const [enabled, setEnabled] = useState(row?.agent.enabled ?? true);
  const [prices, setPrices] = useState<Record<number, string>>(
    Object.fromEntries(
      (row?.prices || []).map((p) => [
        p.product_id,
        (p.price_cents / 100).toFixed(2),
      ]),
    ),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await adminApi(row ? `/agents/${row.agent.id}` : "/agents", {
        method: row ? "PUT" : "POST",
        body: JSON.stringify({
          agent: {
            name,
            code,
            contact,
            enabled,
            commission_bps: toCents(commission),
            settlement_days: settlementDays,
          },
          prices: Object.entries(prices)
            .filter(([, v]) => v !== "")
            .map(([id, v]) => ({
              product_id: Number(id),
              price_cents: toCents(v),
            })),
        }),
      });
      onSaved();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <Modal title="代理资料" onClose={onClose}>
      <form className="a-editor" onSubmit={save}>
        <h2>{row ? "编辑代理" : "新增合作伙伴"}</h2>
        <p>专属价格由服务器核算，订单自动归属。</p>
        {error && <Banner>{error}</Banner>}
        <div className="a-form-grid">
          <Field label="代理名称">
            <input
              required
              maxLength={64}
              value={name}
              onChange={(e) => setName(e.target.value)}
            />
          </Field>
          <Field label="代理码" hint="3–32 位字母、数字、_ 或 -">
            <input
              required
              pattern="[A-Za-z0-9_-]{3,32}"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </Field>
        </div>
        <Field label="联系方式">
          <input
            maxLength={128}
            value={contact}
            onChange={(e) => setContact(e.target.value)}
          />
        </Field>
        <div className="a-form-grid">
          <Field
            label="代理佣金比例（%）"
            hint="按优惠后订单实收计算，保存到新订单。"
          >
            <input
              type="number"
              min="0"
              max="100"
              step="0.01"
              required
              value={commission}
              onChange={(e) => setCommission(e.target.value)}
            />
          </Field>
          <Field
            label="结算等待期（天）"
            hint="发货完成且支付超过此天数，才可结算。"
          >
            <input
              type="number"
              required
              min="0"
              max="365"
              value={settlementDays}
              onChange={(e) => setSettlementDays(Number(e.target.value))}
            />
          </Field>
        </div>
        <h3>商品专属价格（元）</h3>
        <p className="a-hint">留空沿用当前售价；待定价商品仍不可购买。</p>
        <div className="a-agent-prices">
          {products.map((p) => (
            <Field
              key={p.id}
              label={p.name}
              hint={`小铺售价 ${p.price_cents ? yuan(p.price_cents) : "待定价"}`}
            >
              <input
                type="number"
                min="0.01"
                step="0.01"
                placeholder="沿用小铺售价"
                value={prices[p.id] || ""}
                onChange={(e) =>
                  setPrices({ ...prices, [p.id]: e.target.value })
                }
              />
            </Field>
          ))}
        </div>
        <label className="a-check">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          启用代理链接
        </label>
        <div className="a-form-actions">
          <button className="a-button" type="button" onClick={onClose}>
            取消
          </button>
          <button className="a-button primary" disabled={busy}>
            {busy ? "保存中…" : "保存代理"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
