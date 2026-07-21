import * as React from "react";

import { cn } from "@/lib/utils";

const Item = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("flex min-w-0 items-center gap-3 rounded-md border px-3 py-2", className)} {...props} />,
);
Item.displayName = "Item";

const ItemMedia = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("flex shrink-0 items-center justify-center text-muted-foreground [&>svg]:h-4 [&>svg]:w-4", className)} {...props} />,
);
ItemMedia.displayName = "ItemMedia";

const ItemContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("min-w-0 flex-1", className)} {...props} />,
);
ItemContent.displayName = "ItemContent";

const ItemTitle = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("truncate text-sm font-medium", className)} {...props} />,
);
ItemTitle.displayName = "ItemTitle";

const ItemDescription = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("mt-0.5 text-xs text-muted-foreground", className)} {...props} />,
);
ItemDescription.displayName = "ItemDescription";

const ItemActions = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("ml-auto flex shrink-0 items-center gap-1", className)} {...props} />,
);
ItemActions.displayName = "ItemActions";

export { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle };
