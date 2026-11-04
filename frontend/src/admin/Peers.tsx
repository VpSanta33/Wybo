import {
  useEffect,
  useId,
  useRef,
  useState,
  type FormEvent,
  type ReactNode,
} from "react";
import {
  ArrowDownUp,
  Check,
  Copy,
  KeyRound,
  Link2,
  Plus,
  RefreshCw,
  ShieldCheck,
  Wallet,
} from "lucide-react";
import { Modal } from "../components";
import {
  adminApi,
  toCents,
  useAdminQuery,
  yuan,
  type AdminProduct,
} from "./api";
import {
  Banner,
  Busy,
  Empty,
  Field,
  PageHeading,
  QueryError,
  shortDate,
} from "./ui";
import "./peers.css";

type Account = {
  id: number;
  name: string;
  api_key: string;
  callback_url: string;
  enabled: boolean;
  balance_cents: number;
};
type Connection = {
  id: number;
  name: string;
  base_url: string;
  api_key: string;
  enabled: boolean;
  site_id: string;
  balance_cents: number;
  last_error: string;
  last_sync_at: string | null;
};
type Grant = {
  id: number;
  account_id: number;
  product_id: number;
  price_cents: number;
  enabled: boolean;
};
type Catalog = {
  id: number;
  connection_id: number;
  remote_product_id: number;
  name: string;
  description: string;
  price_cents: number;
  stock: number;
  delivery_mode: string;
  route: string;
  enabled: boolean;
  updated_at: string;
};
type Mapping = {
  product_id: number;
  connection_id: number;
  remote_product_id: number;
  max_unit_cost_cents: number;
  enabled: boolean;
  sync_price: boolean;
  markup_bps: number;
  markup_cents: number;
};
type Job = {
  id: number;
  order_id: number;
  order_no: string;
  connection_id: number;
  status: string;
  attempts: number;
  last_error: string;
  remote_order_no: string;
  created_at: string;
  wallet_refundable: boolean;
};
type Ledger = {
  id: number;
  account_id: number;
  kind: string;
  amount_cents: number;
  balance_cents: number;
  reference: string;
  note: string;
  created_at: string;
};
type Event = {
  id: number;
  order_id: number;
  status: string;
  attempts: number;
  last_error: string;
};
type Data = {
  site_id: string;
  accounts: Account[];
  connections: Connection[];
  grants: Grant[];
  catalog: Catalog[];
  mappings: Mapping[];
  jobs: Job[];
  ledger: Ledger[];
  events: Event[];
};
type Credential = { api_key: string; api_secret: string };
type Props = { notify: (message: string) => void };
const tabs = [
  ["upstream", "上游连接"],
  ["mapping", "商品对接"],
  ["accounts", "下游账户"],
  ["orders", "订单任务"],
  ["ledger", "资金流水"],
] as const;
type Tab = (typeof tabs)[number][0];
const statusNames: Record<string, string> = {
  pending: "待处理",
  working: "正在采购",
  processing: "处理中",
  attention: "需人工核查",
  unknown: "结果待确认",
  completed: "已完成",
  failed: "明确失败",
  canceled: "已取消",
  cancelled: "已取消",
  sent: "已通知",
  sending: "通知中",
};
const moneyInput = (value: number | undefined) =>
  value == null ? "" : (value / 100).toFixed(2);
const stockText = (value: number) => (value < 0 ? "不限量" : `${value} 份`);
const write = <T,>(path: string, body: unknown = {}, method = "POST") =>
  adminApi<T>(path, { method, body: JSON.stringify(body) });
