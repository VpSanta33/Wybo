import { useEffect, useState, type FormEvent } from "react";
import {
  Check,
  CreditCard,
  ExternalLink,
  KeyRound,
  Mail,
  RefreshCw,
  Save,
  ShieldCheck,
  Webhook,
} from "lucide-react";
import QRCode from "qrcode";
import { useAccess } from "./access";
import { Modal } from "../components";
import {
  adminApi,
  useAdminQuery,
  type Gateway,
  type Notification,
  type AdminUser,
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
type GatewaySpec = {
  title: string;
  description: string;
  logo: string;
  docs: string;
  fields: {
    key: string;
    label: string;
    hint?: string;
    secret?: boolean;
    placeholder?: string;
  }[];
};
const specs: Record<string, GatewaySpec> = {
  stripe: {
    title: "Stripe",
    description: "银行卡 · 支付宝 · 微信支付，经 Stripe Checkout 收款",
    logo: "/payments/stripe.svg",
    docs: "https://docs.stripe.com/payments/checkout",
    fields: [
      {
        key: "secret_key",
        label: "Secret Key",
        secret: true,
        placeholder: "sk_live_…",
      },
      {
        key: "webhook_secret",
        label: "Webhook 签名密钥",
        secret: true,
        placeholder: "whsec_…",
      },
      {
        key: "currency",
        label: "订单收款币种",
        hint: "当前商品使用人民币，请填写 cny",
        placeholder: "cny",
      },
    ],
  },
  epay: {
    title: "易支付",
    description: "支付宝、微信等聚合收款",
    logo: "/payments/epay.svg",
    docs: "",
    fields: [
      {
        key: "api_url",
        label: "网关地址",
        placeholder: "https://pay.example.com",
      },
      { key: "pid", label: "商户 ID（PID）" },
      { key: "key", label: "商户密钥", secret: true },
      {
        key: "type",
        label: "支付类型",
        placeholder: "alipay",
        hint: "例如 alipay 或 wxpay，取决于你的服务商",
      },
    ],
  },
  dujiaopay: {
    title: "DujiaoPay",
    description: "托管收银台 · 加密货币支付",
    logo: "/payments/dujiaopay.webp",
    docs: "https://www.dujiaopay.com/docs",
    fields: [
      {
        key: "api_url",
        label: "API 地址",
        placeholder: "https://www.dujiaopay.com",
      },
      { key: "key_id", label: "DJP Key ID" },
      { key: "secret", label: "API Secret", secret: true },
      { key: "webhook_secret", label: "Webhook Secret", secret: true },
      {
        key: "chain",
        label: "指定链（可选）",
        hint: "chain 和 token_id 同时填写；留空由顾客选择",
      },
      { key: "token_id", label: "Token ID（可选）" },
    ],
  },
  bepusdt: {
    title: "BEpusdt",
    description: "USDT 等链上收款 · 自托管网关",
    logo: "/payments/bepusdt.svg",
    docs: "https://github.com/v03413/BEpusdt/blob/main/docs/api/api.md",
    fields: [
      {
        key: "api_url",
        label: "网关地址",
        placeholder: "https://usdt.example.com",
      },
      { key: "api_token", label: "API Token", secret: true },
      {
        key: "trade_type",
        label: "交易类型",
        placeholder: "usdt.trc20",
        hint: "例如 usdt.trc20，以实际网关支持为准",
      },
    ],
  },
};
export function Payments({ notify }: { notify: (m: string) => void }) {
  const { can } = useAccess();
  const q = useAdminQuery<Gateway[]>("/gateways");
  const s = useAdminQuery<Record<string, string>>(
    can("settings") ? "/settings" : "/me",
  );
  const [edit, setEdit] = useState<Gateway | null>(null);
  return (
    <>
      <PageHeading
        title="支付渠道"
        description="让每一次付款，都顺利抵达小铺。"
      />
      <div className="a-inline-note">
        <ShieldCheck size={18} />
        <p>仅启用且配置完整的渠道会显示在前台。支付成功以验签回调为准。</p>
      </div>
      {q.loading ? (
        <Busy />
      ) : q.error ? (
        <QueryError error={q.error} reload={q.reload} />
      ) : (
        <div className="a-payment-grid">
          {Object.entries(specs).map(([code, spec]) => {
            const g = q.data?.find((g) => g.code === code);
            return (
              <section className="a-panel a-payment-card" key={code}>
                <div className="a-payment-heading">
                  <ProviderLogo spec={spec} />
                  <Status
                    value={g?.enabled ? "completed" : "disabled"}
                    label={g?.enabled ? "已启用" : "未启用"}
                  />
                </div>
                <h2>{spec.title}</h2>
                <p>{spec.description}</p>
                <div className="a-payment-meta">
                  <span>订单币种</span>
                  <strong>CNY · 人民币</strong>
                </div>
                <div className="a-payment-meta">
                  <span>配置方式</span>
                  <strong>服务端密钥 · 验签通知</strong>
                </div>
                <div className="a-payment-actions">
                  <button
                    className="a-button"
                    disabled={!g || !can("payments.write")}
                    onClick={() => setEdit(g || null)}
                  >
                    配置渠道 <KeyRound size={15} />
                  </button>
                  {spec.docs && (
                    <a href={spec.docs} target="_blank" rel="noreferrer">
                      接口文档 <ExternalLink size={13} />
                    </a>
                  )}
                </div>
              </section>
            );
          })}
        </div>
      )}
      <section className="a-panel a-info-panel">
        <Webhook size={22} />
        <div>
          <h3>统一回调与跳转</h3>
          <p>
            在“邮件与推送”页面设置公网 API 回调基础地址和付款返回地址。Stripe
            Webhook 订阅
            checkout.session.completed、checkout.session.async_payment_succeeded。
          </p>
          <code>
            {s.data?.pay_notify_base || "https://你的域名/api/v1/pay/notify"}
            /渠道代码
          </code>
        </div>
      </section>
      {edit && (
        <GatewayEditor
          gateway={edit}
          notifyBase={s.data?.pay_notify_base || ""}
          onClose={() => setEdit(null)}
          onSaved={() => {
            setEdit(null);
            q.reload();
            notify("支付配置已保存");
          }}
        />
      )}
    </>
  );
}
function ProviderLogo({ spec }: { spec: GatewaySpec }) {
  return (
    <span className="a-payment-logo" aria-hidden="true">
      <img src={spec.logo} alt="" width={32} height={32} />
    </span>
  );
}
function GatewayEditor({
  gateway,
  notifyBase,
  onClose,
  onSaved,
}: {
  gateway: Gateway;
  notifyBase: string;
  onClose: () => void;
  onSaved: () => void;
}) {
  const spec = specs[gateway.code];
  const [values, setValues] = useState<Record<string, string>>(() => {
    try {
      return JSON.parse(gateway.config) || {};
    } catch {
      return {};
    }
  });
  const [enabled, setEnabled] = useState(gateway.enabled);
  const [stripeMode, setStripeMode] = useState(() =>
    !values.payment_methods || values.payment_methods === "auto"
      ? "auto"
      : "manual",
  );
  const [stripeMethods, setStripeMethods] = useState<string[]>(() =>
    !values.payment_methods || values.payment_methods === "auto"
      ? ["card"]
      : values.payment_methods.split(",").map((v) => v.trim()),
  );
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function save(e: FormEvent) {
    e.preventDefault();
    if (
      gateway.code === "stripe" &&
      stripeMode === "manual" &&
      !stripeMethods.length
    ) {
      setError("请至少选择一种 Stripe 支付方式");
      return;
    }
    setBusy(true);
    setError("");
    try {
      await adminApi(`/gateways/${gateway.code}`, {
        method: "PUT",
        body: JSON.stringify({
          enabled,
          config: JSON.stringify({
            ...values,
            ...(gateway.code === "stripe"
              ? {
                  payment_methods:
                    stripeMode === "auto" ? "auto" : stripeMethods.join(","),
                }
              : {}),
          }),
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
    <Modal title={spec.title + " 支付配置"} onClose={onClose}>
      <form className="a-editor" onSubmit={save}>
        <span className="a-eyebrow">PAYMENT PROVIDER</span>
        <div className="a-provider-title">
          <ProviderLogo spec={spec} />
          <h2>{spec.title}</h2>
        </div>
        <p>已保存的密钥以 ******** 显示；保留或留空即沿用。</p>
        {error && <Banner>{error}</Banner>}
        {[
          ...spec.fields,
          {
            key: "fee_bps",
            label: "手续费率（基点）",
            hint: "100 基点 = 1%；明确为零请填写 0。留空表示未知。",
            placeholder: "例如 290",
          },
          {
            key: "fee_fixed_cents",
            label: "每笔固定手续费（分）",
            hint: "例如 30 = ¥0.30；无固定费用请填写 0。",
            placeholder: "0",
          },
        ].map((f) => (
          <Field key={f.key} label={f.label} hint={f.hint}>
            <input
              type={f.secret ? "password" : "text"}
              autoComplete="off"
              placeholder={f.placeholder}
              value={values[f.key] || ""}
              onChange={(e) =>
                setValues({ ...values, [f.key]: e.target.value })
              }
            />
          </Field>
        ))}
        {gateway.code === "stripe" && (
          <section
            className="a-stripe-methods"
            aria-label="Stripe 支付方式设置"
          >
            <Field
              label="Checkout 支付方式"
              hint="先在 Stripe 后台开通支付宝 / 微信支付。是否展示取决于账户地区、币种及顾客条件。"
            >
              <select
                aria-label="Checkout 支付方式"
                value={stripeMode}
                onChange={(e) => setStripeMode(e.target.value)}
              >
                <option value="auto">跟随 Stripe 后台（推荐）</option>
                <option value="manual">手动选择支付方式</option>
              </select>
            </Field>
            {stripeMode === "manual" && (
              <fieldset className="a-stripe-method-choices">
                <legend>允许的支付方式</legend>
                {[
                  ["card", "银行卡"],
                  ["alipay", "支付宝"],
                  ["wechat_pay", "微信支付"],
                ].map(([code, label]) => (
                  <label className="a-check" key={code}>
                    <input
                      type="checkbox"
                      checked={stripeMethods.includes(code)}
                      onChange={(e) =>
                        setStripeMethods((current) =>
                          e.target.checked
                            ? [...current, code]
                            : current.filter((v) => v !== code),
                        )
                      }
                    />
                    {label}
                  </label>
                ))}
              </fieldset>
            )}
            <a
              href="https://dashboard.stripe.com/settings/payment_methods"
              target="_blank"
              rel="noreferrer"
            >
              前往 Stripe 开通支付方式{" "}
              <ExternalLink aria-hidden="true" size={13} />
            </a>
          </section>
        )}
        <Field label="回调地址" hint="复制到商户后台的异步通知 / Webhook 设置">
          <input
            readOnly
            value={
              notifyBase
                ? notifyBase.replace(/\/$/, "") + "/" + gateway.code
                : "请先设置公网回调基础地址"
            }
          />
        </Field>
        <label className="a-check">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          启用该支付渠道
        </label>
        <div className="a-form-actions">
          <button className="a-button" type="button" onClick={onClose}>
            取消
          </button>
          <button className="a-button primary" disabled={busy}>
            <Save size={16} />
            {busy ? "保存中…" : "保存配置"}
          </button>
        </div>
      </form>
    </Modal>
  );
}
export function Settings({ notify }: { notify: (m: string) => void }) {
  const q = useAdminQuery<Record<string, string>>("/settings");
  if (q.loading) return <Busy />;
  if (q.error) return <QueryError error={q.error} reload={q.reload} />;
  return <SettingsForm initial={q.data || {}} notify={notify} />;
}
function SettingsForm({
  initial,
  notify,
}: {
  initial: Record<string, string>;
  notify: (m: string) => void;
}) {
  const [v, setV] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const set = (k: string, value: string) => setV((s) => ({ ...s, [k]: value }));
  function input(k: string, label: string, hint?: string, secret = false) {
    return (
      <Field label={label} hint={hint}>
        <input
          type={secret ? "password" : "text"}
          autoComplete="off"
          value={v[k] || ""}
          onChange={(e) => set(k, e.target.value)}
        />
      </Field>
    );
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const allowed = [
        "site_name",
        "site_desc",
        "announcement",
        "customer_service",
        "resend_enabled",
        "resend_api_key",
        "resend_from",
        "order_push_enabled",
        "order_notify_email",
        "order_webhook_url",
        "order_webhook_secret",
        "pay_notify_base",
        "pay_return_url",
      ];
      await adminApi("/settings", {
        method: "PUT",
        body: JSON.stringify(
          Object.fromEntries(
            allowed.filter((k) => k in v).map((k) => [k, v[k]]),
          ),
        ),
      });
      notify("站点、邮件与推送配置已保存");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading title="邮件与推送" description="成交的好消息，及时送达。" />
      <form onSubmit={save} className="a-settings-form">
        {error && <Banner>{error}</Banner>}
        <section className="a-panel a-settings-panel">
          <div className="a-settings-intro">
            <i className="a-icon-blue">
              <Mail size={22} />
            </i>
            <h2>Resend 邮件</h2>
            <p>付款发货后，为顾客发送卡密邮件。</p>
            <a
              href="https://resend.com/domains"
              target="_blank"
              rel="noreferrer"
            >
              管理已验证域名 <ExternalLink size={13} />
            </a>
          </div>
          <div>
            <label className="a-switch-row">
              <span>
                <strong>启用邮件通知</strong>
                <small>需要 Resend API Key 和已验证发件域名</small>
              </span>
              <input
                type="checkbox"
                role="switch"
                checked={v.resend_enabled === "true"}
                onChange={(e) =>
                  set("resend_enabled", String(e.target.checked))
                }
              />
            </label>
            {input(
              "resend_api_key",
              "Resend API Key",
              "已保存密钥显示为 ********；留空沿用。",
              true,
            )}
            {input(
              "resend_from",
              "发件邮箱",
              "例如：望月小铺 <orders@example.com>",
            )}
          </div>
        </section>
        <section className="a-panel a-settings-panel">
          <div className="a-settings-intro">
            <i className="a-icon-violet">
              <Webhook size={22} />
            </i>
            <h2>成交订单推送</h2>
            <p>向店主发送成交金额、成本与销售利润，记录发送状态并自动重试。</p>
          </div>
          <div>
            <label className="a-switch-row">
              <span>
                <strong>启用成交推送</strong>
                <small>支持店主邮件与 Webhook，可分别配置</small>
              </span>
              <input
                type="checkbox"
                role="switch"
                checked={v.order_push_enabled === "true"}
                onChange={(e) =>
                  set("order_push_enabled", String(e.target.checked))
                }
              />
            </label>
            {input(
              "order_notify_email",
              "店主接收邮箱",
              "店主邮件需要同时启用 Resend",
            )}
            {input(
              "order_webhook_url",
              "Webhook 地址",
              "留空则不发送 Webhook；接收端应验证 X-Wybo-Signature",
            )}
            {input(
              "order_webhook_secret",
              "Webhook 签名密钥",
              "建议设置随机长密钥；留空沿用已有密钥。",
              true,
            )}
          </div>
        </section>
        <section className="a-panel a-settings-panel">
          <div className="a-settings-intro">
            <i className="a-icon-green">
              <CreditCard size={22} />
            </i>
            <h2>支付地址</h2>
            <p>用于支付平台发送回调与顾客付款后返回小铺。</p>
          </div>
          <div>
            {input(
              "pay_notify_base",
              "公网回调基础地址",
              "例如：https://shop.example.com/api/v1/pay/notify，不含渠道代码",
            )}
            {input(
              "pay_return_url",
              "付款返回地址",
              "例如：https://shop.example.com/orders",
            )}
          </div>
        </section>
        <section className="a-panel a-settings-panel">
          <div className="a-settings-intro">
            <h2>站点信息</h2>
            <p>管理前台店铺信息与公告。</p>
          </div>
          <div>
            <div className="a-form-grid">
              {input("site_name", "店铺名称")}
              {input("customer_service", "客服联系方式")}
            </div>
            {input("site_desc", "店铺简介")}
            {input("announcement", "店铺公告")}
          </div>
        </section>
        <div className="a-form-actions">
          <button className="a-button primary" disabled={busy}>
            <Save size={16} />
            {busy ? "保存中…" : "保存所有设置"}
          </button>
        </div>
      </form>
    </>
  );
}
export function Notifications({ notify }: { notify: (m: string) => void }) {
  const q = useAdminQuery<Notification[]>("/notifications");
  const [busy, setBusy] = useState(0);
  const [error, setError] = useState("");
  async function retry(id: number) {
    setBusy(id);
    setError("");
    try {
      await adminApi(`/notifications/${id}/retry`, { method: "POST" });
      q.reload();
      notify("已重新加入发送队列");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(0);
    }
  }
  const channels: Record<string, string> = {
    delivery_email: "顾客发货邮件",
    merchant_email: "店主成交邮件",
    merchant_webhook: "成交 Webhook",
  };
  return (
    <>
      <PageHeading
        title="推送记录"
        description="每一份好消息，都有送达的回音。"
      >
        <button className="a-button" onClick={q.reload}>
          <RefreshCw size={16} />
          刷新记录
        </button>
      </PageHeading>
      {error && <Banner>{error}</Banner>}
      <section className="a-panel">
        {q.loading ? (
          <Busy />
        ) : q.error ? (
          <QueryError error={q.error} reload={q.reload} />
        ) : q.data?.length ? (
          <>
            <div className="a-table-wrap">
              <table>
                <thead>
                  <tr>
                    <th>订单</th>
                    <th>推送渠道</th>
                    <th>状态</th>
                    <th>尝试次数</th>
                    <th>创建时间</th>
                    <th>结果</th>
                    <th>操作</th>
                  </tr>
                </thead>
                <tbody>
                  {q.data.map((n) => (
                    <tr key={n.id}>
                      <td>#{n.order_id}</td>
                      <td>{channels[n.channel] || n.channel}</td>
                      <td>
                        <Status
                          value={n.status}
                          label={n.status === "pending" ? "排队中" : undefined}
                        />
                      </td>
                      <td>{n.attempts} 次</td>
                      <td>{shortDate(n.created_at)}</td>
                      <td className="a-error-cell">{n.last_error || "—"}</td>
                      <td>
                        {n.status === "failed" && (
                          <button
                            className="a-table-button"
                            disabled={busy === n.id}
                            onClick={() => retry(n.id)}
                          >
                            重新发送
                          </button>
                        )}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            <div className="a-table-footer">
              最近 {q.data.length} 条推送记录 · 失败后最多自动尝试 5 次
            </div>
          </>
        ) : (
          <Empty
            title="还没有发送记录"
            text="启用邮件或成交推送后，新成交的通知会记录在这里。"
          />
        )}
      </section>
    </>
  );
}
export function Security({
  user,
  logout,
  notify,
}: {
  user: AdminUser;
  logout: () => void;
  notify: (m: string) => void;
}) {
  const [path, setPath] = useState(user.admin_path);
  const [password, setPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [repeat, setRepeat] = useState("");
  const [code, setCode] = useState("");
  const [disable, setDisable] = useState(false);
  const [setup, setSetup] = useState<{ secret: string; uri: string } | null>(
    null,
  );
  const [qr, setQr] = useState("");
  const [otp, setOtp] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    if (setup)
      QRCode.toDataURL(setup.uri, {
        width: 220,
        margin: 2,
        color: { dark: "#26344aff", light: "#ffffffff" },
      })
        .then(setQr)
        .catch(() => setQr(""));
  }, [setup]);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      if (newPassword !== repeat) throw new Error("两次输入的新密码不一致");
      const r = await adminApi<{ admin_path: string; relogin: boolean }>(
        "/security",
        {
          method: "PUT",
          body: JSON.stringify({
            password,
            code,
            new_password: newPassword,
            admin_path: user.role === "super" ? path : "",
            disable_totp: disable,
          }),
        },
      );
      notify("安全设置已保存");
      setPassword("");
      setCode("");
      if (r.admin_path !== user.admin_path)
        window.location.assign(r.admin_path + "/security");
      else if (r.relogin) logout();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function begin() {
    setBusy(true);
    setError("");
    try {
      setSetup(
        await adminApi("/security/totp/setup", {
          method: "POST",
          body: JSON.stringify({ password, code }),
        }),
      );
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function enable(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await adminApi("/security/totp/enable", {
        method: "POST",
        body: JSON.stringify({ code: otp }),
      });
      setSetup(null);
      logout();
      notify("两步验证已启用，请使用密码与验证码重新登录");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <>
      <PageHeading
        title="安全设置"
        description="给小铺多一份安心，也给经营多一份从容。"
      />
      <form className="a-settings-form" onSubmit={save}>
        {error && <Banner>{error}</Banner>}
        <section className="a-panel a-settings-panel">
          <div className="a-settings-intro">
            <i className="a-icon-blue">
              <ShieldCheck size={22} />
            </i>
            <h2>管理入口</h2>
            <p>自定义后台路径，修改后请保存新地址。</p>
          </div>
          <div>
            <Field
              label="后台访问路径"
              hint="以 / 开头，仅一层路径，3–64 位，支持字母、数字、_、-，例如 /moon-console"
            >
              <input
                disabled={user.role !== "super"}
                required
                pattern="/[A-Za-z0-9][A-Za-z0-9_-]{2,63}"
                maxLength={65}
                value={path}
                onChange={(e) => setPath(e.target.value)}
              />
            </Field>
            <p className="a-hint">
              当前入口：{window.location.origin + user.admin_path}
            </p>
          </div>
        </section>
        <section className="a-panel a-settings-panel">
          <div className="a-settings-intro">
            <i className="a-icon-violet">
              <KeyRound size={22} />
            </i>
            <h2>密码与两步验证</h2>
            <p>修改密码或停用验证器后，已有登录会失效。</p>
          </div>
          <div>
            <div className="a-form-grid">
              <Field label="新密码" hint="至少 12 位，最多 72 字节；留空不修改">
                <input
                  type="password"
                  minLength={12}
                  maxLength={72}
                  autoComplete="new-password"
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
                />
              </Field>
              <Field label="确认新密码">
                <input
                  type="password"
                  autoComplete="new-password"
                  value={repeat}
                  onChange={(e) => setRepeat(e.target.value)}
                />
              </Field>
            </div>
            <div className="a-two-factor">
              <div>
                <ShieldCheck size={23} />
                <span>
                  <strong>验证器两步登录</strong>
                  <small>
                    {user.two_factor_enabled
                      ? "已启用 · 登录需要动态验证码"
                      : "未启用 · 支持 Google Authenticator、1Password 等"}
                  </small>
                </span>
                <Status
                  value={user.two_factor_enabled ? "completed" : "disabled"}
                  label={user.two_factor_enabled ? "已保护" : "未绑定"}
                />
              </div>
              {user.two_factor_enabled ? (
                <label className="a-check">
                  <input
                    type="checkbox"
                    checked={disable}
                    onChange={(e) => setDisable(e.target.checked)}
                  />
                  停用两步验证（需要当前密码和验证码）
                </label>
              ) : (
                <button
                  type="button"
                  className="a-button"
                  disabled={busy || !password}
                  onClick={begin}
                >
                  绑定验证器 <KeyRound size={15} />
                </button>
              )}
            </div>
          </div>
        </section>
        <section className="a-panel a-settings-panel">
          <div className="a-settings-intro">
            <h2>确认身份</h2>
            <p>填写当前密码后保存安全设置，或开始绑定验证器。</p>
          </div>
          <div>
            <Field label="当前登录密码">
              <input
                required
                type="password"
                autoComplete="current-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            {user.two_factor_enabled && (
              <Field label="当前两步验证码">
                <input
                  required
                  inputMode="numeric"
                  autoComplete="one-time-code"
                  pattern="[0-9]{6}"
                  maxLength={6}
                  value={code}
                  onChange={(e) => setCode(e.target.value)}
                />
              </Field>
            )}
          </div>
        </section>
        <div className="a-form-actions">
          <button className="a-button primary" disabled={busy}>
            <Save size={16} />
            {busy ? "验证中…" : "保存安全设置"}
          </button>
        </div>
      </form>
      {setup && (
        <Modal
          title="绑定验证器"
          onClose={() => {
            setSetup(null);
            setOtp("");
          }}
        >
          <form className="a-editor a-totp" onSubmit={enable}>
            <h2>为小铺加一把小锁</h2>
            <p>
              用验证器扫描二维码，再输入生成的 6 位验证码完成绑定。此二维码 10
              分钟内有效。
            </p>
            {error && <Banner>{error}</Banner>}
            {qr && (
              <img src={qr} alt="验证器绑定二维码" width={220} height={220} />
            )}
            <Field label="无法扫码？手动输入密钥">
              <input readOnly value={setup.secret} />
            </Field>
            <Field label="验证器生成的验证码">
              <input
                autoFocus
                required
                pattern="[0-9]{6}"
                inputMode="numeric"
                autoComplete="one-time-code"
                maxLength={6}
                value={otp}
                onChange={(e) => setOtp(e.target.value)}
              />
            </Field>
            <p className="a-hint">
              请妥善备份验证器。丢失设备时，需要服务器管理员通过本地恢复工具重置。
            </p>
            <button className="a-button primary" disabled={busy}>
              <Check size={16} />
              {busy ? "验证中…" : "验证并启用"}
            </button>
          </form>
        </Modal>
      )}
    </>
  );
}
