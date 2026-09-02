import { ArrowUpRight, FileText } from "lucide-react";
import { useEffect } from "react";

import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

const filingRecord = "备案号：待审核";

// 公开首页只维护白名单，避免把工作导航中的私有入口带到公网。
const publicLinkGroups = [
  {
    title: "实用工具",
    description: "日常查询、翻译与生活工具。",
    links: [
      { title: "百度翻译", description: "在线翻译中文与外文资料。", url: "https://fanyi.baidu.com/" },
      { title: "测速网", description: "查看网络速度与线路质量。", url: "https://www.speedtest.cn/" },
      { title: "电信测速", description: "使用中国电信公开测速服务。", url: "https://10000.gd.cn/html5-speedtest/index.html" },
      { title: "影厅指南", description: "查询电影放映信息与观影安排。", url: "https://cinema.gaoliang.me/" },
      { title: "魔方简历", description: "制作简洁的在线简历。", url: "https://magicv.art/zh" },
    ],
  },
  {
    title: "编程工具",
    description: "图片、二维码和网页调试相关工具。",
    links: [
      { title: "二维码美化", description: "生成和美化二维码。", url: "https://passer-by.com/widget-qrcode/" },
      { title: "网站测速", description: "测试网页访问速度与响应情况。", url: "https://zhale.me/http/" },
      { title: "配色工具", description: "查找网页设计和开发配色。", url: "https://www.ysdaima.com/" },
      { title: "文转印", description: "把文字转换为便于分享的图片。", url: "https://easyvoice.ioplus.tech/" },
      { title: "拼接图片", description: "在线拼接多张图片。", url: "https://img.ops-coffee.cn/" },
    ],
  },
  {
    title: "论坛资讯",
    description: "阅读中文社区、资讯和公开内容。",
    links: [
      { title: "V2EX", description: "技术与创意主题社区。", url: "https://v2ex.com/" },
      { title: "知乎", description: "发现问题、回答与知识讨论。", url: "https://www.zhihu.com/" },
      { title: "究极摸鱼", description: "工作间隙浏览的轻量内容集合。", url: "https://momoyu.cc/" },
      { title: "微信读书", description: "阅读书籍与公开文章。", url: "https://weread.qq.com/" },
    ],
  },
  {
    title: "视频",
    description: "公开的视频内容和直播频道。",
    links: [
      { title: "哔哩哔哩", description: "观看公开的视频与直播内容。", url: "https://www.bilibili.com/" },
      { title: "央视体育", description: "查看央视体育公开直播内容。", url: "https://tv.cctv.com/live/cctv5/" },
    ],
  },
  {
    title: "金融资讯",
    description: "公开的市场数据与投资资料。",
    links: [
      { title: "Tushare", description: "获取公开金融数据与研究资料。", url: "https://tushare.pro/" },
      { title: "集思录", description: "查看公开的投资数据与讨论。", url: "https://www.jisilu.cn/" },
    ],
  },
  {
    title: "地图与地理",
    description: "公开地图和地理信息工具。",
    links: [
      { title: "中国地图", description: "查看自然资源部公开地图服务。", url: "http://bzdt.ch.mnr.gov.cn/index.html" },
      { title: "天地图", description: "使用国家地理信息公共服务。", url: "https://www.tianditu.gov.cn/" },
    ],
  },
];

function getHost(url: string) {
  try {
    return new URL(url).hostname.replace(/^www\./, "");
  } catch {
    return url;
  }
}

