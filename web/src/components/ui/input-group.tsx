import * as React from "react";

import { Button, type ButtonProps } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

const InputGroup = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("flex h-9 w-full items-center rounded-md border border-input bg-background shadow-sm focus-within:ring-1 focus-within:ring-ring", className)} {...props} />,
);
InputGroup.displayName = "InputGroup";

const InputGroupInput = React.forwardRef<HTMLInputElement, React.ComponentPropsWithoutRef<typeof Input>>(
  ({ className, ...props }, ref) => <Input ref={ref} className={cn("h-full flex-1 border-0 bg-transparent shadow-none focus-visible:ring-0", className)} {...props} />,
);
InputGroupInput.displayName = "InputGroupInput";

const InputGroupAddon = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
  ({ className, ...props }, ref) => <div ref={ref} className={cn("flex shrink-0 items-center justify-center px-3 text-muted-foreground [&>svg]:h-4 [&>svg]:w-4", className)} {...props} />,
);
InputGroupAddon.displayName = "InputGroupAddon";

const InputGroupButton = React.forwardRef<HTMLButtonElement, ButtonProps>(({ className, size = "icon", variant = "ghost", ...props }, ref) => (
  <Button ref={ref} size={size} variant={variant} className={cn("mr-1 h-7 w-7", className)} {...props} />
));
InputGroupButton.displayName = "InputGroupButton";

export { InputGroup, InputGroupAddon, InputGroupButton, InputGroupInput };
