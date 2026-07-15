import { ChevronRight } from "lucide-react";
import type { MouseEvent } from "react";

import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  useSidebar,
} from "@/components/ui/sidebar";
import { type FinancePageKey, getFinancePagePath } from "@/lib/finance";
import type { NavGroup } from "./app-navigation";

export function SidebarNavigation({
  groups,
  activePage,
  openGroups,
  onToggleGroup,
  onNavigate,
}: {
  groups: NavGroup[];
  activePage: FinancePageKey;
  openGroups: Record<string, boolean>;
  onToggleGroup: (groupId: string) => void;
  onNavigate: (pageKey: FinancePageKey) => void;
}) {
  const { isMobile, setOpenMobile } = useSidebar();

  function handleNavigate(event: MouseEvent<HTMLAnchorElement>, pageKey: FinancePageKey) {
    event.preventDefault();
    onNavigate(pageKey);
    if (isMobile) {
      setOpenMobile(false);
    }
  }

  return (
    <>
      {groups.map((group) => {
        const isOpen = openGroups[group.id] ?? false;
        const GroupIcon = group.icon;

        return (
          <SidebarGroup key={group.id} className="py-1">
            <SidebarGroupLabel asChild>
              <button
                type="button"
                className="h-8 w-full justify-between px-2 text-sm font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground"
                onClick={() => onToggleGroup(group.id)}
                aria-expanded={isOpen}
              >
                <span className="flex min-w-0 items-center gap-2">
                  <GroupIcon className="h-4 w-4 shrink-0" />
                  <span className="truncate">{group.label}</span>
                </span>
                <span className="flex shrink-0 items-center text-muted-foreground">
                  <ChevronRight className={["h-4 w-4 transition-transform duration-200", isOpen ? "rotate-90" : ""].join(" ")} />
                </span>
              </button>
            </SidebarGroupLabel>
            <div
              className={[
                "grid transition-all duration-200 ease-out",
                isOpen ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0",
              ].join(" ")}
            >
              <SidebarGroupContent className="min-h-0 overflow-hidden">
                <SidebarMenuSub className="mt-1">
                  {group.items.map((item) => {
                    const ItemIcon = item.icon;
                    return (
                      <SidebarMenuSubItem key={item.key}>
                        <SidebarMenuSubButton asChild isActive={activePage === item.key} className="text-xs">
                          <a href={getFinancePagePath(item.key)} onClick={(event) => handleNavigate(event, item.key)}>
                            <ItemIcon className="h-4 w-4" />
                            <span>{item.label}</span>
                          </a>
                        </SidebarMenuSubButton>
                      </SidebarMenuSubItem>
                    );
                  })}
                </SidebarMenuSub>
              </SidebarGroupContent>
            </div>
          </SidebarGroup>
        );
      })}
    </>
  );
}
