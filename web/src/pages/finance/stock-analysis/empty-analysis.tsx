import { Button } from "@/components/ui/button";
import { Empty, EmptyContent, EmptyDescription, EmptyHeader, EmptyTitle } from "@/components/ui/empty";

export function EmptyAnalysis() {
  return (
    <Empty>
      <EmptyHeader>
        <EmptyTitle>暂无仓位数据</EmptyTitle>
        <EmptyDescription>先在股票仓位导入页面上传同花顺持仓截图，这里会自动生成资产、总仓位、变化和持仓分布。</EmptyDescription>
      </EmptyHeader>
      <EmptyContent>
        <Button asChild className="mt-4">
          <a href="/positions">去导入仓位截图</a>
        </Button>
      </EmptyContent>
    </Empty>
  );
}
