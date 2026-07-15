import { CalendarClock, Pencil, Plus, RefreshCw, Save, Trash2, X } from "lucide-react";
import { useEffect, useMemo, useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import {
  type SubscriptionRecord,
  type SubscriptionRecordPayload,
  createSubscription,
  deleteSubscription,
  fetchSubscriptions,
  updateSubscription,
} from "@/lib/subscriptions";
import { notify } from "@/lib/notify";

const emptyForm: SubscriptionRecordPayload = {
  service_name: "",
  note: "",
  category: "生活",
  annual_fee: "0",
  monthly_fee: "0",
  starts_on: "",
  expires_on: "",
};

export function SubscriptionsPage() {
  const [records, setRecords] = useState<SubscriptionRecord[]>([]);
  const [form, setForm] = useState<SubscriptionRecordPayload>(emptyForm);
  const [editingId, setEditingId] = useState<number | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const summary = useMemo(() => buildSummary(records), [records]);

  useEffect(() => {
    void loadRecords();
  }, []);

  async function loadRecords() {
    setIsLoading(true);
    try {
      setRecords(await fetchSubscriptions());
    } catch (error) {
      notify.errorFrom(error, "订阅记录加载失败");
    } finally {
      setIsLoading(false);
    }
  }

  async function handleSave() {
    if (!form.service_name.trim()) {
      notify.warning("请输入订阅服务名");
      return;
    }
    if (!form.expires_on) {
      notify.warning("请选择到期日期");
      return;
    }

    setIsSaving(true);
    try {
      const payload = normalizePayload(form);
      if (editingId) {
        await updateSubscription(editingId, payload);
        notify.success("订阅记录已更新");
      } else {
        await createSubscription(payload);
        notify.success("订阅记录已新增");
      }
      resetForm();
      await loadRecords();
    } catch (error) {
      notify.errorFrom(error, "保存订阅记录失败");
    } finally {
      setIsSaving(false);
    }
  }

  async function handleDelete(record: SubscriptionRecord) {
    if (!window.confirm(`确定删除「${record.service_name}」吗？`)) {
      return;
    }
    try {
      await deleteSubscription(record.id);
      notify.success("订阅记录已删除");
      await loadRecords();
      if (editingId === record.id) {
        resetForm();
      }
    } catch (error) {
      notify.errorFrom(error, "删除订阅记录失败");
    }
  }

  function handleEdit(record: SubscriptionRecord) {
    setEditingId(record.id);
    setForm({
      service_name: record.service_name,
      note: record.note || "",
      category: record.category,
      annual_fee: record.annual_fee,
      monthly_fee: record.monthly_fee,
      starts_on: record.starts_on || "",
      expires_on: record.expires_on,
    });
  }

  function resetForm() {
    setEditingId(null);
    setForm(emptyForm);
  }

  return (
    <div className="space-y-4">
      <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
        <SummaryCard label="订阅总数" value={`${summary.totalCount}`} detail={`订阅中 ${summary.activeCount} 项`} />
        <SummaryCard label="已结束" value={`${summary.expiredCount}`} detail="按到期日自动计算" />
        <SummaryCard label="30 天内到期" value={`${summary.upcomingCount}`} detail="需要续费关注" />
        <SummaryCard label="年费合计" value={formatMoney(summary.annualTotal)} detail={`月费约 ${formatMoney(summary.monthlyTotal)}`} />
      </div>

      <SubscriptionEditorCard
        form={form}
        editingId={editingId}
        isSaving={isSaving}
        isLoading={isLoading}
        onChange={setForm}
        onSave={handleSave}
        onCancel={resetForm}
        onRefresh={loadRecords}
      />

      <SubscriptionTable records={records} isLoading={isLoading} onEdit={handleEdit} onDelete={handleDelete} />
    </div>
  );
}

function SubscriptionEditorCard({
  form,
  editingId,
  isSaving,
  isLoading,
  onChange,
  onSave,
  onCancel,
  onRefresh,
}: {
  form: SubscriptionRecordPayload;
  editingId: number | null;
  isSaving: boolean;
  isLoading: boolean;
  onChange: (form: SubscriptionRecordPayload) => void;
  onSave: () => void;
  onCancel: () => void;
  onRefresh: () => void;
}) {
  function updateField(key: keyof SubscriptionRecordPayload, value: string) {
    onChange({ ...form, [key]: value });
  }

  return (
    <Card>
      <CardHeader className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
        <div>
          <CardTitle>{editingId ? "编辑订阅" : "新增订阅"}</CardTitle>
          <CardDescription>状态和离到期天数由到期日期自动计算。</CardDescription>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={onRefresh} disabled={isLoading || isSaving}>
          <RefreshCw className={["h-4 w-4", isLoading ? "animate-spin" : ""].join(" ")} />
          刷新
        </Button>
      </CardHeader>
      <CardContent className="space-y-3">
        <div className="grid gap-3 md:grid-cols-2 xl:grid-cols-4">
          <Field label="订阅服务名">
            <Input value={form.service_name} onChange={(event) => updateField("service_name", event.target.value)} placeholder="例如 阿里云服务器" />
          </Field>
          <Field label="类型">
            <Input value={form.category} onChange={(event) => updateField("category", event.target.value)} placeholder="IT / 生活" />
          </Field>
          <Field label="年费用">
            <Input type="number" inputMode="decimal" value={form.annual_fee} onChange={(event) => updateField("annual_fee", event.target.value)} />
          </Field>
          <Field label="月费用">
            <Input type="number" inputMode="decimal" value={form.monthly_fee} onChange={(event) => updateField("monthly_fee", event.target.value)} />
          </Field>
          <Field label="开始日期">
            <Input type="date" value={form.starts_on || ""} onChange={(event) => updateField("starts_on", event.target.value)} />
          </Field>
          <Field label="到期日期">
            <Input type="date" value={form.expires_on} onChange={(event) => updateField("expires_on", event.target.value)} />
          </Field>
          <div className="md:col-span-2">
            <Field label="备注">
              <Input value={form.note} onChange={(event) => updateField("note", event.target.value)} />
            </Field>
          </div>
        </div>
        <div className="flex flex-col gap-2 sm:flex-row sm:justify-end">
          {editingId ? (
            <Button type="button" variant="outline" onClick={onCancel} disabled={isSaving}>
              <X className="h-4 w-4" />
              取消编辑
            </Button>
          ) : null}
          <Button type="button" onClick={onSave} disabled={isSaving}>
            {isSaving ? <RefreshCw className="h-4 w-4 animate-spin" /> : <Save className="h-4 w-4" />}
            保存
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      {children}
    </div>
  );
}

function SubscriptionTable({
  records,
  isLoading,
  onEdit,
  onDelete,
}: {
  records: SubscriptionRecord[];
  isLoading: boolean;
  onEdit: (record: SubscriptionRecord) => void;
  onDelete: (record: SubscriptionRecord) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <div className="flex items-start justify-between gap-3">
          <div>
            <CardTitle>订阅列表</CardTitle>
            <CardDescription>到期日前 30 天以内会标为临近，到期日小于今天自动标为已结束。</CardDescription>
          </div>
          <CalendarClock className="h-4 w-4 shrink-0 text-muted-foreground" />
        </div>
      </CardHeader>
      <CardContent>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>订阅服务名</TableHead>
              <TableHead>类型</TableHead>
              <TableHead className="text-right">年费用</TableHead>
              <TableHead className="text-right">月费用</TableHead>
              <TableHead>开始日期</TableHead>
              <TableHead>到期日期</TableHead>
              <TableHead>状态</TableHead>
              <TableHead className="text-right">离到期日数</TableHead>
              <TableHead>创建人</TableHead>
              <TableHead className="text-right">操作</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {records.map((record) => (
              <TableRow key={record.id}>
                <TableCell>
                  <div className="font-medium">{record.service_name}</div>
                  {record.note ? <div className="text-xs text-muted-foreground">{record.note}</div> : null}
                </TableCell>
                <TableCell>
                  <Badge variant="secondary">{record.category}</Badge>
                </TableCell>
                <TableCell className="text-right tabular-nums">{formatMoney(record.annual_fee)}</TableCell>
                <TableCell className="text-right tabular-nums">{formatMoney(record.monthly_fee)}</TableCell>
                <TableCell className="text-muted-foreground">{record.starts_on || "-"}</TableCell>
                <TableCell>{record.expires_on}</TableCell>
                <TableCell>
                  <SubscriptionStatusBadge record={record} />
                </TableCell>
                <TableCell className={["text-right tabular-nums", record.days_until_expiry < 0 ? "text-red-700" : ""].join(" ")}>
                  {record.days_until_expiry}
                </TableCell>
                <TableCell className="text-muted-foreground">{record.created_by || "-"}</TableCell>
                <TableCell>
                  <div className="flex justify-end gap-1">
                    <Button type="button" variant="ghost" size="icon" onClick={() => onEdit(record)} title="编辑">
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button type="button" variant="ghost" size="icon" onClick={() => onDelete(record)} title="删除">
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </TableCell>
              </TableRow>
            ))}
            {!records.length ? (
              <TableRow>
                <TableCell colSpan={10} className="py-8 text-center text-sm text-muted-foreground">
                  {isLoading ? "正在加载订阅记录..." : "暂无订阅记录"}
                </TableCell>
              </TableRow>
            ) : null}
          </TableBody>
        </Table>
      </CardContent>
    </Card>
  );
}

function SubscriptionStatusBadge({ record }: { record: SubscriptionRecord }) {
  if (record.current_status === "已结束") {
    return <Badge variant="danger">已结束</Badge>;
  }
  if (record.days_until_expiry <= 30) {
    return <Badge variant="warning">临近到期</Badge>;
  }
  return <Badge variant="success">订阅中</Badge>;
}

function SummaryCard({ label, value, detail }: { label: string; value: string; detail: string }) {
  return (
    <Card>
      <CardHeader className="pb-3">
        <CardTitle className="text-sm text-muted-foreground">{label}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="text-2xl font-semibold tabular-nums">{value}</div>
        <p className="mt-1 text-xs text-muted-foreground">{detail}</p>
      </CardContent>
    </Card>
  );
}

function buildSummary(records: SubscriptionRecord[]) {
  const activeRecords = records.filter((record) => record.current_status === "订阅中");
  return {
    totalCount: records.length,
    activeCount: activeRecords.length,
    expiredCount: records.length - activeRecords.length,
    upcomingCount: activeRecords.filter((record) => record.days_until_expiry <= 30).length,
    annualTotal: activeRecords.reduce((sum, record) => sum + toNumber(record.annual_fee), 0),
    monthlyTotal: activeRecords.reduce((sum, record) => sum + toNumber(record.monthly_fee), 0),
  };
}

function normalizePayload(form: SubscriptionRecordPayload): SubscriptionRecordPayload {
  return {
    ...form,
    service_name: form.service_name.trim(),
    note: form.note.trim(),
    category: form.category.trim() || "生活",
    annual_fee: form.annual_fee || "0",
    monthly_fee: form.monthly_fee || "0",
    starts_on: form.starts_on || null,
  };
}

function toNumber(value: string | number) {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

function formatMoney(value: string | number) {
  return toNumber(value).toLocaleString("zh-CN", {
    minimumFractionDigits: 2,
    maximumFractionDigits: 2,
  });
}
