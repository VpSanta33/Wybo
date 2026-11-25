import {
  useEffect,
  useCallback,
  useRef,
  useState,
  type FormEvent,
  type KeyboardEvent,
} from "react";
import { Link } from "react-router-dom";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  BookOpen,
  Check,
  CheckCircle2,
  ChevronDown,
  ChevronLeft,
  ChevronRight,
  CreditCard,
  Gamepad2,
  Headphones,
  LayoutGrid,
  LoaderCircle,
  LockKeyhole,
  Mail,
  Minus,
  PackageCheck,
  Plus,
  ShoppingBag,
  Sparkles,
  Ticket,
  WandSparkles,
  Waves,
  Zap,
} from "lucide-react";
import { api, track, createOrder, type Catalog, type ShopOrder, type PaymentGateway } from "./api";
import { money, type Category, type Product } from "./data";
import { SiteBrand, SiteLogo, useSite } from "./Site";
import { IslandPostmark } from "./Illustrations";
import ProductVisual, { BrandImage, ProductThumbnail } from "./ProductVisual";
import { pricePending, productBrand } from "./productBrands";
import { CashierFeeNotice, ErrorMessage, Modal, OrderSummary, PaymentAction, PaymentChoices, PaymentLimitNotice, payableGateway } from "./components";
import { cashierEstimate } from "./payments";
import CheckoutVerification, {
  type VerificationState,
} from "./CheckoutVerification";

function categoryIcon(name: string) {
  if (/影音|音乐|video|spotify|youtube/i.test(name)) return Headphones;
  if (/工具|效率|tool|claude/i.test(name)) return WandSparkles;
  if (/游戏|game/i.test(name)) return Gamepad2;
  if (/grok/i.test(name)) return Zap;
  return Sparkles;
}

function compactPlanName(name: string) {
  const brand = productBrand(name);
  const label =
    brand && name.toLowerCase().startsWith(brand.name.toLowerCase())
      ? name.slice(brand.name.length).trim()
      : name;
  return label.replace(/\s*会员\s*·\s*/, " · ").replace(/^[·\s]+/, "") || name;
}

function ProductInformation({ product }: { product: Product }) {
  const details = useRef<HTMLDetailsElement>(null);
  useEffect(() => {
    const desktop = window.matchMedia("(min-width: 701px)");
    const setDefaultOpen = () => {
      if (details.current) details.current.open = desktop.matches;
    };
    setDefaultOpen();
    desktop.addEventListener("change", setDefaultOpen);
    return () => desktop.removeEventListener("change", setDefaultOpen);
  }, []);

  return (
    <details className="showcase-details" ref={details}>
      <summary>
        {product.redeem_guide ? "商品说明与兑换方式" : "商品说明"}
        <ChevronDown size={16} aria-hidden="true" />
      </summary>
      {product.description && <p>{product.description}</p>}
      {product.tags.length > 0 && (
        <div className="showcase-tags">
          {product.tags.map((tag) => <span key={tag}>{tag}</span>)}
        </div>
      )}
      {product.redeem_guide && <p><strong>兑换方式</strong>{product.redeem_guide}</p>}
    </details>
  );
}

