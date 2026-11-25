import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link, NavLink, Route, Routes, useLocation } from "react-router-dom";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Gift,
  Headphones,
  LoaderCircle,
  Mail,
  PackageCheck,
  Plus,
  Search,
  ShieldCheck,
  ShoppingBag,
  Sparkles,
} from "lucide-react";
import {
  api,
  demoMode,
  loadCatalog,
  queryOrder,
  checkOrderPayment,
  recentOrders,
  type Catalog,
  type ShopOrder,
} from "./api";
import { faqs } from "./data";
import OrderResult from "./OrderResult";
import { ErrorMessage, Modal } from "./components";
import Storefront from "./Storefront";
import { isProductPath } from "./metadata";
import { SiteBrand, SiteLogo, useSite } from "./Site";
import { ContactButton, SupportContacts } from "./SupportContacts";
import { paymentSession, closeCashierWindow, hasCashierWindow } from "./paymentSession";

function App() {
  const { site } = useSite();
  const [listing, setListing] = useState<{ path: string; catalog: Catalog } | null>(null);
  const [info, setInfo] = useState<"help" | "terms" | "privacy" | null>(null);
  const [toast, setToast] = useState("");
  const location = useLocation();
  const detail = isProductPath(location.pathname);
  const isShop = location.pathname === "/" || detail;
  const shopPath = isShop ? location.pathname : null;
  const catalog = listing && listing.path === shopPath ? listing.catalog : null;
  const mainRef = useRef<HTMLElement>(null);
  const firstRoute = useRef(true);
  const visitPath = useRef("");
  useEffect(() => {
    const agent = new URLSearchParams(location.search).get("agent");
    if (agent) sessionStorage.setItem("wybo-agent", agent);
  }, [location.search]);
  useEffect(() => {
    if (demoMode || visitPath.current === location.pathname)
      return;
    visitPath.current = location.pathname;
    void api("/visits", {
      method: "POST",
      body: JSON.stringify({
        path: location.pathname,
        source: document.referrer,
      }),
    }).catch(() => {});
  }, [location.pathname]);

  useEffect(() => {
    if (shopPath === null) return;
    const controller = new AbortController();
    loadCatalog(controller.signal, shopPath === "/" ? undefined : shopPath.slice(3))
      .then((result) => {
        if (!controller.signal.aborted) setListing({ path: shopPath, catalog: result });
      })
      .catch(() => {});
    return () => controller.abort();
  }, [shopPath]);

  useEffect(() => {
    if (firstRoute.current) {
      firstRoute.current = false;
      return;
    }
    window.scrollTo({ top: 0 });
    mainRef.current?.focus({ preventScroll: true });
  }, [location.pathname]);

  useEffect(() => {
    if (!toast) return;
    const timer = window.setTimeout(() => setToast(""), 3200);
    return () => clearTimeout(timer);
  }, [toast]);

  async function copy(value: string) {
    try {
      await navigator.clipboard.writeText(value);
      setToast("已复制到剪贴板");
    } catch {
      setToast("暂时无法自动复制，请选中文字手动复制。");
    }
  }

  const shop = (
    <Storefront
      key={location.pathname}
      detail={detail}
      catalog={catalog}
      onTerms={() => setInfo("terms")}
      onHelp={() => setInfo("help")}
      onCopy={copy}
    />
  );

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>
      {!isShop && (
        <header className="store-header container">
          <SiteBrand />
          <nav className="inner-nav" aria-label="主导航">
            <NavLink to="/">选购商品</NavLink>
            <NavLink to="/orders">我的订单</NavLink>
            <NavLink to="/guide">购买指南</NavLink>
          </nav>
          <ContactButton onClick={() => setInfo("help")} />
        </header>
      )}
      <main id="main-content" ref={mainRef} tabIndex={-1}>
        {site.is_station === "true" && site.station_status !== "active" && (
          <p className="container store-paused-banner" role="status">
            店铺暂停营业，已有订单仍可查询及申请售后。
          </p>
        )}
        <Routes>
          <Route path="/" element={shop} />
          <Route path="/p/*" element={shop} />
          <Route path="/orders" element={<Orders onCopy={copy} />} />
          <Route
            path="/guide"
            element={
              <Guide
                onHelp={() => setInfo("help")}
                demo={demoMode}
              />
            }
          />
          <Route
            path="*"
            element={
              <div className="empty-page container">
                <SiteLogo url={site.site_logo} />
                <h1>这个页面还没有开放</h1>
                <p>返回首页，继续浏览当前在售的数字商品。</p>
                <Link className="button button-primary" to="/">
                  返回首页
                  <ArrowRight size={17} />
                </Link>
              </div>
            }
          />
        </Routes>
      </main>
      <footer className="store-footer container">
        <span>
          © {new Date().getFullYear()} {site.site_name}
          {site.site_footer && <span className="footer-tagline">{site.site_footer}</span>}
        </span>
        <div>
          {site.station_apply_enabled === "true" &&
            site.is_station !== "true" && <Link to="/apply">申请代理分站</Link>}
          <Link to="/guide">购买指南</Link>
          <button onClick={() => setInfo("privacy")}>隐私说明</button>
          <button
            className="footer-contact"
            onClick={() => setInfo("help")}
            aria-haspopup="dialog"
          >
            <Headphones size={17} aria-hidden="true" />
            <span>联系客服</span>
          </button>
        </div>
      </footer>
      {info && (
        <Modal
          title={
            info === "help"
              ? "联系客服"
              : info === "terms"
                ? "购买条款"
                : "隐私说明"
          }
          onClose={() => setInfo(null)}
        >
          <div className={info === "help" ? "support-dialog-heading" : undefined}>
            <div className="info-icon">
              <SiteLogo url={site.site_logo} />
            </div>
            <h2>
              {info === "help"
                ? "联系客服"
                : info === "terms"
                  ? "购买前，请先阅读"
                  : "隐私与凭证保护"}
            </h2>
          </div>
          {info === "help" ? (
            <>
              <SupportContacts placement="dialog" />
              <div className="help-links">
                <Link to="/orders" onClick={() => setInfo(null)}>
                  <PackageCheck size={22} />
                  <span>
                    <strong>订单与卡密</strong>
                    <small>查询订单、继续支付与领取卡密</small>
                  </span>
                  <ChevronRight size={18} />
                </Link>
                <Link to="/guide" onClick={() => setInfo(null)}>
                  <ShoppingBag size={22} />
                  <span>
                    <strong>购买指南</strong>
                    <small>从选购到收卡，一步一步来</small>
                  </span>
                  <ChevronRight size={18} />
                </Link>
              </div>
              {demoMode && <p className="soft-note">当前为演示预览，订单不会扣款，也不会发放真实卡密。</p>}
            </>
          ) : (
            <div className="legal-copy">
              {info === "terms" ? (
                <>
                  <p>
                    下单前请确认商品说明、适用地区、兑换方式与有效期。数字商品的具体售后条件以商品页面说明为准。
                  </p>
                  <p>
                    标注「演示预览」的商品用于体验前端流程，不会产生真实付款或卡密。正式商品需要通过店铺配置的支付渠道完成付款。
                  </p>
                  <p>
                    支付完成后，请在「我的订单」中查询卡密。请妥善保管订单号、接收邮箱和已领取的兑换信息。
                  </p>
                  <p>
                    本站与展示的产品品牌无隶属关系。产品名称仅用于说明对应商品。
                  </p>
                </>
              ) : (
                <>
                  <p>订单号与下单邮箱是查询凭证，请勿与卡密一同分享给他人。</p>
                  <p>
                    浏览器仅保存最近十笔订单的订单号、商品名与创建时间，不会将接收邮箱保存到本地历史列表。
                  </p>
                  <p>
                    演示订单与联系方式仅保存在当前标签页内存中，刷新后失效。正式下单信息会发送到本站订单接口。
                  </p>
                  {site.turnstile_enabled === "true" && (
                    <p>
                      下单时使用 Cloudflare Turnstile 验证访问者，验证数据与访问
                      IP 会发送至 Cloudflare，用于减少机器人恶意下单。
                    </p>
                  )}
                </>
              )}
            </div>
          )}
        </Modal>
      )}
      <div
        className={`toast ${toast ? "toast-visible" : ""}`}
        role="status"
        aria-live="polite"
      >
        {toast && (
          <>
            <CheckCircle2 size={18} />
            {toast}
          </>
        )}
      </div>
    </div>
  );
}

