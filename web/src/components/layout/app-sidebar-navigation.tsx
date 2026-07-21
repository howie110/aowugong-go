import { ChevronRight } from "lucide-react";
import type { MouseEvent } from "react";

import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible";
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
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
  onGroupOpenChange,
  onNavigate,
}: {
  groups: NavGroup[];
  activePage: FinancePageKey;
  openGroups: Record<string, boolean>;
  onGroupOpenChange: (groupId: string, open: boolean) => void;
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
          <Collapsible
            key={group.id}
            asChild
            open={isOpen}
            onOpenChange={(open) => onGroupOpenChange(group.id, open)}
            className="group/collapsible"
          >
            <SidebarGroup className="py-1">
              <SidebarMenu>
                <SidebarMenuItem>
                  <CollapsibleTrigger asChild>
                    <SidebarMenuButton tooltip={group.label} className="font-medium">
                      <GroupIcon />
                      <span>{group.label}</span>
                      <ChevronRight className="ml-auto transition-transform duration-200 group-data-[state=open]/collapsible:rotate-90" />
                    </SidebarMenuButton>
                  </CollapsibleTrigger>
                </SidebarMenuItem>
              </SidebarMenu>
              <CollapsibleContent>
                <SidebarGroupContent>
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
              </CollapsibleContent>
            </SidebarGroup>
          </Collapsible>
        );
      })}
    </>
  );
}
