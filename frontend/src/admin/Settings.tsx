import { useEffect, useState, type FormEvent } from "react";
import {
  Check,
  ExternalLink,
  KeyRound,
  RefreshCw,
  Save,
  ShieldCheck,
  Upload,
  Webhook,
} from "lucide-react";
import QRCode from "qrcode";
import { useAccess } from "./access";
import { Modal, PaymentLogo } from "../components";
import { defaultPaymentName, gatewayValues } from "../payments";
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
  docs: string;
  fields: {
    key: string;
    label: string;
    hint?: string;
    secret?: boolean;
    placeholder?: string;
    options?: { value: string; label: string }[];
  }[];
};
const specs: Record<string, GatewaySpec> = {
  stripe: {
    title: "Stripe",
    description: "银行卡 · 支付宝 · 微信支付，经 Stripe Checkout 收款",
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
    description: "ZPAY / 易支付 · 支付宝、微信聚合收款",
    docs: "",
    fields: [
      {
        key: "api_url",
        label: "网关地址",
        placeholder: "https://zpayz.cn",
        hint: "填写商户平台的网关地址，也可粘贴 submit.php 或 mapi.php 完整地址。",
      },
      { key: "pid", label: "商户 ID（PID）" },
      { key: "key", label: "商户密钥", secret: true },
      {
        key: "type",
        label: "支付类型",
        placeholder: "alipay",
        hint: "例如 alipay 或 wxpay，取决于你的服务商",
        options: [
          { value: "alipay", label: "支付宝" },
          { value: "wxpay", label: "微信支付" },
          { value: "qqpay", label: "QQ 钱包（需服务商支持）" },
          { value: "tenpay", label: "财付通（需服务商支持）" },
        ],
      },
      {
        key: "mode",
        label: "下单模式",
        hint: "页面跳转适合大多数站点；API 模式可在本站显示渠道返回的付款码。",
        options: [
          { value: "submit", label: "页面跳转（submit.php，推荐）" },
          { value: "mapi", label: "API 下单（mapi.php）" },
        ],
      },
      {
        key: "submit_method",
        label: "页面跳转方式",
        options: [
          { value: "post", label: "POST 表单（推荐）" },
          { value: "get", label: "GET 链接（兼容模式）" },
        ],
      },
      {
        key: "cid",
        label: "支付渠道 ID（可选）",
        placeholder: "1234 或 1234,5678",
        hint: "留空由平台选择。多个渠道 ID 使用英文逗号分隔。",
      },
      {
        key: "sitename",
        label: "收银台网站名称（可选）",
        hint: "留空使用当前店铺名称；仅用于页面跳转模式。",
      },
    ],
  },
  niuniupay: {
    title: "牛牛支付",
    description: "V1 旧版接口 · 支付宝、微信等聚合收款",
    docs: "https://niuniupay.cc/doc/v1_legacy_api.html",
    fields: [
      { key: "api_url", label: "网关地址", placeholder: "https://niuniuapi.qihongmy.cn", hint: "V1 文档的 API 网关地址，可粘贴 submit.php 或 mapi.php 完整地址。" },
      { key: "pid", label: "商户 ID（PID）" },
      { key: "key", label: "商户密钥", secret: true },
      { key: "type", label: "支付类型", hint: "只选择商户实际开通的支付方式。", options: [
        { value: "alipay", label: "支付宝" }, { value: "wxpay", label: "微信支付" },
        { value: "qqpay", label: "QQ 钱包" }, { value: "bank", label: "银联支付" },
        { value: "jdpay", label: "京东支付" }, { value: "paypal", label: "PayPal" }, { value: "douyinpay", label: "抖音支付" },
      ] },
      { key: "mode", label: "下单模式", options: [ { value: "submit", label: "页面跳转（submit.php，推荐）" }, { value: "mapi", label: "API 下单（mapi.php）" } ] },
      { key: "submit_method", label: "页面跳转方式", options: [ { value: "post", label: "POST 表单（推荐）" }, { value: "get", label: "GET 链接（兼容模式）" } ] },
      { key: "limit_enabled", label: "单笔限额", hint: "开启后，订单金额超过限额时前台不能选择该支付方式，并提示顾客联系客服下单。", options: [ { value: "false", label: "不限制" }, { value: "true", label: "启用单笔限额" } ] },
      { key: "max_amount", label: "单笔最高金额（元）", placeholder: "500", hint: "例如牛牛支付单笔不能超过 500 元，填写 500。" },
    ],
  },
  dujiaopay: {
    title: "DujiaoPay",
    description: "托管收银台 · 加密货币支付",
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
            const values = gatewayValues(g?.config || "{}");
            return (
              <section className="a-panel a-payment-card" key={code}>
                <div className="a-payment-heading">
                  <ProviderLogo code={code} logoUrl={values.logo_url} />
                  <Status
                    value={g?.enabled ? "completed" : "disabled"}
                    label={g?.enabled ? "已启用" : "未启用"}
                  />
                </div>
                <h2>{spec.title}</h2>
                <p>{spec.description}</p>
                <div className="a-payment-meta">
                  <span>对外名称</span>
                  <strong>
                    {values.display_name || defaultPaymentName(code, values)}
                  </strong>
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
            在“站点设置 → 支付地址”设置公网 API
            回调基础地址和付款返回地址。Stripe Webhook 订阅
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
function ProviderLogo({ code, logoUrl }: { code: string; logoUrl?: string }) {
  return (
    <span className="a-payment-logo" aria-hidden="true">
      <PaymentLogo code={code} logoUrl={logoUrl} size={32} />
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
  const [values, setValues] = useState<Record<string, string>>(() => ({
    ...(gateway.code === "niuniupay" ? { api_url: "https://niuniuapi.qihongmy.cn", max_amount: "500" } : {}),
    ...gatewayValues(gateway.config),
  }));
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
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  const defaultName = defaultPaymentName(gateway.code, {
    ...values,
    ...(gateway.code === "stripe"
      ? {
          payment_methods:
            stripeMode === "auto" ? "auto" : stripeMethods.join(","),
        }
      : {}),
  });
  async function uploadLogo(file: File) {
    setError("");
    if (file.size > 2 * 1024 * 1024) {
      setError("Logo 不能超过 2 MB");
      return;
    }
    if (!/\.(png|jpe?g|gif)$/i.test(file.name)) {
      setError("请选择 PNG、JPG 或 GIF 图片");
      return;
    }
    setUploading(true);
    try {
      const data = new FormData();
      data.append("file", file);
      const result = await adminApi<{ url: string }>(
        `/gateways/${gateway.code}/logo`,
        { method: "POST", body: data },
      );
      setValues((current) => ({ ...current, logo_url: result.url }));
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setUploading(false);
    }
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (uploading || busy) return;
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
    <Modal
      title={spec.title + " 支付配置"}
      onClose={() => {
        if (!busy && !uploading) onClose();
      }}
    >
      <form className="a-editor" onSubmit={save}>
        <span className="a-eyebrow">PAYMENT PROVIDER</span>
        <div className="a-provider-title">
          <ProviderLogo code={gateway.code} />
          <h2>{spec.title}</h2>
        </div>
        <p>已保存的密钥以 ******** 显示；保留或留空即沿用。</p>
        {error && <Banner>{error}</Banner>}
        <section className="a-gateway-branding" aria-label="前台展示设置">
          <div className="a-branding-heading">
            <h3>前台展示</h3>
            <span>顾客看到的支付方式</span>
          </div>
          <Field
            label="对外名称"
            hint="留空跟随支付方式自动命名，最多 60 个字。"
          >
            <input
              maxLength={60}
              value={values.display_name || ""}
              placeholder={defaultName}
              onChange={(e) =>
                setValues((current) => ({
                  ...current,
                  display_name: e.target.value,
                }))
              }
            />
          </Field>
          <div className="a-branding-preview" aria-label="前台效果预览">
            <ProviderLogo code={gateway.code} logoUrl={values.logo_url} />
            <div>
              <strong>{values.display_name?.trim() || defaultName}</strong>
              <small>前台效果预览</small>
            </div>
          </div>
          <div className="a-logo-actions">
            <label
              className={`a-button a-logo-upload ${uploading || busy ? "is-disabled" : ""}`}
            >
              <Upload size={15} />
              {uploading ? "上传中…" : "上传 Logo"}
              <input
                type="file"
                aria-label="上传支付渠道 Logo"
                accept="image/png,image/jpeg,image/gif"
                disabled={uploading || busy}
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (file) void uploadLogo(file);
                }}
              />
            </label>
            {values.logo_url && (
              <button
                type="button"
                className="a-button"
                disabled={uploading || busy}
                onClick={() =>
                  setValues((current) => ({ ...current, logo_url: "" }))
                }
              >
                恢复默认 Logo
              </button>
            )}
          </div>
          <p className="a-logo-hint" role="status">
            {uploading
              ? "正在上传图片，请稍候…"
              : "PNG / JPG / GIF，最大 2 MB，建议使用正方形图片。保存配置后在前台生效。"}
          </p>
        </section>
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
        ]
          .filter(
            (f) =>
              !(
                ["epay", "niuniupay"].includes(gateway.code) &&
                values.mode === "mapi" &&
                ["submit_method", "sitename"].includes(f.key)
              ) &&
              !(f.key === "max_amount" && values.limit_enabled !== "true"),
          )
          .map((f) => (
            <Field key={f.key} label={f.label} hint={f.hint}>
              {"options" in f && f.options ? (
                <select
                  value={values[f.key] || f.options[0].value}
                  onChange={(e) =>
                    setValues({ ...values, [f.key]: e.target.value })
                  }
                >
                  {f.options.map((o) => (
                    <option key={o.value} value={o.value}>
                      {o.label}
                    </option>
                  ))}
                </select>
              ) : (
                <input
                  type={f.secret ? "password" : "text"}
                  autoComplete="off"
                  placeholder={f.placeholder}
                  value={values[f.key] || ""}
                  onChange={(e) =>
                    setValues({ ...values, [f.key]: e.target.value })
                  }
                />
              )}
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
          <button
            className="a-button"
            type="button"
            onClick={onClose}
            disabled={busy || uploading}
          >
            取消
          </button>
          <button className="a-button primary" disabled={busy || uploading}>
            <Save size={16} />
            {busy ? "保存中…" : "保存配置"}
          </button>
        </div>
      </form>
    </Modal>
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
    merchant_bark: "Bark 成交通知",
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
