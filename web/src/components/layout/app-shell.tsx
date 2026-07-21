import { Activity, LogOut, ShieldCheck } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";

import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { UserProfile, clearToken } from "@/lib/auth";
import { FinancePageKey } from "@/lib/finance";
import { AppBreadcrumb } from "./app-breadcrumb";
import { NAV_GROUP_STORAGE_KEY, navGroups, pageGroupMap } from "./app-navigation";
import { SidebarNavigation } from "./app-sidebar-navigation";

type AppShellProps = {
  user: UserProfile | null;
  activePage: FinancePageKey;
  onNavigate: (pageKey: FinancePageKey) => void;
  children: ReactNode;
};

export function AppShell({ user, activePage, onNavigate, children }: AppShellProps) {
  const roleCodes = user?.roles ?? [];
  const permissionCodes = user?.permissions ?? [];
  const canViewAllPages = roleCodes.includes("admin");
  const visibleNavGroups = navGroups
    .map((group) => ({
      ...group,
      items: group.items.filter((item) => canViewAllPages || permissionCodes.includes(item.permission)),
    }))
    .filter((group) => group.items.length > 0);
  const [openGroups, setOpenGroups] = useState<Record<string, boolean>>(() => readOpenGroups(activePage));

  useEffect(() => {
    const activeGroupId = pageGroupMap[activePage];
    if (!activeGroupId) {
      return;
    }
    setOpenGroups((current) => (current[activeGroupId] ? current : { ...current, [activeGroupId]: true }));
  }, [activePage]);

  useEffect(() => {
    if (typeof window === "undefined") {
      return;
    }
    window.localStorage.setItem(NAV_GROUP_STORAGE_KEY, JSON.stringify(openGroups));
  }, [openGroups]);

  function setGroupOpen(groupId: string, open: boolean) {
    setOpenGroups((current) => ({ ...current, [groupId]: open }));
  }

  function handleLogout() {
    clearToken();
    window.location.href = "/login";
  }

  return (
    <SidebarProvider>
      <Sidebar collapsible="icon">
        <SidebarHeader className="h-16 justify-center border-b">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip="Aowugong 工作台"
                className="h-11 group-data-[collapsible=icon]:!p-0"
                onClick={() => onNavigate("overview")}
              >
                <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-md bg-sidebar-primary text-sidebar-primary-foreground">
                  <Activity className="h-5 w-5" />
                </span>
                <span className="min-w-0">
                  <span className="block truncate text-sm font-semibold">Aowugong</span>
                  <span className="block truncate text-xs font-normal text-muted-foreground">个人工作台</span>
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarHeader>
        <SidebarContent className="px-1 py-2">
          <SidebarNavigation
            groups={visibleNavGroups}
            activePage={activePage}
            openGroups={openGroups}
            onGroupOpenChange={setGroupOpen}
            onNavigate={onNavigate}
          />
        </SidebarContent>
        <SidebarFooter className="border-t">
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                tooltip={user?.username || "未登录"}
                className="h-11 group-data-[collapsible=icon]:!p-0"
              >
                <Avatar className="h-8 w-8">
                  <AvatarFallback><ShieldCheck className="h-4 w-4" /></AvatarFallback>
                </Avatar>
                <span className="min-w-0">
                  <span className="block truncate text-sm font-medium">{user?.username || "未登录"}</span>
                  <span className="block truncate text-xs font-normal text-muted-foreground">{user?.email || "local session"}</span>
                </span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarFooter>
        <SidebarRail />
      </Sidebar>
      <SidebarInset>
        <header className="sticky top-0 z-10 flex h-16 items-center justify-between border-b bg-background/95 px-4 backdrop-blur md:px-6">
          <div className="flex min-w-0 items-center gap-3">
            <SidebarTrigger />
            <Separator orientation="vertical" className="h-4" />
            <div className="min-w-0">
              <div className="text-sm font-semibold">Aowugong 工作台</div>
              <AppBreadcrumb activePage={activePage} onNavigate={onNavigate} />
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={handleLogout}>
              <LogOut className="h-4 w-4" />
              退出
            </Button>
          </div>
        </header>
        {children}
      </SidebarInset>
    </SidebarProvider>
  );
}

function readOpenGroups(activePage: FinancePageKey) {
  const fallback = defaultOpenGroups(activePage);
  if (typeof window === "undefined") {
    return fallback;
  }

  try {
    const rawValue = window.localStorage.getItem(NAV_GROUP_STORAGE_KEY);
    if (!rawValue) {
      return fallback;
    }
    const parsedValue = JSON.parse(rawValue) as Record<string, boolean>;
    return { ...fallback, ...parsedValue, [pageGroupMap[activePage]]: true };
  } catch {
    return fallback;
  }
}

function defaultOpenGroups(activePage: FinancePageKey) {
  const activeGroupId = pageGroupMap[activePage];
  return Object.fromEntries(navGroups.map((group) => [group.id, group.id === "general" || group.id === activeGroupId])) as Record<
    string,
    boolean
  >;
}
