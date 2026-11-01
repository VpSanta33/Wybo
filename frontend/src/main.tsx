import React, { lazy, Suspense } from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, useLocation } from "react-router-dom";
import App from "./App";
import "./styles.css";
import { SiteProvider } from "./Site";

const AgentPortal = lazy(() => import("./AgentPortal"));
const AdminApp = lazy(() => import("./admin/AdminApp"));
function Entry() {
  const { pathname } = useLocation();
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
