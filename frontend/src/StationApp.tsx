import { useCallback, useEffect, useState, type FormEvent } from "react";
import {
  Link,
  NavLink,
  Navigate,
  Route,
  Routes,
  useLocation,
} from "react-router-dom";
import {
  ArrowUpRight,
  LayoutDashboard,
  Package,
  ShoppingBag,
  MessagesSquare,
  HandCoins,
  Settings,
  LogOut,
  ShieldCheck,
  Store,
} from "lucide-react";
import QRCode from "qrcode";
import { useSite, SiteLogo } from "./Site";
import {
  Banner,
  Busy,
  Empty,
  Field,
  PageHeading,
  Status,
  shortDate,
} from "./admin/ui";
import { yuan } from "./admin/api";
import { Modal } from "./components";
import {
  stationApi,
  STATION_TOKEN,
  type StationDashboard,
  type StationProduct,
  type StationOrder,
  type Station,
} from "./stations";
import {
  StationBrandFields,
  StationOrderTable,
  StationProductRow,
} from "./StationComponents";
import "./admin/admin.css";
import "./stations.css";

function useQuery<T>(path: string) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [revision, setRevision] = useState(0);
  const reload = useCallback(() => setRevision((v) => v + 1), []);
  useEffect(() => {
    const c = new AbortController();
    setLoading(true);
    setError("");
    stationApi<T>("/admin" + path, { signal: c.signal })
      .then(setData)
      .catch((e) => {
        if (!c.signal.aborted) setError(e.message);
      })
      .finally(() => {
        if (!c.signal.aborted) setLoading(false);
      });
    return () => c.abort();
  }, [path, revision]);
  return { data, error, loading, reload };
}
function QueryState({
  error,
  loading,
  reload,
}: {
  error: string;
  loading: boolean;
  reload: () => void;
}) {
  return loading ? (
    <Busy />
  ) : error ? (
    <>
      <Banner>{error}</Banner>
      <button className="a-button" onClick={reload}>
        重新加载
      </button>
    </>
  ) : null;
}
function Login({ onLogin }: { onLogin: () => void }) {
  const { site } = useSite();
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const r = await stationApi<{ token: string }>("/login", {
        method: "POST",
        body: JSON.stringify({ password, code }),
      });
      sessionStorage.setItem(STATION_TOKEN, r.token);
      onLogin();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <section className="a-panel station-login">
      <SiteLogo url={site.site_logo} />
      <h1>
        {site.site_name}
        <br />
        分站工作台
      </h1>
      <p className="station-note">
        使用激活时设置的密码登录，管理自己的商品与收益。
      </p>
      <form onSubmit={submit}>
        <fieldset disabled={busy}>
          <input
            hidden
            readOnly
            autoComplete="username"
            value={site.station_slug || ""}
          />
          <Field label="登录密码">
            <input
              required
              type="password"
              autoComplete="current-password"
              maxLength={72}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
            />
          </Field>
          <Field label="2FA 验证码（已开启时填写）">
            <input
              inputMode="numeric"
              autoComplete="one-time-code"
              maxLength={6}
              pattern="[0-9]{6}"
              value={code}
              onChange={(e) => setCode(e.target.value)}
            />
          </Field>
          {error && <Banner>{error}</Banner>}
          <button className="a-button primary">
            {busy ? "正在验证…" : "进入分站后台"}
          </button>
        </fieldset>
      </form>
      <p className="station-note">
        尚未激活？请打开开通邮件中的激活链接。忘记密码或遗失验证器时联系主站。
      </p>
    </section>
  );
}
const navigation = [
  ["", "经营概览", LayoutDashboard],
  ["/products", "商品定价", Package],
  ["/orders", "订单管理", ShoppingBag],
  ["/tickets", "售后工单", MessagesSquare],
  ["/settlements", "收益结算", HandCoins],
  ["/settings", "店铺设置", Settings],
  ["/security", "账号安全", ShieldCheck],
] as const;
export default function StationApp() {
  const { site } = useSite();
  const [logged, setLogged] = useState(!!sessionStorage.getItem(STATION_TOKEN));
  const [notice, setNotice] = useState("");
  const location = useLocation();
  const logout = useCallback(() => {
    sessionStorage.removeItem(STATION_TOKEN);
    setLogged(false);
  }, []);
  useEffect(() => {
    const expired = () => {
      logout();
      setNotice("登录已过期，请重新登录");
    };
    window.addEventListener("wybo-station-expired", expired);
    return () => window.removeEventListener("wybo-station-expired", expired);
  }, [logout]);
  useEffect(() => {
    window.scrollTo(0, 0);
  }, [location.pathname]);
  return (
    <div className="a-root station-shell">
      <header className="station-header">
        <Link to="/" className="station-brand">
          <SiteLogo url={site.site_logo} />
          <span>
            <strong>{site.site_name}</strong>
            <small>分站工作台</small>
          </span>
        </Link>
        <div className="station-actions">
          <Link className="a-button" to="/">
            查看店铺
            <ArrowUpRight size={15} />
          </Link>
          {logged && (
            <button className="a-button" onClick={logout}>
              <LogOut size={15} />
              退出
            </button>
          )}
        </div>
      </header>
      {logged && (
        <nav className="station-nav" aria-label="分站管理导航">
          {navigation.map(([path, label, Icon]) => (
            <NavLink key={path} to={"/admin" + path} end={!path}>
              <Icon size={16} aria-hidden="true" />
              {label}
            </NavLink>
          ))}
        </nav>
      )}
      <main className="station-main">
        {notice && <Banner success>{notice}</Banner>}
        {!logged ? (
          <Login
            onLogin={() => {
              setLogged(true);
              setNotice("");
            }}
          />
        ) : (
          <>
            <div className="station-note">
              {site.station_slug}.{site.station_root} ·{" "}
              {site.station_status === "active"
                ? "店铺营业中"
                : "店铺暂停营业，已有订单仍可查询及售后"}
            </div>
            <Routes>
              <Route path="/admin" element={<Overview />} />
              <Route path="/admin/products" element={<Products />} />
              <Route path="/admin/orders" element={<Orders />} />
              <Route path="/admin/tickets" element={<Tickets />} />
              <Route path="/admin/settlements" element={<Settlements />} />
              <Route path="/admin/settings" element={<Brand />} />
              <Route
                path="/admin/security"
                element={<Security logout={logout} />}
              />
              <Route path="*" element={<Navigate replace to="/admin" />} />
            </Routes>
          </>
        )}
      </main>
    </div>
  );
}
function Overview() {
  const q = useQuery<StationDashboard>("/dashboard");
  return (
    <>
      <PageHeading
        title="每一份成交，都有收获"
        description="独立经营自己的小铺，由主站负责收款与交付。"
      />
      <QueryState {...q} />
      {q.data && (
        <>
          <div className="a-metrics">
            {[
              ["累计实收", yuan(q.data.revenue_cents)],
              ["分站收益", yuan(q.data.profit_cents)],
              ["已付款订单", q.data.paid_orders],
              ["近 30 天访问", q.data.pv],
            ].map(([label, value]) => (
              <article className="a-metric" key={label}>
                <div>{label}</div>
                <strong>{value}</strong>
              </article>
            ))}
          </div>
          <section className="a-panel">
            <h2>收益与待办</h2>
            <p className="station-note">
              可结算 {yuan(q.data.balance.available_cents)} · 等待期内{" "}
              {yuan(q.data.balance.pending_cents)} · 待付款订单{" "}
              {q.data.pending_orders} 笔。主站承担支付手续费，完成交付后等待{" "}
              {q.data.settlement_days} 天可申请结算。
            </p>
            <div className="station-actions">
              <Link className="a-button primary" to="/admin/products">
                管理商品
              </Link>
              <Link className="a-button" to="/admin/settlements">
                查看结算
              </Link>
              <Link className="a-button" to="/admin/tickets">
                处理售后
              </Link>
            </div>
          </section>
          <section className="a-panel">
            <h2>最近订单</h2>
            <StationOrderTable orders={q.data.orders.slice(0, 8)} />
            <Link className="a-button" to="/admin/orders">
              查看全部订单
            </Link>
          </section>
        </>
      )}
    </>
  );
}
function Products() {
  const q = useQuery<StationProduct[]>("/products");
  return (
    <>
      <PageHeading
        title="商品与定价"
        description="选择上架已授权的商品，零售价不能低于供货价。库存与发货由主站统一管理。"
      />
      <QueryState {...q} />
      {q.data?.length ? (
        <div className="station-product-grid">
          {q.data.map((r) => (
            <StationProductRow
              key={`${r.product_id}-${r.wholesale_cents}`}
              row={r}
              owner={false}
              onSave={async (row) => {
                await stationApi(`/admin/products/${row.product_id}`, {
                  method: "PUT",
                  body: JSON.stringify(row),
                });
              }}
            />
          ))}
        </div>
      ) : (
        !q.loading &&
        !q.error && (
          <Empty
            title="还没有授权商品"
            text="请联系主站为你的分站开通商品与供货价。"
          />
        )
      )}
    </>
  );
}
function Orders() {
  const [page, setPage] = useState(1);
  const q = useQuery<{ list: StationOrder[]; total: number }>(
    `/orders?page=${page}&size=20`,
  );
  return (
    <>
      <PageHeading
        title="订单管理"
        description="查看本店全部订单。发货、补卡与退款由主站处理。"
      />
      <QueryState {...q} />
      {q.data && (
        <section className="a-panel">
          <StationOrderTable orders={q.data.list} />
          <div className="station-row-actions">
            <span>
              共 {q.data.total} 笔 · 第 {page} 页
            </span>
            <button
              className="a-button"
              disabled={q.loading || page <= 1}
              onClick={() => setPage((v) => v - 1)}
            >
              上一页
            </button>
            <button
              className="a-button"
              disabled={q.loading || page * 20 >= q.data.total}
              onClick={() => setPage((v) => v + 1)}
            >
              下一页
            </button>
          </div>
        </section>
      )}
    </>
  );
}
function Brand() {
  const q = useQuery<StationDashboard>("/dashboard");
  const [value, setValue] = useState<Station | null>(null);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  const { refresh } = useSite();
  useEffect(() => {
    if (q.data) setValue(q.data.station);
  }, [q.data]);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    setNotice("");
    try {
      await stationApi("/admin/settings", {
        method: "PUT",
        body: JSON.stringify(value),
      });
      await refresh();
      setNotice("店铺设置已保存");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading
        title="把小铺，布置成你的样子"
        description="设置独立品牌、客服与兑换说明，仅对本分站生效。"
      />
      <QueryState {...q} />
      {value && (
        <form onSubmit={submit} className="a-panel station-settings">
          <StationBrandFields
            value={value}
            onChange={setValue}
            disabled={busy}
            onUpload={async (file) => {
              setBusy(true);
              try {
                const data = new FormData();
                data.append("file", file);
                return (
                  await stationApi<{ url: string }>("/admin/upload", {
                    method: "POST",
                    body: data,
                  })
                ).url;
              } finally {
                setBusy(false);
              }
            }}
          />
          {error && <Banner>{error}</Banner>}
          {notice && <Banner success>{notice}</Banner>}
          <button className="a-button primary" disabled={busy}>
            {busy ? "处理中…" : "保存店铺设置"}
          </button>
        </form>
      )}
    </>
  );
}
function Settlements() {
  const q = useQuery<StationDashboard>("/dashboard");
  const [open, setOpen] = useState(false);
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  async function request(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await stationApi("/admin/settlements", {
        method: "POST",
        body: JSON.stringify({ password, code }),
      });
      setOpen(false);
      setPassword("");
      setCode("");
      setNotice("结算申请已提交，等待主站核对并打款");
      q.reload();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading
        title="收益结算"
        description="按订单成交时的供货价计算差价，历史收益不随调价变化。"
      />
      <QueryState {...q} />
      {notice && <Banner success>{notice}</Banner>}
      {q.data && (
        <>
          <div className="a-metrics">
            {[
              ["可申请结算", q.data.balance.available_cents],
              ["等待期内", q.data.balance.pending_cents],
              ["待主站打款", q.data.balance.reserved_cents],
              ["已登记打款", q.data.balance.paid_cents],
            ].map(([label, v]) => (
              <article className="a-metric" key={label}>
                <div>{label}</div>
                <strong>{yuan(Number(v))}</strong>
              </article>
            ))}
          </div>
          <section className="a-panel">
            <h2>结算规则</h2>
            <p className="station-note">
              主站承担支付手续费。订单完成发货后等待 {q.data.settlement_days}{" "}
              天可结算。退款会取消未结算收益，已实际打款的收益按主站售后流程处理。点击申请只生成结算单，实际打款由主站完成。
            </p>
            <button
              className="a-button primary"
              disabled={q.data.balance.available_cents <= 0}
              onClick={() => {
                setError("");
                setOpen(true);
              }}
            >
              申请结算
            </button>
          </section>
          <section className="a-panel">
            <h2>结算记录</h2>
            <div className="a-table-wrap">
              <table className="a-table">
                <thead>
                  <tr>
                    <th>编号 / 时间</th>
                    <th>订单数</th>
                    <th>金额</th>
                    <th>状态</th>
                    <th>打款凭证</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.settlements.map((r) => (
                    <tr key={r.id}>
                      <td>
                        #{r.id}
                        <small>{shortDate(r.created_at)}</small>
                      </td>
                      <td>{r.order_count}</td>
                      <td>{yuan(r.amount_cents)}</td>
                      <td>
                        <Status
                          value={r.status}
                          label={
                            r.status === "pending"
                              ? "待主站打款"
                              : r.status === "paid"
                                ? "已登记打款"
                                : "已取消"
                          }
                        />
                      </td>
                      <td>{r.reference || "—"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {!q.data.settlements.length && (
              <p className="station-note">暂无结算记录。</p>
            )}
          </section>
        </>
      )}
      {open && (
        <Modal
          title="申请收益结算"
          onClose={() => {
            if (!busy) setOpen(false);
          }}
        >
          <h2>申请结算</h2>
          <p className="station-note">核对可结算金额，并验证当前账号后提交。</p>
          <form className="station-editor" onSubmit={request}>
            <Field label="当前密码">
              <input
                required
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            <Field label="2FA 验证码（已开启时填写）">
              <input
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                value={code}
                onChange={(e) => setCode(e.target.value)}
              />
            </Field>
            {error && <Banner>{error}</Banner>}
            <button className="a-button primary" disabled={busy}>
              {busy ? "提交中…" : "确认申请"}
            </button>
          </form>
        </Modal>
      )}
    </>
  );
}
type Ticket = {
  number: string;
  order_no: string;
  subject: string;
  status: string;
  updated_at: string;
};
type TicketDetail = {
  ticket: Ticket;
  messages: {
    id: number;
    author: string;
    content: string;
    created_at: string;
  }[];
};
function Tickets() {
  const q = useQuery<Ticket[]>("/tickets");
  const [detail, setDetail] = useState<TicketDetail | null>(null);
  const [content, setContent] = useState("");
  const [status, setStatus] = useState("waiting");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function view(number: string) {
    setBusy(true);
    setError("");
    try {
      setDetail(
        await stationApi<TicketDetail>(
          `/admin/tickets/${encodeURIComponent(number)}`,
        ),
      );
      setContent("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function reply(e: FormEvent) {
    e.preventDefault();
    if (!detail) return;
    setBusy(true);
    setError("");
    try {
      await stationApi(`/admin/tickets/${detail.ticket.number}/replies`, {
        method: "POST",
        body: JSON.stringify({ content, status }),
      });
      setDetail(null);
      q.reload();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading
        title="售后工单"
        description="回复本店顾客。涉及补卡、退款与交付的问题请联系主站处理。"
      />
      <QueryState {...q} />
      {error && !detail && <Banner>{error}</Banner>}
      <div className="station-ticket-list">
        {q.data?.map((t) => (
          <button
            disabled={busy}
            key={t.number}
            className="station-ticket"
            onClick={() => void view(t.number)}
          >
            <strong>{t.subject}</strong>
            <small>
              {t.order_no} · {shortDate(t.updated_at)}
            </small>
            <Status value={t.status} />
          </button>
        ))}
      </div>
      {q.data?.length === 0 && (
        <Empty title="暂无售后工单" text="顾客提交的售后问题会显示在这里。" />
      )}
      {detail && (
        <Modal
          wide
          title={detail.ticket.subject}
          onClose={() => {
            if (!busy) setDetail(null);
          }}
        >
          <h2>{detail.ticket.subject}</h2>
          <p className="station-note">订单 {detail.ticket.order_no}</p>
          {detail.messages.map((m) => (
            <article className="station-message" key={m.id}>
              <small>
                {m.author === "customer" ? "顾客" : "店铺客服"} ·{" "}
                {shortDate(m.created_at)}
              </small>
              <p>{m.content}</p>
            </article>
          ))}
          <form onSubmit={reply} className="station-editor">
            <Field label="回复内容">
              <textarea
                required
                minLength={2}
                maxLength={4000}
                rows={4}
                value={content}
                onChange={(e) => setContent(e.target.value)}
              />
            </Field>
            <Field label="处理状态">
              <select
                value={status}
                onChange={(e) => setStatus(e.target.value)}
              >
                <option value="waiting">等待顾客回复</option>
                <option value="open">继续处理中</option>
                <option value="resolved">已解决</option>
              </select>
            </Field>
            {error && <Banner>{error}</Banner>}
            <button className="a-button primary" disabled={busy}>
              {busy ? "发送中…" : "发送回复"}
            </button>
          </form>
        </Modal>
      )}
    </>
  );
}
function Security({ logout }: { logout: () => void }) {
  const q = useQuery<StationDashboard>("/dashboard");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [disable, setDisable] = useState(false);
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(
    null,
  );
  const [qr, setQr] = useState("");
  const [verify, setVerify] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (setup)
      QRCode.toDataURL(setup.uri, { width: 220, margin: 1 })
        .then(setQr)
        .catch(() => setError("二维码生成失败，可使用下方密钥手动绑定"));
  }, [setup]);
  async function run(task: () => Promise<unknown>, done?: () => void) {
    setBusy(true);
    setError("");
    try {
      await task();
      done?.();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading
        title="账号安全"
        description="密码和两步验证只用于本分站，修改后需要重新登录。"
      />
      <QueryState {...q} />
      {error && <Banner>{error}</Banner>}
      {q.data && (
        <div className="station-security-grid">
          <form
            className="a-panel station-editor"
            onSubmit={(e) => {
              e.preventDefault();
              void run(
                () =>
                  stationApi("/admin/security", {
                    method: "PUT",
                    body: JSON.stringify({
                      password,
                      code,
                      new_password: newPassword,
                      disable_totp: disable,
                    }),
                  }),
                logout,
              );
            }}
          >
            <h2>修改安全设置</h2>
            <Field label="当前密码">
              <input
                required
                type="password"
                autoComplete="current-password"
                maxLength={72}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            <Field label="2FA 验证码（已开启时填写）">
              <input
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                value={code}
                onChange={(e) => setCode(e.target.value)}
              />
            </Field>
            <Field label="新密码" hint="12–72 字节。仅停用 2FA 时可留空。">
              <input
                type="password"
                autoComplete="new-password"
                minLength={12}
                maxLength={72}
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
              />
            </Field>
            {q.data.two_factor_enabled && (
              <label className="station-check">
                <input
                  type="checkbox"
                  checked={disable}
                  onChange={(e) => setDisable(e.target.checked)}
                />
                停用两步验证
              </label>
            )}
            <button
              className="a-button primary"
              disabled={busy || (!newPassword && !disable)}
            >
              {busy ? "保存中…" : "保存并重新登录"}
            </button>
          </form>
          <section className="a-panel">
            <h2>两步验证</h2>
            <Status
              value={q.data.two_factor_enabled ? "available" : "disabled"}
              label={q.data.two_factor_enabled ? "已开启" : "未开启"}
            />
            <p className="station-note">
              使用 Google Authenticator、1Password
              等验证器。先填写左侧当前密码，再生成绑定信息。
            </p>
            {!q.data.two_factor_enabled && !setup && (
              <button
                className="a-button"
                disabled={busy || !password}
                onClick={() =>
                  void run(async () =>
                    setSetup(
                      await stationApi("/admin/security/totp/setup", {
                        method: "POST",
                        body: JSON.stringify({ password, code }),
                      }),
                    ),
                  )
                }
              >
                生成绑定二维码
              </button>
            )}
            {setup && (
              <form
                className="station-editor"
                onSubmit={(e) => {
                  e.preventDefault();
                  void run(
                    () =>
                      stationApi("/admin/security/totp/enable", {
                        method: "POST",
                        body: JSON.stringify({ code: verify }),
                      }),
                    logout,
                  );
                }}
              >
                {qr && (
                  <img src={qr} className="station-qr" alt="验证器绑定二维码" />
                )}
                <p className="station-secret">{setup.secret}</p>
                <Field label="验证器当前验证码">
                  <input
                    required
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    pattern="[0-9]{6}"
                    maxLength={6}
                    value={verify}
                    onChange={(e) => setVerify(e.target.value)}
                  />
                </Field>
                <button className="a-button primary" disabled={busy}>
                  确认开启并重新登录
                </button>
              </form>
            )}
          </section>
        </div>
      )}
    </>
  );
}
export function StationActivate() {
  const { site } = useSite();
  const [token] = useState(
    () => new URLSearchParams(location.hash.slice(1)).get("token") || "",
  );
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  useEffect(() => {
    history.replaceState(null, "", location.pathname + location.search);
  }, []);
  async function submit(e: FormEvent) {
    e.preventDefault();
    if (password !== confirm) {
      setError("两次密码不一致");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await stationApi("/activate", {
        method: "POST",
        body: JSON.stringify({ token, password }),
      });
      setDone(true);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="a-root station-shell">
      <main className="station-main">
        <section className="a-panel station-login">
          <Store size={30} />
          <h1>{done ? "小铺已准备好" : "激活你的分站"}</h1>
          <p className="station-note">
            {site.site_name} · {site.station_slug}.{site.station_root}
          </p>
          {done ? (
            <>
              <p>登录密码已设置，可以开始管理店铺。</p>
              <Link className="a-button primary" to="/admin">
                进入分站后台
              </Link>
            </>
          ) : !token ? (
            <Banner>激活链接不完整，请重新打开邮件中的完整链接。</Banner>
          ) : (
            <form onSubmit={submit}>
              <Field label="设置登录密码">
                <input
                  required
                  minLength={12}
                  maxLength={72}
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </Field>
              <Field label="再次输入密码">
                <input
                  required
                  minLength={12}
                  maxLength={72}
                  type="password"
                  autoComplete="new-password"
                  value={confirm}
                  onChange={(e) => setConfirm(e.target.value)}
                />
              </Field>
              {error && <Banner>{error}</Banner>}
              <button className="a-button primary" disabled={busy}>
                {busy ? "正在激活…" : "设置密码并激活"}
              </button>
            </form>
          )}
        </section>
      </main>
    </div>
  );
}
