import { useEffect, useMemo, useState } from "react";
import { Compass, Search, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Card, CardContent } from "@/components/ui/card";
import { Empty, EmptyDescription, EmptyHeader, EmptyMedia, EmptyTitle } from "@/components/ui/empty";
import { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput } from "@/components/ui/input-group";
import { Skeleton } from "@/components/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip";
import { fetchWorkNavigation, type WorkNavigationData, type WorkNavigationGroup, type WorkNavigationLink } from "@/lib/work";

const ALL_GROUPS = "all";

export function WorkPage() {
  const [navigationData, setNavigationData] = useState<WorkNavigationData | null>(null);
  const [activeGroup, setActiveGroup] = useState(ALL_GROUPS);
  const [searchTerm, setSearchTerm] = useState("");
  const [message, setMessage] = useState("");
  const [isLoading, setIsLoading] = useState(true);

  useEffect(() => {
    let isCancelled = false;

    async function loadNavigation() {
      setIsLoading(true);
      setMessage("");
      try {
        const data = await fetchWorkNavigation();
        if (!isCancelled) {
          setNavigationData(data);
        }
      } catch (error) {
        if (!isCancelled) {
          setMessage(error instanceof Error ? error.message : "工作导航数据加载失败");
        }
      } finally {
        if (!isCancelled) {
          setIsLoading(false);
        }
      }
    }

    void loadNavigation();

    return () => {
      isCancelled = true;
    };
  }, []);

  const normalizedSearchTerm = searchTerm.trim().toLowerCase();
  const groups = navigationData?.groups ?? [];
  const visibleGroups = useMemo(
    () => filterNavigationGroups(groups, activeGroup, normalizedSearchTerm),
    [groups, activeGroup, normalizedSearchTerm],
  );
  const visibleTotal = visibleGroups.reduce((total, group) => total + group.links.length, 0);

  if (isLoading) {
    return <WorkNavigationSkeleton />;
  }

  if (message) {
    return (
      <Alert variant="destructive">
        <AlertTitle>工作导航加载失败</AlertTitle>
        <AlertDescription>{message}</AlertDescription>
      </Alert>
    );
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <InputGroup className="max-w-xl flex-1">
          <InputGroupAddon><Search /></InputGroupAddon>
          <InputGroupInput
            value={searchTerm}
            onChange={(event) => setSearchTerm(event.target.value)}
            placeholder="搜索网站、分组或域名"
          />
          {searchTerm ? (
            <InputGroupButton type="button" onClick={() => setSearchTerm("")} aria-label="清空搜索">
              <X />
            </InputGroupButton>
          ) : null}
        </InputGroup>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline">{groups.length} 个分组</Badge>
          <Badge variant="secondary">{visibleTotal} / {navigationData?.total ?? 0} 个入口</Badge>
        </div>
      </div>

      <ToggleGroup
        type="single"
        value={activeGroup}
        onValueChange={(value) => value && setActiveGroup(value)}
        variant="outline"
        size="sm"
        className="justify-start overflow-x-auto pb-1"
      >
        <ToggleGroupItem value={ALL_GROUPS} className="shrink-0 data-[state=on]:bg-primary data-[state=on]:text-primary-foreground">
          全部
        </ToggleGroupItem>
        {groups.map((group) => (
          <ToggleGroupItem
            key={group.title}
            value={group.title}
            className="shrink-0 data-[state=on]:bg-primary data-[state=on]:text-primary-foreground"
          >
            {group.title}
            <span className="text-xs opacity-70">{group.links.length}</span>
          </ToggleGroupItem>
        ))}
      </ToggleGroup>

      {!navigationData?.is_configured ? (
        <StatusCard message="还没有配置工作导航数据，请在 storage/private/work/navigation.json 中放入链接。" />
      ) : visibleGroups.length ? (
        <div className="space-y-4">
          {visibleGroups.map((group) => (
            <NavigationSection key={group.title} group={group} />
          ))}
        </div>
      ) : (
        <StatusCard message="没有匹配的工作导航入口。" />
      )}
    </div>
  );
}

function NavigationSection({ group }: { group: WorkNavigationGroup }) {
  return (
    <section className="space-y-2">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <div className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md border bg-background">
            <Compass className="h-3.5 w-3.5 text-muted-foreground" />
          </div>
          <h2 className="truncate text-sm font-semibold">{group.title}</h2>
        </div>
        <Badge variant="outline" className="shrink-0">
          {group.links.length}
        </Badge>
      </div>
      <div className="grid grid-cols-[repeat(auto-fill,4.5rem)] justify-start gap-1.5">
        {group.links.map((link) => (
          <NavigationCard key={`${group.title}-${link.title}-${link.url}`} link={link} />
        ))}
      </div>
    </section>
  );
}

function NavigationCard({ link }: { link: WorkNavigationLink }) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <a href={link.url} target="_blank" rel="noreferrer" className="group block">
          <Card className="h-full transition-colors hover:border-foreground/30 hover:bg-muted/30">
            <CardContent className="flex h-[3.25rem] flex-col items-center justify-center gap-0.5 p-1">
              <div
                className="flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-xs font-semibold leading-none text-white"
                style={{ backgroundColor: stringToColor(link.title) }}
              >
                {getInitial(link.title)}
              </div>
              <div className="w-full truncate text-center text-xs font-semibold leading-4">{link.title}</div>
            </CardContent>
          </Card>
        </a>
      </TooltipTrigger>
      <TooltipContent>{link.title}</TooltipContent>
    </Tooltip>
  );
}

function StatusCard({ message }: { message: string }) {
  return (
    <Empty>
      <EmptyHeader>
        <EmptyMedia><Compass /></EmptyMedia>
        <EmptyTitle>暂无工作导航</EmptyTitle>
        <EmptyDescription>{message}</EmptyDescription>
      </EmptyHeader>
    </Empty>
  );
}

/** WorkNavigationSkeleton 展示工作导航加载占位，无副作用。 */
function WorkNavigationSkeleton() {
  // 1. 固定搜索区和分组区尺寸，避免数据返回时页面跳动。
  return (
    <div className="space-y-4">
      <Skeleton className="h-9 max-w-xl" />
      <div className="flex gap-2">
        <Skeleton className="h-8 w-16" />
        <Skeleton className="h-8 w-24" />
        <Skeleton className="h-8 w-24" />
      </div>
      <Skeleton className="h-32 w-full" />
    </div>
  );
}

function filterNavigationGroups(groups: WorkNavigationGroup[], activeGroup: string, searchTerm: string) {
  return groups
    .filter((group) => activeGroup === ALL_GROUPS || group.title === activeGroup)
    .map((group) => ({
      ...group,
      links: group.links.filter((link) => matchesSearch(group, link, searchTerm)),
    }))
    .filter((group) => group.links.length > 0);
}

function matchesSearch(group: WorkNavigationGroup, link: WorkNavigationLink, searchTerm: string) {
  if (!searchTerm) {
    return true;
  }

  return [group.title, link.title, link.host, link.url].some((value) => value.toLowerCase().includes(searchTerm));
}

function getInitial(title: string) {
  return Array.from(title.trim())[0]?.toUpperCase() || "?";
}

function stringToColor(value: string) {
  let hash = 0;
  for (const char of value) {
    hash = char.charCodeAt(0) + ((hash << 5) - hash);
    hash |= 0;
  }
  const hue = Math.abs(hash) % 360;
  return `hsl(${hue}, 45%, 38%)`;
}
