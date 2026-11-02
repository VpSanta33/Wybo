import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import {
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  Store,
  ShieldCheck,
} from "lucide-react";
import { useSite, SiteBrand } from "./Site";
import { stationApi } from "./stations";
import { Banner, Field } from "./admin/ui";
import CheckoutVerification, {
  type VerificationState,
} from "./CheckoutVerification";
import "./admin/admin.css";
import "./stations.css";
export default function StationApply() {
  const { site } = useSite();
  const [form, setForm] = useState({
    name: "",
    slug: "",
    email: "",
    contact: "",
    description: "",
  });
  const [verify, setVerify] = useState<VerificationState>({
    ready: false,
    token: "",
  });
  const [revision, setRevision] = useState(0);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await stationApi("/apply", {
        method: "POST",
        body: JSON.stringify({ ...form, turnstile_token: verify.token }),
      });
      setDone(true);
    } catch (e) {
      setError((e as Error).message);
      setRevision((v) => v + 1);
    } finally {
      setBusy(false);
    }
  }
  const enabled =
    site.station_apply_enabled === "true" && site.is_station !== "true";
  return (
    <div className="a-root station-public">
      <header className="station-public-header">
        <SiteBrand />
        <Link className="a-button" to="/">
          <ArrowLeft size={16} />
          返回小铺
        </Link>
      </header>
      <main className="station-application">
        <section className="station-application-intro">
          <span className="station-kicker">
            <Store size={16} />
            一起经营
          </span>
          <h1>
            开一家属于你的
            <br />
            望月小铺。
          </h1>
          <p>
            独立店铺与管理后台，共享商品和库存。无需准备服务器和域名，把时间留给经营。
          </p>
          <ol>
            <li>
              <strong>提交申请</strong>
              <span>填写店铺名称和期望子域名</span>
            </li>
            <li>
              <strong>等待审核</strong>
              <span>主站审核后发送账号激活邮件</span>
            </li>
            <li>
              <strong>开始经营</strong>
              <span>设置零售价，分享店铺，查看收益</span>
            </li>
          </ol>
          <div className="station-trust">
            <ShieldCheck size={20} />
            <p>主站统一收款与发货，代理获得销售差价；支付手续费由主站承担。</p>
          </div>
        </section>
        <section className="a-panel station-application-form">
          {done ? (
            <div className="station-success" role="status">
              <CheckCircle2 size={42} />
              <h2>申请已提交</h2>
              <p>
                审核通过后，我们会将开通邮件发送至 {form.email}
                。请留意收件箱与垃圾邮件。
              </p>
              <Link className="a-button primary" to="/">
                回到小铺
              </Link>
            </div>
          ) : !enabled ? (
            <div className="station-success">
              <h2>暂未开放分站申请</h2>
              <p>请通过店铺客服咨询开通事宜。</p>
            </div>
          ) : (
            <>
              <h2>分站申请</h2>
              <p>
                审核通过后获得 {form.slug || "your-shop"}.{site.station_root}
              </p>
              <form onSubmit={submit}>
                <fieldset disabled={busy}>
                  <Field label="店铺名称">
                    <input
                      required
                      autoComplete="organization"
                      maxLength={60}
                      value={form.name}
                      onChange={(e) =>
                        setForm({ ...form, name: e.target.value })
                      }
                    />
                  </Field>
                  <Field
                    label="期望子域名"
                    hint={`3–32 位小写字母、数字或短横线，例如 alice.${site.station_root}`}
                  >
                    <div className="station-domain-input">
                      <input
                        required
                        pattern="[a-z0-9][a-z0-9-]{1,30}[a-z0-9]"
                        maxLength={32}
                        value={form.slug}
                        onChange={(e) =>
                          setForm({
                            ...form,
                            slug: e.target.value.toLowerCase(),
                          })
                        }
                      />
                      <span>.{site.station_root}</span>
                    </div>
                  </Field>
                  <Field
                    label="接收邮箱"
                    hint="用于接收审核后的账号激活邮件，请确认填写正确。"
                  >
                    <input
                      type="email"
                      required
                      autoComplete="email"
                      maxLength={254}
                      value={form.email}
                      onChange={(e) =>
                        setForm({ ...form, email: e.target.value })
                      }
                    />
                  </Field>
                  <Field label="其他联系方式（选填）">
                    <input
                      maxLength={128}
                      value={form.contact}
                      onChange={(e) =>
                        setForm({ ...form, contact: e.target.value })
                      }
                    />
                  </Field>
                  <Field label="申请说明（选填）">
                    <textarea
                      rows={3}
                      maxLength={2000}
                      value={form.description}
                      onChange={(e) =>
                        setForm({ ...form, description: e.target.value })
                      }
                    />
                  </Field>
                </fieldset>
                <CheckoutVerification
                  key={revision}
                  action="station_apply"
                  onChange={setVerify}
                />
                {error && <Banner>{error}</Banner>}
                <button
                  className="a-button primary station-submit"
                  disabled={busy || !verify.ready}
                >
                  {busy ? "正在提交…" : "提交分站申请"}
                  <ArrowRight size={16} />
                </button>
              </form>
            </>
          )}
        </section>
      </main>
    </div>
  );
}
