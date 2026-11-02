import React, { lazy, Suspense } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, useLocation } from "react-router-dom";
import App from "./App";
import "./styles.css";
import { SiteProvider, useSite } from "./Site";

const StationApp = lazy(() => import("./StationApp"));
const StationActivate = lazy(() =>
  import("./StationApp").then((m) => ({ default: m.StationActivate })),
);
const StationApply = lazy(() => import("./StationApply"));
const AgentPortal = lazy(() => import("./AgentPortal"));
const AdminApp = lazy(() => import("./admin/AdminApp"));
function Entry() {
  const { pathname } = useLocation();
  const { site, loading, error, refresh } = useSite();
  if (loading)
    return (
      <div className="a-loading" role="status">
        正在连接小铺…
      </div>
    );
  if (error)
    return (
      <main className="container inner-page">
        <h1>暂时无法打开小铺</h1>
        <p role="alert">{error}</p>
        <button
          className="button button-primary"
          onClick={() => void refresh()}
        >
          重新连接
        </button>
      </main>
    );
  if (pathname === "/apply")
    return (
      <Suspense fallback={<p role="status">正在打开申请页…</p>}>
        <StationApply />
      </Suspense>
    );
  if (site.is_station === "true" && pathname === "/activate")
    return (
      <Suspense fallback={<p role="status">正在打开激活页…</p>}>
        <StationActivate />
      </Suspense>
    );
  if (
    site.is_station === "true" &&
    pathname !== "/" &&
    pathname !== "/orders" &&
    pathname !== "/guide"
  )
    return (
      <Suspense fallback={<p role="status">正在打开分站后台…</p>}>
        <StationApp />
      </Suspense>
    );
  return ["/", "/orders", "/guide"].includes(pathname) ? (
    <App />
  ) : (
    <Suspense
      fallback={
        <div style={{ padding: 60, textAlign: "center", color: "#8293b8" }}>
          正在打开工作台…
        </div>
      }
    >
      {pathname === "/agent" ? <AgentPortal /> : <AdminApp />}
    </Suspense>
  );
}
ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <SiteProvider>
        <Entry />
      </SiteProvider>
    </BrowserRouter>
  </React.StrictMode>,
);
