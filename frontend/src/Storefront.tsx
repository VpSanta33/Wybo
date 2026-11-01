import {
  useEffect,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from "react";
import { Link } from "react-router-dom";
import {
  ArrowLeft,
  ArrowRight,
  Check,
  CheckCircle2,
  ChevronDown,
  CreditCard,
  Gamepad2,
  Headphones,
  LoaderCircle,
  LockKeyhole,
  Mail,
  Minus,
  Moon,
  PackageCheck,
  Plus,
  ShoppingBag,
  Sparkles,
  Ticket,
  WandSparkles,
  Zap,
} from "lucide-react";
import { track, createOrder, type Catalog, type ShopOrder } from "./api";
import { money, type Product } from "./data";
import { SiteBrand } from "./Site";
import ProductVisual, { BrandImage, ProductThumbnail } from "./ProductVisual";
import { pricePending, productBrand } from "./productBrands";
import { ErrorMessage, Modal, OrderSummary, PaymentAction } from "./components";
import CheckoutVerification, {
  type VerificationState,
} from "./CheckoutVerification";

function categoryPresentation(name = "") {
  if (/claude/i.test(name))
    return { icon: WandSparkles, caption: "认真倾听，回应每个好想法。" };
  if (/grok/i.test(name))
    return { icon: Zap, caption: "让好奇，发现更大的世界。" };
  if (/影音|音乐|video|spotify|youtube/i.test(name))
    return { icon: Headphones, caption: "把时间，留给喜欢。" };
  if (/工具|效率|tool/i.test(name))
    return { icon: WandSparkles, caption: "让每一天，轻盈一点。" };
  if (/游戏|game/i.test(name))
    return { icon: Gamepad2, caption: "下一站，快乐世界。" };
  return { icon: Sparkles, caption: "一点灵感，无限可能。" };
}

function compactPlanName(name: string) {
  const brand = productBrand(name);
  const label =
    brand && name.toLowerCase().startsWith(brand.name.toLowerCase())
      ? name.slice(brand.name.length).trim()
      : name;
  return label.replace(/\s*会员\s*·\s*/, " · ").replace(/^[·\s]+/, "") || name;
}

export default function Storefront({
  catalog,
  onTerms,
  onHelp,
  onCopy,
}: {
  catalog: Catalog | null;
  onTerms: () => void;
  onHelp: () => void;
  onCopy: (value: string) => void;
}) {
  const [categoryId, setCategoryId] = useState<number | null>(null);
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [quantity, setQuantity] = useState(1);
  const [email, setEmail] = useState("");
  const [emailError, setEmailError] = useState("");
  const [coupon, setCoupon] = useState("");
  const [agreed, setAgreed] = useState(false);
  const [review, setReview] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [order, setOrder] = useState<ShopOrder | null>(null);
  const [verification, setVerification] = useState<VerificationState>({
    ready: false,
    token: "",
  });
  const [verificationRevision, setVerificationRevision] = useState(0);
  const resultRef = useRef<HTMLHeadingElement>(null);
  const demo = catalog?.demo ?? true;
  const categories =
    catalog?.categories.filter((c) =>
      catalog.products.some((p) => p.category_id === c.id),
    ) ?? [];
  // Include uncategorized catalog entries so all real products remain purchasable.
  for (const p of catalog?.products ?? []) {
    if (!categories.some((c) => c.id === p.category_id))
      categories.push({ id: p.category_id, name: "精选商品" });
  }
  const activeCategory =
    categories.find((c) => c.id === categoryId) ?? categories[0];
  const categoryIndex = Math.max(
    0,
    categories.findIndex((c) => c.id === activeCategory?.id),
  );
  const products =
    catalog?.products.filter((p) => p.category_id === activeCategory?.id) ?? [];
  const product =
    products.find((p) => p.id === selectedId) ??
    products.find((p) => p.delivery_mode === "manual" || p.stock > 0) ??
    products[0];
  useEffect(() => {
    if (product && !demo) {
      track("visit");
    }
  }, [product?.id, demo]);
  const pendingPrice = product ? pricePending(product) : false;
  const maxQuantity = product
    ? Math.max(
        1,
        Math.min(product.delivery_mode === "manual" ? 99 : product.stock, 99),
      )
    : 1;
  const actualQuantity = Math.min(quantity, maxQuantity);
  const total = (product?.price_cents ?? 0) * actualQuantity;

  useEffect(() => {
    if (order) resultRef.current?.focus();
  }, [order]);

  function chooseCategory(id: number) {
    if (busy) return;
    setCategoryId(id);
    setSelectedId(null);
    setQuantity(1);
    setError("");
  }

  function chooseProduct(p: Product) {
    if (
      busy ||
      (p.delivery_mode !== "manual" && p.stock <= 0 && !pricePending(p))
    )
      return;
    if (!demo) track("select", p.id);
    setSelectedId(p.id);
    setQuantity(1);
    setError("");
  }

  function categoryKey(event: KeyboardEvent<HTMLButtonElement>, index: number) {
    let next = index;
    if (event.key === "ArrowRight") next = (index + 1) % categories.length;
    else if (event.key === "ArrowLeft")
      next = (index - 1 + categories.length) % categories.length;
    else if (event.key === "Home") next = 0;
    else if (event.key === "End") next = categories.length - 1;
    else return;
    event.preventDefault();
    chooseCategory(categories[next].id);
    document.getElementById(`category-${categories[next].id}`)?.focus();
  }

  function validateEmail() {
    const invalid = !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
    setEmailError(invalid ? "请填写完整的邮箱地址，例如 you@example.com" : "");
    return !invalid;
  }

  function reviewOrder(event: FormEvent) {
    event.preventDefault();
    if (
      !product ||
      pendingPrice ||
      (product.delivery_mode !== "manual" && product.stock <= 0) ||
      busy
    )
      return;
    if (!validateEmail()) {
      document.getElementById("delivery-email")?.focus();
      return;
    }
    if (!agreed) {
      setError("请先阅读并同意购买条款。");
      return;
    }
    setError("");
    if (product && !demo) track("select", product.id);
    setVerification({ ready: false, token: "" });
    setReview(true);
  }

  async function placeOrder() {
    if (
      !product ||
      pendingPrice ||
      (product.delivery_mode !== "manual" && product.stock <= 0) ||
      busy
    )
      return;
    if (!demo && !verification.ready) return;
    setBusy(true);
    setError("");
    try {
      setOrder(
        await createOrder(
          product,
          actualQuantity,
          email.trim(),
          coupon.trim(),
          demo,
          verification.token,
        ),
      );
    } catch (e) {
      setError(e instanceof Error ? e.message : "订单创建失败，请稍后重试。");
      setVerification({ ready: false, token: "" });
      setVerificationRevision((v) => v + 1);
    } finally {
      setBusy(false);
    }
  }

  return (
    <>
      <header className="store-header container">
        <SiteBrand />
        <div className="product-tabs" role="tablist" aria-label="选择商品类别">
          {categories.length ? (
            categories.map((c, index) => {
              const Icon = categoryPresentation(c.name).icon;
              const active = c.id === activeCategory?.id;
              return (
                <button
                  key={c.id}
                  type="button"
                  role="tab"
                  id={`category-${c.id}`}
                  aria-selected={active}
                  aria-controls="store-product-panel"
                  tabIndex={active ? 0 : -1}
                  disabled={busy}
                  onClick={() => chooseCategory(c.id)}
                  onKeyDown={(e) => categoryKey(e, index)}
                  className={active ? "is-active" : ""}
                >
                  {productBrand(c.name) ? (
                    <BrandImage name={c.name} />
                  ) : (
                    <Icon size={17} aria-hidden="true" />
                  )}
                  <span>{c.name}</span>
                </button>
              );
            })
          ) : (
            <span className="tabs-loading">正在准备小美好…</span>
          )}
        </div>
        <div className="store-header-actions">
          <button
            className="language-button"
            type="button"
            onClick={onHelp}
            aria-label="打开帮助中心"
          >
            <Headphones size={16} aria-hidden="true" />
            <span>帮助中心</span>
          </button>
          <Link to="/orders" className="my-orders">
            <ShoppingBag size={17} aria-hidden="true" />
            <span>我的订单</span>
          </Link>
        </div>
      </header>

      <section
        className="store-stage container"
        id="store-product-panel"
        role={activeCategory ? "tabpanel" : undefined}
        aria-labelledby={
          activeCategory ? `category-${activeCategory.id}` : undefined
        }
      >
        <div className="product-showcase">
          <div className="showcase-art" key={product?.id ?? "loading"}>
            <span className="showcase-halo" />
            <ProductVisual
              product={product}
              categoryName={activeCategory?.name ?? ""}
              variant={categoryIndex}
            />
          </div>
          <div className="showcase-caption">
            <span className="showcase-overline">A LITTLE MOON MAGIC</span>
            <h2>{product?.name ?? activeCategory?.name ?? "望月小铺"}</h2>
            <p>{categoryPresentation(activeCategory?.name).caption}</p>
            <div className="showcase-pagination" aria-hidden="true">
              {(categories.length ? categories : [0, 1, 2, 3]).map((c, i) => (
                <span
                  className={i === categoryIndex ? "current" : ""}
                  key={typeof c === "number" ? c : c.id}
                />
              ))}
            </div>
          </div>
          <div className="showcase-footnote">
            <Zap size={13} aria-hidden="true" />
            <span>自助选购</span>
            <span className="dot-separator" />
            <span>
              {product?.delivery_mode === "manual"
                ? "付款后由店主手工发货"
                : "支付后自动发货"}
            </span>
          </div>
        </div>

        <div className="purchase-card" aria-busy={!catalog}>
          {!catalog ? (
            <div className="store-loading" role="status">
              <LoaderCircle className="spin" size={26} />
              <h2>正在准备小美好</h2>
              <p>马上就好，请稍等一下。</p>
            </div>
          ) : !product ? (
            <div className="store-loading">
              <PackageCheck size={30} />
              <h2>{catalog.notice ? "小铺暂时无法加载" : "新品即将到来"}</h2>
              <p>{catalog.notice || "月兔正在整理商品，稍后再来看看。"}</p>
              {catalog.notice && (
                <button
                  type="button"
                  className="button button-secondary"
                  onClick={() => window.location.reload()}
                >
                  重新加载
                </button>
              )}
            </div>
          ) : (
            <>
              <div className="purchase-heading">
                <div className="purchase-heading-top">
                  <span className="purchase-eyebrow">
                    PICK YOUR LITTLE HAPPINESS
                  </span>
                  {demo && (
                    <span className="demo-badge">
                      <span />
                      演示预览
                    </span>
                  )}
                </div>
                <div className="purchase-title">
                  <BrandImage name={activeCategory?.name ?? ""} />
                  <h1>{activeCategory?.name}</h1>
                </div>
                <p>选一份喜欢的，美好即刻抵达。</p>
              </div>
              {catalog.notice && (
                <p className="store-notice">
                  <Moon size={14} aria-hidden="true" />
                  {catalog.notice}
                </p>
              )}
              <form className="store-form" onSubmit={reviewOrder}>
                <fieldset className="plan-fieldset">
                  <legend>选择套餐</legend>
                  <div className="plan-options">
                    {products.map((p) => (
                      <label
                        className={`plan-option ${p.id === product.id ? "is-selected" : ""} ${p.delivery_mode !== "manual" && p.stock <= 0 && !pricePending(p) ? "is-unavailable" : ""} ${pricePending(p) ? "is-pending" : ""}`}
                        key={p.id}
                      >
                        <input
                          type="radio"
                          name="product-plan"
                          aria-label={p.name}
                          aria-describedby={`plan-price-${p.id}`}
                          value={p.id}
                          checked={p.id === product.id}
                          disabled={
                            (p.delivery_mode !== "manual" &&
                              p.stock <= 0 &&
                              !pricePending(p)) ||
                            busy
                          }
                          onChange={() => chooseProduct(p)}
                        />
                        <span className="plan-radio" aria-hidden="true">
                          {p.id === product.id && (
                            <Check size={11} strokeWidth={3} />
                          )}
                        </span>
                        <ProductThumbnail product={p} />
                        <span className="plan-copy">
                          <strong title={p.name}>
                            {compactPlanName(p.name)}
                          </strong>
                        </span>
                        <span className="plan-price" id={`plan-price-${p.id}`}>
                          {pricePending(p) ? (
                            <span className="pending-price">待定价</span>
                          ) : p.delivery_mode !== "manual" && p.stock <= 0 ? (
                            <span>暂时售罄</span>
                          ) : (
                            <>
                              <small>¥</small>
                              <strong>{money(p.price_cents)}</strong>
                            </>
                          )}
                        </span>
                      </label>
                    ))}
                  </div>
                </fieldset>
                {pendingPrice && (
                  <p className="pending-plan-note" role="status">
                    套餐即将上架，定价后开放购买。
                  </p>
                )}

                <div className="purchase-inputs">
                  <div className="store-field">
                    <label htmlFor="delivery-email">接收邮箱</label>
                    <div
                      className={`email-input ${emailError ? "has-error" : ""}`}
                    >
                      <Mail size={17} aria-hidden="true" />
                      <input
                        id="delivery-email"
                        name="email"
                        type="email"
                        required
                        autoComplete="email"
                        placeholder="输入邮箱地址"
                        value={email}
                        maxLength={128}
                        aria-invalid={!!emailError}
                        aria-describedby="delivery-email-hint"
                        disabled={busy}
                        onChange={(e) => {
                          setEmail(e.target.value);
                          setEmailError("");
                        }}
                        onBlur={() => {
                          if (email) validateEmail();
                        }}
                      />
                    </div>
                  </div>
                  <div className="quantity-row">
                    <label htmlFor="purchase-quantity">购买数量</label>
                    <div className="store-stepper">
                      <button
                        type="button"
                        aria-label="减少数量"
                        onClick={() => setQuantity((q) => Math.max(1, q - 1))}
                        disabled={
                          actualQuantity <= 1 ||
                          busy ||
                          pendingPrice ||
                          (product.delivery_mode !== "manual" &&
                            product.stock <= 0)
                        }
                      >
                        <Minus size={15} />
                      </button>
                      <input
                        id="purchase-quantity"
                        type="number"
                        min={1}
                        max={maxQuantity}
                        value={actualQuantity}
                        disabled={
                          busy ||
                          pendingPrice ||
                          (product.delivery_mode !== "manual" &&
                            product.stock <= 0)
                        }
                        onChange={(e) =>
                          setQuantity(
                            Math.min(
                              maxQuantity,
                              Math.max(
                                1,
                                Math.floor(Number(e.target.value) || 1),
                              ),
                            ),
                          )
                        }
                      />
                      <button
                        type="button"
                        aria-label="增加数量"
                        onClick={() =>
                          setQuantity((q) => Math.min(maxQuantity, q + 1))
                        }
                        disabled={
                          actualQuantity >= maxQuantity ||
                          busy ||
                          pendingPrice ||
                          (product.delivery_mode !== "manual" &&
                            product.stock <= 0)
                        }
                      >
                        <Plus size={15} />
                      </button>
                    </div>
                  </div>
                  <small
                    id="delivery-email-hint"
                    className={`purchase-input-hint ${emailError ? "field-error" : ""}`}
                    role={emailError ? "alert" : undefined}
                  >
                    {emailError || "邮箱用于接收商品和查询订单，请准确填写。"}
                  </small>
                </div>

                <div className="checkout-service-note">
                  <span>
                    <CreditCard size={14} aria-hidden="true" />
                    在线支付
                  </span>
                  <span>
                    <PackageCheck size={14} aria-hidden="true" />
                    {product.delivery_mode === "manual"
                      ? "付款后手工发货"
                      : "付款后自动发卡"}
                  </span>
                </div>

                {!demo && !pendingPrice && (
                  <details className="coupon-disclosure">
                    <summary>
                      <Ticket size={14} aria-hidden="true" />
                      使用优惠码
                      <ChevronDown size={14} aria-hidden="true" />
                    </summary>
                    <label className="sr-only" htmlFor="purchase-coupon">
                      优惠码
                    </label>
                    <input
                      id="purchase-coupon"
                      placeholder="输入优惠码"
                      value={coupon}
                      disabled={busy}
                      maxLength={32}
                      onChange={(e) => setCoupon(e.target.value)}
                    />
                    <small>优惠以最终订单金额为准。</small>
                  </details>
                )}
                <div className="purchase-agreement">
                  <label>
                    <input
                      type="checkbox"
                      required
                      checked={agreed}
                      disabled={busy}
                      onChange={(e) => setAgreed(e.target.checked)}
                    />
                    <span>我已阅读并同意</span>
                  </label>
                  <button type="button" onClick={onTerms}>
                    购买条款
                  </button>
                </div>
                {error && !review && <ErrorMessage>{error}</ErrorMessage>}
                <div className="checkout-actions">
                  <div className="purchase-total">
                    <div>
                      <span>合计金额</span>
                    </div>
                    <strong aria-live="polite">
                      {pendingPrice ? (
                        <span className="total-pending-price">待定价</span>
                      ) : (
                        <>
                          <small>CNY</small>
                          {money(total)}
                        </>
                      )}
                    </strong>
                  </div>
                  <button
                    className="review-button"
                    type="submit"
                    disabled={
                      busy ||
                      pendingPrice ||
                      (product.delivery_mode !== "manual" && product.stock <= 0)
                    }
                  >
                    <span>
                      {pendingPrice
                        ? "套餐待上架"
                        : product.delivery_mode !== "manual" &&
                            product.stock <= 0
                          ? "暂时售罄"
                          : "确认订单"}
                    </span>
                    <ArrowRight size={18} aria-hidden="true" />
                  </button>
                </div>
                <p className="purchase-secure">
                  <LockKeyhole size={12} aria-hidden="true" />
                  {demo
                    ? "当前为演示预览，可完整体验下单流程"
                    : "安全下单 · 订单随时可查"}
                </p>
              </form>
              <details className="product-description" key={product.id}>
                <summary>
                  商品说明与兑换方式
                  <Plus size={15} aria-hidden="true" />
                </summary>
                <p>{product.description}</p>
                {product.tags.length > 0 && <p>{product.tags.join(" · ")}</p>}
                {product.redeem_guide && <p>{product.redeem_guide}</p>}
              </details>
            </>
          )}
        </div>
      </section>

      {review && product && (
        <Modal
          title={order ? "订单已创建" : "确认你的订单"}
          onClose={() => {
            if (!busy) {
              setReview(false);
              setError("");
              setOrder(null);
            }
          }}
        >
          {order ? (
            <div className="checkout-result">
              <div className="result-icon">
                <CheckCircle2 size={32} />
              </div>
              <span className="eyebrow">YOUR ORDER IS READY</span>
              <h2 ref={resultRef} tabIndex={-1}>
                {demo ? "演示订单已创建" : "订单已创建，等待支付"}
              </h2>
              <p className="muted">
                {demo
                  ? "本次体验不会扣款，也不会发放真实卡密。"
                  : "请保存订单号，在有效期内完成支付。"}
              </p>
              <OrderSummary order={order} onCopy={onCopy} />
              {!demo && <PaymentAction order={order} />}
              <Link
                className={`button ${demo ? "button-primary" : "button-secondary"} full-width`}
                to={`/orders?order_no=${encodeURIComponent(order.order_no)}`}
              >
                查看我的订单
                <ArrowRight size={17} />
              </Link>
              {demo && (
                <p className="result-footnote">
                  演示订单在本标签页内有效，刷新后失效。
                </p>
              )}
            </div>
          ) : (
            <div className="order-review">
              <span className="review-icon">
                <ShoppingBag size={26} />
              </span>
              <span className="eyebrow">ONE MORE LITTLE STEP</span>
              <h2>确认你的订单</h2>
              <p className="muted">核对一下，美好就要出发了。</p>
              <div className="review-summary">
                <div>
                  <span>商品</span>
                  <strong>{product.name}</strong>
                </div>
                <div>
                  <span>数量</span>
                  <strong>{actualQuantity} 份</strong>
                </div>
                <div>
                  <span>接收邮箱</span>
                  <strong>{email.trim()}</strong>
                </div>
                <div className="review-summary-total">
                  <span>合计</span>
                  <strong>¥{money(total)}</strong>
                </div>
              </div>
              {demo && (
                <p className="soft-note">
                  <Sparkles size={16} />
                  这是一笔演示订单，不会产生真实付款。
                </p>
              )}
              {error && <ErrorMessage>{error}</ErrorMessage>}
              {!demo && (
                <CheckoutVerification
                  key={verificationRevision}
                  onChange={setVerification}
                />
              )}
              <button
                type="button"
                className="button button-primary full-width"
                disabled={busy || (!demo && !verification.ready)}
                onClick={placeOrder}
              >
                {busy ? (
                  <>
                    <LoaderCircle className="spin" size={17} />
                    正在创建订单…
                  </>
                ) : (
                  <>
                    {demo ? "创建演示订单" : "提交订单"}
                    <ArrowRight size={17} />
                  </>
                )}
              </button>
              <button
                className="review-back"
                type="button"
                disabled={busy}
                onClick={() => {
                  setReview(false);
                  setError("");
                }}
              >
                <ArrowLeft size={14} />
                返回修改
              </button>
            </div>
          )}
        </Modal>
      )}
    </>
  );
}
