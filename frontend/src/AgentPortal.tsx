import { useEffect, useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import { LogOut, Copy, Moon, ArrowLeft, HandCoins } from "lucide-react";
import { SiteLogo, useSite } from "./Site";
import { api } from "./api";
import { yuan, type Agent } from "./admin/api";
import { Banner, Busy, Empty, Field, Status, shortDate } from "./admin/ui";
import "./admin/admin.css";
const key = "wybo-agent-session";
type Portal = {
  agent: Agent;
  balance: {
    available_cents: number;
    pending_cents: number;
    reserved_cents: number;
    paid_cents: number;
  };
  orders: {
    order_no: string;
    status: string;
    total_cents: number;
    commission_cents: number;
    created_at: string;
  }[];
  settlements: {
    id: number;
    amount_cents: number;
    order_count: number;
    status: string;
    reference: string;
    created_at: string;
  }[];
};
export default function AgentPortal() {
  const { site } = useSite();
  const [token, setToken] = useState(() => sessionStorage.getItem(key) || ""),
    [data, setData] = useState<Portal | null>(null),
    [code, setCode] = useState(""),
    [password, setPassword] = useState(""),
    [busy, setBusy] = useState(false),
    [loading, setLoading] = useState(false),
    [error, setError] = useState(""),
    [notice, setNotice] = useState("");

  useEffect(() => {
    if (!token) return;
    const ctrl = new AbortController();
    setLoading(true);
    api<Portal>("/agent/portal", {
      headers: { Authorization: `Bearer ${token}` },
      signal: ctrl.signal,
    })
      .then(setData)
      .catch((e) => {
        if (!ctrl.signal.aborted) {
          setError(e.message);
          sessionStorage.removeItem(key);
          setToken("");
        }
      })
      .finally(() => {
        if (!ctrl.signal.aborted) setLoading(false);
      });
    return () => ctrl.abort();
  }, [token]);
  async function login(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const r = await api<{ token: string }>("/agent/login", {
        method: "POST",
        body: JSON.stringify({ code, password }),
      });
      sessionStorage.setItem(key, r.token);
      setToken(r.token);
      setPassword("");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="a-root a-agent-portal">
      <header className="a-agent-header">
        <Link to="/" className="a-brand">
          <SiteLogo url={site.site_logo} />
          <span>
            {site.site_brand}
            <b>.</b>
            <small>伙伴工作台</small>
          </span>
        </Link>
        {token ? (
          <button
            className="a-button"
            onClick={() => {
              sessionStorage.removeItem(key);
              setToken("");
              setData(null);
              setError("");
            }}
          >
            <LogOut size={15} />
            退出登录
          </button>
        ) : (
          <Link to="/" className="a-button">
            <ArrowLeft size={15} />
            返回小铺
          </Link>
        )}
      </header>
      <main>
        {!token ? (
          <section className="a-panel a-agent-login">
            <span className="a-agent-moon">
              <Moon size={28} />
            </span>
            <h1>一起经营，一起成长</h1>
            <p>使用商家为你开通的代理码和密码登录。</p>
            {error && <Banner>{error}</Banner>}
            <form onSubmit={login}>
              <Field label="代理码">
                <input
                  required
                  autoComplete="username"
                  maxLength={32}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
              </Field>
              <Field label="工作台密码">
                <input
                  required
                  type="password"
                  autoComplete="current-password"
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                />
              </Field>
              <button className="a-button primary" disabled={busy}>
                {busy ? "正在登录…" : "进入代理工作台"}
              </button>
            </form>
          </section>
        ) : loading ? (
          <Busy />
        ) : (
          data && (
            <>
              <div className="a-page-heading">
                <div>
                  <span className="a-eyebrow">A LITTLE GROWTH, TOGETHER</span>
                  <h1>你好，{data.agent.name}</h1>
                  <p>
                    佣金比例 {data.agent.commission_bps / 100}% · 结算等待期{" "}
                    {data.agent.settlement_days} 天
                  </p>
                </div>
                <button
                  className="a-button"
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(
                        `${location.origin}/?agent=${encodeURIComponent(data.agent.code)}`,
                      );
                      setNotice("专属推广链接已复制");
                    } catch {
                      setNotice("复制失败，请使用代理码 " + data.agent.code);
                    }
                  }}
                >
                  <Copy size={15} />
                  复制推广链接
                </button>
              </div>
              {notice && <Banner success>{notice}</Banner>}
              <div className="a-metrics">
                {[
                  ["可结算佣金", data.balance.available_cents],
                  ["等待期内", data.balance.pending_cents],
                  ["待打款", data.balance.reserved_cents],
                  ["已到账佣金", data.balance.paid_cents],
                ].map(([label, value]) => (
                  <article className="a-metric" key={label}>
                    <div>
                      <span>{label}</span>
                      <HandCoins size={17} />
                    </div>
                    <strong>{yuan(Number(value))}</strong>
                  </article>
                ))}
              </div>
              <section className="a-panel">
                <div className="a-panel-heading">
                  <h2>我的推广订单</h2>
                  <p>最近 100 笔归属订单</p>
                </div>
                {data.orders.length ? (
                  <div className="a-table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>订单 / 时间</th>
                          <th>成交金额</th>
                          <th>佣金</th>
                          <th>状态</th>
                        </tr>
                      </thead>
                      <tbody>
                        {data.orders.map((o) => (
                          <tr key={o.order_no}>
                            <td>
                              {o.order_no}
                              <small>{shortDate(o.created_at)}</small>
                            </td>
                            <td>{yuan(o.total_cents)}</td>
                            <td>
                              {o.status === "refunded"
                                ? "已退回"
                                : yuan(o.commission_cents)}
                            </td>
                            <td>
                              <Status value={o.status} />
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty
                    title="等待第一份推广成果"
                    text="通过你的专属链接成交后，订单会显示在这里。"
                  />
                )}
              </section>
              <section className="a-panel a-section-gap">
                <div className="a-panel-heading">
                  <h2>我的结算记录</h2>
                </div>
                {data.settlements.length ? (
                  <div className="a-table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>结算单</th>
                          <th>金额</th>
                          <th>订单数</th>
                          <th>状态</th>
                          <th>打款凭证</th>
                        </tr>
                      </thead>
                      <tbody>
                        {data.settlements.map((s) => (
                          <tr key={s.id}>
                            <td>
                              #{s.id}
                              <small>{shortDate(s.created_at)}</small>
                            </td>
                            <td>{yuan(s.amount_cents)}</td>
                            <td>{s.order_count}</td>
                            <td>
                              <Status
                                value={s.status}
                                label={
                                  s.status === "paid" ? "已打款" : "待打款"
                                }
                              />
                            </td>
                            <td>{s.reference || "等待商家打款"}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty title="暂无结算记录" />
                )}
              </section>
            </>
          )
        )}
      </main>
      <footer className="a-footer">
        Wybo · 望月小铺 · 伙伴工作台 <Moon size={13} />
      </footer>
    </div>
  );
}