function CategoryNavigation({
  categories,
  activeId,
  disabled,
  onSelect,
}: {
  categories: Category[];
  activeId?: number;
  disabled: boolean;
  onSelect: (id: number) => void;
}) {
  const navigationRef = useRef<HTMLDivElement>(null);
  const viewportRef = useRef<HTMLDivElement>(null);
  const listRef = useRef<HTMLDivElement>(null);
  const [scroll, setScroll] = useState({
    overflow: false,
    back: false,
    next: false,
  });
  const signature = JSON.stringify(categories);
  const measure = useCallback(() => {
    const navigation = navigationRef.current;
    const viewport = viewportRef.current;
    const list = listRef.current;
    if (!navigation || !viewport || !list) return;
    const next = {
      overflow: list.scrollWidth > navigation.clientWidth - 10 + 1,
      back: viewport.scrollLeft > 1,
      next:
        viewport.scrollLeft + viewport.clientWidth < viewport.scrollWidth - 1,
    };
    setScroll((current) =>
      current.overflow === next.overflow &&
      current.back === next.back &&
      current.next === next.next
        ? current
        : next,
    );
  }, []);
  const revealActive = useCallback((smooth = false) => {
    const viewport = viewportRef.current;
    const active = listRef.current?.querySelector<HTMLButtonElement>(
      '[aria-selected="true"]',
    );
    if (!viewport || !active) return;
    const frame = viewport.getBoundingClientRect();
    const tab = active.getBoundingClientRect();
    if (tab.left < frame.left + 5 || tab.right > frame.right - 5) {
      viewport.scrollTo({
        left:
          viewport.scrollLeft +
          tab.left -
          frame.left -
          (viewport.clientWidth - tab.width) / 2,
        behavior:
          smooth &&
          !window.matchMedia("(prefers-reduced-motion: reduce)").matches
            ? "smooth"
            : "auto",
      });
    }
  }, []);
  useEffect(() => {
    const observer = new ResizeObserver(() => {
      measure();
      revealActive();
    });
    for (const element of [
      navigationRef.current,
      viewportRef.current,
      listRef.current,
    ]) {
      if (element) observer.observe(element);
    }
    measure();
    return () => observer.disconnect();
  }, [signature, measure, revealActive]);
  useEffect(() => {
    revealActive(true);
  }, [activeId, signature, revealActive]);

  function browse(direction: number) {
    const viewport = viewportRef.current;
    if (!viewport) return;
    viewport.scrollBy({
      left: direction * Math.max(120, viewport.clientWidth * 0.8),
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "auto"
        : "smooth",
    });
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
    onSelect(categories[next].id);
    document
      .getElementById(`category-${categories[next].id}`)
      ?.focus({ preventScroll: true });
  }
  return (
    <div
      ref={navigationRef}
      className={`category-navigation${scroll.overflow ? " is-overflowing" : ""}`}
    >
      {scroll.overflow && (
        <button
          type="button"
          className="category-scroll-button"
          aria-label="向左浏览商品分类"
          aria-controls="store-category-tabs"
          disabled={!scroll.back}
          onClick={() => browse(-1)}
        >
          <ChevronLeft size={18} aria-hidden="true" />
        </button>
      )}
      <div ref={viewportRef} className="product-tabs" onScroll={measure}>
        <div
          ref={listRef}
          className="product-tabs-list"
          role="tablist"
          aria-label="选择商品类别"
          id="store-category-tabs"
        >
          {categories.length ? (
            categories.map((c, index) => {
              const Icon = categoryIcon(c.name);
              const active = c.id === activeId;
              return (
                <button
                  key={c.id}
                  type="button"
                  role="tab"
                  id={`category-${c.id}`}
                  aria-selected={active}
                  aria-controls="store-product-panel"
                  tabIndex={active ? 0 : -1}
                  disabled={disabled}
                  onClick={() => onSelect(c.id)}
                  onKeyDown={(event) => categoryKey(event, index)}
                  className={active ? "is-active" : ""}
                  title={c.name}
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
            <span className="tabs-loading">正在加载商品分类…</span>
          )}
        </div>
      </div>
      {scroll.overflow && (
        <button
          type="button"
          className="category-scroll-button"
          aria-label="向右浏览商品分类"
          aria-controls="store-category-tabs"
          disabled={!scroll.next}
          onClick={() => browse(1)}
        >
          <ChevronRight size={18} aria-hidden="true" />
        </button>
      )}
    </div>
  );
}

export default function Storefront({
  catalog,
  detail,
  onTerms,
  onHelp,
  onCopy,
}: {
  catalog: Catalog | null;
  detail: boolean;
  onTerms: () => void;
  onHelp: () => void;
  onCopy: (value: string) => void;
}) {
  const { site } = useSite();
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
  const [gateways, setGateways] = useState<PaymentGateway[] | null>(null);
  const [gateway, setGateway] = useState("");
  const [gatewayError, setGatewayError] = useState("");
  const [gatewayRevision, setGatewayRevision] = useState(0);
  const [verification, setVerification] = useState<VerificationState>({
    ready: false,
    token: "",
  });
  const [verificationRevision, setVerificationRevision] = useState(0);
  const resultRef = useRef<HTMLHeadingElement>(null);
  const demo = catalog?.demo ?? true;
  const hasCatalogItems = !catalog || catalog.products.length > 0;
  const catalogError = catalog && !catalog.demo && !catalog.site ? catalog.notice : "";
  useEffect(() => {
    if (demo) { setGateways([]); setGateway(""); setGatewayError(""); return; }
    const controller = new AbortController();
    setGateways(null);
    setGatewayError("");
    api<PaymentGateway[]>("/gateways", { signal: controller.signal }).then((rows) => {
      if (controller.signal.aborted) return;
      setGateways(rows);
      setGateway((current) => rows.some((g) => g.code === current) ? current : rows[0]?.code || "");
    }).catch((e) => { if (!controller.signal.aborted) setGatewayError((e as Error).message); });
    return () => controller.abort();
  }, [demo, gatewayRevision]);
  const selectedGateway = gateways?.find((g) => g.code === gateway);
  const paymentUnavailable = !demo && (!selectedGateway || !!gatewayError || !gateways);
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
    products.find((p) => !pricePending(p) && (p.delivery_mode === "manual" || p.stock > 0)) ??
    products.find((p) => p.delivery_mode === "manual" || p.stock > 0) ??
    products[0];
  useEffect(() => {
    if (product && !demo) {
      track("visit");
    }
  }, [product?.id, demo]);
  const paused =
    catalog?.site?.is_station === "true" &&
    catalog?.site?.station_status !== "active";
  const pendingPrice = product ? pricePending(product) : false;
  const maxQuantity = product
    ? Math.max(
        1,
        Math.min(product.delivery_mode === "manual" ? 99 : product.stock, 99),
      )
    : 1;
  const actualQuantity = Math.min(quantity, maxQuantity);
  const total = (product?.price_cents ?? 0) * actualQuantity;
  const estimate = cashierEstimate(total, selectedGateway);
  const beforeDiscount = Boolean(coupon.trim());
  const totalLabel = beforeDiscount
    ? "优惠前预计支付"
    : estimate.rateBps ? "预计支付" : "合计金额";
  // 数量变化导致超过渠道单笔限额时，自动改选可用渠道；全部超额则不能提交。
  useEffect(() => {
    if (gateways) setGateway((current) => payableGateway(gateways, total, current));
  }, [gateways, total]);

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

  function validateEmail() {
    const invalid = !/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim());
    setEmailError(invalid ? "请填写完整的邮箱地址，例如 you@example.com" : "");
    return !invalid;
  }

  function reviewOrder(event: FormEvent) {
    event.preventDefault();
    if (
      !product ||
      paused ||
      pendingPrice ||
      (product.delivery_mode !== "manual" && product.stock <= 0) ||
      paymentUnavailable ||
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
      paused ||
      pendingPrice ||
      (product.delivery_mode !== "manual" && product.stock <= 0) ||
      paymentUnavailable ||
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
      <header className="store-header-shell">
        <div className="store-topbar container">
          <SiteBrand />
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
        </div>
      </header>

      {detail ? (
        <nav className="container product-back" aria-label="商品导航">
          <Link className="back-link" to="/"><ArrowLeft size={16} />返回商店</Link>
        </nav>
      ) : (
        <>
          <section className="market-postcard container" aria-labelledby="market-name">
            <div className="postcard-copy">
              <h1 id="market-name">{site.site_name}</h1>
              {site.site_desc && <p>{site.site_desc}</p>}
              <div className="postcard-links">
                {hasCatalogItems && (
                  <a className="postcard-browse" href="#store-catalog">
                    挑选商品 <ArrowDown size={16} aria-hidden="true" />
                  </a>
                )}
                <Link to="/guide" className={hasCatalogItems ? undefined : "postcard-browse"}>
                  <BookOpen size={16} aria-hidden="true" />购买指南
                </Link>
              </div>
            </div>
            <IslandPostmark />
            <img className="postcard-art" src="/images/island-shop.webp" width="1134" height="756" fetchPriority="high" alt="" />
          </section>

          <div id="store-catalog" className="container">
            {categories.length > 0 && (
              <div className="category-dock">
                <h2><LayoutGrid size={19} aria-hidden="true" />商品分类</h2>
                <CategoryNavigation
                  categories={categories}
                  activeId={activeCategory?.id}
                  disabled={busy}
                  onSelect={chooseCategory}
                />
              </div>
            )}
          </div>
        </>
      )}

      <section
        className="store-stage container"
        id="store-product-panel"
        role={!detail && activeCategory ? "tabpanel" : undefined}
        aria-labelledby={
          !detail && activeCategory ? `category-${activeCategory.id}` : undefined
        }
      >
        {product && (
          <article className="product-showcase">
            <div className="showcase-banner">
              <div className="showcase-identity">
                <span className="showcase-delivery-pill">
                  <PackageCheck size={15} aria-hidden="true" />
                  {product.delivery_mode === "manual" ? "手工发货" : "自动发卡"}
                </span>
                {detail ? <h1>{product.name}</h1> : (
                  <h2><Link to={`/p/${product.id}`} title="打开商品详情">{product.name}</Link></h2>
                )}
                <div className="showcase-price">
                  <strong>{pendingPrice ? "待定价" : `¥${money(product.price_cents)}`}</strong>
                  {product.delivery_mode !== "manual" && !pendingPrice && (
                    <span>{product.stock ? `库存 ${product.stock} 份` : "暂时售罄"}</span>
                  )}
                </div>
              </div>
              <div className="showcase-art" key={product.id}>
                <ProductVisual product={product} variant={categoryIndex} />
              </div>
            </div>
            {!detail && (
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
                        form="store-order-form"
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
            )}
            {pendingPrice && (
              <p className="pending-plan-note" role="status">
                套餐即将上架，定价后开放购买。
              </p>
            )}
            {(product.description || product.tags.length > 0 || product.redeem_guide) && (
              <ProductInformation key={product.id} product={product} />
            )}
          </article>
        )}

        <div className="purchase-card" aria-busy={!catalog}>
          {!catalog ? (
            <div className="store-loading" role="status">
              <LoaderCircle className="spin" size={26} />
              <h2>正在加载商品目录</h2>
              <p>马上就好，请稍等片刻。</p>
            </div>
          ) : !product ? (
            <div className="store-loading">
              <SiteLogo url={site.site_logo} />
              <h2>{catalogError ? "商店暂时无法加载" : detail ? "商品不存在或已下架" : "新品即将上架"}</h2>
              <p>{catalogError || (detail ? "返回商店查看当前上架的商品。" : catalog.notice || "商品正在整理上架中，请稍后再来看看。")}</p>
              {catalogError && (
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
                  {demo && (
                    <span className="demo-badge">
                      <span />
                      演示预览
                    </span>
                  )}
                </div>
                <div className="purchase-title">
                  <span className="checkout-bag"><ShoppingBag size={21} aria-hidden="true" /></span>
                  <h2>订单结算</h2>
                </div>
                <p className="checkout-product-name">{product.name}</p>
              </div>
              {catalog.notice && (
                <p className="store-notice">
                  <Waves size={14} aria-hidden="true" />
                  {catalog.notice}
                </p>
              )}
              <form id="store-order-form" className="store-form" onSubmit={reviewOrder}>
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
                          paused ||
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
                          paused ||
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
                          paused ||
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

                {!demo && (
                  <div className="checkout-payment-section">
                    {gateways === null && !gatewayError && <p className="payment-loading" role="status"><LoaderCircle size={14} className="spin" aria-hidden="true" />正在读取支付方式…</p>}
                    {!!gateways?.length && <PaymentChoices gateways={gateways} value={gateway} onChange={setGateway} disabled={busy || paused || pendingPrice} amountCents={total} name="checkout-payment" checkout />}
                    {!!gateways?.length && !pendingPrice && <PaymentLimitNotice gateways={gateways} amountCents={total} />}
                    {gatewayError && <ErrorMessage>{gatewayError}<button type="button" className="payment-reload" onClick={() => setGatewayRevision((v) => v + 1)}>重新读取支付方式</button></ErrorMessage>}
                    {gateways?.length === 0 && !gatewayError && <p className="soft-note" role="status">商店暂未启用支付方式，请稍后再试或联系店主。</p>}
                  </div>
                )}

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

                {!demo &&
                  !pendingPrice &&
                  catalog?.site?.is_station !== "true" && (
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
                {!pendingPrice && <CashierFeeNotice estimate={estimate} beforeDiscount={beforeDiscount} />}
                <div className="checkout-actions">
                  <div className="purchase-total">
                    <div>
                      <span>{totalLabel}</span>
                    </div>
                    <strong aria-live="polite">
                      {pendingPrice ? (
                        <span className="total-pending-price">待定价</span>
                      ) : (
                        <>
                          <small>CNY</small>
                          {money(estimate.totalCents)}
                        </>
                      )}
                    </strong>
                  </div>
                  <button
                    className="review-button"
                    type="submit"
                    disabled={
                      busy ||
                      paused ||
                      pendingPrice ||
                      paymentUnavailable ||
                      (product.delivery_mode !== "manual" && product.stock <= 0)
                    }
                  >
                    <span>
                      {paused
                        ? "店铺暂停营业"
                        : pendingPrice
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
            </>
          )}
        </div>
      </section>

      {!detail && !!catalog?.products.length && (
        <details className="container catalog-links">
          <summary>全部商品（{catalog.products.length}）</summary>
          <ul>{catalog.products.map((p) => <li key={p.id}><Link to={`/p/${p.id}`}>{p.name}</Link></li>)}</ul>
        </details>
      )}

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
              <h2 ref={resultRef} tabIndex={-1}>
                {demo ? "演示订单已创建" : "订单已创建，等待支付"}
              </h2>
              <p className="muted">
                {demo
                  ? "本次体验不会扣款，也不会发放真实卡密。"
                  : "请保存订单号，在有效期内完成支付。"}
              </p>
              <OrderSummary order={order} onCopy={onCopy} />
              {!demo && <PaymentAction order={order} initialGateway={gateway} />}
              <Link
                className={`button ${demo ? "button-primary" : "button-secondary"} full-width`}
                to={`/orders?order_no=${encodeURIComponent(order.order_no)}`}
                state={{ paymentContact: order.contact }}
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
              <h2>确认你的订单</h2>
              <p className="muted">请核对商品、数量与接收邮箱后提交订单。</p>
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
                {!demo && selectedGateway && <div><span>支付方式</span><strong>{selectedGateway.label || selectedGateway.name}</strong></div>}
                {estimate.rateBps === 0 && (
                  <div className="review-summary-total">
                    <span>{totalLabel}</span>
                    <strong>¥{money(total)}</strong>
                  </div>
                )}
              </div>
              <CashierFeeNotice estimate={estimate} beforeDiscount={beforeDiscount} />
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
                disabled={busy || paymentUnavailable || (!demo && !verification.ready)}
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
