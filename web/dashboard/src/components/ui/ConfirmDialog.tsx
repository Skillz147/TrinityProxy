import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useId,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Button } from "./Button";

export interface ConfirmOptions {
  title: string;
  message?: string;
  confirmLabel?: string;
  cancelLabel?: string;
  variant?: "default" | "destructive";
}

interface ConfirmState extends ConfirmOptions {
  resolve: (value: boolean) => void;
}

interface ConfirmContextValue {
  confirm: (options: ConfirmOptions) => Promise<boolean>;
}

const ConfirmContext = createContext<ConfirmContextValue | null>(null);

const FOCUSABLE =
  'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])';

function getFocusableElements(container: HTMLElement): HTMLElement[] {
  return Array.from(container.querySelectorAll<HTMLElement>(FOCUSABLE)).filter(
    (el) => !el.hasAttribute("disabled") && el.offsetParent !== null,
  );
}

function ConfirmDialogModal({
  state,
  onClose,
}: {
  state: ConfirmState;
  onClose: (confirmed: boolean) => void;
}) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const cancelRef = useRef<HTMLButtonElement>(null);
  const titleId = useId();
  const descriptionId = useId();
  const previousFocusRef = useRef<HTMLElement | null>(null);

  useEffect(() => {
    previousFocusRef.current = document.activeElement as HTMLElement | null;
    cancelRef.current?.focus();

    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        onClose(false);
        return;
      }

      if (event.key !== "Tab" || !dialogRef.current) return;

      const focusable = getFocusableElements(dialogRef.current);
      if (focusable.length === 0) return;

      const first = focusable[0];
      const last = focusable[focusable.length - 1];

      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };

    document.addEventListener("keydown", handleKeyDown);
    document.body.style.overflow = "hidden";

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      document.body.style.overflow = "";
      previousFocusRef.current?.focus();
    };
  }, [onClose]);

  const handleBackdropClick = () => onClose(false);

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center p-4"
      role="presentation"
    >
      <div
        className="absolute inset-0 bg-background/80 backdrop-blur-sm"
        onClick={handleBackdropClick}
        aria-hidden="true"
      />
      <div
        ref={dialogRef}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby={titleId}
        aria-describedby={state.message ? descriptionId : undefined}
        className="relative z-10 w-full max-w-md rounded-lg border border-border bg-card p-6 shadow-lg"
      >
        <h2 id={titleId} className="text-lg font-semibold text-card-foreground">
          {state.title}
        </h2>
        {state.message && (
          <p id={descriptionId} className="mt-2 text-sm text-muted-foreground">
            {state.message}
          </p>
        )}
        <div className="mt-6 flex justify-end gap-3">
          <Button ref={cancelRef} variant="outline" onClick={() => onClose(false)}>
            {state.cancelLabel ?? "Cancel"}
          </Button>
          <Button
            variant={state.variant === "destructive" ? "destructive" : "default"}
            onClick={() => onClose(true)}
          >
            {state.confirmLabel ?? "Confirm"}
          </Button>
        </div>
      </div>
    </div>
  );
}

export function ConfirmDialogProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<ConfirmState | null>(null);

  const confirm = useCallback((options: ConfirmOptions): Promise<boolean> => {
    return new Promise((resolve) => {
      setState({ ...options, resolve });
    });
  }, []);

  const handleClose = useCallback((confirmed: boolean) => {
    setState((current) => {
      current?.resolve(confirmed);
      return null;
    });
  }, []);

  return (
    <ConfirmContext.Provider value={{ confirm }}>
      {children}
      {state && <ConfirmDialogModal state={state} onClose={handleClose} />}
    </ConfirmContext.Provider>
  );
}

export function useConfirm() {
  const context = useContext(ConfirmContext);
  if (!context) {
    throw new Error("useConfirm must be used within a ConfirmDialogProvider");
  }
  return context.confirm;
}

export default ConfirmDialogProvider;
