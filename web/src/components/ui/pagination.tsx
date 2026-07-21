import * as React from "react";
import { ChevronLeft, ChevronRight, MoreHorizontal } from "lucide-react";

import { Button, type ButtonProps } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const Pagination = ({ className, ...props }: React.ComponentProps<"nav">) => (
  <nav role="navigation" aria-label="分页" className={cn("mx-auto flex w-full justify-center", className)} {...props} />
);
Pagination.displayName = "Pagination";

const PaginationContent = React.forwardRef<HTMLUListElement, React.ComponentProps<"ul">>(
  ({ className, ...props }, ref) => <ul ref={ref} className={cn("flex flex-row items-center gap-1", className)} {...props} />,
);
PaginationContent.displayName = "PaginationContent";

const PaginationItem = React.forwardRef<HTMLLIElement, React.ComponentProps<"li">>((props, ref) => <li ref={ref} {...props} />);
PaginationItem.displayName = "PaginationItem";

const PaginationLink = React.forwardRef<HTMLButtonElement, ButtonProps & { isActive?: boolean }>(
  ({ className, isActive, size = "icon", variant, ...props }, ref) => (
    <Button ref={ref} size={size} variant={variant || (isActive ? "outline" : "ghost")} aria-current={isActive ? "page" : undefined} className={className} {...props} />
  ),
);
PaginationLink.displayName = "PaginationLink";

const PaginationPrevious = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, ...props }, ref) => (
  <PaginationLink ref={ref} aria-label="上一页" className={cn("h-8 w-8", className)} {...props}><ChevronLeft className="h-4 w-4" /></PaginationLink>
));
PaginationPrevious.displayName = "PaginationPrevious";

const PaginationNext = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, ...props }, ref) => (
  <PaginationLink ref={ref} aria-label="下一页" className={cn("h-8 w-8", className)} {...props}><ChevronRight className="h-4 w-4" /></PaginationLink>
));
PaginationNext.displayName = "PaginationNext";

const PaginationEllipsis = ({ className, ...props }: React.ComponentProps<"span">) => (
  <span aria-hidden className={cn("flex h-9 w-9 items-center justify-center", className)} {...props}><MoreHorizontal className="h-4 w-4" /></span>
);
PaginationEllipsis.displayName = "PaginationEllipsis";

export { Pagination, PaginationContent, PaginationEllipsis, PaginationItem, PaginationLink, PaginationNext, PaginationPrevious };
