import {
  lazy,
  Suspense,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import {
  NavLink,
  Navigate,
  Route,
  Routes,
  Link,
  useLocation,
} from "react-router-dom";
import {
  ArrowLeft,
  ArrowUpRight,
  Bell,
  CheckCircle2,
  ChevronRight,
  CreditCard,
  LayoutDashboard,
  LogOut,
  Mail,
  Menu,
  Moon,
  Package,
  ShieldCheck,
  ShoppingBag,
  Ticket,
  Users,
  X,
  BarChart3,
  ListChecks,
  HandCoins,
  MessagesSquare,
  Database,
  Boxes,
  Store,
} from "lucide-react";
import { BunnyMark } from "../Illustrations";
import {
  adminApi,
  adminBase,
  checkEntry,
  token,
  TOKEN_KEY,
  type AdminUser,
} from "./api";
import { Banner, Busy } from "./ui";
const Stations = lazy(() => import("./Stations"));
const Dashboard = lazy(() => import("./Dashboard"));
import Products from "./Products";
import { Orders, Inventory, Agents } from "./Commerce";
import { Payments, Notifications, Security } from "./Settings";
import "./admin.css";
import { SiteLogo, useSite } from "../Site";
import { Settings } from "./SiteSettings";
import { AccessContext, StepUp } from "./access";
import AfterSalesVoice from "./AfterSalesVoice";
import {
  Team,
  Readiness,
  Runtime,
  Procurement,
  AfterSales,
  Conversion,
} from "./Growth";
import {
  Reports,
  Reconciliation,
  Coupons,
  Settlements,
  StockOperations,
  Maintenance,
  Tickets,
} from "./Operations";
const navigation = [
  {
    label: "代理分站",
    path: "/stations",
    icon: Store,
    permission: "stations.manage",
  },
  {
    label: "经营概览",
    path: "",
    icon: LayoutDashboard,
    permission: "analytics",
  },
  {
    label: "转化分析",
    path: "/conversion",
    icon: BarChart3,
    permission: "analytics",
  },
  {
    label: "利润与月报",
    path: "/reports",
    icon: BarChart3,
    permission: "finance",
  },
  {
    label: "商品与定价",
    path: "/products",
    icon: Package,
    permission: "catalog.read",
  },
  {
    label: "订单管理",
    path: "/orders",
    icon: ShoppingBag,
    permission: "orders.read",
  },
  {
    label: "售后处理",
    path: "/after-sales",
    icon: MessagesSquare,
    permission: "aftersales.read",
  },
  {
    label: "售后工单",
    path: "/tickets",
    icon: MessagesSquare,
    permission: "tickets",
  },
  {
    label: "卡密库存",
    path: "/inventory",
    icon: Ticket,
    permission: "inventory",
  },
  {
    label: "库存预警与批次",
    path: "/stock",
    icon: Boxes,
    permission: "inventory",
  },
  {
    label: "供应商与采购",
    path: "/procurement",
    icon: Boxes,
    permission: "purchases.read",
  },
  {
    label: "分销代理",
    path: "/agents",
    icon: Users,
    permission: "agents.read",
  },
  {
    label: "代理结算",
    path: "/settlements",
    icon: HandCoins,
    permission: "settlements",
  },
  {
    label: "优惠与营销",
    path: "/coupons",
    icon: Ticket,
    permission: "marketing",
  },
  {
    label: "支付渠道",
    path: "/payments",
    icon: CreditCard,
    permission: "payments.read",
  },
  {
    label: "支付对账",
    path: "/reconciliation",
    icon: ListChecks,
    permission: "reconcile",
  },
  {
    label: "站点设置",
    path: "/settings",
    icon: Mail,
    permission: "settings",
  },
  {
    label: "推送记录",
    path: "/notifications",
    icon: Bell,
    permission: "notifications",
  },
  {
    label: "上线检查",
    path: "/readiness",
    icon: ListChecks,
    permission: "diagnostics.read",
  },
  { label: "运行监控", path: "/runtime", icon: Bell, permission: "monitor" },
  {
    label: "日志与运维",
    path: "/maintenance",
    icon: Database,
    permission: "backups",
  },
  { label: "团队与权限", path: "/team", icon: Users, permission: "staff" },
  {
    label: "安全设置",
    path: "/security",
    icon: ShieldCheck,
    permission: "self",
  },
];

function Login({ onLogin }: { onLogin: () => void }) {
  const { site } = useSite();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const d = await adminApi<{ token: string }>("/login", {
        method: "POST",
        body: JSON.stringify({ username, password, code }),
      });
      sessionStorage.setItem(TOKEN_KEY, d.token);
      onLogin();
    } catch (e) {
      setError((e as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return (
    <div className="a-login">
      <Link className="a-login-back" to="/">
        <ArrowLeft size={16} />
        返回小铺
      </Link>
      <div className="a-login-art">
        <div className="a-login-orbit">
          <Moon size={38} />
          <BunnyMark />
        </div>
        <span className="a-eyebrow">WYBO MERCHANT CONSOLE</span>
        <h1>
          好好经营，
          <br />
          每一份小美好。
        </h1>
        <p>
          从第一份期待，到每一次成交。
          <br />
          让月兔陪你，照顾小铺的每一天。
        </p>
        <small>{site.site_name} · 商家工作台</small>
      </div>
      <section className="a-login-card">
        <span className="a-login-mark">
          <SiteLogo url={site.site_logo} /> {site.site_brand}
          <span>.</span>
        </span>
        <h2>欢迎回到小铺</h2>
        <p>登录商家后台，开始今天的好生意。</p>
        <form onSubmit={submit}>
          {error && <Banner>{error}</Banner>}
          <label>
            管理员账号
            <input
              required
              autoComplete="username"
              autoFocus
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="请输入管理员账号"
            />
          </label>
          <label>
            登录密码
            <input
              required
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="请输入登录密码"
            />
          </label>
          <label>
            两步验证码 <small>已绑定验证器时填写</small>
            <input
              inputMode="numeric"
              pattern="[0-9]{6}"
              maxLength={6}
              autoComplete="one-time-code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="6 位验证码（可选）"
            />
          </label>
          <button className="a-button primary" disabled={busy}>
            {busy ? "正在验证…" : "进入商家工作台"}
            <ChevronRight size={17} />
          </button>
        </form>
        <div className="a-login-note">
          <ShieldCheck size={15} />
          连接验证后，安全管理你的店铺
        </div>
      </section>
    </div>
  );
}
export default function AdminApp() {
  const base = adminBase();
  const { site } = useSite();
  const [entry, setEntry] = useState<boolean | null>(null);
  const [error, setError] = useState("");
  const [logged, setLogged] = useState(!!token());
  const [user, setUser] = useState<AdminUser | null>(null);
  const [toast, setToast] = useState("");
  const [open, setOpen] = useState(false);
  const [mobile, setMobile] = useState(
    () => window.matchMedia("(max-width:760px)").matches,
  );
  const sidebar = useRef<HTMLElement>(null);
  const workspace = useRef<HTMLDivElement>(null);
  useEffect(() => {
    const media = window.matchMedia("(max-width:760px)");
    const changed = () => setMobile(media.matches);
    media.addEventListener("change", changed);
    return () => media.removeEventListener("change", changed);
  }, []);
  useEffect(() => {
    if (sidebar.current) sidebar.current.inert = mobile && !open;
    if (workspace.current) workspace.current.inert = mobile && open;
    if (!mobile || !open) return;
    const previous =
      document.activeElement instanceof HTMLElement
        ? document.activeElement
        : null;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const controls = () =>
      Array.from(
        sidebar.current?.querySelectorAll<HTMLElement>(
          "a[href],button:not([disabled])",
        ) || [],
      );
    controls()[0]?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        setOpen(false);
        return;
      }
      if (e.key !== "Tab") return;
      const items = controls(),
        first = items[0],
        last = items[items.length - 1];
      if (e.shiftKey && document.activeElement === first) {
        e.preventDefault();
        last?.focus();
      } else if (!e.shiftKey && document.activeElement === last) {
        e.preventDefault();
        first?.focus();
      }
    };
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("keydown", key);
      document.body.style.overflow = previousOverflow;
      if (workspace.current) workspace.current.inert = false;
      previous?.focus();
    };
  }, [mobile, open]);
  const location = useLocation();
  function logout() {
    sessionStorage.removeItem(TOKEN_KEY);
    setLogged(false);
    setUser(null);
  }
  useEffect(() => {
    checkEntry(base)
      .then(setEntry)
      .catch((e) => setError(e.message));
  }, [base]);
  useEffect(() => {
    const expired = () => {
      logout();
      setToast("登录已过期，请重新登录");
    };
    window.addEventListener("wybo-admin-expired", expired);
    return () => window.removeEventListener("wybo-admin-expired", expired);
  }, []);
  useEffect(() => {
    if (logged && entry)
      adminApi<AdminUser>("/me")
        .then(setUser)
        .catch((e) => {
          if (token()) setError(e.message);
        });
  }, [logged, entry]);
  useEffect(() => {
    setOpen(false);
    window.scrollTo(0, 0);
  }, [location.pathname]);
  useEffect(() => {
    if (!toast) return;
    const t = setTimeout(() => setToast(""), 4000);
    return () => clearTimeout(t);
  }, [toast]);
  if (error && !entry)
    return (
      <div className="a-root">
        <div className="a-entry-error">
          <Banner>{error}</Banner>
          <button className="a-button" onClick={() => window.location.reload()}>
            重新连接
          </button>
        </div>
      </div>
    );
  if (entry === null)
    return (
      <div className="a-root">
        <Busy />
      </div>
    );
  if (!entry)
    return (
      <div className="a-root a-not-found">
        <Moon size={38} />
        <h1>这个角落，还没有开门。</h1>
        <p>页面不存在，或后台入口已修改。</p>
        <Link className="a-button primary" to="/">
          返回望月小铺
        </Link>
      </div>
    );
  if (!logged)
    return (
      <div className="a-root">
        <Login
          onLogin={() => {
            setError("");
            setLogged(true);
          }}
        />
      </div>
    );
  if (!user)
    return (
      <div className="a-root">
        {error ? <Banner>{error}</Banner> : <Busy />}
      </div>
    );
  const can = (p: string) =>
    p === "self" ||
    user.permissions.includes("*") ||
    user.permissions.includes(p);
  const visible = navigation.filter((n) => can(n.permission));
  const current = navigation.find((n) =>
    n.path
      ? location.pathname.replace(/\/$/, "") === base + n.path
      : location.pathname === base || location.pathname === base + "/",
  );
  if (current && !can(current.permission))
    return (
      <Navigate
        replace
        to={
          base +
          (visible.find((n) => n.path !== "/security")?.path || "/security")
        }
      />
    );
  return (
    <AccessContext.Provider value={user}>
      <div className="a-root a-shell">
        <StepUp />
        <a href="#admin-main" className="skip-link">
          跳到主要内容
        </a>
        {open && (
          <button
            className="a-sidebar-overlay"
            aria-label="关闭导航"
            onClick={() => setOpen(false)}
          />
        )}
        <aside
          ref={sidebar}
          aria-hidden={mobile && !open}
          className={`a-sidebar ${open ? "is-open" : ""}`}
        >
          {mobile && (
            <button
              className="a-icon-button a-drawer-close"
              aria-label="关闭菜单"
              onClick={() => setOpen(false)}
            >
              <X size={19} />
            </button>
          )}
          <Link to={base} className="a-brand">
            <SiteLogo url={site.site_logo} />
            <span>
              {site.site_brand}
              <b>.</b>
              <small>{site.site_name} · 商家后台</small>
            </span>
          </Link>
          <span className="a-nav-label">小铺工作台</span>
          <nav aria-label="后台导航">
            {visible.map((n, i) => (
              <NavLink
                key={n.path}
                to={base + n.path}
                end={!n.path}
                className={({ isActive }) =>
                  `${isActive ? "active" : ""} ${i === 5 ? "a-nav-divider" : ""}`
                }
              >
                <n.icon size={18} />
                <span>{n.label}</span>
                {!n.path && <span className="a-nav-dot" />}
              </NavLink>
            ))}
          </nav>
          <div className="a-sidebar-bottom">
            <div className="a-sidebar-tip">
              <Moon size={20} />
              <strong>一点一滴，皆是成长</strong>
              <p>
                认真经营的每一天，
                <br />
                都在靠近更好的小铺。
              </p>
            </div>
            <a href="/" target="_blank" rel="noreferrer">
              前往小铺 <ArrowUpRight size={16} />
            </a>
          </div>
        </aside>
        <div ref={workspace} className="a-workspace">
          <header className="a-topbar">
            <div>
              <button
                className="a-icon-button a-mobile-menu"
                aria-expanded={open}
                aria-label={open ? "关闭导航" : "打开导航"}
                onClick={() => setOpen(!open)}
              >
                {open ? <X size={21} /> : <Menu size={21} />}
              </button>
              <span>商家工作台</span>
              <ChevronRight size={13} />
              <strong>
                {navigation.find((n) =>
                  n.path
                    ? location.pathname.startsWith(base + n.path)
                    : location.pathname === base ||
                      location.pathname === base + "/",
                )?.label || "管理页面"}
              </strong>
            </div>
            <div>
              {can("tickets") && (
                <AfterSalesVoice
                  key={user.id}
                  userId={user.id}
                  base={base}
                  notify={setToast}
                />
              )}
              <span className="a-live">
                <i />
                小铺营业中
              </span>
              <span className="a-user-avatar">
                {user?.username.slice(0, 1).toUpperCase() || "W"}
              </span>
              <span className="a-user-name">{user?.username || "管理员"}</span>
              <button
                className="a-icon-button"
                aria-label="退出登录"
                title="退出登录"
                onClick={logout}
              >
                <LogOut size={17} />
              </button>
            </div>
          </header>
          <main id="admin-main" className="a-main" tabIndex={-1}>
            {error && <Banner>{error}</Banner>}
            <Suspense fallback={<Busy />}>
              <Routes>
                <Route
                  path={base + "/stations"}
                  element={<Stations notify={setToast} />}
                />
                <Route
                  path={base + "/readiness"}
                  element={<Readiness notify={setToast} />}
                />
                <Route path={base + "/conversion"} element={<Conversion />} />
                <Route
                  path={base + "/procurement"}
                  element={<Procurement notify={setToast} />}
                />
                <Route
                  path={base + "/after-sales"}
                  element={<AfterSales notify={setToast} />}
                />
                <Route
                  path={base + "/team"}
                  element={<Team notify={setToast} />}
                />
                <Route
                  path={base + "/runtime"}
                  element={<Runtime notify={setToast} />}
                />
                <Route path={base + "/reports"} element={<Reports />} />
                <Route
                  path={base + "/reconciliation"}
                  element={<Reconciliation notify={setToast} />}
                />
                <Route
                  path={base + "/coupons"}
                  element={<Coupons notify={setToast} />}
                />
                <Route
                  path={base + "/settlements"}
                  element={<Settlements notify={setToast} />}
                />
                <Route
                  path={base + "/stock"}
                  element={<StockOperations notify={setToast} />}
                />
                <Route
                  path={base + "/maintenance"}
                  element={<Maintenance notify={setToast} />}
                />
                <Route
                  path={base + "/tickets"}
                  element={<Tickets notify={setToast} />}
                />
                <Route path={base} element={<Dashboard base={base} />} />
                <Route
                  path={base + "/products"}
                  element={<Products base={base} notify={setToast} />}
                />
                <Route
                  path={base + "/orders"}
                  element={<Orders notify={setToast} />}
                />
                <Route
                  path={base + "/inventory"}
                  element={<Inventory notify={setToast} />}
                />
                <Route
                  path={base + "/agents"}
                  element={<Agents notify={setToast} />}
                />
                <Route
                  path={base + "/payments"}
                  element={<Payments notify={setToast} />}
                />
                <Route
                  path={base + "/settings"}
                  element={<Settings notify={setToast} />}
                />
                <Route
                  path={base + "/notifications"}
                  element={<Notifications notify={setToast} />}
                />
                <Route
                  path={base + "/security"}
                  element={
                    user ? (
                      <Security user={user} logout={logout} notify={setToast} />
                    ) : (
                      <Busy />
                    )
                  }
                />
                <Route
                  path="*"
                  element={
                    <div className="a-not-found">
                      <h1>页面未找到</h1>
                      <Link to={base}>返回经营概览</Link>
                    </div>
                  }
                />
              </Routes>
            </Suspense>
            <footer className="a-footer">
              <span>
                {site.site_brand}
                <span className="a-muted"> · {site.site_name}</span>
              </span>
              <span>
                认真记录每一份美好 <Moon size={12} />
              </span>
            </footer>
          </main>
        </div>
        {toast && (
          <div className="a-toast" role="status">
            <CheckCircle2 size={18} />
            {toast}
          </div>
        )}
      </div>
    </AccessContext.Provider>
  );
}
