import { useState } from "react";
import {
  Activity,
  CheckCircle2,
  ClipboardCheck,
  Plus,
  RefreshCw,
  ShieldCheck,
  Truck,
  Users,
} from "lucide-react";
import {
  adminApi,
  useAdminQuery,
  yuan,
  toCents,
  type AdminProduct,
  type Card,
} from "./api";
import { useAccess } from "./access";
import { ActionDialog, Metric, QueryPanel } from "./Operations";
import { Banner, Empty, Field, PageHeading, Status, shortDate } from "./ui";
type Props = { notify: (v: string) => void };
const post = (path: string, data: unknown = {}) =>
  adminApi(path, { method: "POST", body: JSON.stringify(data) });
const roleNames: Record<string, string> = {
  super: "店主",
  operator: "运营",
  support: "客服",
  finance: "财务",
  inventory: "库存",
};
const roleNotes: Record<string, string> = {
  super: "全部功能；管理团队、密钥及后台入口",
  operator: "经营分析、商品、营销、订单与工单；客户联系方式脱敏",
  support: "订单、工单、换卡与升级；不显示成本和利润",
  finance: "财务、对账、采购查看、退款与结算；客户联系方式脱敏",
  inventory: "库存、卡密查看、采购入库与换卡；不查看客户联系方式",
};
export function Team({ notify }: Props) {
  type Staff = {
    id: number;
    username: string;
    role: string;
    disabled: boolean;
    two_factor_enabled: boolean;
    last_login_at: string | null;
  };
  const q = useAdminQuery<Staff[]>("/staff");
  const [edit, setEdit] = useState<Staff | true | null>(null),
    [name, setName] = useState(""),
    [password, setPassword] = useState(""),
    [role, setRole] = useState("support"),
    [disabled, setDisabled] = useState(false);
  const { user } = useAccess();
  function open(v: Staff | true) {
    setEdit(v);
    setName(v === true ? "" : v.username);
    setRole(v === true ? "support" : v.role);
    setDisabled(v === true ? false : v.disabled);
    setPassword("");
  }
  return (
    <>
      <PageHeading
        eyebrow="TEAM & ACCESS"
        title="团队与权限"
        description="各司其职，让每一份数据都交给合适的人。"
      >
        <button className="a-button primary" onClick={() => open(true)}>
          <Plus aria-hidden="true" size={16} />
          添加成员
        </button>
      </PageHeading>
      <div className="a-role-grid">
        {Object.entries(roleNames).map(([k, n]) => (
          <article className="a-panel a-role-card" key={k}>
            <ShieldCheck aria-hidden="true" size={19} />
            <h3>{n}</h3>
            <p>{roleNotes[k]}</p>
          </article>
        ))}
      </div>
      <QueryPanel {...q}>
        <section className="a-panel">
          <div className="a-panel-heading">
            <h2>团队成员</h2>
            <span>角色调整与停用将立即使旧会话失效</span>
          </div>
          <div className="a-table-wrap">
            <table>
              <thead>
                <tr>
                  <th>账号</th>
                  <th>角色</th>
                  <th>两步验证</th>
                  <th>最近登录</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {q.data?.map((v) => (
                  <tr key={v.id}>
                    <td>
                      <strong>{v.username}</strong>
                      {v.id === user?.id && " · 我"}
                    </td>
                    <td>{roleNames[v.role] || v.role}</td>
                    <td>{v.two_factor_enabled ? "已绑定" : "未绑定"}</td>
                    <td>
                      {v.last_login_at
                        ? shortDate(v.last_login_at)
                        : "尚未登录"}
                    </td>
                    <td>
                      <Status
                        value={v.disabled ? "disabled" : "available"}
                        label={v.disabled ? "已停用" : "启用中"}
                      />
                    </td>
                    <td>
                      <button className="a-text-button" onClick={() => open(v)}>
                        管理
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      </QueryPanel>
      {edit && (
        <ActionDialog
          title={edit === true ? "添加团队成员" : "管理成员"}
          description="每位成员独立设置 2FA。此处不提供验证器绕过；留空密码保持原密码。"
          onClose={() => {
            setEdit(null);
            setPassword("");
          }}
          submit={async () => {
            await adminApi(edit === true ? "/staff" : `/staff/${edit.id}`, {
              method: edit === true ? "POST" : "PUT",
              body: JSON.stringify({
                username: name,
                role,
                password,
                disabled,
              }),
            });
            notify("成员设置已保存");
            q.reload();
            if (edit !== true && edit.id === user?.id)
              window.dispatchEvent(new Event("wybo-admin-expired"));
          }}
          fields={
            <>
              <Field label="成员账号">
                <input
                  required
                  disabled={edit !== true}
                  pattern="[A-Za-z0-9_-]{3,32}"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  autoComplete="off"
                />
              </Field>
              <Field label="权限角色">
                <select value={role} onChange={(e) => setRole(e.target.value)}>
                  {Object.entries(roleNames).map(([k, n]) => (
                    <option key={k} value={k}>
                      {n}
                    </option>
                  ))}
                </select>
              </Field>
              <p className="a-muted">{roleNotes[role]}</p>
              <Field
                label={edit === true ? "初始密码" : "重设密码（可选）"}
                hint="12–72 字节；通过安全方式交给成员"
              >
                <input
                  type="password"
                  minLength={12}
                  maxLength={72}
                  required={edit === true}
                  value={password}
                  onChange={(e) => setPassword(e.target.value)}
                  autoComplete="new-password"
                />
              </Field>
              <label className="a-checkbox">
                <input
                  type="checkbox"
                  checked={disabled}
                  onChange={(e) => setDisabled(e.target.checked)}
                />
                停用此成员
              </label>
            </>
          }
        />
      )}
    </>
  );
}

type ReadinessData = {
  checks: { key: string; title: string; status: string; detail: string }[];
  verified: number;
  missing: number;
  site_url: string;
  note: string;
  checked_at: string;
};
export function Readiness({ notify }: Props) {
  const q = useAdminQuery<ReadinessData>("/readiness");
  const { can } = useAccess();
  const [busy, setBusy] = useState(""),
    [error, setError] = useState(""),
    [action, setAction] = useState(""),
    [site, setSite] = useState("");
  async function run(kind: string) {
    setBusy(kind);
    setError("");
    try {
      await post("/readiness/" + kind);
      notify(
        kind === "email"
          ? "Resend 已接受测试邮件，请核对收件箱"
          : "公网 HTTPS 检测通过",
      );
      q.reload();
    } catch (e) {
      setError((e as Error).message);
      q.reload();
    } finally {
      setBusy("");
    }
  }
  return (
    <>
      <PageHeading
        eyebrow="LAUNCH CHECKLIST"
        title="上线检查"
        description="从配置到联调，每一步都有真实依据。"
      >
        <button className="a-button" onClick={q.reload}>
          <RefreshCw aria-hidden="true" size={15} />
          重新检查
        </button>
        {can("settings") && (
          <button
            className="a-button primary"
            onClick={() => {
              setSite(q.data?.site_url || "");
              setAction("site");
            }}
          >
            设置站点域名
          </button>
        )}
      </PageHeading>
      {error && <Banner>{error}</Banner>}
      <QueryPanel {...q}>
        {q.data && (
          <>
            <div className="a-metrics">
              <Metric
                label="检查项目"
                value={String(q.data.checks.length)}
                note="每次读取当前配置"
              />
              <Metric
                label="已有验证证据"
                value={String(q.data.verified)}
                note="证据有效期 7 天"
              />
              <Metric
                label="等待配置"
                value={String(q.data.missing)}
                note="先完成这些基础设置"
              />
              <Metric
                label="最近检查"
                value={new Date(q.data.checked_at).toLocaleTimeString("zh-CN", {
                  hour: "2-digit",
                  minute: "2-digit",
                })}
                note="上海时间"
              />
            </div>
            <div className="a-panel a-readiness-note">
              <ClipboardCheck aria-hidden="true" size={24} />
              <div>
                <h2>先准备，再开门</h2>
                <p>{q.data.note}</p>
              </div>
            </div>
            <section className="a-panel">
              <div className="a-check-list">
                {q.data.checks.map((v) => (
                  <article key={v.key}>
                    <span
                      className={`a-check-icon ${v.status === "verified" ? "done" : ""}`}
                    >
                      {v.status === "verified" ? (
                        <CheckCircle2 aria-hidden="true" size={20} />
                      ) : (
                        <ClipboardCheck aria-hidden="true" size={20} />
                      )}
                    </span>
                    <div>
                      <h3>{v.title}</h3>
                      <p>{v.detail}</p>
                    </div>
                    <Status
                      value={v.status}
                      label={
                        {
                          pending: "待验证",
                          verified: "已验证",
                          configured: "已配置",
                          missing: "待配置",
                        }[v.status]
                      }
                    />
                  </article>
                ))}
              </div>
            </section>
            {can("diagnostics.write") && (
              <div className="a-panel a-test-actions">
                <div>
                  <h2>主动联调</h2>
                  <p>
                    HTTPS
                    检测访问已配置的公开站点。测试邮件仅发送至已配置的商家通知邮箱。
                  </p>
                </div>
                <button
                  className="a-button"
                  disabled={!!busy}
                  onClick={() => void run("https")}
                >
                  {busy === "https" ? "检测中…" : "检测公网 HTTPS"}
                </button>
                <button
                  className="a-button"
                  disabled={!!busy}
                  onClick={() => setAction("email")}
                >
                  {busy === "email" ? "发送中…" : "发送测试邮件"}
                </button>
              </div>
            )}
          </>
        )}
      </QueryPanel>
      {action === "site" && (
        <ActionDialog
          title="设置正式站点地址"
          description="填写对外访问的完整网址。保存后可手动检测 HTTPS；支付回调请在支付设置中单独配置。"
          onClose={() => setAction("")}
          submit={async () => {
            await adminApi("/settings", {
              method: "PUT",
              body: JSON.stringify({ site_url: site }),
            });
            q.reload();
            notify("站点地址已保存");
          }}
          fields={
            <Field label="站点 URL">
              <input
                required
                type="url"
                value={site}
                onChange={(e) => setSite(e.target.value)}
                placeholder="https://shop.example.com"
              />
            </Field>
          }
        />
      )}{" "}
      {action === "email" && (
        <ActionDialog
          title="发送联调测试邮件"
          description="将通过当前 Resend 配置向商家通知邮箱发送一封真实测试邮件，可能计入服务商用量。"
          onClose={() => setAction("")}
          label="确认发送"
          fields={<p>发送成功表示 Resend 接受请求，最终投递请核对收件箱。</p>}
          submit={async () => {
            setAction("");
            await run("email");
          }}
        />
      )}
    </>
  );
}

type MonitorData = {
  alerts: {
    id: number;
    title: string;
    detail: string;
    status: string;
    notified: boolean;
    attempts: number;
    last_error: string;
    updated_at: string;
  }[];
  database_ok: boolean;
  uptime_seconds: number;
  memory_bytes: number;
  goroutines: number;
  runtime_alert_email: string;
  backup_retention_days: string;
  backup_interval_hours: string;
};
export function Runtime({ notify }: Props) {
  const q = useAdminQuery<MonitorData>("/monitor"),
    [edit, setEdit] = useState(false),
    [cleanup, setCleanup] = useState(false),
    [email, setEmail] = useState(false),
    [days, setDays] = useState("0"),
    [hours, setHours] = useState("0");
  return (
    <>
      <PageHeading
        eyebrow="RUNTIME HEALTH"
        title="运行监控"
        description="及时发现发货积压、支付异常与备份问题。"
      >
        <button className="a-button" onClick={q.reload}>
          <RefreshCw aria-hidden="true" size={15} />
          刷新状态
        </button>
        <button
          className="a-button primary"
          onClick={() => {
            setEmail(q.data?.runtime_alert_email === "true");
            setDays(q.data?.backup_retention_days || "0");
            setHours(q.data?.backup_interval_hours || "0");
            setEdit(true);
          }}
        >
          告警与保留策略
        </button>
      </PageHeading>
      <QueryPanel {...q}>
        {q.data && (
          <>
            <div className="a-metrics">
              <Metric
                label="数据库连接"
                value={q.data.database_ok ? "正常" : "异常"}
                note="服务端即时连通性"
              />
              <Metric
                label="连续运行"
                value={`${Math.floor(q.data.uptime_seconds / 3600)} 小时`}
                note={`${Math.floor(q.data.uptime_seconds / 60)} 分钟`}
              />
              <Metric
                label="Go 已分配内存"
                value={`${(q.data.memory_bytes / 1048576).toFixed(1)} MB`}
                note={`${q.data.goroutines} 个协程`}
              />
              <Metric
                label="待处理告警"
                value={String(
                  q.data.alerts.filter((v) => v.status === "open").length,
                )}
                note={
                  q.data.runtime_alert_email === "true"
                    ? "邮件告警已开启"
                    : "邮件告警未开启"
                }
              />
            </div>
            <section className="a-panel">
              <div className="a-panel-heading">
                <h2>运行告警</h2>
                <span>异常恢复后自动关闭，复发重新通知</span>
              </div>
              {q.data.alerts.length ? (
                <div className="a-check-list">
                  {q.data.alerts.map((v) => (
                    <article key={v.id}>
                      <Activity aria-hidden="true" size={20} />
                      <div>
                        <h3>{v.title}</h3>
                        <p>{v.detail}</p>
                        <small>
                          {shortDate(v.updated_at)} ·{" "}
                          {v.notified ? "已推送" : `推送尝试 ${v.attempts}/5`}
                          {v.last_error && ` · ${v.last_error}`}
                        </small>
                      </div>
                      <Status value={v.status} />
                    </article>
                  ))}
                </div>
              ) : (
                <Empty title="小铺运行平稳" text="发现异常时会在这里显示。" />
              )}
            </section>
            <section className="a-panel a-test-actions">
              <div>
                <h2>备份保留</h2>
                <p>
                  {q.data.backup_interval_hours === "0"
                    ? "定时备份未开启"
                    : `每 ${q.data.backup_interval_hours} 小时备份`}{" "}
                  ·{" "}
                  {q.data.backup_retention_days === "0"
                    ? "自动清理未开启"
                    : `保留 ${q.data.backup_retention_days} 天`}
                  。仅清理过期定时备份，始终保留最近 3
                  份有效定时快照，手动和开发安全备份保留。
                </p>
              </div>
              <button
                className="a-button"
                disabled={q.data.backup_retention_days === "0"}
                onClick={() => setCleanup(true)}
              >
                按策略清理
              </button>
            </section>
          </>
        )}
      </QueryPanel>
      {edit && (
        <ActionDialog
          title="告警与保留策略"
          description="后台每分钟检查。邮件告警复用已配置的 Resend 和商家通知邮箱，同一轮异常最多重试 5 次。"
          onClose={() => setEdit(false)}
          submit={async () => {
            await adminApi("/monitor", {
              method: "PUT",
              body: JSON.stringify({
                runtime_alert_email: String(email),
                backup_retention_days: days,
                backup_interval_hours: hours,
              }),
            });
            q.reload();
            notify("运行策略已保存");
          }}
          fields={
            <>
              <Field label="定时备份间隔（小时）" hint="0 表示关闭，1–720 小时">
                <input
                  type="number"
                  min="0"
                  max="720"
                  required
                  value={hours}
                  onChange={(e) => setHours(e.target.value)}
                />
              </Field>
              <Field
                label="定时备份保留天数"
                hint="0 表示不自动清理，最多 3650 天"
              >
                <input
                  type="number"
                  min="0"
                  max="3650"
                  required
                  value={days}
                  onChange={(e) => setDays(e.target.value)}
                />
              </Field>
              <label className="a-checkbox">
                <input
                  type="checkbox"
                  checked={email}
                  onChange={(e) => setEmail(e.target.checked)}
                />
                通过 Resend 推送运行告警
              </label>
            </>
          }
        />
      )}{" "}
      {cleanup && (
        <ActionDialog
          title="执行备份保留策略"
          description="将永久删除符合当前策略的过期定时快照。手动快照和最近 3 份校验通过的定时快照将保留。"
          label="确认清理"
          onClose={() => setCleanup(false)}
          fields={<p>当前保留天数：{q.data?.backup_retention_days}</p>}
          submit={async () => {
            const r = (await post("/monitor/cleanup")) as { deleted: number };
            notify(`已清理 ${r.deleted} 份过期定时备份`);
            q.reload();
          }}
        />
      )}
    </>
  );
}

type Supplier = {
  id: number;
  name: string;
  contact: string;
  note: string;
  enabled: boolean;
};
type Purchase = {
  id: number;
  number: string;
  supplier_id: number;
  product_id: number;
  quantity: number;
  unit_cost_cents: number;
  paid_cents: number;
  status: string;
  batch_no: string;
  created_at: string;
};
type ProcurementData = {
  suppliers: Supplier[];
  purchases: Purchase[];
  payments: {
    id: number;
    purchase_id: number;
    amount_cents: number;
    reference: string;
    created_at: string;
  }[];
  stats: {
    supplier_id: number;
    issued_cards: number;
    replacement_cards: number;
    revenue_cents: number;
    cost_cents: number;
    gross_profit_cents: number;
  }[];
  basis: string;
};
export function Procurement({ notify }: Props) {
  const q = useAdminQuery<ProcurementData>("/purchases"),
    products = useAdminQuery<{ list: AdminProduct[] }>("/products?size=100");
  const { can } = useAccess();
  const write = can("purchases.write");
  const [tab, setTab] = useState("purchases"),
    [action, setAction] = useState(""),
    [selected, setSelected] = useState<Purchase | null>(null),
    [supplier, setSupplier] = useState<Supplier | null>(null),
    [name, setName] = useState(""),
    [contact, setContact] = useState(""),
    [note, setNote] = useState(""),
    [enabled, setEnabled] = useState(true),
    [supplierID, setSupplierID] = useState(""),
    [productID, setProductID] = useState(""),
    [qty, setQty] = useState("1"),
    [amount, setAmount] = useState(""),
    [reference, setReference] = useState(""),
    [lines, setLines] = useState(""),
    [confirm, setConfirm] = useState(false);
  function open(kind: string, p?: Purchase) {
    setAction(kind);
    setSelected(p || null);
    setAmount("");
    setReference("");
    setLines("");
    setConfirm(false);
    setNote("");
    if (kind === "supplier") {
      setSupplier(null);
      setName("");
      setContact("");
      setEnabled(true);
    }
    if (kind === "purchase") {
      setSupplierID(String(q.data?.suppliers.find((v) => v.enabled)?.id || ""));
      setProductID(String(products.data?.list[0]?.id || ""));
      setQty("1");
    }
  }
  const productName = (id: number) =>
    products.data?.list.find((v) => v.id === id)?.name || `商品 #${id}`;
  const supplierName = (id: number) =>
    q.data?.suppliers.find((v) => v.id === id)?.name || `供应商 #${id}`;
  return (
    <>
      <PageHeading
        eyebrow="SUPPLY & PROCUREMENT"
        title="供应商与采购"
        description="从采购付款到卡密入库，让每一批成本有据可查。"
      >
        {write && (
          <>
            <button className="a-button" onClick={() => open("supplier")}>
              <Users aria-hidden="true" size={15} />
              添加供应商
            </button>
            <button
              className="a-button primary"
              onClick={() => open("purchase")}
            >
              <Plus aria-hidden="true" size={16} />
              新建采购单
            </button>
          </>
        )}
      </PageHeading>
      <div className="a-tabs" role="group" aria-label="采购视图">
        {[
          ["purchases", "采购订单"],
          ["suppliers", "供应商表现"],
          ["payments", "付款记录"],
        ].map(([k, n]) => (
          <button
            key={k}
            aria-pressed={tab === k}
            className={tab === k ? "active" : ""}
            onClick={() => setTab(k)}
          >
            {n}
          </button>
        ))}
      </div>
      <QueryPanel {...q}>
        {q.data && (
          <>
            <div className="a-metrics">
              <Metric
                label="采购总额（最近 200 单）"
                value={yuan(
                  q.data.purchases
                    .filter((p) => p.status !== "canceled")
                    .reduce((n, p) => n + p.quantity * p.unit_cost_cents, 0),
                )}
              />
              <Metric
                label="已登记付款"
                value={yuan(
                  q.data.purchases.reduce((n, p) => n + p.paid_cents, 0),
                )}
              />
              <Metric
                label="等待入库"
                value={String(
                  q.data.purchases.filter((p) => p.status === "ordered").length,
                )}
              />
              <Metric
                label="供应商"
                value={String(q.data.suppliers.filter((s) => s.enabled).length)}
                note="启用中的合作伙伴"
              />
            </div>
            <section className="a-panel">
              {tab === "purchases" ? (
                q.data.purchases.length ? (
                  <div className="a-table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>采购单</th>
                          <th>供应商 / 商品</th>
                          <th>数量 / 单位成本</th>
                          <th>应付 / 已付</th>
                          <th>状态</th>
                          <th>操作</th>
                        </tr>
                      </thead>
                      <tbody>
                        {q.data.purchases.map((p) => (
                          <tr key={p.id}>
                            <td>
                              <strong className="a-mono">{p.number}</strong>
                              <small>{shortDate(p.created_at)}</small>
                            </td>
                            <td>
                              {supplierName(p.supplier_id)}
                              <small>{productName(p.product_id)}</small>
                            </td>
                            <td>
                              {p.quantity} 份
                              <small>{yuan(p.unit_cost_cents)} / 份</small>
                            </td>
                            <td>
                              {yuan(p.quantity * p.unit_cost_cents)}
                              <small>已付 {yuan(p.paid_cents)}</small>
                            </td>
                            <td>
                              <Status value={p.status} />
                              {p.batch_no && <small>{p.batch_no}</small>}
                            </td>
                            <td>
                              {write && p.status !== "canceled" && (
                                <div className="a-row-actions">
                                  {p.paid_cents <
                                    p.quantity * p.unit_cost_cents && (
                                    <button
                                      className="a-text-button"
                                      onClick={() => open("payment", p)}
                                    >
                                      登记付款
                                    </button>
                                  )}
                                  {p.status === "ordered" && (
                                    <button
                                      className="a-text-button"
                                      onClick={() => open("receive", p)}
                                    >
                                      卡密入库
                                    </button>
                                  )}
                                  {p.status === "ordered" &&
                                    p.paid_cents === 0 && (
                                      <button
                                        className="a-text-button danger"
                                        onClick={() => open("cancel", p)}
                                      >
                                        取消采购
                                      </button>
                                    )}
                                </div>
                              )}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty
                    title="还没有采购单"
                    text="先添加供应商，再建立第一笔采购。"
                  />
                )
              ) : tab === "suppliers" ? (
                q.data.suppliers.length ? (
                  <>
                    <div className="a-table-wrap">
                      <table>
                        <thead>
                          <tr>
                            <th>供应商</th>
                            <th>已发卡 / 换出卡</th>
                            <th>分配收入</th>
                            <th>已发卡成本</th>
                            <th>分配毛利</th>
                            <th>操作</th>
                          </tr>
                        </thead>
                        <tbody>
                          {q.data.suppliers.map((s) => {
                            const stat = q.data!.stats.find(
                              (v) => v.supplier_id === s.id,
                            );
                            return (
                              <tr key={s.id}>
                                <td>
                                  <strong>{s.name}</strong>
                                  <small>
                                    {s.contact || "未填写联系信息"} ·{" "}
                                    {s.enabled ? "合作中" : "已停用"}
                                  </small>
                                </td>
                                <td>
                                  {stat?.issued_cards || 0} /{" "}
                                  {stat?.replacement_cards || 0}
                                </td>
                                <td>{yuan(stat?.revenue_cents || 0)}</td>
                                <td>{yuan(stat?.cost_cents || 0)}</td>
                                <td>{yuan(stat?.gross_profit_cents || 0)}</td>
                                <td>
                                  {write && (
                                    <button
                                      className="a-text-button"
                                      onClick={() => {
                                        setSupplier(s);
                                        setName(s.name);
                                        setContact(s.contact);
                                        setNote(s.note);
                                        setEnabled(s.enabled);
                                        setAction("supplier");
                                      }}
                                    >
                                      编辑
                                    </button>
                                  )}
                                </td>
                              </tr>
                            );
                          })}
                        </tbody>
                      </table>
                    </div>
                    <p className="a-section-note">{q.data.basis}</p>
                  </>
                ) : (
                  <Empty
                    title="还没有供应商"
                    text="供应商归属会随采购批次保留。"
                  />
                )
              ) : q.data.payments.length ? (
                <div className="a-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>采购单</th>
                        <th>实际付款</th>
                        <th>凭证 / 流水</th>
                        <th>登记时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {q.data.payments.map((p) => (
                        <tr key={p.id}>
                          <td>
                            {q.data!.purchases.find(
                              (v) => v.id === p.purchase_id,
                            )?.number || `#${p.purchase_id}`}
                          </td>
                          <td>{yuan(p.amount_cents)}</td>
                          <td>{p.reference}</td>
                          <td>{shortDate(p.created_at)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <Empty
                  title="暂无付款记录"
                  text="系统记录外部已完成的付款，不发起银行转账。"
                />
              )}
            </section>
          </>
        )}
      </QueryPanel>
      {action && (
        <ActionDialog
          title={
            {
              supplier: supplier ? "编辑供应商" : "添加供应商",
              purchase: "新建采购单",
              payment: "登记采购付款",
              receive: "采购卡密入库",
              cancel: "取消采购单",
            }[action] || ""
          }
          description={
            {
              supplier: "保留合作资料；停用后不能新增采购。",
              purchase: "采购单建立后按整批入库，单位成本会写入每张卡密。",
              payment: "请先在外部付款，再登记实际金额与唯一流水号。",
              receive: `本次须一次性导入 ${selected?.quantity} 张；空行忽略，重复卡密会整批回滚。`,
              cancel: "仅未付款、未入库的采购单可以取消。",
            }[action] || ""
          }
          onClose={() => setAction("")}
          submit={async () => {
            if (action === "supplier") {
              await adminApi(
                supplier ? `/suppliers/${supplier.id}` : "/suppliers",
                {
                  method: supplier ? "PUT" : "POST",
                  body: JSON.stringify({ name, contact, note, enabled }),
                },
              );
            } else if (action === "purchase") {
              await post("/purchases", {
                supplier_id: Number(supplierID),
                product_id: Number(productID),
                quantity: Number(qty),
                unit_cost_cents: toCents(amount),
                note,
              });
            } else if (action === "payment") {
              await post(`/purchases/${selected!.id}/payment`, {
                amount_cents: toCents(amount),
                reference,
                confirm,
              });
            } else if (action === "receive") {
              await post(`/purchases/${selected!.id}/receive`, { lines });
            } else {
              await post(`/purchases/${selected!.id}/cancel`);
            }
            q.reload();
            notify("采购记录已更新");
          }}
          fields={
            action === "supplier" ? (
              <>
                <Field label="供应商名称">
                  <input
                    required
                    maxLength={80}
                    value={name}
                    onChange={(e) => setName(e.target.value)}
                  />
                </Field>
                <Field label="联系方式">
                  <input
                    maxLength={128}
                    value={contact}
                    onChange={(e) => setContact(e.target.value)}
                  />
                </Field>
                <Field label="合作备注">
                  <textarea
                    maxLength={500}
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                  />
                </Field>
                <label className="a-checkbox">
                  <input
                    type="checkbox"
                    checked={enabled}
                    onChange={(e) => setEnabled(e.target.checked)}
                  />
                  启用供应商
                </label>
              </>
            ) : action === "purchase" ? (
              <>
                <Field label="供应商">
                  <select
                    required
                    value={supplierID}
                    onChange={(e) => setSupplierID(e.target.value)}
                  >
                    <option value="">选择供应商</option>
                    {q.data?.suppliers
                      .filter((s) => s.enabled)
                      .map((s) => (
                        <option key={s.id} value={s.id}>
                          {s.name}
                        </option>
                      ))}
                  </select>
                </Field>
                <Field label="采购商品">
                  <select
                    required
                    value={productID}
                    onChange={(e) => setProductID(e.target.value)}
                  >
                    <option value="">选择商品</option>
                    {products.data?.list.map((p) => (
                      <option key={p.id} value={p.id}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                </Field>
                <div className="a-form-grid">
                  <Field label="数量">
                    <input
                      required
                      type="number"
                      min="1"
                      max="10000"
                      value={qty}
                      onChange={(e) => setQty(e.target.value)}
                    />
                  </Field>
                  <Field label="单位成本（元）">
                    <input
                      required
                      inputMode="decimal"
                      value={amount}
                      onChange={(e) => setAmount(e.target.value)}
                    />
                  </Field>
                </div>
                <Field label="采购备注">
                  <textarea
                    maxLength={500}
                    value={note}
                    onChange={(e) => setNote(e.target.value)}
                  />
                </Field>
              </>
            ) : action === "payment" ? (
              <>
                <Field
                  label="本次实际付款（元）"
                  hint={`剩余应付 ${yuan(selected!.quantity * selected!.unit_cost_cents - selected!.paid_cents)}`}
                >
                  <input
                    required
                    inputMode="decimal"
                    value={amount}
                    onChange={(e) => setAmount(e.target.value)}
                  />
                </Field>
                <Field label="付款流水 / 凭证">
                  <input
                    required
                    minLength={4}
                    maxLength={200}
                    value={reference}
                    onChange={(e) => setReference(e.target.value)}
                  />
                </Field>
                <label className="a-checkbox">
                  <input
                    required
                    type="checkbox"
                    checked={confirm}
                    onChange={(e) => setConfirm(e.target.checked)}
                  />
                  我已在外部完成此笔付款
                </label>
              </>
            ) : action === "receive" ? (
              <Field
                label="卡密内容"
                hint="每行一张；账号密码使用 账号----密码 格式"
              >
                <textarea
                  rows={9}
                  required
                  spellCheck={false}
                  value={lines}
                  onChange={(e) => setLines(e.target.value)}
                />
              </Field>
            ) : (
              <p>采购单 {selected?.number}</p>
            )
          }
        />
      )}
    </>
  );
}

type AfterSale = {
  id: number;
  number: string;
  order_no: string;
  kind: string;
  status: string;
  original_card_id: number | null;
  new_card_id: number | null;
  child_order_no: string;
  amount_cents: number;
  reason: string;
  reference: string;
  created_at: string;
};
export function AfterSales({ notify }: Props) {
  const q = useAdminQuery<AfterSale[]>("/after-sales"),
    products = useAdminQuery<{ list: AdminProduct[] }>("/products?size=100");
  const { can } = useAccess();
  const [action, setAction] = useState(""),
    [no, setNo] = useState(""),
    [reason, setReason] = useState(""),
    [cardID, setCardID] = useState(""),
    [productID, setProductID] = useState(""),
    [amount, setAmount] = useState(""),
    [reference, setReference] = useState(""),
    [confirm, setConfirm] = useState(false),
    [cards, setCards] = useState<Card[]>([]),
    [lookup, setLookup] = useState(false),
    [lookupError, setLookupError] = useState("");
  function open(kind: string) {
    setAction(kind);
    setNo("");
    setReason("");
    setCards([]);
    setCardID("");
    setProductID("");
    setAmount("");
    setReference("");
    setConfirm(false);
    setLookupError("");
  }
  async function load() {
    setLookup(true);
    setLookupError("");
    try {
      const v = await adminApi<{ cards: Card[] }>(
        `/orders/${encodeURIComponent(no.trim())}`,
      );
      setCards(v.cards || []);
      setCardID(String(v.cards?.[0]?.id || ""));
      if (!v.cards?.length) setLookupError("该订单没有当前有效的已发卡密");
    } catch (e) {
      setLookupError((e as Error).message);
    } finally {
      setLookup(false);
    }
  }
  return (
    <>
      <PageHeading
        eyebrow="AFTER-SALES CARE"
        title="售后处理"
        description="换卡、升级与退款，让每一笔售后都能追溯。"
      >
        {can("aftersales.replace") && (
          <>
            <button className="a-button" onClick={() => open("replace")}>
              换发卡密
            </button>
            <button
              className="a-button primary"
              onClick={() => open("upgrade")}
            >
              创建升级补差
            </button>
          </>
        )}
        {can("aftersales.refund") && (
          <button className="a-button" onClick={() => open("refund")}>
            登记已发货退款
          </button>
        )}
      </PageHeading>
      <div className="a-panel a-readiness-note">
        <Truck aria-hidden="true" size={24} />
        <div>
          <h2>成本持续记录</h2>
          <p>
            换卡累计原卡与新卡成本；升级先收取补差价，发出新卡后替换原卡。已发货退款保留实际商品成本与已打款佣金。
          </p>
        </div>
      </div>
      <QueryPanel {...q}>
        <section className="a-panel">
          {q.data?.length ? (
            <div className="a-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>售后 / 原订单</th>
                    <th>类型</th>
                    <th>处理内容</th>
                    <th>原因 / 凭证</th>
                    <th>状态</th>
                    <th>时间</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.map((v) => (
                    <tr key={v.id}>
                      <td>
                        <strong className="a-mono">{v.number}</strong>
                        <small>{v.order_no}</small>
                      </td>
                      <td>
                        {
                          {
                            replacement: "换卡",
                            upgrade: "升级补差",
                            refund: "售后退款",
                          }[v.kind]
                        }
                      </td>
                      <td>
                        {v.kind === "replacement"
                          ? `#${v.original_card_id} → #${v.new_card_id}`
                          : yuan(v.amount_cents)}
                        {v.child_order_no && (
                          <small>补差订单 {v.child_order_no}</small>
                        )}
                      </td>
                      <td>
                        {v.reason}
                        {v.reference && <small>凭证：{v.reference}</small>}
                      </td>
                      <td>
                        <Status
                          value={v.status}
                          label={
                            v.status === "waiting" ? "等待补差付款" : undefined
                          }
                        />
                      </td>
                      <td>{shortDate(v.created_at)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          ) : (
            <Empty
              title="每一份售后，都被认真对待"
              text="创建换卡、补差或退款登记后，会在这里留下记录。"
            />
          )}
        </section>
      </QueryPanel>
      {action && (
        <ActionDialog
          title={
            {
              replace: "换发同商品卡密",
              upgrade: "创建升级补差订单",
              refund: "登记已发货全额退款",
            }[action] || ""
          }
          description={
            {
              replace:
                "原卡标记为已替换，客户查询会显示新卡；原卡与新卡成本均保留。",
              upgrade:
                "支持单份订单。填写已与客户约定的补差金额，新订单付款后才替换原卡。",
              refund:
                "请先在支付渠道完成全额退款。此操作只登记凭证，不向渠道发起退款。",
            }[action] || ""
          }
          onClose={() => setAction("")}
          submit={async () => {
            if (action === "upgrade") {
              const v = (await post("/after-sales/upgrade", {
                order_no: no.trim(),
                product_id: Number(productID),
                amount_cents: toCents(amount),
                reason,
              })) as { order_no: string };
              notify(
                `补差订单 ${v.order_no} 已创建，客户可用原联系方式查询付款`,
              );
            } else if (action === "replace") {
              await post("/after-sales/replace", {
                order_no: no.trim(),
                card_id: Number(cardID),
                reason,
              });
              notify("已换发新卡，客户查询页已更新");
            } else {
              await post("/after-sales/refund", {
                order_no: no.trim(),
                reference,
                reason,
                confirm,
              });
              notify("售后退款已登记，成本与已打款佣金保留");
            }
            q.reload();
          }}
          fields={
            <>
              <Field label="原订单号">
                <input
                  required
                  value={no}
                  onChange={(e) => {
                    setNo(e.target.value);
                    setCards([]);
                    setCardID("");
                  }}
                />
              </Field>
              {action === "replace" && (
                <>
                  <button
                    type="button"
                    className="a-button"
                    disabled={lookup || !no}
                    onClick={() => void load()}
                  >
                    {lookup ? "核对中…" : "读取订单卡密编号"}
                  </button>
                  {lookupError && <Banner>{lookupError}</Banner>}
                  <Field label="要替换的卡密">
                    <select
                      required
                      value={cardID}
                      onChange={(e) => setCardID(e.target.value)}
                    >
                      <option value="">先读取订单卡密</option>
                      {cards.map((c) => (
                        <option key={c.id} value={c.id}>
                          卡密 #{c.id} · 商品 #{c.product_id}
                        </option>
                      ))}
                    </select>
                  </Field>
                </>
              )}
              {action === "upgrade" && (
                <>
                  <Field label="升级到">
                    <select
                      required
                      value={productID}
                      onChange={(e) => setProductID(e.target.value)}
                    >
                      <option value="">选择已定价商品</option>
                      {products.data?.list
                        .filter((p) => p.enabled && p.price_cents > 0)
                        .map((p) => (
                          <option key={p.id} value={p.id}>
                            {p.name}
                          </option>
                        ))}
                    </select>
                  </Field>
                  <Field label="约定补差金额（元）">
                    <input
                      required
                      inputMode="decimal"
                      value={amount}
                      onChange={(e) => setAmount(e.target.value)}
                    />
                  </Field>
                </>
              )}
              <Field label="处理原因">
                <textarea
                  required
                  minLength={2}
                  maxLength={500}
                  value={reason}
                  onChange={(e) => setReason(e.target.value)}
                />
              </Field>
              {action === "refund" && (
                <>
                  <Field label="实际全额退款流水 / 凭证">
                    <input
                      required
                      minLength={4}
                      maxLength={200}
                      value={reference}
                      onChange={(e) => setReference(e.target.value)}
                    />
                  </Field>
                  <label className="a-checkbox">
                    <input
                      required
                      type="checkbox"
                      checked={confirm}
                      onChange={(e) => setConfirm(e.target.checked)}
                    />
                    已在渠道完成全额退款，并核对凭证
                  </label>
                </>
              )}
            </>
          }
        />
      )}
    </>
  );
}

type FunnelRow = {
  source: string;
  campaign: string;
  visits: number;
  selected: number;
  ordered: number;
  paid: number;
  revenue_cents: number;
};
type FunnelData = {
  total: FunnelRow;
  sources: FunnelRow[];
  agents: {
    name: string;
    orders: number;
    paid: number;
    revenue_cents: number;
  }[];
  abandoned: {
    order_no: string;
    status: string;
    total_cents: number;
    source: string;
    campaign: string;
    created_at: string;
  }[];
  unattributed_orders: number;
  basis: string;
};
export function Conversion() {
  const [days, setDays] = useState(30),
    [tab, setTab] = useState("source");
  const q = useAdminQuery<FunnelData>(`/analytics/funnel?days=${days}`);
  const rate = (n: number, d: number) =>
    d ? `${((n / d) * 100).toFixed(1)}%` : "—";
  return (
    <>
      <PageHeading
        eyebrow="CONVERSION INSIGHTS"
        title="转化分析"
        description="从第一次浏览，到每一笔真实成交。"
      >
        <select
          aria-label="统计时间范围"
          value={days}
          onChange={(e) => setDays(Number(e.target.value))}
        >
          <option value="7">最近 7 天</option>
          <option value="30">最近 30 天</option>
          <option value="90">最近 90 天</option>
        </select>
        <button className="a-button" onClick={q.reload}>
          <RefreshCw aria-hidden="true" size={15} />
          刷新
        </button>
      </PageHeading>
      <QueryPanel {...q}>
        {q.data && (
          <>
            <div className="a-funnel">
              {[
                ["访问会话", q.data.total.visits],
                ["选购商品", q.data.total.selected],
                ["创建订单", q.data.total.ordered],
                ["完成付款", q.data.total.paid],
              ].map(([n, v], i, all) => (
                <article className="a-panel" key={n}>
                  <span>
                    0{i + 1} · {n}
                  </span>
                  <strong>{v}</strong>
                  <div className="a-funnel-track">
                    <i
                      style={{
                        width: `${Number(v) && q.data!.total.visits ? (Number(v) / q.data!.total.visits) * 100 : 0}%`,
                      }}
                    />
                  </div>
                  <small>
                    {i === 0
                      ? "每日浏览器会话去重"
                      : `上一步转化 ${rate(Number(v), Number(all[i - 1][1]))}`}
                  </small>
                </article>
              ))}
            </div>
            <p className="a-section-note">
              {q.data.basis} · {q.data.unattributed_orders}{" "}
              笔订单未进入完整漏斗。
            </p>
            <div className="a-tabs" role="group" aria-label="转化视图">
              {[
                ["source", "来源与活动"],
                ["agents", "代理成交"],
                ["abandoned", "未付款订单"],
              ].map(([k, n]) => (
                <button
                  key={k}
                  aria-pressed={tab === k}
                  className={tab === k ? "active" : ""}
                  onClick={() => setTab(k)}
                >
                  {n}
                </button>
              ))}
            </div>
            <section className="a-panel">
              {tab === "source" ? (
                q.data.sources.length ? (
                  <div className="a-table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>访问来源 / 活动</th>
                          <th>访问</th>
                          <th>选购</th>
                          <th>下单</th>
                          <th>付款</th>
                          <th>成交转化</th>
                          <th>净收款</th>
                        </tr>
                      </thead>
                      <tbody>
                        {[...q.data.sources]
                          .sort((a, b) => b.visits - a.visits)
                          .map((r, i) => (
                            <tr key={i}>
                              <td>
                                <strong>
                                  {r.source === "direct"
                                    ? "直接访问"
                                    : r.source}
                                </strong>
                                <small>{r.campaign || "无活动标签"}</small>
                              </td>
                              <td>{r.visits}</td>
                              <td>{r.selected}</td>
                              <td>{r.ordered}</td>
                              <td>{r.paid}</td>
                              <td>{rate(r.paid, r.visits)}</td>
                              <td>{yuan(r.revenue_cents)}</td>
                            </tr>
                          ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty
                    title="等待第一位访客"
                    text="统计启用后的真实访问将在这里显示。"
                  />
                )
              ) : tab === "agents" ? (
                q.data.agents.length ? (
                  <div className="a-table-wrap">
                    <table>
                      <thead>
                        <tr>
                          <th>代理</th>
                          <th>下单数</th>
                          <th>付款数</th>
                          <th>付款率</th>
                          <th>净收款</th>
                        </tr>
                      </thead>
                      <tbody>
                        {q.data.agents.map((r, i) => (
                          <tr key={i}>
                            <td>{r.name}</td>
                            <td>{r.orders}</td>
                            <td>{r.paid}</td>
                            <td>{rate(r.paid, r.orders)}</td>
                            <td>{yuan(r.revenue_cents)}</td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  </div>
                ) : (
                  <Empty
                    title="暂无代理成交"
                    text="按订单代理归属统计，不需要完整漏斗记录。"
                  />
                )
              ) : q.data.abandoned.length ? (
                <div className="a-table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th>未付款订单</th>
                        <th>状态</th>
                        <th>金额</th>
                        <th>来源 / 活动</th>
                        <th>创建时间</th>
                      </tr>
                    </thead>
                    <tbody>
                      {[...q.data.abandoned].reverse().map((r) => (
                        <tr key={r.order_no}>
                          <td className="a-mono">{r.order_no}</td>
                          <td>
                            <Status value={r.status} />
                          </td>
                          <td>{yuan(r.total_cents)}</td>
                          <td>
                            {r.source || "未归因"}
                            <small>{r.campaign || "—"}</small>
                          </td>
                          <td>{shortDate(r.created_at)}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              ) : (
                <Empty
                  title="暂无未付款订单"
                  text="最多展示当前时间范围内最近 100 笔待付款或已关闭订单。"
                />
              )}
            </section>
          </>
        )}
      </QueryPanel>
    </>
  );
}
