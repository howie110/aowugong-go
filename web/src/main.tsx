import React from "react";
import ReactDOM from "react-dom/client";

import "@/index.css";
import { Toaster } from "@/components/ui/sonner";
import { isAuthenticated } from "@/lib/auth";
import { getFinancePageFromPath } from "@/lib/finance";
import { DashboardPage } from "@/pages/dashboard";
import { LoginPage } from "@/pages/login";

function App() {
  const path = window.location.pathname;
  if (path === "/login") {
    return <LoginPage />;
  }
  if (!isAuthenticated()) {
    window.location.href = "/login";
    return null;
  }
  return <DashboardPage initialPage={getFinancePageFromPath(path)} />;
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
    <Toaster />
  </React.StrictMode>,
);
