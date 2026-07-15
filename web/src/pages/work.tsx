import { useEffect, useMemo, useState } from "react";
import { Compass, Search, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
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
    return <StatusCard message="正在加载工作导航..." />;
  }

  if (message) {
    return <StatusCard message={message} />;
  }

  return (
    <div className="space-y-4">
      <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
        <div className="relative max-w-xl flex-1">
          <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            value={searchTerm}
            onChange={(event) => setSearchTerm(event.target.value)}
            placeholder="搜索网站、分组或域名"
            className="pl-9 pr-9"
          />
          {searchTerm ? (
            <button
              type="button"
              className="absolute right-2 top-1/2 inline-flex h-6 w-6 -translate-y-1/2 items-center justify-center rounded-sm text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
              onClick={() => setSearchTerm("")}
              title="清空搜索"
              aria-label="清空搜索"
            >
              <X className="h-3.5 w-3.5" />
            </button>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <Badge variant="outline">{groups.length} 个分组</Badge>
          <Badge variant="secondary">{visibleTotal} / {navigationData?.total ?? 0} 个入口</Badge>
        </div>
      </div>

      <div className="flex gap-2 overflow-x-auto pb-1">
        <Button
          type="button"
          size="sm"
          variant={activeGroup === ALL_GROUPS ? "default" : "outline"}
          onClick={() => setActiveGroup(ALL_GROUPS)}
          className="shrink-0"
        >
          全部
        </Button>
        {groups.map((group) => (
          <Button
            key={group.title}
            type="button"
            size="sm"
            variant={activeGroup === group.title ? "default" : "outline"}
            onClick={() => setActiveGroup(group.title)}
            className="shrink-0"
          >
            {group.title}
            <span className="text-xs opacity-70">{group.links.length}</span>
          </Button>
        ))}
      </div>

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
    <a href={link.url} target="_blank" rel="noreferrer" className="group block" title={link.title}>
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
  );
}

function StatusCard({ message }: { message: string }) {
  return (
    <Card>
      <CardContent className="p-6 text-sm text-muted-foreground">{message}</CardContent>
    </Card>
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
