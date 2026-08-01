import {
  forwardRef,
  useCallback,
  type ChangeEvent,
  type InputHTMLAttributes,
} from "react";
import { cn } from "@/lib/utils";
import {
  maxLength,
  sanitizePassword,
  sanitizeUsername,
  stripHtml,
} from "@/lib/sanitize";

export type InputSanitizeMode = "none" | "text" | "username" | "password";

export interface InputProps extends InputHTMLAttributes<HTMLInputElement> {
  sanitize?: InputSanitizeMode;
  maxLen?: number;
  error?: boolean;
}

function applySanitization(
  value: string,
  mode: InputSanitizeMode,
  maxLen?: number,
): string {
  let sanitized: string;

  switch (mode) {
    case "username":
      sanitized = sanitizeUsername(value);
      break;
    case "password":
      sanitized = sanitizePassword(value);
      break;
    case "text":
      sanitized = stripHtml(value);
      break;
    default:
      sanitized = value;
  }

  if (maxLen !== undefined) {
    sanitized = maxLength(sanitized, maxLen);
  }

  return sanitized;
}

export const Input = forwardRef<HTMLInputElement, InputProps>(
  (
    {
      className,
      type = "text",
      sanitize = "none",
      maxLen,
      error,
      onChange,
      ...props
    },
    ref,
  ) => {
    const resolvedSanitize: InputSanitizeMode =
      sanitize === "none" && type === "password" ? "password" : sanitize;

    const handleChange = useCallback(
      (event: ChangeEvent<HTMLInputElement>) => {
        const sanitized = applySanitization(event.target.value, resolvedSanitize, maxLen);
        if (sanitized !== event.target.value) {
          event.target.value = sanitized;
        }
        onChange?.(event);
      },
      [resolvedSanitize, maxLen, onChange],
    );

    return (
      <input
        ref={ref}
        type={type}
        onChange={handleChange}
        className={cn(
          "flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm",
          "ring-offset-background file:border-0 file:bg-transparent file:text-sm file:font-medium",
          "placeholder:text-muted-foreground",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2",
          "disabled:cursor-not-allowed disabled:opacity-50",
          error && "border-destructive focus-visible:ring-destructive",
          className,
        )}
        {...props}
      />
    );
  },
);

Input.displayName = "Input";

export default Input;
