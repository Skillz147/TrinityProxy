import hotToast, { Toaster as HotToaster, type ToastOptions } from "react-hot-toast";

const defaultOptions: ToastOptions = {
  duration: 4000,
  style: {
    background: "hsl(var(--card))",
    color: "hsl(var(--card-foreground))",
    border: "1px solid hsl(var(--border))",
    borderRadius: "var(--radius)",
    fontSize: "0.875rem",
  },
  success: {
    iconTheme: {
      primary: "hsl(var(--primary))",
      secondary: "hsl(var(--primary-foreground))",
    },
  },
  error: {
    iconTheme: {
      primary: "hsl(var(--destructive))",
      secondary: "hsl(var(--destructive-foreground))",
    },
  },
};

export function Toaster() {
  return (
    <HotToaster
      position="top-right"
      toastOptions={defaultOptions}
      containerClassName="!z-[100]"
    />
  );
}

export const toast = {
  success: (message: string, options?: ToastOptions) =>
    hotToast.success(message, { ...defaultOptions, ...options }),

  error: (message: string, options?: ToastOptions) =>
    hotToast.error(message, { ...defaultOptions, ...options }),

  loading: (message: string, options?: ToastOptions) =>
    hotToast.loading(message, { ...defaultOptions, ...options }),

  dismiss: hotToast.dismiss,
  promise: hotToast.promise,
};

export default toast;
