import { type ReactNode } from "react";
import { toast, type ExternalToast } from "sonner";

type NotifyOptions = Omit<ExternalToast, "description"> & {
  description?: ReactNode;
};

const persistentToastDefaults: ExternalToast = {
  duration: Infinity,
  closeButton: true,
  action: {
    label: "清空全部",
    onClick: () => toast.dismiss(),
  },
};

function buildToastOptions(description?: ReactNode, options: NotifyOptions = {}): ExternalToast {
  const { description: optionDescription, ...restOptions } = options;
  return {
    ...persistentToastDefaults,
    ...restOptions,
    description: optionDescription ?? description,
  };
}

function getErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : fallback;
}

export const notify = {
  success(title: ReactNode, description?: ReactNode, options?: NotifyOptions) {
    return toast.success(title, buildToastOptions(description, options));
  },
  error(title: ReactNode, description?: ReactNode, options?: NotifyOptions) {
    return toast.error(title, buildToastOptions(description, options));
  },
  info(title: ReactNode, description?: ReactNode, options?: NotifyOptions) {
    return toast.info(title, buildToastOptions(description, options));
  },
  warning(title: ReactNode, description?: ReactNode, options?: NotifyOptions) {
    return toast.warning(title, buildToastOptions(description, options));
  },
  clear() {
    toast.dismiss();
  },
  errorFrom(error: unknown, fallback: string, title = "操作失败") {
    return toast.error(title, buildToastOptions(getErrorMessage(error, fallback)));
  },
};