function Orders({ onCopy }: { onCopy: (value: string) => void }) {
  const { site } = useSite();
  const location = useLocation();
  const [orderNo, setOrderNo] = useState(
    new URLSearchParams(location.search).get("order_no") || "",
  );
  const [contact, setContact] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [order, setOrder] = useState<ShopOrder | null>(null);
  const [history] = useState(recentOrders);
  const [cancelBusy, setCancelBusy] = useState(false);
  const resultRef = useRef<HTMLHeadingElement>(null);

  useEffect(() => {
    const number = new URLSearchParams(location.search).get("order_no") || "";
    const stateContact: unknown = location.state?.paymentContact;
    const proof = typeof stateContact === "string" ? stateContact : paymentSession(number)?.contact;
    if (!number || !proof) return;
    const controller = new AbortController();
    setOrderNo(number);
    setContact(proof);
    setBusy(true);
    queryOrder(number, proof, controller.signal).then((next) => {
      if (!controller.signal.aborted) setOrder(next);
    }).catch((e) => {
      if (!controller.signal.aborted) setError(e.message);
    }).finally(() => { if (!controller.signal.aborted) setBusy(false); });
    return () => controller.abort();
  }, [location.search, location.state]);

  useEffect(() => {
    if (order && ["paid", "completed", "refunded"].includes(order.status) && closeCashierWindow(order.order_no)) {
      window.focus();
    }
  }, [order?.order_no, order?.status]);

  useEffect(() => {
    if (order?.order_no) resultRef.current?.focus({ preventScroll: true });
  }, [order?.order_no]);

  useEffect(() => {
    if (
      !order ||
      busy ||
      cancelBusy ||
      (!["pending", "paid"].includes(order.status) && !(order.status === "closed" && hasCashierWindow(order.order_no)))
    )
      return;
    const ctrl = new AbortController();
    let fetching = false;
    const refresh = async () => {
      if (order.status === "closed" && (!paymentSession(order.order_no) || Date.now() - Date.parse(order.expired_at || order.created_at) > 30 * 60 * 1000)) return;
      if ((document.visibilityState !== "visible" && !hasCashierWindow(order.order_no)) || fetching) return;
      fetching = true;
      try {
        if (["pending", "closed"].includes(order.status)) {
          await checkOrderPayment(order.order_no, order.contact || "", ctrl.signal).catch(() => {});
        }
        const current = await queryOrder(
          order.order_no,
          order.contact || "",
          ctrl.signal,
        );
        if (!ctrl.signal.aborted)
          setOrder((previous) =>
            previous?.order_no === current.order_no ? current : previous,
          );
      } catch {
        /* Keep the last known state and retry; never infer payment success locally. */
      } finally {
        fetching = false;
      }
    };
    const timer = window.setInterval(refresh, 6000);
    window.addEventListener("focus", refresh);
    document.addEventListener("visibilitychange", refresh);
    void refresh();
    return () => {
      ctrl.abort();
      window.clearInterval(timer);
      window.removeEventListener("focus", refresh);
      document.removeEventListener("visibilitychange", refresh);
    };
  }, [order?.order_no, order?.status, order?.contact, busy, cancelBusy]);

  async function lookup(event?: FormEvent, number = orderNo) {
    event?.preventDefault();
    if (!number.trim() || !contact.trim() || busy) return;
    setBusy(true);
    setError("");
    try {
      const next = await queryOrder(number.trim(), contact.trim());
      setOrder(next);
      setOrderNo(next.order_no);
    } catch (e) {
      setError(e instanceof Error ? e.message : "查询失败，请重试。");
    } finally {
      setBusy(false);
    }
  }
  async function cancel() {
    if (!order || !window.confirm("确认取消这笔待支付订单吗？")) return;
    setCancelBusy(true);
    setError("");
    try {
      await api(`/orders/${encodeURIComponent(order.order_no)}/cancel`, {
        method: "POST",
        body: JSON.stringify({ contact: order.contact || contact }),
      });
      closeCashierWindow(order.order_no);
      setOrder({ ...order, status: "closed" });
    } catch (e) {
      setError(e instanceof Error ? e.message : "取消失败，请重试。");
    } finally {
      setCancelBusy(false);
    }
  }
  return (
    <section className="container inner-page order-page">
      <Link to="/" className="back-link">
        <ArrowLeft size={16} />
        返回商店
      </Link>
      <div className="page-intro">
        <h1>
          我的订单 <Sparkles size={22} aria-hidden="true" />
        </h1>
        <p>查询付款状态、领取卡密，找到对应的兑换入口。</p>
      </div>
      <div className="order-page-layout">
        <div className="order-query-column">
          <div className={`query-card ${order ? "has-result" : ""}`}>
            <div className="query-card-heading">
              <span className="benefit-icon lavender">
                <PackageCheck size={22} />
              </span>
              <div>
                <h2>{order ? "已找到你的订单" : "查询订单"}</h2>
                <p>
                  {order
                    ? "卡密和兑换入口见查询结果"
                    : "凭订单号与下单联系方式查询"}
                </p>
              </div>
            </div>
            {error && <ErrorMessage>{error}</ErrorMessage>}
            <details className="order-query-form" open={!order}>
              <summary>
                查询其他订单<span>展开查询表单</span>
              </summary>
              <form onSubmit={lookup}>
                <div className="form-field">
                  <label htmlFor="query-order">
                    订单号 <span className="required">*</span>
                  </label>
                  <input
                    id="query-order"
                    value={orderNo}
                    onChange={(e) => {
                      setOrderNo(e.target.value);
                      setOrder(null);
                      setError("");
                    }}
                    required
                    maxLength={64}
                    placeholder="输入购买时的订单号"
                    autoComplete="off"
                  />
                </div>
                <div className="form-field">
                  <label htmlFor="query-contact">
                    下单时的联系方式 <span className="required">*</span>
                  </label>
                  <input
                    id="query-contact"
                    value={contact}
                    onChange={(e) => {
                      setContact(e.target.value);
                      setOrder(null);
                      setError("");
                    }}
                    required
                    maxLength={128}
                    placeholder="邮箱 / QQ / 其他联系方式"
                    autoComplete="email"
                    aria-describedby="query-contact-hint"
                  />
                  <small id="query-contact-hint">
                    须与下单时填写的信息完全一致。
                  </small>
                </div>
                <button
                  className="button button-primary full-width"
                  disabled={busy || cancelBusy}
                >
                  {busy ? (
                    <LoaderCircle className="spin" size={18} />
                  ) : (
                    <Search size={18} />
                  )}
                  {busy ? "正在查询…" : "查询订单"}
                  <ArrowRight size={17} />
                </button>
              </form>
            </details>
          </div>
          <details className="order-history" open={!order}>
            <summary>
              <Clock3 size={16} aria-hidden="true" />
              最近订单
              <span>
                {history.length ? `${history.length} 笔` : "当前浏览器"}
              </span>
            </summary>
            {history.length ? (
              <div className="recent-orders">
                {history.map((item) => (
                  <button
                    key={item.order_no}
                    disabled={busy || cancelBusy}
                    onClick={() => {
                      setOrderNo(item.order_no);
                      setOrder(null);
                      setError("");
                      document.getElementById("query-contact")?.focus();
                    }}
                  >
                    <ShoppingBag size={17} aria-hidden="true" />
                    <span>
                      <strong>{item.name}</strong>
                      <small>{item.order_no}</small>
                    </span>
                    <ChevronRight size={16} aria-hidden="true" />
                  </button>
                ))}
              </div>
            ) : (
              <p>这里会显示本浏览器最近创建的订单。</p>
            )}
            <p className="order-aside-note">
              <ShieldCheck size={14} aria-hidden="true" />
              仅保存订单记录，不保存联系方式。
            </p>
          </details>
        </div>
        <section
          className={`query-result-card ${!order ? "is-empty" : ""}`}
          aria-label="订单查询结果"
          aria-busy={busy}
        >
          {order ? (
            <>
              <div className="query-result-heading">
                <h2 ref={resultRef} tabIndex={-1}>
                  订单查询结果
                </h2>
                {["pending", "paid"].includes(order.status) && (
                  <span>状态自动更新</span>
                )}
              </div>
              <OrderResult
                order={order}
                onCopy={onCopy}
                busy={busy}
                cancelBusy={cancelBusy}
                onRefresh={() => lookup()}
                onCancel={cancel}
                onViewOrder={(number) => lookup(undefined, number)}
              />
            </>
          ) : (
            <div className="order-lookup-empty">
              <SiteLogo url={site.site_logo} />
              <h2>输入订单号与联系方式</h2>
              <p>提交查询信息后，即可查看付款状态、卡密内容与兑换说明。</p>
              <span>
                <ShieldCheck size={15} aria-hidden="true" />
                无需登录，验证订单凭证即可领取
              </span>
            </div>
          )}
        </section>
      </div>
    </section>
  );
}

