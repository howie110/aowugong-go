import { Toaster as Sonner, type ToasterProps } from "sonner";

function Toaster(props: ToasterProps) {
  return (
    <Sonner
      theme="light"
      position="top-right"
      closeButton
      visibleToasts={3}
      duration={Infinity}
      toastOptions={{
        closeButton: true,
        classNames: {
          toast:
            "group toast border-border bg-background text-foreground shadow-lg data-[type=error]:border-border data-[type=error]:bg-background data-[type=error]:text-foreground data-[type=info]:border-border data-[type=info]:bg-background data-[type=info]:text-foreground data-[type=success]:border-border data-[type=success]:bg-background data-[type=success]:text-foreground data-[type=warning]:border-border data-[type=warning]:bg-background data-[type=warning]:text-foreground",
          description: "group-[.toast]:text-muted-foreground",
          actionButton: "group-[.toast]:bg-primary group-[.toast]:text-primary-foreground",
          cancelButton: "group-[.toast]:bg-muted group-[.toast]:text-muted-foreground",
          closeButton: "group-[.toast]:border-border group-[.toast]:bg-background group-[.toast]:text-foreground",
          icon: "text-foreground",
          error: "text-foreground",
          info: "text-foreground",
          success: "text-foreground",
          warning: "text-foreground",
        },
      }}
      {...props}
    />
  );
}

export { Toaster };
