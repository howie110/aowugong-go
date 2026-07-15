import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";

export function EmptyAnalysis() {
  return (
    <Card>
      <CardContent className="p-6">
        <div className="text-sm font-medium">暂无仓位数据</div>
        <p className="mt-1 text-sm text-muted-foreground">先在股票仓位导入页面上传同花顺持仓截图，这里会自动生成资产、总仓位、变化和持仓分布。</p>
        <Button asChild className="mt-4">
          <a href="/positions">去导入仓位截图</a>
        </Button>
      </CardContent>
    </Card>
  );
}