function Faq({ items }: { items: typeof faqs }) {
  return (
    <div className="faq-list">
      {items.map((item, i) => (
        <details key={item.q}>
          <summary>
            <span className="faq-number">0{i + 1}</span>
            {item.q}
            <Plus size={17} aria-hidden="true" />
          </summary>
          <p>{item.a}</p>
        </details>
      ))}
    </div>
  );
}

function Guide({ onHelp, demo }: { onHelp: () => void; demo: boolean }) {
  const steps = [
    {
      icon: ShoppingBag,
      title: "选择商品套餐",
      text: "在商品类目中切换所需产品，确认套餐规格、适用范围与兑换说明。",
    },
    {
      icon: Mail,
      title: "填写接收邮箱",
      text: "填写准确的接收邮箱与购买数量，核对订单并保存订单号。",
    },
    {
      icon: ShieldCheck,
      title: "完成在线支付",
      text: "正式订单会跳转收银台或展示收款码。请在订单有效期内完成付款。",
    },
    {
      icon: Gift,
      title: "领取卡密与兑换",
      text: "自动发货商品付款后即刻发卡；手工发货商品由店主处理。前往我的订单领取使用。",
    },
  ];
  return (
    <section className="container inner-page guide-page">
      <Link to="/" className="back-link">
        <ArrowLeft size={16} />
        返回商店
      </Link>
      <div className="page-intro">
        <h1>
          四步完成选购与收货<span>·</span>
        </h1>
        <p>从选择套餐到领取卡密，流程清晰透明，随时可查。</p>
      </div>
      <div className="guide-steps">
        {steps.map((step, i) => (
          <article key={step.title}>
            <span className="step-number">{i + 1}</span>
            <div className="step-icon">
              <step.icon size={31} strokeWidth={1.4} />
            </div>
            <h2>{step.title}</h2>
            <p>{step.text}</p>
            {i < 3 && (
              <ArrowRight className="step-arrow" size={21} aria-hidden="true" />
            )}
          </article>
        ))}
      </div>
      <div className="guide-preview-note">
        <Sparkles size={19} />
        <p>
          <strong>
            {demo ? "演示预览说明" : "请妥善保管订单凭证"}
          </strong>
          {demo
            ? "当前样例商品可创建演示订单，不会产生真实付款。演示订单只在当前标签页内有效，刷新后可以重新体验。"
            : "订单号和下单时填写的联系方式是找回卡密的凭证。付款完成后，请返回订单查询页刷新状态，卡密以实际到账结果为准。"}
        </p>
      </div>
      <div className="guide-faq-heading">
        <h2>常见问题解答</h2>
      </div>
      <Faq items={faqs} />
      <div className="guide-bottom">
        <Headphones size={28} aria-hidden="true" />
        <div>
          <h2>还有其他疑问？</h2>
          <p>联系店铺客服，或进入订单查询页申请售后。</p>
        </div>
        <button onClick={onHelp} className="button button-secondary">
          联系客服
          <Headphones size={17} />
        </button>
      </div>
      <Link className="button button-primary guide-cta" to="/">
        前往选购商品
        <ArrowDown size={17} />
      </Link>
    </section>
  );
}

export default App;
