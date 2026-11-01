import { useEffect, useRef, useState, type FormEvent } from "react";
import { Link, NavLink, Route, Routes, useLocation } from "react-router-dom";
import {
  ArrowDown,
  ArrowLeft,
  ArrowRight,
  CheckCircle2,
  ChevronRight,
  Clock3,
  Copy,
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
  loadCatalog,
  queryOrder,
  recentOrders,
  type Catalog,
  type ShopOrder,
} from "./api";
import { faqs } from "./data";
import { BunnyMark } from "./Illustrations";
import SupportTickets from "./SupportTickets";
import { ErrorMessage, Modal, OrderSummary, PaymentAction } from "./components";
import Storefront from "./Storefront";

function Brand({ name }: { name?: string }) {
  return (
    <Link className="store-brand" to="/" aria-label="Wybo 望月小铺首页">
      <BunnyMark />
      <span>
        Wybo<span className="brand-period">.</span>
      </span>
      <span className="brand-chinese">{name || "望月小铺"}</span>
    </Link>
  );
}

function App() {
  const [catalog, setCatalog] = useState<Catalog | null>(null);
  const [info, setInfo] = useState<"help" | "terms" | "privacy" | null>(null);
  const [toast, setToast] = useState("");
  const location = useLocation();
  const mainRef = useRef<HTMLElement>(null);
  const firstRoute = useRef(true);
  const visitPath = useRef("");
  useEffect(() => {
    const agent = new URLSearchParams(location.search).get("agent");
    if (agent) sessionStorage.setItem("wybo-agent", agent);
  }, [location.search]);
  useEffect(() => {
    if (!catalog || catalog.demo || visitPath.current === location.pathname)
      return;
    visitPath.current = location.pathname;
    void api("/visits", {
      method: "POST",
      body: JSON.stringify({
        path: location.pathname,
        source: document.referrer,
      }),
    }).catch(() => {});
  }, [catalog, location.pathname]);
  const isHome = location.pathname === "/";

  useEffect(() => {
    const controller = new AbortController();
    loadCatalog(controller.signal)
      .then(setCatalog)
      .catch(() => {});
    return () => controller.abort();
  }, []);

  useEffect(() => {
    if (firstRoute.current) {
      firstRoute.current = false;
      return;
    }
    window.scrollTo({ top: 0 });
    mainRef.current?.focus({ preventScroll: true });
  }, [location.pathname]);

  useEffect(() => {
    if (catalog?.site?.site_name)
      document.title = catalog.site.site_name + " · Wybo";
  }, [catalog?.site?.site_name]);
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

  return (
    <div className="app-shell">
      <a className="skip-link" href="#main-content">
        跳到主要内容
      </a>
      {!isHome && (
        <header className="store-header container">
          <Brand name={catalog?.site?.site_name} />
          <nav className="inner-nav" aria-label="主导航">
            <NavLink to="/">选购商品</NavLink>
            <NavLink to="/orders">我的订单</NavLink>
            <NavLink to="/guide">购买指南</NavLink>
          </nav>
          <button className="header-support" onClick={() => setInfo("help")}>
            <Headphones size={17} aria-hidden="true" />
            <span>帮助中心</span>
          </button>
        </header>
      )}
      <main id="main-content" ref={mainRef} tabIndex={-1}>
        <Routes>
          <Route
            path="/"
            element={
              <Storefront
                catalog={catalog}
                onTerms={() => setInfo("terms")}
                onHelp={() => setInfo("help")}
                onCopy={copy}
              />
            }
          />
          <Route path="/orders" element={<Orders onCopy={copy} />} />
          <Route
            path="/guide"
            element={
              <Guide
                onHelp={() => setInfo("help")}
                demo={catalog?.demo ?? true}
              />
            }
          />
          <Route
            path="*"
            element={
              <div className="empty-page container">
                <BunnyMark />
                <h1>这个页面还没有开门</h1>
                <p>回到小铺，看看新的小美好。</p>
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
          © {new Date().getFullYear()} Wybo
          <span className="footer-moon">一点月光，刚好喜欢。</span>
        </span>
        <div>
          <Link to="/guide">购买指南</Link>
          <button onClick={() => setInfo("privacy")}>隐私说明</button>
          <button
            className="footer-contact"
            onClick={() => setInfo("help")}
            aria-label="联系与帮助"
          >
            <Headphones size={17} aria-hidden="true" />
            <span>联系小兔</span>
          </button>
        </div>
      </footer>
      {info && (
        <Modal
          title={
            info === "help"
              ? "帮助中心"
              : info === "terms"
                ? "购买条款"
                : "隐私说明"
          }
          onClose={() => setInfo(null)}
        >
          <div className="info-icon">
            <BunnyMark />
          </div>
          <span className="eyebrow">WYBO STORE</span>
          <h2>
            {info === "help"
              ? "小兔在，慢慢说。"
              : info === "terms"
                ? "购买前，请读一读"
                : "好好保管你的信息"}
          </h2>
          {info === "help" ? (
            <>
              <p className="muted">
                关于订单、卡密和购买方式，在这里找到答案。
              </p>
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
              <p className="soft-note">
                {catalog?.demo
                  ? "当前为演示预览，订单不会扣款，也不会发放真实卡密。"
                  : catalog?.site?.customer_service ||
                    "店主暂未公布人工客服联系方式，请先查看购买指南或查询订单。"}
              </p>
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
  async function lookup(event?: FormEvent) {
    event?.preventDefault();
    if (!orderNo.trim() || !contact.trim() || busy) return;
    setBusy(true);
    setError("");
    setOrder(null);
    try {
      setOrder(await queryOrder(orderNo.trim(), contact.trim()));
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
        回小铺逛逛
      </Link>
      <div className="page-intro">
        <span className="eyebrow">FIND YOUR LITTLE HAPPINESS</span>
        <h1>
          每一份美好，都有迹可循<span>✧</span>
        </h1>
        <p>填好你的小凭证，让月兔帮你找回订单与卡密。</p>
      </div>
      <div className="order-page-layout">
        <div className="query-card">
          <div className="query-card-heading">
            <span className="benefit-icon lavender">
              <PackageCheck size={24} />
            </span>
            <div>
              <h2>找一找，我的订单</h2>
              <p>无需登录，有订单号就能安心找回。</p>
            </div>
          </div>
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
              />
              <small>需要与下单时填写的信息完全一致哦。</small>
            </div>
            {error && <ErrorMessage>{error}</ErrorMessage>}
            <button
              className="button button-primary full-width"
              disabled={busy}
            >
              {busy ? (
                <LoaderCircle className="spin" size={18} />
              ) : (
                <Search size={18} />
              )}
              {busy ? "小兔正在寻找…" : "找回我的小美好"}
              <ArrowRight size={17} />
            </button>
          </form>
          {order && (
            <div className="query-result" aria-live="polite">
              <h3>
                <CheckCircle2 size={18} />
                找到你的订单啦
              </h3>
              <OrderSummary order={order} onCopy={onCopy} />

              {order.after_sales
                ?.filter((v) => v.kind === "upgrade")
                .map((v) => (
                  <div className="soft-note" key={v.child_order_no}>
                    <span>
                      {v.status === "completed"
                        ? "已完成升级；原卡已被替换。"
                        : v.status === "canceled"
                          ? "升级补差订单已关闭。"
                          : "有一笔升级补差订单等待付款。"}
                      <br />
                      补差订单：{v.child_order_no}
                    </span>
                    <button
                      className="text-button"
                      onClick={async () => {
                        try {
                          setOrder(
                            await queryOrder(v.child_order_no, contact.trim()),
                          );
                          setOrderNo(v.child_order_no);
                        } catch (e) {
                          setError((e as Error).message);
                        }
                      }}
                    >
                      查看补差订单
                    </button>
                  </div>
                ))}
              {order.after_sales
                ?.filter((v) => v.kind !== "upgrade")
                .map((v, i) => (
                  <p className="soft-note" key={`${v.kind}-${i}`}>
                    {v.kind === "replacement"
                      ? "此订单已换发卡密，下方为当前有效卡密。"
                      : "此订单已登记售后退款。"}
                  </p>
                ))}
              {order.status === "preview" && (
                <p className="soft-note">
                  <Sparkles size={17} />
                  演示订单仅用于体验，不产生付款或真实卡密。
                </p>
              )}
              {order.status === "completed" && (
                <div className="delivered-cards">
                  <h3>你的卡密，请好好收下</h3>
                  {order.cards?.length ? (
                    order.cards.map((card, index) => (
                      <div className="delivered-card" key={index}>
                        <code>
                          {card.code}
                          {card.secret ? `\n密码：${card.secret}` : ""}
                        </code>
                        <button
                          className="icon-button"
                          onClick={() =>
                            onCopy(
                              `${card.code}${card.secret ? `\n${card.secret}` : ""}`,
                            )
                          }
                          aria-label={`复制第${index + 1}张卡密`}
                        >
                          <Copy size={17} />
                        </button>
                      </div>
                    ))
                  ) : (
                    <p>
                      {order.after_sales?.some(
                        (v) => v.kind === "upgrade" && v.status === "completed",
                      )
                        ? "原卡已升级，请查看补差订单获取新卡密。"
                        : "卡密正在准备中，请稍后刷新订单状态。"}
                    </p>
                  )}
                </div>
              )}
              {order.status === "pending" && (
                <>
                  <PaymentAction order={order} />
                  <button
                    className="cancel-order"
                    onClick={cancel}
                    disabled={cancelBusy}
                  >
                    {cancelBusy ? "正在取消…" : "取消这笔订单"}
                  </button>
                </>
              )}
              {["pending", "paid", "refunding"].includes(order.status) && (
                <button
                  className="button button-secondary full-width refresh-order"
                  onClick={() => lookup()}
                  disabled={busy}
                >
                  刷新订单状态
                </button>
              )}
              {order.status !== "preview" && (
                <SupportTickets
                  key={order.order_no}
                  orderNo={order.order_no}
                  contact={order.contact || contact.trim()}
                />
              )}
            </div>
          )}
        </div>
        <aside className="order-aside">
          <div className="order-bunny">
            <BunnyMark />
            <span>别着急，美好不会走丢。</span>
          </div>
          <h2>
            <Clock3 size={18} />
            最近的小期待
          </h2>
          <p className="muted">只记录当前浏览器中的最近订单。</p>
          {history.length ? (
            <div className="recent-orders">
              {history.map((item) => (
                <button
                  key={item.order_no}
                  onClick={() => {
                    setOrderNo(item.order_no);
                    setOrder(null);
                    setError("");
                    document.getElementById("query-contact")?.focus();
                  }}
                >
                  <span className="recent-icon">
                    <ShoppingBag size={18} />
                  </span>
                  <span>
                    <strong>{item.name}</strong>
                    <small>{item.order_no}</small>
                  </span>
                  <ChevronRight size={16} />
                </button>
              ))}
            </div>
          ) : (
            <div className="recent-empty">
              <Mail size={29} strokeWidth={1.3} />
              <p>
                还没有留下购物足迹。
                <br />
                第一份小美好，正等你发现。
              </p>
              <Link to="/" className="text-link">
                去挑选喜欢的
                <ArrowRight size={15} />
              </Link>
            </div>
          )}
          <p className="order-aside-note">
            <ShieldCheck size={15} />
            联系方式不保存在浏览器订单历史中。
          </p>
        </aside>
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
      title: "挑一份喜欢",
      text: "在顶部切换商品类别，选择商品，并阅读适用范围与兑换说明。",
    },
    {
      icon: Mail,
      title: "留下小凭证",
      text: "填写接收邮箱、确认购买数量，核对订单后保存好订单号。",
    },
    {
      icon: ShieldCheck,
      title: "安心去支付",
      text: "正式订单会跳转收银台。请在订单有效期内完成付款。",
    },
    {
      icon: Gift,
      title: "收下小美好",
      text: "支付完成后自动发卡，在订单查询页领取卡密，按说明兑换。",
    },
  ];
  return (
    <section className="container inner-page guide-page">
      <Link to="/" className="back-link">
        <ArrowLeft size={16} />
        回小铺逛逛
      </Link>
      <div className="page-intro">
        <span className="eyebrow">YOUR FIRST LITTLE ADVENTURE</span>
        <h1>
          第一次来？让小兔带路<span>✧</span>
        </h1>
        <p>把复杂的事交给我们，你只管期待下一份美好。</p>
      </div>
      <div className="guide-steps">
        {steps.map((step, i) => (
          <article key={step.title}>
            <span className="step-number">STEP 0{i + 1}</span>
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
            {demo ? "来一次轻松的橱窗体验" : "把购物凭证，好好保管"}
          </strong>
          {demo
            ? "当前样例商品可创建演示订单，不会产生真实付款。演示订单只在当前标签页内有效，刷新后可以重新体验。"
            : "订单号和下单时填写的联系方式是找回卡密的凭证。付款完成后，请返回订单查询页刷新状态，卡密以实际到账结果为准。"}
        </p>
      </div>
      <div className="guide-faq-heading">
        <span className="eyebrow">A FEW LITTLE ANSWERS</span>
        <h2>这些小问题，我们替你想到了。</h2>
      </div>
      <Faq items={faqs} />
      <div className="guide-bottom">
        <BunnyMark />
        <div>
          <h2>还有没解开的小问号？</h2>
          <p>小兔的帮助口袋，也为你留着位置。</p>
        </div>
        <button onClick={onHelp} className="button button-secondary">
          找小兔帮忙
          <Headphones size={17} />
        </button>
      </div>
      <Link className="button button-primary guide-cta" to="/">
        准备好了，去逛逛
        <ArrowDown size={17} />
      </Link>
    </section>
  );
}

export default App;
