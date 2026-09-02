import React from "react";
import ReactDOM from "react-dom/client";
import { useState } from "react";

import "@/index.css";
import { Toaster } from "@/components/ui/sonner";
import { TooltipProvider } from "@/components/ui/tooltip";
import { isAuthenticated } from "@/lib/auth";
import { getFinancePageFromPath } from "@/lib/finance";
import { DashboardPage } from "@/pages/dashboard";
import { LoginPage } from "@/pages/login";
import { PublicHomePage } from "@/pages/public-home";

function App() {
  const [, refreshRoute] = useState(0);
  const path = normalizePath(window.location.pathname);

  function enterWorkbench() {
    window.history.replaceState({}, "", "/work");
    refreshRoute((value) => value + 1);
  }

  if (path === "/login") {
    return <LoginPage onLoggedIn={enterWorkbench} />;
  }

  if (path === "/") {
    return <PublicHomePage />;
  }

  if (!isAuthenticated()) {
    return <LoginPage onLoggedIn={enterWorkbench} />;
  }

  return <DashboardPage initialPage={getFinancePageFromPath(path)} />;
}

function normalizePath(pathname: string) {
  const normalized = pathname.replace(/\/+$/, "");
  return normalized || "/";
}

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <TooltipProvider>
      <App />
      <Toaster />
    </TooltipProvider>
  </React.StrictMode>,
);
