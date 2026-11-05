import { useState, type FormEvent, type KeyboardEvent } from "react";
import { useSearchParams } from "react-router-dom";
import {
  BellRing,
  CreditCard,
  ExternalLink,
  Globe,
  Mail,
  Save,
  Send,
  ShieldCheck,
  Upload,
  Webhook,
} from "lucide-react";
import { SiteLogo, useSite } from "../Site";
import { adminApi, useAdminQuery } from "./api";
import { Banner, Busy, Field, PageHeading, QueryError } from "./ui";

const tabs = [
  {
    id: "general",
    title: "基本信息",
    icon: Globe,
    keys: [
      "site_name",
      "site_brand",
      "site_logo",
      "site_favicon",
      "site_desc",
      "announcement",
      "customer_service",
      "site_footer",
      "default_redeem_url",
    ],
  },
  {
    id: "protection",
    title: "下单防护",
    icon: ShieldCheck,
    keys: [
      "site_url",
      "turnstile_enabled",
      "turnstile_site_key",
      "turnstile_secret_key",
    ],
  },
  {
    id: "email",
    title: "邮件与推送",
    icon: Mail,
    keys: [
      "resend_enabled",
      "resend_api_key",
      "resend_from",
      "order_push_enabled",
      "order_notify_email",
      "order_webhook_url",
      "order_webhook_secret",
      "order_bark_enabled",
      "order_bark_server",
      "order_bark_key",
      "order_bark_group",
    ],
  },
  {
    id: "payment",
    title: "支付地址",
    icon: CreditCard,
    keys: ["pay_notify_base", "pay_return_url"],
  },
];
const barkKeys = ["order_bark_server", "order_bark_key", "order_bark_group"];
export function Settings({ notify }: { notify: (message: string) => void }) {
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
  notify: (message: string) => void;
}) {
  const [v, setV] = useState(initial);
  const [saved, setSaved] = useState(initial);
  const [params, setParams] = useSearchParams();
  const active = tabs.find((tab) => tab.id === params.get("tab")) || tabs[0];
  const [busy, setBusy] = useState(false);
  const [uploading, setUploading] = useState("");
  const [error, setError] = useState("");
  const [barkTesting, setBarkTesting] = useState(false);
  const [barkResult, setBarkResult] = useState<{
    message: string;
    success: boolean;
  } | null>(null);
  const { refresh } = useSite();
  const set = (key: string, value: string) => {
    if (barkKeys.includes(key)) setBarkResult(null);
    setV((current) => ({ ...current, [key]: value }));
  };
  const dirty = (keys: string[]) =>
    keys.some((key) => (v[key] || "") !== (saved[key] || ""));
  function selectTab(id: string) {
    setParams({ tab: id }, { replace: true });
    setError("");
  }
  function tabKey(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % tabs.length;
    else if (event.key === "ArrowLeft")
      next = (index + tabs.length - 1) % tabs.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = tabs.length - 1;
    else return;
    event.preventDefault();
    selectTab(tabs[next].id);
    document.getElementById(`settings-tab-${tabs[next].id}`)?.focus();
  }
  function input(
    key: string,
    label: string,
    hint?: string,
    type = "text",
    maxLength = 255,
  ) {
    return (
      <Field label={label} hint={hint}>
        <input
          type={type}
          name={key}
          maxLength={maxLength}
          autoComplete={type === "password" ? "new-password" : "off"}
          value={v[key] || ""}
          required={key === "site_name" || key === "site_brand"}
          onChange={(e) => set(key, e.target.value)}
        />
      </Field>
    );
  }
  function toggle(key: string, title: string, hint: string) {
    return (
      <label className="a-switch-row">
        <span>
          <strong>{title}</strong>
          <small>{hint}</small>
        </span>
        <input
          type="checkbox"
          role="switch"
          checked={v[key] === "true"}
          onChange={(e) => set(key, String(e.target.checked))}
        />
      </label>
    );
  }
  async function uploadImage(key: string, file: File) {
    setError("");
    if (file.size > 2 * 1024 * 1024 || !/\.(png|jpe?g|gif)$/i.test(file.name)) {
      setError("请选择不超过 2 MB 的 PNG、JPG 或 GIF 图片");
      return;
    }
    setUploading(key);
    try {
      const data = new FormData();
      data.append("file", file);
      const result = await adminApi<{ url: string }>("/settings/image", {
        method: "POST",
        body: data,
      });
      set(key, result.url);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setUploading("");
    }
  }
  function imageField(key: string, title: string, hint: string) {
    return (
      <div className="a-site-image-field">
        <span className="a-site-image-label">{title}</span>
        <div className="a-site-image-controls">
          <span className="a-site-image-preview">
            <SiteLogo url={v[key]} />
          </span>
          <div>
            <div className="a-logo-actions">
              <label className="a-button a-logo-upload">
                <Upload size={15} />
                {uploading === key ? "上传中…" : "上传" + title}
                <input
                  type="file"
                  aria-label={"上传" + title}
                  accept="image/png,image/jpeg,image/gif"
                  disabled={!!uploading || busy}
                  onChange={(e) => {
                    const file = e.target.files?.[0];
                    e.target.value = "";
                    if (file) void uploadImage(key, file);
                  }}
                />
              </label>
              {v[key] && (
                <button
                  type="button"
                  className="a-button"
                  onClick={() => set(key, "")}
                  disabled={!!uploading || busy}
                >
                  恢复默认{title}
                </button>
              )}
            </div>
            <small>{hint}</small>
          </div>
        </div>
      </div>
    );
  }
  async function save(event: FormEvent) {
    event.preventDefault();
    if (busy || uploading || barkTesting) return;
    setBusy(true);
    setError("");
    try {
      const body = Object.fromEntries(
        active.keys.filter((key) => key in v).map((key) => [key, v[key]]),
      );
      await adminApi("/settings", {
        method: "PUT",
        body: JSON.stringify(body),
      });
      const latest = await adminApi<Record<string, string>>("/settings");
      const patch = Object.fromEntries(
        active.keys.map((key) => [key, latest[key] || ""]),
      );
      setV((current) => ({ ...current, ...patch }));
      setSaved((current) => ({ ...current, ...patch }));
      await refresh();
      notify(active.title + "已保存");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function testBark() {
    if (barkTesting || busy || dirty(barkKeys) || !saved.order_bark_key)
      return;
    setBarkTesting(true);
    setBarkResult(null);
    try {
      const result = await adminApi<{ message: string }>("/settings/bark/test", {
        method: "POST",
      });
      setBarkResult({ message: result.message, success: true });
    } catch (e) {
      setBarkResult({ message: (e as Error).message, success: false });
    } finally {
      setBarkTesting(false);
    }
  }
  return (
    <>
      <PageHeading
        title="站点设置"
        description="从小铺的模样，到每一笔安心的订单。"
      />
      <div className="a-site-tabs" role="tablist" aria-label="站点设置分类">
        {tabs.map((tab, index) => {
          const Icon = tab.icon;
          return (
            <button
              key={tab.id}
              type="button"
              role="tab"
              id={`settings-tab-${tab.id}`}
              aria-controls={`settings-panel-${tab.id}`}
              aria-selected={tab.id === active.id}
              tabIndex={tab.id === active.id ? 0 : -1}
              disabled={busy || !!uploading || barkTesting}
              onClick={() => selectTab(tab.id)}
              onKeyDown={(e) => tabKey(e, index)}
            >
              <Icon size={17} />
              {tab.title}
              {dirty(tab.keys) && (
                <span className="a-unsaved-dot" aria-label="有未保存修改" />
              )}
            </button>
          );
        })}
      </div>
      <form
        onSubmit={save}
        className="a-settings-form a-site-settings"
        role="tabpanel"
        id={`settings-panel-${active.id}`}
        aria-labelledby={`settings-tab-${active.id}`}
      >
        {error && <Banner>{error}</Banner>}
        <fieldset className="a-settings-fields" disabled={busy || barkTesting}>
          {active.id === "general" && (
            <>
              <section className="a-panel a-settings-panel">
                <div className="a-settings-intro">
                  <i className="a-icon-blue">
                    <Globe size={22} />
                  </i>
                  <h2>小铺品牌</h2>
                  <p>
                    首页、订单页、浏览器标签和后台品牌同步使用，保存后生效。
                  </p>
                </div>
                <div>
                  <div className="a-form-grid">
                    {input(
                      "site_name",
                      "网站名称",
                      "最多 60 个字，用于网页标题与店铺名称。",
                      "text",
                      60,
                    )}
                    {input(
                      "site_brand",
                      "品牌短名称",
                      "例如 Wybo，用于 Logo 旁的品牌文字。",
                      "text",
                      60,
                    )}
                  </div>
                  {imageField(
                    "site_logo",
                    "网站 Logo",
                    "正方形透明底更清晰；PNG / JPG / GIF，最大 2 MB。",
                  )}
                  {imageField(
                    "site_favicon",
                    "浏览器图标",
                    "建议 64 × 64 或 128 × 128；留空使用默认月兔图标。",
                  )}
                  <div className="a-site-brand-preview">
                    <SiteLogo url={v.site_logo} />
                    <div>
                      <strong>{v.site_brand || "品牌短名称"}</strong>
                      <small>{v.site_name || "网站名称"}</small>
                    </div>
                    <span>品牌预览</span>
                  </div>
                </div>
              </section>
              <section className="a-panel a-settings-panel">
                <div className="a-settings-intro">
                  <h2>内容与兑换</h2>
                  <p>让顾客找到店铺介绍、客服和购买后的兑换入口。</p>
                </div>
                <div>
                  <Field label="网站简介">
                    <textarea
                      rows={2}
                      maxLength={500}
                      value={v.site_desc || ""}
                      onChange={(e) => set("site_desc", e.target.value)}
                    />
                  </Field>
                  {input("announcement", "首页公告")}
                  {input("customer_service", "客服联系方式")}
                  {input("site_footer", "页脚文案")}
                  {input(
                    "default_redeem_url",
                    "默认兑换地址",
                    "未单独设置兑换地址的商品使用此入口，订单页和发货邮件同步生效；留空关闭默认入口。",
                    "url",
                  )}
                </div>
              </section>
            </>
          )}
          {active.id === "protection" && (
            <>
              <section className="a-panel a-settings-panel">
                <div className="a-settings-intro">
                  <i className="a-icon-blue">
                    <ShieldCheck size={22} />
                  </i>
                  <h2>Cloudflare Turnstile</h2>
                  <p>
                    提交订单前验证访问者，减少机器人占单。域名和密钥由你自行配置。
                  </p>
                  <a
                    href="https://dash.cloudflare.com/?to=/:account/turnstile"
                    target="_blank"
                    rel="noreferrer"
                  >
                    打开 Turnstile 控制台 <ExternalLink size={13} />
                  </a>
                </div>
                <div>
                  {toggle(
                    "turnstile_enabled",
                    "启用下单人机验证",
                    "开启后，网页与直接调用下单接口都必须通过验证。",
                  )}
                  {input(
                    "site_url",
                    "网站正式地址",
                    "例如 https://shop.example.com；必须与 Cloudflare 允许的主机名一致。",
                    "url",
                  )}
                  {input(
                    "turnstile_site_key",
                    "Turnstile Site Key",
                    "公开的网站密钥，可显示在前台。",
                  )}
                  {input(
                    "turnstile_secret_key",
                    "Turnstile Secret Key",
                    "仅在服务器加密保存；******** 或留空沿用已保存的密钥。",
                    "password",
                  )}
                </div>
              </section>
              <section className="a-panel a-turnstile-help">
                <h2>三步完成配置</h2>
                <ol>
                  <li>
                    在 Cloudflare 的 Turnstile 中添加小组件，选择 Managed
                    模式，添加网站主机名（例如 shop.example.com，不带 https://
                    或路径）。
                  </li>
                  <li>
                    将对应的 Site Key 和 Secret Key 填到上方，填写网站正式地址。
                  </li>
                  <li>
                    开启下单人机验证并保存，打开前台核对订单时会显示验证组件。
                  </li>
                </ol>
                <p>
                  验证未通过、过期或服务无法连接时，订单不会创建，库存也不会被占用。已有订单付款与支付回调照常处理。现有下单频率限制继续生效。
                </p>
              </section>
            </>
          )}
          {active.id === "email" && (
            <>
              <section className="a-panel a-settings-panel">
                <div className="a-settings-intro">
                  <i className="a-icon-violet">
                    <BellRing size={22} />
                  </i>
                  <h2>成交订单推送</h2>
                  <p>订单付款后，用 Bark 将好消息送到手机。</p>
                  <a
                    href="https://bark.day.app/"
                    target="_blank"
                    rel="noreferrer"
                  >
                    Bark 使用说明 <ExternalLink size={13} />
                  </a>
                </div>
                <div>
                  {toggle(
                    "order_push_enabled",
                    "启用成交推送",
                    "支持 Bark、店主邮件与 Webhook，可同时使用。",
                  )}
                  {v.order_push_enabled !== "true" &&
                    (v.order_bark_enabled === "true" || v.order_webhook_url?.trim() || v.order_notify_email?.trim()) && (
                      <Banner>
                        <strong>成交推送总开关未开启</strong>
                        <p>当前不会发送 Bark、店主成交邮件或成交 Webhook。请开启总开关并保存，顾客发货邮件由邮件通知开关单独控制。</p>
                        <button type="button" className="a-button" onClick={() => set("order_push_enabled", "true")}>
                          开启成交推送（需保存）
                        </button>
                      </Banner>
                    )}
                  <p className="a-note">开启后，已成交但遗漏通知的订单可到「支付对账」点击「核对渠道」补发；已发送的通知不会重复发送。</p>
                  <h3>Bark 手机通知</h3>
                  {toggle(
                    "order_bark_enabled",
                    "启用 Bark 通知",
                    "需同时开启成交推送；不依赖 Resend。",
                  )}
                  {input(
                    "order_bark_server",
                    "Bark 服务器地址",
                    "留空使用 https://api.day.app；自建服务器填写根地址，不包含设备 Key。",
                    "url",
                    2048,
                  )}
                  {input(
                    "order_bark_key",
                    "Bark 设备 Key",
                    "例如链接 https://api.day.app/YourKey/测试 中，只填写 YourKey。保存后加密并显示为 ********；留空沿用。",
                    "password",
                  )}
                  {input(
                    "order_bark_group",
                    "Bark 消息分组",
                    "留空使用「Wybo 成交订单」，最多 60 个字。",
                    "text",
                    60,
                  )}
                  <p className="a-inline-note">
                    推送包含商品、订单号、实收、成本与销售利润，不包含卡密或买家联系方式。
                  </p>
                  <button
                    type="button"
                    className="a-button"
                    disabled={
                      barkTesting || busy || dirty(barkKeys) || !saved.order_bark_key
                    }
                    onClick={testBark}
                  >
                    <Send size={15} aria-hidden="true" />
                    {barkTesting ? "正在发送测试…" : "发送 Bark 测试通知"}
                  </button>
                  <p className="a-inline-note">
                    请先保存配置。测试仅使用已保存的地址和 Key，不发送真实订单。
                  </p>
                  {barkResult && (
                    <Banner success={barkResult.success}>
                      {barkResult.message}
                    </Banner>
                  )}
                </div>
              </section>
              <section className="a-panel a-settings-panel">
                <div className="a-settings-intro">
                  <i className="a-icon-blue">
                    <Mail size={22} />
                  </i>
                  <h2>Resend 邮件</h2>
                  <p>付款发货后，为顾客发送卡密与兑换入口。</p>
                  <a
                    href="https://resend.com/domains"
                    target="_blank"
                    rel="noreferrer"
                  >
                    管理已验证域名 <ExternalLink size={13} />
                  </a>
                </div>
                <div>
                  {toggle(
                    "resend_enabled",
                    "启用邮件通知",
                    "需要 Resend API Key 和已验证发件域名。",
                  )}
                  {input(
                    "resend_api_key",
                    "Resend API Key",
                    "已保存密钥显示为 ********；留空沿用。",
                    "password",
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
                  <h2>其他成交推送</h2>
                  <p>开启成交推送后，也可通过邮件和 Webhook 接收。</p>
                </div>
                <div>
                  {input(
                    "order_notify_email",
                    "店主接收邮箱",
                    "店主邮件需要同时启用 Resend。",
                  )}
                  {input(
                    "order_webhook_url",
                    "Webhook 地址",
                    "留空则不发送 Webhook。",
                    "url",
                  )}
                  {input(
                    "order_webhook_secret",
                    "Webhook 签名密钥",
                    "留空沿用已有密钥；接收端须校验 X-Wybo-Signature。",
                    "password",
                  )}
                </div>
              </section>
            </>
          )}
          {active.id === "payment" && (
            <section className="a-panel a-settings-panel">
              <div className="a-settings-intro">
                <i className="a-icon-green">
                  <CreditCard size={22} />
                </i>
                <h2>支付地址</h2>
                <p>支付平台回调与顾客付款后返回小铺的地址。</p>
              </div>
              <div>
                {input(
                  "pay_notify_base",
                  "公网回调基础地址",
                  "例如 https://shop.example.com/api/v1/pay/notify，不含渠道代码。",
                  "url",
                )}
                {input(
                  "pay_return_url",
                  "付款返回地址",
                  "例如 https://shop.example.com/orders。",
                  "url",
                )}
              </div>
            </section>
          )}
        </fieldset>
        <div className="a-site-save">
          <span role="status">
            {uploading
              ? "图片上传中，请稍候…"
              : dirty(active.keys)
                ? "当前分类有未保存的修改"
                : "设置已同步"}
          </span>
          <button className="a-button primary" disabled={busy || !!uploading || barkTesting}>
            <Save size={16} />
            {busy ? "保存中…" : "保存" + active.title}
          </button>
        </div>
      </form>
    </>
  );
}