/** 渲染根域名公开主页，供工具分享站点备案展示使用。 */
export function PublicHomePage() {
  useEffect(() => {
    document.title = "嗷呜公 · 工具分享";
  }, []);

  return (
    <main className="min-h-screen bg-background">
      <header className="border-b">
        <div className="mx-auto flex h-16 max-w-5xl items-center px-5 sm:px-6">
          <a href="/" className="flex items-center gap-3" aria-label="嗷呜公首页">
            <span className="flex h-9 w-9 items-center justify-center rounded-md bg-foreground text-sm font-semibold text-background">
              嗷
            </span>
            <span>
              <span className="block font-semibold leading-5">嗷呜公</span>
              <span className="block text-xs leading-4 text-muted-foreground">工具分享</span>
            </span>
          </a>
        </div>
      </header>

      <section className="mx-auto max-w-5xl px-5 pb-14 pt-14 sm:px-6 sm:pb-16 sm:pt-16">
        <div className="text-center">
          <Badge variant="secondary">嗷呜公 · 工具分享</Badge>
          <h1 className="mt-4 text-3xl font-semibold tracking-normal sm:text-4xl">网站导航</h1>
          <p className="mx-auto mt-4 max-w-xl text-sm leading-7 text-muted-foreground sm:text-base">
            整理日常会用到的公开网站，按用途分类，点击即可访问。
          </p>
        </div>

        <div className="mt-12 grid gap-10 lg:grid-cols-[minmax(0,1fr)_18rem] lg:items-start">
          <section aria-labelledby="public-navigation-title">
            <div className="flex items-center justify-between gap-4 border-b pb-3">
              <div className="flex items-center gap-2">
                <FileText className="h-4 w-4 text-muted-foreground" />
                <h2 id="public-navigation-title" className="text-base font-semibold">
                  网站导航
                </h2>
              </div>
              <span className="text-xs text-muted-foreground">公开网站</span>
            </div>

            <div className="divide-y">
              {publicLinkGroups.map((group) => (
                <section key={group.title} className="py-6 first:pt-5 last:pb-0">
                  <div className="flex items-start justify-between gap-4">
                    <div>
                      <h3 className="text-sm font-semibold">{group.title}</h3>
                      <p className="mt-1 text-xs leading-5 text-muted-foreground">{group.description}</p>
                    </div>
                    <Badge variant="outline" className="shrink-0">
                      {group.links.length}
                    </Badge>
                  </div>

                  <div className="mt-4 divide-y border-y">
                    {group.links.map((link) => (
                      <a
                        key={`${group.title}-${link.title}`}
                        href={link.url}
                        target="_blank"
                        rel="noreferrer"
                        className="group flex items-start gap-4 py-4 outline-none transition-colors focus-visible:ring-2 focus-visible:ring-ring"
                      >
                        <div className="min-w-0 flex-1">
                          <div className="flex items-center gap-2">
                            <h4 className="text-sm font-medium group-hover:underline">{link.title}</h4>
                            <span className="hidden text-xs text-muted-foreground sm:inline">{getHost(link.url)}</span>
                          </div>
                          <p className="mt-1 text-sm leading-6 text-muted-foreground">{link.description}</p>
                        </div>
                        <ArrowUpRight className="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" />
                      </a>
                    ))}
                  </div>
                </section>
              ))}
            </div>

            <div className="mt-5 border-t pt-5">
              <p className="text-sm text-muted-foreground">公开网站持续整理中。</p>
            </div>
          </section>

          <Card>
            <CardHeader className="gap-2 pb-4">
              <CardTitle>本站说明</CardTitle>
              <CardDescription>本站整理常用的公开工具与网站。</CardDescription>
            </CardHeader>
            <CardContent>
              <dl className="space-y-3 text-sm">
                <div className="flex items-start justify-between gap-4 border-b pb-3">
                  <dt className="text-muted-foreground">网站性质</dt>
                  <dd className="text-right font-medium">工具分享</dd>
                </div>
                <div className="flex items-start justify-between gap-4 border-b pb-3">
                  <dt className="text-muted-foreground">站点名称</dt>
                  <dd className="text-right font-medium">嗷呜公</dd>
                </div>
                <div className="flex items-start justify-between gap-4">
                  <dt className="text-muted-foreground">公开内容</dt>
                  <dd className="max-w-28 text-right font-medium leading-6">工具与网站导航</dd>
                </div>
              </dl>
            </CardContent>
          </Card>
        </div>
      </section>

      <footer className="border-t">
        <div className="mx-auto flex max-w-5xl flex-col gap-1 px-5 py-5 text-xs text-muted-foreground sm:flex-row sm:items-center sm:justify-between sm:px-6">
          <span>© {new Date().getFullYear()} 嗷呜公</span>
          <span>{filingRecord}</span>
        </div>
      </footer>
    </main>
  );
}
