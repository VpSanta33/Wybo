import { useState, type FormEvent } from "react";
import { Link } from "react-router-dom";
import {
  ArrowRight,
  Check,
  FolderPlus,
  ImagePlus,
  LoaderCircle,
  Pencil,
  Plus,
  Search,
  Tags,
} from "lucide-react";
import { BrandImage } from "../ProductVisual";
import { useAccess } from "./access";
import { Modal } from "../components";
import {
  adminApi,
  useAdminQuery,
  yuan,
  toCents,
  type AdminProduct,
  type AdminCategory,
} from "./api";
import {
  Banner,
  Busy,
  Empty,
  Field,
  PageHeading,
  QueryError,
  Status,
} from "./ui";
export function ProductEditor({
  product,
  categories,
  onClose,
  onSaved,
}: {
  product: AdminProduct | null;
  categories: AdminCategory[];
  onClose: () => void;
  onSaved: () => void;
}) {
  const [name, setName] = useState(product?.name || "");
  const [category, setCategory] = useState(
    product?.category_id || categories[0]?.id || 0,
  );
  const [cost, setCost] = useState(
    product?.cost_cents == null ? "" : (product.cost_cents / 100).toFixed(2),
  );
  const [price, setPrice] = useState(
    product ? (product.price_cents / 100).toFixed(2) : "",
  );
  const [description, setDescription] = useState(product?.description || "");
  const [cover, setCover] = useState(product?.cover || "");
  const [guide, setGuide] = useState(product?.redeem_guide || "");
  const [redeemUrl, setRedeemUrl] = useState(product?.redeem_url || "");
  const [enabled, setEnabled] = useState(product?.enabled ?? true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  let profit: number | null = null;
  try {
    if (price && cost) profit = toCents(price) - toCents(cost);
  } catch {
    /* Incomplete decimal input has no preview. */
  }
  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await adminApi(product ? `/products/${product.id}` : "/products", {
        method: product ? "PUT" : "POST",
        body: JSON.stringify({
          name: name.trim(),
          category_id: category,
          description,
          cover,
          cost_cents: toCents(cost),
          price_cents: toCents(price),
          redeem_guide: guide,
          redeem_url: redeemUrl.trim(),
          enabled,
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
      title={product ? "编辑商品" : "新增商品"}
      onClose={() => {
        if (!busy) onClose();
      }}
    >
      <form className="a-editor" onSubmit={save}>
        <span className="a-eyebrow">PRODUCT & PRICING</span>
        <h2>{product ? "编辑商品" : "添一份新好物"}</h2>
        <p>成本与售价分开记录，利润自然清楚。</p>
        {error && <Banner>{error}</Banner>}
        <Field label="商品名称">
          <input
            required
            maxLength={128}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="例如 ChatGPT Plus · 一个月"
          />
        </Field>
        <Field label="商品分类">
          <select
            required
            value={category}
            onChange={(e) => setCategory(Number(e.target.value))}
          >
            <option value={0} disabled>
              请选择分类
            </option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name}
                {!c.enabled ? "（已停用）" : ""}
              </option>
            ))}
          </select>
        </Field>
        <div className="a-form-grid">
          <Field label="采购成本（元）">
            <input
              required
              type="number"
              min="0"
              max="100000000"
              step="0.01"
              inputMode="decimal"
              placeholder="105.00"
              value={cost}
              onChange={(e) => setCost(e.target.value)}
            />
          </Field>
          <Field label="销售价格（元）" hint="填 0 表示待定价，前台不可购买">
            <input
              required
              type="number"
              min="0"
              max="100000000"
              step="0.01"
              inputMode="decimal"
              placeholder="115.00"
              value={price}
              onChange={(e) => setPrice(e.target.value)}
            />
          </Field>
        </div>
        <div
          className={`a-price-preview ${profit !== null && profit < 0 ? "is-loss" : ""}`}
        >
          <span>预计每份销售利润</span>
          <strong>{profit === null ? "—" : yuan(profit)}</strong>
          <small>
            {profit !== null && price && Number(price) > 0
              ? `利润率 ${((profit / toCents(price)) * 100).toFixed(2)}%`
              : "售价 − 采购成本"}
          </small>
        </div>
        <Field label="商品图片地址" hint="留空时自动使用品牌图片">
          <input
            value={cover}
            onChange={(e) => setCover(e.target.value)}
            placeholder="https://… 或 /uploads/…"
          />
        </Field>
        <Field label="商品说明">
          <textarea
            rows={3}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </Field>
        <Field
          label="兑换地址"
          hint="留空使用“邮件与推送 → 站点信息”中的默认兑换地址；单独填写可覆盖，订单页和发货邮件同步生效"
        >
          <input
            type="url"
            maxLength={255}
            value={redeemUrl}
            onChange={(e) => setRedeemUrl(e.target.value)}
            placeholder="留空使用站点默认兑换地址"
          />
        </Field>
        <Field
          label="兑换说明"
          hint="说明如何使用卡密、需要登录哪个账号，以及兑换步骤"
        >
          <textarea
            rows={2}
            value={guide}
            onChange={(e) => setGuide(e.target.value)}
          />
        </Field>
        <label className="a-check">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          在前台上架销售
        </label>
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
            {busy ? (
              <LoaderCircle size={16} className="spin" />
            ) : (
              <Check size={16} />
            )}
            保存商品
          </button>
        </div>
      </form>
    </Modal>
  );
}
export default function Products({
  base,
  notify,
}: {
  base: string;
  notify: (m: string) => void;
}) {
  const { can, user } = useAccess();
  const q = useAdminQuery<{ list: AdminProduct[]; total: number }>(
    "/products?size=100",
  );
  const c = useAdminQuery<AdminCategory[]>("/categories");
  const [search, setSearch] = useState("");
  const [edit, setEdit] = useState<AdminProduct | null | undefined>(undefined);
  const [categoryOpen, setCategoryOpen] = useState(false);
  const [newCategory, setNewCategory] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function categorySave(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await adminApi("/categories", {
        method: "POST",
        body: JSON.stringify({ name: newCategory.trim() }),
      });
      setNewCategory("");
      c.reload();
      notify("分类已添加");
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  if ((q.loading && !q.data) || (c.loading && !c.data)) return <Busy />;
  if (q.error || c.error)
    return (
      <QueryError
        error={q.error || c.error}
        reload={() => {
          q.reload();
          c.reload();
        }}
      />
    );
  const products = q.data?.list || [];
  const filtered = products.filter((p) =>
    p.name.toLowerCase().includes(search.toLowerCase()),
  );
  const missing = products.filter((p) => p.cost_cents === null).length;
  return (
    <>
      <PageHeading
        title="商品与定价"
        description="好商品，合理定价；每一份利润都有依据。"
      >
        <button
          className="a-button"
          disabled={!can("catalog.write")}
          onClick={() => setCategoryOpen(true)}
        >
          <Tags size={15} />
          商品分类
        </button>
        <button
          className="a-button primary"
          onClick={() => setEdit(null)}
          disabled={!c.data?.length || !can("catalog.write")}
        >
          <Plus size={16} />
          新增商品
        </button>
      </PageHeading>
      {missing > 0 && user?.role !== "support" && (
        <div className="a-inline-note">
          <ImagePlus size={17} />
          <p>
            <strong>{missing} 件商品尚未设置采购成本</strong> ·
            保存成本后，新订单就会自动记录利润。
          </p>
        </div>
      )}
      <section className="a-panel">
        <div className="a-list-toolbar">
          <div>
            <h2>
              全部商品 <span className="a-count">{q.data?.total || 0}</span>
            </h2>
          </div>
          <label className="a-search">
            <Search size={16} />
            <input
              aria-label="搜索商品"
              placeholder="搜索商品名称…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
            />
          </label>
        </div>
        {filtered.length ? (
          <div className="a-table-wrap">
            <table className="a-products-table">
              <thead>
                <tr>
                  <th>商品</th>
                  {user?.role !== "support" && <th>采购成本</th>}
                  <th>售价</th>
                  {user?.role !== "support" && <th>预计单份利润</th>}
                  <th>库存</th>
                  <th>状态</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((p) => (
                  <tr key={p.id}>
                    <td>
                      <div className="a-product-cell">
                        <span className="a-product-avatar">
                          <BrandImage name={p.name} />
                        </span>
                        <div>
                          <strong>{p.name}</strong>
                          <small>
                            {c.data?.find((c) => c.id === p.category_id)
                              ?.name || "未分类"}{" "}
                            · #{p.id}
                          </small>
                        </div>
                      </div>
                    </td>
                    {user?.role !== "support" && (
                      <td>
                        {p.cost_cents === null ? (
                          <span className="a-muted">待设置</span>
                        ) : (
                          yuan(p.cost_cents)
                        )}
                      </td>
                    )}
                    <td>
                      {p.price_cents > 0 ? (
                        yuan(p.price_cents)
                      ) : (
                        <span className="a-muted">待定价</span>
                      )}
                    </td>
                    {user?.role !== "support" && (
                      <td
                        className={
                          p.cost_cents !== null &&
                          p.price_cents - p.cost_cents < 0
                            ? "a-negative"
                            : "a-profit"
                        }
                      >
                        {p.cost_cents === null || p.price_cents === 0
                          ? "—"
                          : yuan(p.price_cents - p.cost_cents)}
                      </td>
                    )}
                    <td>
                      <Link
                        className={p.stock <= 5 ? "a-stock-low" : "a-text-link"}
                        to={
                          can("inventory")
                            ? `${base}/inventory?product=${p.id}`
                            : `${base}/products`
                        }
                      >
                        {p.stock} 份
                      </Link>
                    </td>
                    <td>
                      <Status
                        value={p.enabled ? "completed" : "disabled"}
                        label={p.enabled ? "已上架" : "已下架"}
                      />
                    </td>
                    <td>
                      <button
                        className="a-table-button"
                        aria-label={`编辑 ${p.name}`}
                        disabled={!can("catalog.write")}
                        onClick={() => setEdit(p)}
                      >
                        <Pencil size={14} />
                        编辑
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        ) : (
          <Empty
            title={search ? "没有找到这件商品" : "小铺还没有商品"}
            text={search ? "试试其他名称。" : "先添加分类，再创建第一件商品。"}
          />
        )}
        <div className="a-table-footer">
          当前显示 {products.length} 件商品
          <span>
            库存由可售卡密自动计算 <ArrowRight size={12} />
          </span>
        </div>
      </section>
      {edit !== undefined && (
        <ProductEditor
          product={edit}
          categories={c.data || []}
          onClose={() => setEdit(undefined)}
          onSaved={() => {
            setEdit(undefined);
            q.reload();
            notify("商品已保存，前台下次加载即会更新");
          }}
        />
      )}
      {categoryOpen && (
        <Modal title="商品分类" onClose={() => setCategoryOpen(false)}>
          <div className="a-editor">
            <span className="a-eyebrow">CATEGORIES</span>
            <h2>商品分类</h2>
            <p>用于前台顶部分类和商品归属。</p>
            {error && <Banner>{error}</Banner>}
            <div className="a-category-list">
              {c.data?.map((x) => (
                <div key={x.id}>
                  <span>
                    <FolderPlus size={16} />
                    {x.name}
                  </span>
                  <Status
                    value={x.enabled ? "completed" : "disabled"}
                    label={x.enabled ? "启用中" : "已停用"}
                  />
                </div>
              ))}
            </div>
            <form onSubmit={categorySave}>
              <Field label="新增分类名称">
                <input
                  required
                  maxLength={64}
                  value={newCategory}
                  onChange={(e) => setNewCategory(e.target.value)}
                  placeholder="例如 Gemini"
                />
              </Field>
              <button className="a-button primary" disabled={busy}>
                <Plus size={15} />
                添加分类
              </button>
            </form>
          </div>
        </Modal>
      )}
    </>
  );
}
