import { useState, type FormEvent } from "react";
import { Banner, Field, Status, shortDate } from "./admin/ui";
import { yuan } from "./admin/api";
import { SiteLogo } from "./Site";
import type { Station, StationOrder, StationProduct } from "./stations";

export function StationOrderTable({ orders }: { orders: StationOrder[] }) {
  return (
    <div className="a-table-wrap">
      <table className="a-table">
        <thead>
          <tr>
            <th>订单 / 时间</th>
            <th>联系方式</th>
            <th>实收</th>
            <th>供货总额</th>
            <th>分站收益</th>
            <th>状态</th>
          </tr>
        </thead>
        <tbody>
          {orders.map((o) => (
            <tr key={o.order_no}>
              <td>
                <strong>{o.order_no}</strong>
                <small>{shortDate(o.created_at)}</small>
              </td>
              <td>{o.contact}</td>
              <td>{yuan(o.total_cents - o.refunded_cents)}</td>
              <td>
                {o.wholesale_cents === null ? "—" : yuan(o.wholesale_cents)}
              </td>
              <td>{yuan(o.profit_cents)}</td>
              <td>
                <Status value={o.status} />
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {!orders.length && (
        <p className="station-note">暂无订单，新成交会出现在这里。</p>
      )}
    </div>
  );
}

export function StationProductRow({
  row,
  owner,
  onSave,
}: {
  row: StationProduct;
  owner: boolean;
  onSave: (r: StationProduct) => Promise<void>;
}) {
  const [value, setValue] = useState(row);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    setSaved(false);
    try {
      await onSave(value);
      setSaved(true);
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const update = (next: Partial<StationProduct>) => {
    setValue((v) => ({ ...v, ...next }));
    setSaved(false);
  };
  return (
    <article className="station-product">
      <div className="station-product-heading">
        <div>
          <h3>{row.name}</h3>
          <p>
            {row.delivery_mode === "manual"
              ? "主站手工发货"
              : `共享库存 ${row.stock} 份`}
            {!row.product_enabled && " · 主站已下架"}
          </p>
        </div>
        <Status
          value={value.authorized ? "available" : "disabled"}
          label={value.authorized ? "已授权" : "未授权"}
        />
      </div>
      <form onSubmit={save}>
        <fieldset disabled={busy}>
          <div className="station-product-fields">
            {owner ? (
              <Field label="供货价（元）">
                <input
                  required
                  type="number"
                  min="0.01"
                  max="100000000"
                  step="0.01"
                  value={
                    value.wholesale_cents ? value.wholesale_cents / 100 : ""
                  }
                  onChange={(e) =>
                    update({
                      wholesale_cents: Math.round(Number(e.target.value) * 100),
                    })
                  }
                />
              </Field>
            ) : (
              <div className="station-price">
                <span>供货价</span>
                <strong>{yuan(value.wholesale_cents)}</strong>
              </div>
            )}
            <Field label="零售价（元）">
              <input
                required
                type="number"
                min={value.listed ? value.wholesale_cents / 100 : 0}
                max="100000000"
                step="0.01"
                value={value.retail_cents ? value.retail_cents / 100 : ""}
                onChange={(e) =>
                  update({
                    retail_cents: Math.round(Number(e.target.value) * 100),
                  })
                }
              />
            </Field>
            <div className="station-price">
              <span>每份差价</span>
              <strong>
                {yuan(value.retail_cents - value.wholesale_cents)}
              </strong>
            </div>
          </div>
          <div className="station-row-actions">
            {owner && (
              <label className="station-check">
                <input
                  type="checkbox"
                  checked={value.authorized}
                  onChange={(e) => update({ authorized: e.target.checked })}
                />
                允许分站销售
              </label>
            )}
            <label className="station-check">
              <input
                type="checkbox"
                checked={value.listed}
                onChange={(e) => update({ listed: e.target.checked })}
              />
              在分站上架
            </label>
            <button className="a-button primary" disabled={busy}>
              {busy ? "保存中…" : "保存定价"}
            </button>
            {saved && <span role="status">已保存</span>}
          </div>
        </fieldset>
        {error && <Banner>{error}</Banner>}
      </form>
    </article>
  );
}

export function StationBrandFields({
  value,
  onChange,
  onUpload,
  disabled = false,
}: {
  value: Station;
  onChange: (v: Station) => void;
  onUpload?: (file: File) => Promise<string>;
  disabled?: boolean;
}) {
  const [uploading, setUploading] = useState(false);
  const [error, setError] = useState("");
  async function upload(
    file: File | undefined,
    key: "site_logo" | "site_favicon",
  ) {
    if (!file || !onUpload) return;
    setUploading(true);
    setError("");
    try {
      if (file.size > 2 * 1024 * 1024) throw new Error("图片不能超过 2 MB");
      onChange({ ...value, [key]: await onUpload(file) });
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setUploading(false);
    }
  }
  return (
    <fieldset disabled={disabled || uploading}>
      <div className="a-form-grid">
        <Field label="店铺名称">
          <input
            required
            maxLength={60}
            value={value.site_name}
            onChange={(e) => onChange({ ...value, site_name: e.target.value })}
          />
        </Field>
        <Field label="品牌简称">
          <input
            required
            maxLength={60}
            value={value.site_brand}
            onChange={(e) => onChange({ ...value, site_brand: e.target.value })}
          />
        </Field>
      </div>
      {onUpload && (
        <div className="station-brand-uploads">
          {(
            [
              ["site_logo", "店铺 Logo"],
              ["site_favicon", "浏览器图标"],
            ] as const
          ).map(([key, label]) => (
            <div className="station-upload" key={key}>
              <SiteLogo url={value[key]} />
              <label>
                {label}
                <input
                  type="file"
                  accept="image/png,image/jpeg,image/gif"
                  onChange={(e) => {
                    void upload(e.target.files?.[0], key);
                    e.target.value = "";
                  }}
                />
              </label>
              <button
                type="button"
                className="a-button"
                onClick={() => onChange({ ...value, [key]: "" })}
              >
                恢复默认
              </button>
            </div>
          ))}
          <small>
            PNG、JPG 或 GIF，最大 2 MB；上传完成后保存设置。
            {uploading && " 正在上传…"}
          </small>
        </div>
      )}
      <Field label="店铺介绍">
        <textarea
          maxLength={1000}
          rows={2}
          value={value.site_desc}
          onChange={(e) => onChange({ ...value, site_desc: e.target.value })}
        />
      </Field>
      <Field label="店铺公告">
        <textarea
          maxLength={4000}
          rows={3}
          value={value.announcement}
          onChange={(e) => onChange({ ...value, announcement: e.target.value })}
        />
      </Field>
      <Field label="客服联系方式">
        <input
          maxLength={1000}
          value={value.customer_service}
          onChange={(e) =>
            onChange({ ...value, customer_service: e.target.value })
          }
        />
      </Field>
      <Field label="兑换地址" hint="留空时使用主站商品的兑换入口。">
        <input
          type="url"
          maxLength={1000}
          value={value.default_redeem_url}
          onChange={(e) =>
            onChange({ ...value, default_redeem_url: e.target.value })
          }
        />
      </Field>
      <Field label="兑换说明">
        <textarea
          maxLength={4000}
          rows={3}
          value={value.redeem_guide}
          onChange={(e) => onChange({ ...value, redeem_guide: e.target.value })}
        />
      </Field>
      {error && <Banner>{error}</Banner>}
    </fieldset>
  );
}
