import { useState, type FormEvent } from "react";
import { ArrowUpRight, Plus, Store } from "lucide-react";
import { Modal } from "../components";
import { adminApi, useAdminQuery } from "./api";
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
import { StationBrandFields, StationProductRow } from "../StationComponents";
import { emptyStation, type Station, type StationProduct } from "../stations";
import "../stations.css";
type Application = {
  id: number;
  name: string;
  slug: string;
  email: string;
  contact: string;
  description: string;
  status: string;
  review_note: string;
  created_at: string;
};
type Row = {
  station: Station;
  url: string;
  agent: { id: number; settlement_days: number };
  activated: boolean;
  invitations: { status: string; last_error: string; expires_at: string }[];
};
type Data = {
  root: string;
  apply_enabled: boolean;
  list: Row[];
  applications: Application[];
};
export default function Stations({ notify }: { notify: (m: string) => void }) {
  const q = useAdminQuery<Data>("/stations");
  const [tab, setTab] = useState("shops");
  const [editing, setEditing] = useState<{
    station: Station;
    days: number;
    applicationID?: number;
  } | null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [invite, setInvite] = useState("");
  const [products, setProducts] = useState<{
    station: Station;
    rows: StationProduct[];
  } | null>(null);
  const [search, setSearch] = useState("");
  const [reject, setReject] = useState<Application | null>(null);
  const [note, setNote] = useState("");
  const [security, setSecurity] = useState<Row | null>(null);
  const [password, setPassword] = useState("");
  async function action(fn: () => Promise<void>) {
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
  function create(app?: Application) {
    setError("");
    setEditing({
      station: {
        ...emptyStation,
        slug: app?.slug || "",
        site_name: app?.name || "",
        site_brand: app?.name || "",
        email: app?.email || "",
      },
      days: 7,
      applicationID: app?.id,
    });
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    if (!editing) return;
    await action(async () => {
      const result = await adminApi<{ activation_url?: string }>(
        editing.station.id ? `/stations/${editing.station.id}` : "/stations",
        {
          method: editing.station.id ? "PUT" : "POST",
          body: JSON.stringify({
            station: editing.station,
            settlement_days: editing.days,
            application_id: editing.applicationID || 0,
          }),
        },
      );
      setEditing(null);
      q.reload();
      notify("分站已保存");
      if (result?.activation_url) setInvite(result.activation_url);
    });
  }
  const pending =
    q.data?.applications.filter((a) => a.status === "pending").length || 0;
  return (
    <>
      <PageHeading
        title="代理分站"
        description="为代理开通独立店铺，共享主站商品、库存与支付，按供货差价结算。"
      >
        <button
          className="a-button primary"
          disabled={!q.data?.root || busy}
          onClick={() => create()}
        >
          <Plus size={16} />
          创建分站
        </button>
      </PageHeading>
      {q.loading && !q.data ? (
        <Busy />
      ) : q.error ? (
        <QueryError error={q.error} reload={q.reload} />
      ) : null}
      {q.data && (
        <>
          <section className="station-root-status">
            <Store size={24} />
            <div>
              <strong>
                {q.data.root
                  ? `分站域名：*.${q.data.root}`
                  : "尚未配置分站域名"}
              </strong>
              <p>
                {q.data.root
                  ? "新分站使用独立子域名，主站承担支付手续费。"
                  : "先按 docs/代理分站.md 配置泛域名解析与 HTTPS，再开启申请或创建分站。"}
              </p>
            </div>
            <button
              className="a-button"
              disabled={!q.data.root || busy}
              onClick={() =>
                void action(async () => {
                  await adminApi("/settings", {
                    method: "PUT",
                    body: JSON.stringify({
                      station_apply_enabled: q.data!.apply_enabled
                        ? "false"
                        : "true",
                    }),
                  });
                  q.reload();
                  notify(
                    q.data!.apply_enabled ? "已关闭公开申请" : "已开放公开申请",
                  );
                })
              }
            >
              {q.data.apply_enabled ? "关闭公开申请" : "开放公开申请"}
            </button>
            {q.data.apply_enabled && (
              <a
                className="a-button"
                href="/apply"
                target="_blank"
                rel="noreferrer"
              >
                查看申请页
                <ArrowUpRight size={14} />
              </a>
            )}
          </section>
          <div className="station-tabs" aria-label="分站管理分类">
            <button
              aria-pressed={tab === "shops"}
              onClick={() => setTab("shops")}
            >
              分站列表（{q.data.list.length}）
            </button>
            <button
              aria-pressed={tab === "applications"}
              onClick={() => setTab("applications")}
            >
              申请审核（待审核 {pending}）
            </button>
          </div>
          {error && !editing && !products && !reject && !security && (
            <Banner>{error}</Banner>
          )}
          {tab === "shops" ? (
            <div className="station-card-list">
              {q.data.list.map((row) => (
                <article key={row.station.id} className="station-admin-card">
                  <div className="station-product-heading">
                    <h3>{row.station.site_name}</h3>
                    <Status
                      value={
                        row.station.status === "active"
                          ? "available"
                          : "disabled"
                      }
                      label={
                        row.station.status === "active" ? "营业中" : "已暂停"
                      }
                    />
                  </div>
                  <p>
                    <a href={row.url} target="_blank" rel="noreferrer">
                      {row.url}
                    </a>
                    <br />
                    {row.station.email} · 结算等待 {row.agent.settlement_days}{" "}
                    天 · {row.activated ? "账号已激活" : "等待账号激活"}
                  </p>
                  {!row.activated && row.invitations[0] && (
                    <p>
                      开通邮件：
                      {(
                        {
                          pending: "等待发送",
                          sent: "已发送",
                          failed: "发送失败",
                          used: "已使用",
                          cancelled: "已失效",
                        } as Record<string, string>
                      )[row.invitations[0].status] || row.invitations[0].status}
                      {row.invitations[0].last_error &&
                        ` · ${row.invitations[0].last_error}`}
                    </p>
                  )}
                  <div className="station-actions">
                    <button
                      className="a-button"
                      disabled={busy}
                      onClick={() => {
                        setError("");
                        setEditing({
                          station: row.station,
                          days: row.agent.settlement_days,
                        });
                      }}
                    >
                      店铺配置
                    </button>
                    <button
                      className="a-button"
                      disabled={busy}
                      onClick={() =>
                        void action(async () => {
                          setProducts({
                            station: row.station,
                            rows: await adminApi(
                              `/stations/${row.station.id}/products`,
                            ),
                          });
                          setSearch("");
                        })
                      }
                    >
                      商品授权与供货价
                    </button>
                    <button
                      className="a-button"
                      disabled={busy}
                      onClick={() =>
                        void action(async () => {
                          await adminApi(`/stations/${row.station.id}/status`, {
                            method: "PUT",
                            body: JSON.stringify({
                              status:
                                row.station.status === "active"
                                  ? "paused"
                                  : "active",
                            }),
                          });
                          q.reload();
                          notify(
                            row.station.status === "active"
                              ? "已暂停新订单，保留已有订单服务"
                              : "分站已恢复营业",
                          );
                        })
                      }
                    >
                      {row.station.status === "active"
                        ? "暂停营业"
                        : "恢复营业"}
                    </button>
                    {!row.activated ? (
                      <button
                        className="a-button"
                        disabled={busy}
                        onClick={() =>
                          void action(async () => {
                            const v = await adminApi<{
                              activation_url: string;
                            }>(`/stations/${row.station.id}/invite`, {
                              method: "POST",
                            });
                            setInvite(v.activation_url);
                            q.reload();
                          })
                        }
                      >
                        重新生成邀请
                      </button>
                    ) : (
                      <button
                        className="a-button"
                        disabled={busy}
                        onClick={() => {
                          setError("");
                          setPassword("");
                          setSecurity(row);
                        }}
                      >
                        账号管理
                      </button>
                    )}
                  </div>
                </article>
              ))}
              {q.data.list.length === 0 && (
                <Empty
                  title="开通第一家代理分站"
                  text="设置供货价和授权商品，让代理拥有自己的店铺。"
                >
                  <button
                    className="a-button"
                    disabled={!q.data.root}
                    onClick={() => create()}
                  >
                    创建分站
                  </button>
                </Empty>
              )}
            </div>
          ) : (
            <div className="station-card-list">
              {q.data.applications.map((app) => (
                <article className="station-admin-card" key={app.id}>
                  <div className="station-product-heading">
                    <h3>{app.name}</h3>
                    <Status
                      value={
                        app.status === "approved"
                          ? "available"
                          : app.status === "rejected"
                            ? "disabled"
                            : "waiting"
                      }
                      label={
                        app.status === "approved"
                          ? "已通过"
                          : app.status === "rejected"
                            ? "已拒绝"
                            : "待审核"
                      }
                    />
                  </div>
                  <p>
                    {app.slug}.{q.data!.root}
                    <br />
                    {app.email} · {app.contact || "未留其他联系方式"}
                    <br />
                    {shortDate(app.created_at)}
                  </p>
                  {app.description && <p>{app.description}</p>}
                  {app.review_note && <p>审核说明：{app.review_note}</p>}
                  {app.status === "pending" && (
                    <div className="station-actions">
                      <button
                        className="a-button primary"
                        disabled={busy}
                        onClick={() => create(app)}
                      >
                        审核并开通
                      </button>
                      <button
                        className="a-button"
                        disabled={busy}
                        onClick={() => {
                          setError("");
                          setReject(app);
                          setNote("");
                        }}
                      >
                        拒绝申请
                      </button>
                    </div>
                  )}
                </article>
              ))}
              {q.data.applications.length === 0 && (
                <Empty
                  title="暂无分站申请"
                  text="开放公开申请后，用户可以从前台提交申请。"
                />
              )}
            </div>
          )}
        </>
      )}
      {editing && (
        <Modal
          wide
          title={editing.station.id ? "店铺配置" : "开通代理分站"}
          onClose={() => {
            if (!busy) setEditing(null);
          }}
        >
          <h2>{editing.station.id ? "店铺配置" : "开通代理分站"}</h2>
          <p className="station-note">
            {editing.station.id
              ? "子域名与激活邮箱创建后保持不变，保护已有订单和账号归属。"
              : "开通后生成 48 小时有效的激活链接，并按主站 Resend 配置发送邮件。"}
          </p>
          <form className="station-editor" onSubmit={save}>
            <fieldset disabled={busy}>
              <Field label="子域名">
                <div className="station-domain-input">
                  <input
                    required
                    disabled={!!editing.station.id || !!editing.applicationID}
                    pattern="[a-z0-9][a-z0-9-]{1,30}[a-z0-9]"
                    maxLength={32}
                    value={editing.station.slug}
                    onChange={(e) =>
                      setEditing({
                        ...editing,
                        station: {
                          ...editing.station,
                          slug: e.target.value.toLowerCase(),
                        },
                      })
                    }
                  />
                  <span>.{q.data?.root}</span>
                </div>
              </Field>
              <Field label="激活邮箱">
                <input
                  required
                  disabled={!!editing.station.id || !!editing.applicationID}
                  type="email"
                  maxLength={254}
                  value={editing.station.email}
                  onChange={(e) =>
                    setEditing({
                      ...editing,
                      station: { ...editing.station, email: e.target.value },
                    })
                  }
                />
              </Field>
              <StationBrandFields
                value={editing.station}
                onChange={(v) => setEditing({ ...editing, station: v })}
              />
              <Field label="结算等待天数">
                <input
                  required
                  type="number"
                  min={0}
                  max={365}
                  value={editing.days}
                  onChange={(e) =>
                    setEditing({ ...editing, days: Number(e.target.value) })
                  }
                />
              </Field>
              <Field label="营业状态">
                <select
                  value={editing.station.status}
                  onChange={(e) =>
                    setEditing({
                      ...editing,
                      station: {
                        ...editing.station,
                        status: e.target.value as Station["status"],
                      },
                    })
                  }
                >
                  <option value="active">营业中</option>
                  <option value="paused">暂停新订单</option>
                </select>
              </Field>
            </fieldset>
            {error && <Banner>{error}</Banner>}
            <button className="a-button primary" disabled={busy}>
              {busy ? "保存中…" : editing.station.id ? "保存配置" : "确认开通"}
            </button>
          </form>
        </Modal>
      )}
      {invite && (
        <Modal title="分站已开通" onClose={() => setInvite("")}>
          <h2>分站激活链接</h2>
          <p className="station-note">
            链接有效期 48 小时，仅用于首次设置密码。邮件会在 Resend
            启用后发送；也可将此链接交给对应代理，请勿公开。
          </p>
          <p className="station-invite">{invite}</p>
          <button
            className="a-button primary"
            onClick={() =>
              void navigator.clipboard
                .writeText(invite)
                .then(() => notify("激活链接已复制"))
                .catch(() => notify("请选中链接手动复制"))
            }
          >
            复制激活链接
          </button>
        </Modal>
      )}
      {products && (
        <Modal
          wide
          title="商品授权与供货价"
          onClose={() => {
            if (!busy) setProducts(null);
          }}
        >
          <h2>{products.station.site_name} · 商品授权</h2>
          <p className="station-note">
            授权后由代理设置零售价并上架；主站商品下架后分站也无法销售。提高供货价时请同步调整零售价，或先取消上架。
          </p>
          <Field label="搜索商品">
            <input
              type="search"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              placeholder="按商品名称搜索"
            />
          </Field>
          <div className="station-card-list">
            {products.rows
              .filter((r) =>
                r.name.toLowerCase().includes(search.toLowerCase()),
              )
              .map((r) => (
                <StationProductRow
                  key={r.product_id}
                  row={r}
                  owner
                  onSave={async (row) => {
                    await adminApi(
                      `/stations/${products.station.id}/products`,
                      { method: "PUT", body: JSON.stringify(row) },
                    );
                    notify("商品授权与供货价已保存");
                  }}
                />
              ))}
          </div>
        </Modal>
      )}
      {reject && (
        <Modal
          title="拒绝分站申请"
          onClose={() => {
            if (!busy) setReject(null);
          }}
        >
          <h2>拒绝 {reject.name} 的申请</h2>
          <form
            className="station-editor"
            onSubmit={(e) => {
              e.preventDefault();
              void action(async () => {
                await adminApi(`/station-applications/${reject.id}/reject`, {
                  method: "POST",
                  body: JSON.stringify({ note }),
                });
                setReject(null);
                q.reload();
                notify("申请已拒绝");
              });
            }}
          >
            <Field label="审核说明">
              <textarea
                maxLength={500}
                rows={3}
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </Field>
            {error && <Banner>{error}</Banner>}
            <button className="a-button primary" disabled={busy}>
              确认拒绝
            </button>
          </form>
        </Modal>
      )}
      {security && (
        <Modal
          title="分站账号管理"
          onClose={() => {
            if (!busy) setSecurity(null);
          }}
        >
          <h2>{security.station.site_name} · 账号管理</h2>
          <p className="station-note">
            重设密码会撤销旧登录，不会自动解除
            2FA。操作需要验证当前主站管理员身份。
          </p>
          <form
            className="station-editor"
            onSubmit={(e) => {
              e.preventDefault();
              void action(async () => {
                await adminApi(`/agents/${security.agent.id}/password`, {
                  method: "PUT",
                  body: JSON.stringify({ password }),
                });
                setSecurity(null);
                notify("分站登录密码已重设");
              });
            }}
          >
            <Field label="新的分站登录密码">
              <input
                required
                type="password"
                minLength={12}
                maxLength={72}
                autoComplete="new-password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            <button className="a-button primary" disabled={busy}>
              重设登录密码
            </button>
          </form>
          <hr />
          <p className="station-note">
            仅在确认代理遗失验证器后解除 2FA，解除后要求代理重新绑定。
          </p>
          <button
            className="a-button"
            disabled={busy}
            onClick={() =>
              void action(async () => {
                await adminApi(`/stations/${security.station.id}/reset-2fa`, {
                  method: "POST",
                });
                setSecurity(null);
                notify("已解除分站 2FA，并撤销旧登录");
              })
            }
          >
            验证主站身份并解除 2FA
          </button>
          {error && <Banner>{error}</Banner>}
        </Modal>
      )}
    </>
  );
}
