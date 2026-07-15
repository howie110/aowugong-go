import { useEffect, useRef, useState } from "react";

const COMPACT_TABLE_PAGE_SIZE = 8;
const DEFAULT_DENSE_TABLE_PAGE_SIZE = 15;
const MIN_DENSE_TABLE_PAGE_SIZE = 8;
const MAX_DENSE_TABLE_PAGE_SIZE = 80;
const FALLBACK_TABLE_HEAD_HEIGHT = 40;
const FALLBACK_TABLE_ROW_HEIGHT = 45;
const DENSE_TABLE_MEDIA_QUERY = "(min-width: 1280px)";

export function useResponsiveTablePageSize(deps: unknown[] = []) {
  const tableRef = useRef<HTMLDivElement>(null);
  const [pageSize, setPageSize] = useState(() => {
    if (typeof window === "undefined") {
      return COMPACT_TABLE_PAGE_SIZE;
    }
    return window.matchMedia(DENSE_TABLE_MEDIA_QUERY).matches
      ? DEFAULT_DENSE_TABLE_PAGE_SIZE
      : COMPACT_TABLE_PAGE_SIZE;
  });

  useEffect(() => {
    let frameId = 0;
    const denseTableQuery = window.matchMedia(DENSE_TABLE_MEDIA_QUERY);

    function calculatePageSize() {
      if (!denseTableQuery.matches) {
        setPageSize((current) => (current === COMPACT_TABLE_PAGE_SIZE ? current : COMPACT_TABLE_PAGE_SIZE));
        return;
      }
      const tableContainer = tableRef.current;
      if (!tableContainer) {
        return;
      }

      const tableTop = tableContainer.getBoundingClientRect().top;
      const headerHeight =
        tableContainer.querySelector("thead")?.getBoundingClientRect().height || FALLBACK_TABLE_HEAD_HEIGHT;
      const rowHeights = Array.from(tableContainer.querySelectorAll("tbody tr"))
        .map((row) => row.getBoundingClientRect().height)
        .filter((height) => height > 0);
      const rowHeight = rowHeights.length
        ? rowHeights.reduce((sum, height) => sum + height, 0) / rowHeights.length
        : FALLBACK_TABLE_ROW_HEIGHT;
      const availableHeight = tableContainer.getBoundingClientRect().height || window.innerHeight - tableTop;
      const nextPageSize = clampNumber(
        Math.floor((availableHeight - headerHeight) / rowHeight),
        MIN_DENSE_TABLE_PAGE_SIZE,
        MAX_DENSE_TABLE_PAGE_SIZE,
      );

      setPageSize((current) => (current === nextPageSize ? current : nextPageSize));
    }

    function scheduleCalculate() {
      window.cancelAnimationFrame(frameId);
      frameId = window.requestAnimationFrame(calculatePageSize);
    }

    scheduleCalculate();
    window.addEventListener("resize", scheduleCalculate);
    denseTableQuery.addEventListener("change", scheduleCalculate);
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(scheduleCalculate);
    if (tableRef.current && observer) {
      observer.observe(tableRef.current);
    }

    return () => {
      window.cancelAnimationFrame(frameId);
      window.removeEventListener("resize", scheduleCalculate);
      denseTableQuery.removeEventListener("change", scheduleCalculate);
      observer?.disconnect();
    };
  }, deps);

  return { tableRef, pageSize };
}


function clampNumber(value: number, min: number, max: number) {
  if (!Number.isFinite(value)) {
    return min;
  }
  return Math.min(Math.max(value, min), max);
}
