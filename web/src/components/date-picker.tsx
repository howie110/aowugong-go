import { format, isValid, parse } from "date-fns";
import { zhCN } from "date-fns/locale";
import { CalendarIcon, X } from "lucide-react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Calendar } from "@/components/ui/calendar";
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover";
import { cn } from "@/lib/utils";

type DatePickerProps = {
  value: string;
  onChange: (value: string) => void;
  id?: string;
  placeholder?: string;
  disabled?: boolean;
  clearable?: boolean;
  className?: string;
};

/** DatePicker 选择本地日历日期并输出 yyyy-MM-dd 文本，不访问外部服务。 */
export function DatePicker({
  value,
  onChange,
  id,
  placeholder = "选择日期",
  disabled = false,
  clearable = false,
  className,
}: DatePickerProps) {
  // 1. 将接口日期文本解析成本地日期，避免时区换日。
  const selectedDate = useMemo(() => parseDate(value), [value]);
  const [open, setOpen] = useState(false);

  /** handleSelect 写回标准日期文本并关闭日历，无其他副作用。 */
  function handleSelect(date?: Date) {
    // 1. 忽略未选中的日期。
    if (!date) {
      return;
    }
    // 2. 输出接口统一使用的日期格式并收起浮层。
    onChange(format(date, "yyyy-MM-dd"));
    setOpen(false);
  }

  return (
    <div className={cn("flex min-w-0 gap-1", className)}>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            id={id}
            type="button"
            variant="outline"
            disabled={disabled}
            className={cn("min-w-0 flex-1 justify-start px-3 font-normal", !selectedDate && "text-muted-foreground")}
          >
            <CalendarIcon className="h-4 w-4 shrink-0" />
            <span className="truncate">
              {selectedDate ? format(selectedDate, "yyyy年MM月dd日", { locale: zhCN }) : placeholder}
            </span>
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-auto p-0" align="start">
          <Calendar mode="single" selected={selectedDate} onSelect={handleSelect} initialFocus />
        </PopoverContent>
      </Popover>
      {clearable && value ? (
        <Button type="button" variant="ghost" size="icon" onClick={() => onChange("")} disabled={disabled} aria-label="清除日期">
          <X className="h-4 w-4" />
        </Button>
      ) : null}
    </div>
  );
}

/** parseDate 将 yyyy-MM-dd 文本解析成本地日期；无效值返回 undefined。 */
function parseDate(value: string) {
  // 1. 空文本不产生日期。
  if (!value) {
    return undefined;
  }
  // 2. 严格按接口日期格式解析并校验结果。
  const date = parse(value, "yyyy-MM-dd", new Date());
  return isValid(date) ? date : undefined;
}