function State({
  value,
  notification = false,
}: {
  value: string;
  notification?: boolean;
}) {
  return (
    <span
      className={`p-state ${value === "completed" || value === "sent" ? "is-good" : ["failed", "unknown", "attention"].includes(value) ? "is-attention" : ""}`}
    >
      {notification
        ? {
            pending: "待通知",
            sending: "通知中",
            processing: "通知中",
            sent: "已通知",
            failed: "通知失败",
            canceled: "已取消",
            cancelled: "已取消",
          }[value] || value
        : statusNames[value] || value}
    </span>
  );
}
function ToggleState({ enabled }: { enabled: boolean }) {
  return (
    <span className={`p-state ${enabled ? "is-good" : ""}`}>
      {enabled ? "已启用" : "已停用"}
    </span>
  );
}
function useProducts(revision: number) {
  const [list, setList] = useState<AdminProduct[]>([]);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  useEffect(() => {
    const ctrl = new AbortController();
    setLoading(true);
    setError("");
    async function load() {
      const rows: AdminProduct[] = [];
      for (let page = 1; ; page++) {
        const result = await adminApi<{ list: AdminProduct[]; total: number }>(
          `/products?page=${page}&size=100`,
          { signal: ctrl.signal },
        );
        if (ctrl.signal.aborted) return;
        rows.push(...(result.list || []));
        if (
          !result.list?.length ||
          rows.length >= result.total ||
          result.list.length < 100
        )
          break;
      }
      setList(rows);
    }
    load()
      .catch((e: Error) => {
        if (!ctrl.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setLoading(false);
      });
    return () => ctrl.abort();
  }, [revision]);
  return { list, error, loading };
}

export default function Peers({ notify }: Props) {
  const q = useAdminQuery<Data>("/peers");
  const [revision, setRevision] = useState(0);
  const products = useProducts(revision);
  const [tab, setTab] = useState<Tab>("upstream");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [search, setSearch] = useState("");
  const [connection, setConnection] = useState<Connection | null | undefined>();
  const [account, setAccount] = useState<Account | null | undefined>();
  const [mapping, setMapping] = useState<Mapping | null | undefined>();
  const [credit, setCredit] = useState<Account | null>(null);
  const [grant, setGrant] = useState<Account | null>(null);
  const [credential, setCredential] = useState<Credential | null>(null);
  const [rotation, setRotation] = useState<Account | null>(null);
  const [refund, setRefund] = useState<Job | null>(null);
  const tabId = useId();
  const tabRefs = useRef<Array<HTMLButtonElement | null>>([]);
  useEffect(() => {
    if (tab !== "orders") return;
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") q.reload();
    }, 15000);
    return () => clearInterval(timer);
  }, [tab, q.reload]);
  const data = q.data;
  const accounts = data?.accounts || [],
    connections = data?.connections || [];
  const productName = (id: number) =>
    products.list.find((p) => p.id === id)?.name || `商品 #${id}`;
  const connectionName = (id: number) =>
    connections.find((c) => c.id === id)?.name || `连接 #${id}`;
  const accountName = (id: number) =>
    accounts.find((a) => a.id === id)?.name || `账户 #${id}`;
  function reload() {
    q.reload();
    setRevision((v) => v + 1);
  }
  async function act(
    key: string,
    task: () => Promise<unknown>,
    message: string,
  ) {
    if (busy) return;
    setBusy(key);
    setError("");
    try {
      await task();
      q.reload();
      notify(message);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy("");
    }
  }
  async function copyCallback(id: number) {
    try {
      await navigator.clipboard.writeText(
        `${window.location.origin}/api/v1/peer/callback/${id}`,
      );
      notify("回调地址已复制，请填写到上游对应的下游账户");
    } catch {
      setError("复制失败，请按连接编号填写本站的交付回调地址");
    }
  }
  const matching = (value: string) =>
    value.toLowerCase().includes(search.trim().toLowerCase());
  const mappings = (data?.mappings || []).filter((m) =>
    matching(`${productName(m.product_id)} ${connectionName(m.connection_id)}`),
  );
  const catalog = (data?.catalog || []).filter((p) =>
    matching(`${p.name} ${connectionName(p.connection_id)}`),
  );
  const jobs = (data?.jobs || []).filter((j) =>
    matching(
      `${j.order_no || j.order_id} ${j.remote_order_no} ${connectionName(j.connection_id)}`,
    ),
  );
  const ledger = (data?.ledger || []).filter((l) =>
    matching(`${accountName(l.account_id)} ${l.reference} ${l.note}`),
  );
  return (
    <div className="p-page">
      <PageHeading
        title="站点互联"
        description="与其他 Wybo 站点连接，自动采购商品，也向下游开放供货。"
      >
        <button
          className="a-button"
          disabled={!!busy || q.loading}
          onClick={reload}
        >
          <RefreshCw size={16} aria-hidden="true" />
          刷新
        </button>
        {tab === "upstream" && (
          <button
            className="a-button primary"
            disabled={!!busy || !data}
            onClick={() => setConnection(null)}
          >
            <Plus size={16} aria-hidden="true" />
            新增上游
          </button>
        )}
        {tab === "accounts" && (
          <button
            className="a-button primary"
            disabled={!!busy || !data}
            onClick={() => setAccount(null)}
          >
            <Plus size={16} aria-hidden="true" />
            新增下游
          </button>
        )}
        {tab === "mapping" && (
          <button
            className="a-button primary"
            disabled={
              !!busy ||
              !data ||
              !products.list.length ||
              !(data.catalog || []).length
            }
            onClick={() => setMapping(null)}
          >
            <Link2 size={16} aria-hidden="true" />
            对接商品
          </button>
        )}
      </PageHeading>
      {error && <Banner>{error}</Banner>}
      {products.error && <Banner>本地商品读取失败：{products.error}</Banner>}
      {q.loading && !data ? (
        <Busy />
      ) : q.error ? (
        <QueryError error={q.error} reload={q.reload} />
      ) : (
        data && (
          <>
            <div className="p-summary" aria-label="互联概况">
              <div>
                <ArrowDownUp size={21} aria-hidden="true" />
                <span>
                  本站标识
                  <strong className="p-site-id">
                    {data.site_id || "尚未生成"}
                  </strong>
                </span>
              </div>
              <div>
                <Link2 size={21} aria-hidden="true" />
                <span>
                  上游连接<strong>{connections.length}</strong>
                </span>
              </div>
              <div>
                <ShieldCheck size={21} aria-hidden="true" />
                <span>
                  下游账户<strong>{accounts.length}</strong>
                </span>
              </div>
              <div>
                <Wallet size={21} aria-hidden="true" />
                <span>
                  待确认采购
                  <strong>
                    {
                      (data.jobs || []).filter((j) => j.status === "unknown")
                        .length
                    }
                  </strong>
                </span>
              </div>
            </div>
            <div className="p-tabs" role="tablist" aria-label="站点互联管理">
              {tabs.map(([key, label], index) => (
                <button
                  key={key}
                  ref={(node) => {
                    tabRefs.current[index] = node;
                  }}
                  id={`${tabId}-${key}`}
                  role="tab"
                  aria-selected={tab === key}
                  aria-controls={`${tabId}-panel`}
                  tabIndex={tab === key ? 0 : -1}
                  onClick={() => {
                    setTab(key);
                    setSearch("");
                  }}
                  onKeyDown={(e) => {
                    let next = index;
                    if (e.key === "ArrowRight")
                      next = (index + 1) % tabs.length;
                    else if (e.key === "ArrowLeft")
                      next = (index + tabs.length - 1) % tabs.length;
                    else if (e.key === "Home") next = 0;
                    else if (e.key === "End") next = tabs.length - 1;
                    else return;
                    e.preventDefault();
                    setTab(tabs[next][0]);
                    setSearch("");
                    tabRefs.current[next]?.focus();
                  }}
                >
                  {label}
                </button>
              ))}
            </div>
            <section
              className="a-panel p-panel"
              id={`${tabId}-panel`}
              role="tabpanel"
              aria-labelledby={`${tabId}-${tab}`}
            >
              <div className="a-list-toolbar">
                <h2>{tabs.find(([key]) => key === tab)?.[1]}</h2>
                <input
                  type="search"
                  aria-label="搜索当前列表"
                  placeholder={
                    tab === "orders" ? "搜索本站或上游订单号" : "搜索名称或流水"
                  }
                  value={search}
                  onChange={(e) => setSearch(e.target.value)}
                />
              </div>
              {tab === "upstream" &&
                (connections.filter((c) => matching(c.name + c.base_url))
                  .length ? (
                  <div className="a-table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>连接 / 站点地址</th>
                          <th>状态</th>
                          <th>上游余额</th>
                          <th>最近同步</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {connections
                          .filter((c) => matching(c.name + c.base_url))
                          .map((c) => (
                            <tr key={c.id}>
                              <td>
                                <strong>{c.name}</strong>
                                <small>本站连接编号 #{c.id}</small>
                                <small className="p-break">{c.base_url}</small>
                                {c.site_id && <small>站点：{c.site_id}</small>}
                                {c.last_error && (
                                  <small className="p-error">
                                    {c.last_error}
                                  </small>
                                )}
                              </td>
                              <td>
                                <ToggleState enabled={c.enabled} />
                              </td>
                              <td>
                                {c.last_sync_at || c.site_id
                                  ? yuan(c.balance_cents)
                                  : "待测试"}
                              </td>
                              <td>
                                {c.last_sync_at
                                  ? shortDate(c.last_sync_at)
                                  : "尚未同步"}
                              </td>
                              <td>
                                <div className="p-actions">
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy}
                                    onClick={() => setConnection(c)}
                                  >
                                    编辑
                                  </button>
                                  <button
                                    className="a-table-button"
                                    onClick={() => void copyCallback(c.id)}
                                  >
                                    复制回调地址
                                  </button>
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy}
                                    onClick={() =>
                                      void act(
                                        `test-${c.id}`,
                                        () =>
                                          write(
                                            `/peers/connections/${c.id}/test`,
                                          ),
                                        "连接测试成功",
                                      )
                                    }
                                  >
                                    {busy === `test-${c.id}`
                                      ? "测试中…"
                                      : "测试连接"}
                                  </button>
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy || !c.enabled}
                                    onClick={() =>
                                      void act(
                                        `sync-${c.id}`,
                                        () =>
                                          write(
                                            `/peers/connections/${c.id}/sync`,
                                          ),
                                        "上游商品已同步",
                                      )
                                    }
                                  >
                                    {busy === `sync-${c.id}`
                                      ? "同步中…"
                                      : "同步商品"}
                                  </button>
                                </div>
                              </td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty
                    title="尚无上游连接"
                    text="新增上游，填写对方提供的 API 凭证，测试连接后同步商品。"
                  />
                ))}
              {tab === "mapping" && (
                <>
                  {mappings.length ? (
                    <div className="a-table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>本地商品</th>
                            <th>上游商品</th>
                            <th>最高采购单价</th>
                            <th>同步售价</th>
                            <th>状态 / 操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {mappings.map((m) => (
                            <tr key={m.product_id}>
                              <td>
                                <strong>{productName(m.product_id)}</strong>
                              </td>
                              <td>
                                {connectionName(m.connection_id)}
                                <small>
                                  {(data.catalog || []).find(
                                    (p) =>
                                      p.connection_id === m.connection_id &&
                                      p.remote_product_id ===
                                        m.remote_product_id,
                                  )?.name || `上游商品 #${m.remote_product_id}`}
                                </small>
                              </td>
                              <td>{yuan(m.max_unit_cost_cents)}</td>
                              <td>
                                {m.sync_price
                                  ? `成本 + ${(m.markup_bps / 100).toFixed(2)}% + ${yuan(m.markup_cents)}`
                                  : "保留本地售价"}
                              </td>
                              <td>
                                <div className="p-actions">
                                  <ToggleState enabled={m.enabled} />
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy}
                                    onClick={() => setMapping(m)}
                                  >
                                    编辑对接
                                  </button>
                                </div>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <Empty
                      title="尚无商品对接"
                      text="先同步上游商品，再将需要销售的规格关联到本地商品。"
                    />
                  )}
                  <div className="p-subheading">
                    <h3>已同步的上游商品</h3>
                    <span>{catalog.length} 件</span>
                  </div>
                  {catalog.length ? (
                    <div className="a-table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>商品 / 来源</th>
                            <th>采购单价</th>
                            <th>库存</th>
                            <th>交付方式</th>
                            <th>状态</th>
                          </tr>
                        </thead>
                        <tbody>
                          {catalog.map((p) => (
                            <tr
                              key={`${p.connection_id}-${p.remote_product_id}`}
                            >
                              <td>
                                <strong>{p.name}</strong>
                                <small>
                                  {connectionName(p.connection_id)} · #
                                  {p.remote_product_id}
                                </small>
                              </td>
                              <td>{yuan(p.price_cents)}</td>
                              <td>{stockText(p.stock)}</td>
                              <td>
                                {p.delivery_mode === "auto"
                                  ? "自动交付"
                                  : "人工处理"}
                              </td>
                              <td>
                                <ToggleState enabled={p.enabled} />
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <p className="p-empty-note">
                      没有匹配的上游商品，可在「上游连接」同步。
                    </p>
                  )}
                </>
              )}
              {tab === "accounts" &&
                (accounts.filter((a) => matching(a.name)).length ? (
                  <div className="a-table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>下游账户</th>
                          <th>钱包余额</th>
                          <th>商品授权</th>
                          <th>状态</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {accounts
                          .filter((a) => matching(a.name))
                          .map((a) => (
                            <tr key={a.id}>
                              <td>
                                <strong>{a.name}</strong>
                                <small>API Key：{mask(a.api_key)}</small>
                                <small className="p-break">
                                  {a.callback_url || "未配置交付回调"}
                                </small>
                              </td>
                              <td>{yuan(a.balance_cents)}</td>
                              <td>
                                {
                                  (data.grants || []).filter(
                                    (g) => g.account_id === a.id && g.enabled,
                                  ).length
                                }{" "}
                                件
                              </td>
                              <td>
                                <ToggleState enabled={a.enabled} />
                              </td>
                              <td>
                                <div className="p-actions">
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy}
                                    onClick={() => setAccount(a)}
                                  >
                                    编辑
                                  </button>
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy}
                                    onClick={() => setGrant(a)}
                                  >
                                    商品授权
                                  </button>
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy || !a.enabled}
                                    onClick={() => setCredit(a)}
                                  >
                                    充值登记
                                  </button>
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy}
                                    onClick={() => setRotation(a)}
                                  >
                                    轮换凭证
                                  </button>
                                </div>
                              </td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty
                    title="尚无下游账户"
                    text="为独立部署的 Wybo 店铺创建采购账户，再配置商品授权和供货价格。"
                  />
                ))}
              {tab === "orders" && (
                <>
                  {jobs.length ? (
                    <div className="a-table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>本站 / 上游订单</th>
                            <th>上游连接</th>
                            <th>采购状态</th>
                            <th>尝试次数</th>
                            <th>操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {jobs.map((j) => (
                            <tr key={j.id}>
                              <td>
                                <strong>
                                  {j.order_no || `订单 #${j.order_id}`}
                                </strong>
                                <small>
                                  {j.remote_order_no || "尚未确认上游单号"}
                                </small>
                                <small>{shortDate(j.created_at)}</small>
                                {j.last_error && (
                                  <small className="p-error">
                                    {j.last_error}
                                  </small>
                                )}
                              </td>
                              <td>{connectionName(j.connection_id)}</td>
                              <td>
                                <State value={j.status} />
                              </td>
                              <td>{j.attempts}</td>
                              <td>
                                <div className="p-actions">
                                  {[
                                    "pending",
                                    "unknown",
                                    "processing",
                                    "attention",
                                  ].includes(j.status) && (
                                    <button
                                      className="a-table-button"
                                      disabled={!!busy}
                                      onClick={() =>
                                        void act(
                                          `job-${j.id}`,
                                          () =>
                                            write(`/peers/jobs/${j.id}/retry`),
                                          "已安排查单 / 重试，仍使用原采购单号",
                                        )
                                      }
                                    >
                                      {busy === `job-${j.id}`
                                        ? "安排中…"
                                        : "查单 / 重试"}
                                    </button>
                                  )}
                                  {j.wallet_refundable && j.order_no && (
                                    <button
                                      className="a-table-button a-danger-text"
                                      disabled={!!busy}
                                      onClick={() => setRefund(j)}
                                    >
                                      {j.status === "pending"
                                        ? "取消并退回钱包"
                                        : "处理钱包退款"}
                                    </button>
                                  )}
                                </div>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <Empty
                      title="没有匹配的采购任务"
                      text="互联商品收到付款后，采购任务会出现在这里。"
                    />
                  )}
                  <div className="p-subheading">
                    <h3>交付回调</h3>
                    <span>通知失败时，下游仍可主动查单</span>
                  </div>
                  {(data.events || []).length ? (
                    <div className="a-table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>订单</th>
                            <th>通知状态</th>
                            <th>尝试次数</th>
                            <th>操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {(data.events || []).map((event) => (
                            <tr key={event.id}>
                              <td>
                                订单 #{event.order_id}
                                {event.last_error && (
                                  <small className="p-error">
                                    {event.last_error}
                                  </small>
                                )}
                              </td>
                              <td>
                                <State value={event.status} notification />
                              </td>
                              <td>{event.attempts}</td>
                              <td>
                                {["pending", "failed"].includes(
                                  event.status,
                                ) && (
                                  <button
                                    className="a-table-button"
                                    disabled={!!busy}
                                    onClick={() =>
                                      void act(
                                        `event-${event.id}`,
                                        () =>
                                          write(
                                            `/peers/events/${event.id}/retry`,
                                          ),
                                        "已安排回调重试",
                                      )
                                    }
                                  >
                                    重试通知
                                  </button>
                                )}
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <p className="p-empty-note">尚无交付回调。</p>
                  )}
                </>
              )}
              {tab === "ledger" &&
                (ledger.length ? (
                  <div className="a-table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>时间 / 下游账户</th>
                          <th>类型</th>
                          <th>变动金额</th>
                          <th>变动后余额</th>
                          <th>流水 / 备注</th>
                        </tr>
                      </thead>
                      <tbody>
                        {ledger.map((l) => (
                          <tr key={l.id}>
                            <td>
                              {shortDate(l.created_at)}
                              <small>{accountName(l.account_id)}</small>
                            </td>
                            <td>
                              {{
                                credit: "充值",
                                debit: "订单消费",
                                refund: "订单退款",
                                purchase: "订单消费",
                                adjustment: "人工调整",
                              }[l.kind] || l.kind}
                            </td>
                            <td
                              className={
                                l.amount_cents < 0 ? "a-negative" : "a-profit"
                              }
                            >
                              {l.amount_cents > 0 ? "+" : ""}
                              {yuan(l.amount_cents)}
                            </td>
                            <td>{yuan(l.balance_cents)}</td>
                            <td className="p-break">
                              {l.reference}
                              <small>{l.note}</small>
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty
                    title="没有匹配的资金流水"
                    text="充值、消费和退款逐笔留痕；充值与商品销售分别记账。"
                  />
                ))}
            </section>
          </>
        )
      )}
      {connection !== undefined && (
        <ConnectionEditor
          value={connection}
          onClose={() => setConnection(undefined)}
          onSaved={() => {
            reload();
            notify("上游连接已保存");
          }}
        />
      )}
      {account !== undefined && (
        <AccountEditor
          value={account}
          onClose={() => setAccount(undefined)}
          onSaved={(value) => {
            reload();
            notify("下游账户已保存");
            if (value) setCredential(value);
          }}
        />
      )}
      {mapping !== undefined && (
        <MappingEditor
          value={mapping}
          products={products.list}
          data={data}
          onClose={() => setMapping(undefined)}
          onSaved={() => {
            reload();
            notify("商品对接已保存，后续订单使用新配置");
          }}
        />
      )}
      {credit && (
        <CreditEditor
          account={credit}
          onClose={() => setCredit(null)}
          onSaved={() => {
            q.reload();
            notify("充值已登记");
          }}
        />
      )}
      {grant && (
        <GrantsEditor
          account={grant}
          products={products.list}
          grants={data?.grants || []}
          onClose={() => setGrant(null)}
          onSaved={() => {
            q.reload();
            notify("商品授权已保存");
          }}
        />
      )}
      {credential && (
        <CredentialDialog
          value={credential}
          onClose={() => setCredential(null)}
        />
      )}
      {rotation && (
        <Editor
          title="轮换下游凭证"
          description={`为「${rotation.name}」生成新凭证后，旧凭证立即失效。已有订单和账户余额保持不变。`}
          label="确认轮换"
          onClose={() => setRotation(null)}
          submit={async () => {
            const result = await write<Credential>(
              `/peers/accounts/${rotation.id}/rotate`,
            );
            setCredential(result);
            q.reload();
          }}
        >
          <p className="a-hint">请及时在对方 Wybo 的上游连接中替换凭证。</p>
        </Editor>
      )}
      {refund && (
        <RefundEditor
          job={refund}
          onClose={() => setRefund(null)}
          onSaved={() => {
            q.reload();
            notify("钱包退款已处理");
          }}
        />
      )}
    </div>
  );
}

function mask(value: string) {
  return value?.length > 8
    ? `${value.slice(0, 4)}••••${value.slice(-4)}`
    : "••••••••";
}
function Editor({
  title,
  description,
  submit,
  children,
  onClose,
  label = "保存",
}: {
  title: string;
  description: string;
  submit: () => Promise<void>;
  children?: ReactNode;
  onClose: () => void;
  label?: string;
}) {
  const [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  async function save(event: FormEvent) {
    event.preventDefault();
    if (busy) return;
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
      <form className="a-editor p-editor" onSubmit={save}>
        <h2>{title}</h2>
        <p>{description}</p>
        {error && <Banner>{error}</Banner>}
        <fieldset disabled={busy}>{children}</fieldset>
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
            {busy ? "处理中…" : label}
          </button>
        </div>
      </form>
    </Modal>
  );
}
function ConnectionEditor({
  value,
  onClose,
  onSaved,
}: {
  value: Connection | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(value?.name || ""),
    [url, setUrl] = useState(value?.base_url || ""),
    [key, setKey] = useState(value?.api_key || ""),
    [secret, setSecret] = useState(""),
    [enabled, setEnabled] = useState(value?.enabled ?? true);
  return (
    <Editor
      title={value ? "编辑上游连接" : "新增上游连接"}
      description="填写上游 Wybo 的站点地址和对方提供的采购凭证。"
      onClose={onClose}
      submit={async () => {
        await write(
          value ? `/peers/connections/${value.id}` : "/peers/connections",
          { name, base_url: url, api_key: key, api_secret: secret, enabled },
          value ? "PUT" : "POST",
        );
        onSaved();
      }}
    >
      <Field label="连接名称">
        <input
          required
          maxLength={80}
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="例如：月兔供货站"
        />
      </Field>
      <Field
        label="上游站点地址"
        hint="填写站点根地址，例如 https://supplier.example.com。"
      >
        <input
          required
          type="url"
          maxLength={500}
          value={url}
          onChange={(e) => setUrl(e.target.value)}
          placeholder="https://supplier.example.com"
        />
      </Field>
      <Field label="API Key">
        <input
          required
          autoComplete="off"
          maxLength={200}
          value={key}
          onChange={(e) => setKey(e.target.value)}
        />
      </Field>
      <Field
        label="API Secret"
        hint={value ? "留空保留现有密钥。" : "由上游创建采购账户后提供。"}
      >
        <input
          required={!value}
          type="password"
          autoComplete="new-password"
          maxLength={500}
          value={secret}
          onChange={(e) => setSecret(e.target.value)}
        />
      </Field>
      <label className="a-check">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        启用连接
      </label>
    </Editor>
  );
}
function AccountEditor({
  value,
  onClose,
  onSaved,
}: {
  value: Account | null;
  onClose: () => void;
  onSaved: (value?: Credential) => void;
}) {
  const [name, setName] = useState(value?.name || ""),
    [callback, setCallback] = useState(value?.callback_url || ""),
    [enabled, setEnabled] = useState(value?.enabled ?? true);
  return (
    <Editor
      title={value ? "编辑下游账户" : "新增下游账户"}
      description="为下游 Wybo 创建独立采购账户，余额与代理佣金分开管理。"
      onClose={onClose}
      submit={async () => {
        const result = await write<Credential>(
          value ? `/peers/accounts/${value.id}` : "/peers/accounts",
          { name, callback_url: callback, enabled },
          value ? "PUT" : "POST",
        );
        onSaved(value ? undefined : result);
      }}
    >
      <Field label="下游名称">
        <input
          required
          maxLength={80}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </Field>
      <Field
        label="交付回调地址"
        hint="可选。由对方提供，通知订单状态与交付结果；下游也可主动查单。"
      >
        <input
          type="url"
          maxLength={500}
          value={callback}
          onChange={(e) => setCallback(e.target.value)}
          placeholder="https://buyer.example.com/api/v1/peer/callback/1"
        />
      </Field>
      <label className="a-check">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        启用采购账户
      </label>
    </Editor>
  );
}
function CreditEditor({
  account,
  onClose,
  onSaved,
}: {
  account: Account;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [amount, setAmount] = useState(""),
    [reference, setReference] = useState(""),
    [note, setNote] = useState("");
  return (
    <Editor
      title="下游充值登记"
      description={`为「${account.name}」登记已核实到账的充值，当前余额 ${yuan(account.balance_cents)}。`}
      label="确认登记充值"
      onClose={onClose}
      submit={async () => {
        const cents = toCents(amount);
        if (cents <= 0) throw new Error("充值金额须大于零");
        await write(`/peers/accounts/${account.id}/credit`, {
          amount_cents: cents,
          reference,
          note,
        });
        onSaved();
      }}
    >
      <Field label="实际到账金额（元）">
        <input
          required
          inputMode="decimal"
          pattern="[0-9]+([.][0-9]{1,2})?"
          value={amount}
          onChange={(e) => setAmount(e.target.value)}
          placeholder="0.00"
        />
      </Field>
      <Field
        label="唯一付款流水"
        hint="填写可核对的外部付款凭证，重复流水不能重复充值。"
      >
        <input
          required
          minLength={4}
          maxLength={200}
          value={reference}
          onChange={(e) => setReference(e.target.value)}
        />
      </Field>
      <Field label="备注">
        <textarea
          maxLength={500}
          rows={3}
          value={note}
          onChange={(e) => setNote(e.target.value)}
        />
      </Field>
    </Editor>
  );
}
function ProductSelect({
  products,
  value,
  onChange,
}: {
  products: AdminProduct[];
  value: number;
  onChange: (value: number) => void;
}) {
  const [search, setSearch] = useState("");
  const filtered = products.filter(
    (p) =>
      p.id === value || p.name.toLowerCase().includes(search.toLowerCase()),
  );
  return (
    <>
      <Field label="查找本地商品">
        <input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="按商品名称查找"
        />
      </Field>
      <Field label="本地商品">
        <select
          required
          value={value || ""}
          onChange={(e) => onChange(Number(e.target.value))}
        >
          <option value="">请选择本地商品</option>
          {filtered.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
              {!p.enabled ? "（已下架）" : ""}
            </option>
          ))}
        </select>
      </Field>
    </>
  );
}
function MappingEditor({
  value,
  products,
  data,
  onClose,
  onSaved,
}: {
  value: Mapping | null;
  products: AdminProduct[];
  data: Data | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [product, setProduct] = useState(value?.product_id || 0),
    [connection, setConnection] = useState(
      value?.connection_id || data?.connections?.[0]?.id || 0,
    ),
    [remote, setRemote] = useState(value?.remote_product_id || 0),
    [maximum, setMaximum] = useState(moneyInput(value?.max_unit_cost_cents)),
    [enabled, setEnabled] = useState(value?.enabled ?? true),
    [syncPrice, setSyncPrice] = useState(value?.sync_price ?? false),
    [percent, setPercent] = useState(moneyInput(value?.markup_bps) || "0.00"),
    [fixed, setFixed] = useState(moneyInput(value?.markup_cents) || "0.00");
  const options = (data?.catalog || []).filter(
    (p) => p.connection_id === connection,
  );
  return (
    <Editor
      title="商品对接"
      description="将已有本地商品关联到一个上游规格。改动只影响后续新订单。"
      onClose={onClose}
      submit={async () => {
        if (!product || !connection || !remote)
          throw new Error("请选择本地商品、上游连接及上游商品");
        await write(
          "/peers/mappings",
          {
            product_id: product,
            connection_id: connection,
            remote_product_id: remote,
            max_unit_cost_cents: toCents(maximum),
            enabled,
            sync_price: syncPrice,
            markup_bps: syncPrice ? toCents(percent) : 0,
            markup_cents: syncPrice ? toCents(fixed) : 0,
          },
          "PUT",
        );
        onSaved();
      }}
    >
      <ProductSelect
        products={products}
        value={product}
        onChange={setProduct}
      />
      <Field label="上游连接">
        <select
          required
          value={connection || ""}
          onChange={(e) => {
            setConnection(Number(e.target.value));
            setRemote(0);
          }}
        >
          <option value="">请选择连接</option>
          {(data?.connections || []).map((c) => (
            <option key={c.id} value={c.id}>
              {c.name}
              {!c.enabled ? "（已停用）" : ""}
            </option>
          ))}
        </select>
      </Field>
      <Field
        label="上游商品"
        hint="仅对接自动交付商品；尚未出现的商品请先同步。"
      >
        <select
          required
          value={remote || ""}
          onChange={(e) => {
            const id = Number(e.target.value);
            setRemote(id);
            const selected = options.find((p) => p.remote_product_id === id);
            if (selected) setMaximum(moneyInput(selected.price_cents));
          }}
        >
          <option value="">请选择上游商品</option>
          {options.map((p) => (
            <option
              key={p.remote_product_id}
              value={p.remote_product_id}
              disabled={p.delivery_mode !== "auto" || !p.enabled}
            >
              {p.name} · {yuan(p.price_cents)} · {stockText(p.stock)}
              {!p.enabled
                ? "（已停售）"
                : p.delivery_mode !== "auto"
                  ? "（人工交付暂不支持）"
                  : ""}
            </option>
          ))}
        </select>
      </Field>
      <Field
        label="最高采购单价（元）"
        hint="超过上限时停止本单采购，避免按上游涨价后的金额扣款。"
      >
        <input
          required
          inputMode="decimal"
          pattern="[0-9]+([.][0-9]{1,2})?"
          value={maximum}
          onChange={(e) => setMaximum(e.target.value)}
        />
      </Field>
      <label className="a-check">
        <input
          type="checkbox"
          checked={syncPrice}
          onChange={(e) => setSyncPrice(e.target.checked)}
        />
        随上游成本同步本地零售价
      </label>
      {syncPrice && (
        <div className="a-form-grid">
          <Field label="成本加价比例（%）">
            <input
              required
              inputMode="decimal"
              pattern="[0-9]+([.][0-9]{1,2})?"
              value={percent}
              onChange={(e) => setPercent(e.target.value)}
            />
          </Field>
          <Field label="固定加价（元）">
            <input
              required
              inputMode="decimal"
              pattern="[0-9]+([.][0-9]{1,2})?"
              value={fixed}
              onChange={(e) => setFixed(e.target.value)}
            />
          </Field>
        </div>
      )}
      <label className="a-check">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        启用上游供货
      </label>
    </Editor>
  );
}
function GrantsEditor({
  account,
  products,
  grants,
  onClose,
  onSaved,
}: {
  account: Account;
  products: AdminProduct[];
  grants: Grant[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [product, setProduct] = useState(0),
    [price, setPrice] = useState(""),
    [enabled, setEnabled] = useState(true);
  const rows = grants.filter((g) => g.account_id === account.id);
  function choose(id: number) {
    setProduct(id);
    const found = rows.find((g) => g.product_id === id);
    setPrice(moneyInput(found?.price_cents));
    setEnabled(found?.enabled ?? true);
  }
  return (
    <Editor
      title="下游商品授权"
      description={`为「${account.name}」逐项设置可采购的商品和专属供货价。`}
      onClose={onClose}
      submit={async () => {
        if (!product) throw new Error("请选择商品");
        const cents = toCents(price);
        if (cents <= 0) throw new Error("供货价须大于零");
        await write(
          `/peers/accounts/${account.id}/products`,
          { product_id: product, price_cents: cents, enabled },
          "PUT",
        );
        onSaved();
      }}
    >
      {rows.length > 0 && (
        <div className="p-grants">
          <span>现有授权</span>
          {rows.map((g) => (
            <button
              type="button"
              key={g.id || g.product_id}
              onClick={() => choose(g.product_id)}
              className={`p-grant ${g.product_id === product ? "is-current" : ""}`}
            >
              <span>
                {products.find((p) => p.id === g.product_id)?.name ||
                  `商品 #${g.product_id}`}
              </span>
              <span>
                {yuan(g.price_cents)} · {g.enabled ? "启用" : "停用"}
              </span>
            </button>
          ))}
        </div>
      )}
      <ProductSelect products={products} value={product} onChange={choose} />
      <Field label="供货单价（元）">
        <input
          required
          inputMode="decimal"
          pattern="[0-9]+([.][0-9]{1,2})?"
          value={price}
          onChange={(e) => setPrice(e.target.value)}
        />
      </Field>
      <label className="a-check">
        <input
          type="checkbox"
          checked={enabled}
          onChange={(e) => setEnabled(e.target.checked)}
        />
        允许该账户采购此商品
      </label>
    </Editor>
  );
}
function RefundEditor({
  job,
  onClose,
  onSaved,
}: {
  job: Job;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [reason, setReason] = useState("");
  return (
    <Editor
      title="互联订单钱包退款"
      description={`订单 ${job.order_no} ${job.status === "pending" ? "尚未向上游提交采购" : "已明确采购失败"}。确认后向原下游账户退回订单余额。`}
      label="确认钱包退款"
      onClose={onClose}
      submit={async () => {
        await write(
          `/peers/orders/${encodeURIComponent(job.order_no)}/refund`,
          { reason },
        );
        onSaved();
      }}
    >
      <Field label="退款原因">
        <textarea
          required
          minLength={2}
          maxLength={500}
          rows={3}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
        />
      </Field>
    </Editor>
  );
}
function CredentialDialog({
  value,
  onClose,
}: {
  value: Credential;
  onClose: () => void;
}) {
  const [copied, setCopied] = useState(false),
    [error, setError] = useState("");
  async function copy() {
    try {
      await navigator.clipboard.writeText(
        `API Key: ${value.api_key}\nAPI Secret: ${value.api_secret}`,
      );
      setCopied(true);
      setError("");
    } catch {
      setError("复制失败，请手动选择并保存凭证。");
    }
  }
  return (
    <Modal title="保存下游 API 凭证" onClose={onClose}>
      <div className="a-editor">
        <h2>保存 API 凭证</h2>
        <p>
          密钥仅在本次展示。请交给对应下游，并在对方 Wybo 的上游连接中填写。
        </p>
        {error && <Banner>{error}</Banner>}
        <Field label="API Key">
          <input
            readOnly
            value={value.api_key || ""}
            onFocus={(e) => e.target.select()}
          />
        </Field>
        <Field label="API Secret">
          <input
            readOnly
            autoComplete="off"
            value={value.api_secret || ""}
            onFocus={(e) => e.target.select()}
          />
        </Field>
        <div className="a-form-actions">
          <button className="a-button" onClick={() => void copy()}>
            {copied ? (
              <Check size={15} aria-hidden="true" />
            ) : (
              <Copy size={15} aria-hidden="true" />
            )}
            {copied ? "已复制" : "复制凭证"}
          </button>
          <button className="a-button primary" onClick={onClose}>
            <KeyRound size={15} aria-hidden="true" />
            我已保存
          </button>
        </div>
      </div>
    </Modal>
  );
}
