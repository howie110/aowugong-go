import { authorizedFetch } from "@/lib/auth";
import { requestJSON, responseError } from "@/lib/request";

export type DatabaseTableSummary = {
  name: string;
  row_count: number;
  column_count: number;
};

export type DatabaseSummary = {
  engine: string;
  journal_mode: string;
  size_bytes: number;
  table_count: number;
  total_rows: number;
  tables: DatabaseTableSummary[];
};

export type DatabaseColumn = {
  name: string;
  type: string;
  not_null: boolean;
  primary_key: boolean;
  sensitive: boolean;
};

export type DatabaseRowsPage = {
  table: string;
  columns: DatabaseColumn[];
  rows: Array<Record<string, unknown>>;
  total: number;
  page: number;
  page_size: number;
};

// fetchDatabaseSummary 读取 PostgreSQL 和应用表概况。
// 输入：无。
// 输出：返回数据库概况。
// 副作用：发送只读 HTTP 请求。
export function fetchDatabaseSummary() {
  // 1. 调用数据库概况接口。
  return requestJSON<DatabaseSummary>("/api/v1/database/summary", {}, "读取数据库失败");
}

// fetchDatabaseRows 读取指定表的筛选分页。
// 输入：table 是表名，page、pageSize 和 search 控制结果范围。
// 输出：返回字段和当前页数据。
// 副作用：发送只读 HTTP 请求。
export function fetchDatabaseRows(table: string, page: number, pageSize: number, search: string) {
  // 1. 对路径和查询值编码后调用分页接口。
  const query = new URLSearchParams({
    page: String(page),
    page_size: String(pageSize),
  });
  if (search.trim()) {
    query.set("search", search.trim());
  }
  return requestJSON<DatabaseRowsPage>(
    `/api/v1/database/tables/${encodeURIComponent(table)}?${query.toString()}`, {}, "读取数据库失败",
  );
}

// downloadDatabaseTable 下载指定表的脱敏 CSV。
// 输入：table 是表名，search 是当前筛选文本。
// 输出：成功下载后返回 void；失败抛出错误。
// 副作用：发送只读 HTTP 请求并触发浏览器文件下载。
export async function downloadDatabaseTable(table: string, search: string) {
  // 1. 请求与当前筛选一致的 CSV 内容。
  const query = new URLSearchParams();
  if (search.trim()) {
    query.set("search", search.trim());
  }
  const suffix = query.size ? `?${query.toString()}` : "";
  const response = await authorizedFetch(
    `/api/v1/database/tables/${encodeURIComponent(table)}/export${suffix}`,
  );
  if (!response.ok) {
    throw await responseError(response, "导出数据库表失败");
  }

  // 2. 使用临时对象地址触发下载并立即释放资源。
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const link = document.createElement("a");
  link.href = url;
  link.download = `${table}.csv`;
  document.body.appendChild(link);
  link.click();
  link.remove();
  URL.revokeObjectURL(url);
}
