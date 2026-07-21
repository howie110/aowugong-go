import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";

import { Button, type ButtonProps } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const attachmentVariants = cva("flex min-w-0 items-center gap-3 rounded-md border bg-background p-3 text-sm", {
  variants: {
    state: {
      idle: "",
      uploading: "",
      processing: "",
      error: "border-destructive/40 bg-destructive/5",
      done: "",
    },
    size: { default: "p-3", sm: "p-2", xs: "gap-2 px-2 py-1.5 text-xs" },
  },
  defaultVariants: { state: "idle", size: "default" },
});

const Attachment = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement> & VariantProps<typeof attachmentVariants>
>(({ className, state, size, ...props }, ref) => (
  <div ref={ref} data-state={state} className={cn(attachmentVariants({ state, size }), className)} {...props} />
));
Attachment.displayName = "Attachment";

const AttachmentMedia = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => (
    <div ref={ref} className={cn("flex h-9 w-9 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground [&>svg]:h-4 [&>svg]:w-4", className)} {...props} />
  ),
);
AttachmentMedia.displayName = "AttachmentMedia";

const AttachmentContent = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("min-w-0 flex-1", className)} {...props} />,
);
AttachmentContent.displayName = "AttachmentContent";

const AttachmentTitle = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("truncate font-medium", className)} {...props} />,
);
AttachmentTitle.displayName = "AttachmentTitle";

const AttachmentDescription = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("mt-0.5 truncate text-xs text-muted-foreground", className)} {...props} />,
);
AttachmentDescription.displayName = "AttachmentDescription";

const AttachmentActions = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("ml-auto flex shrink-0 items-center gap-1", className)} {...props} />,
);
AttachmentActions.displayName = "AttachmentActions";

const AttachmentAction = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, size = "icon", variant = "ghost", ...props }, ref) => (
  <Button ref={ref} size={size} variant={variant} className={cn("h-8 w-8", className)} {...props} />
));
AttachmentAction.displayName = "AttachmentAction";

const AttachmentGroup = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("grid gap-2", className)} {...props} />,
);
AttachmentGroup.displayName = "AttachmentGroup";

export {
  Attachment,
  AttachmentAction,
  AttachmentActions,
  AttachmentContent,
  AttachmentDescription,
  AttachmentGroup,
  AttachmentMedia,
  AttachmentTitle,
};
