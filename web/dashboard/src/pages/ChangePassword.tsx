import { FormEvent, useMemo, useState } from "react";
import toast from "react-hot-toast";
import { useNavigate } from "react-router-dom";
import { AuthLayout } from "@/components/AuthLayout";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Label } from "@/components/ui/Label";
import { useAuth } from "@/context/AuthContext";
import { ApiError } from "@/lib/api";
import { evaluatePasswordStrength } from "@/lib/passwordStrength";
import { sanitizePassword } from "@/lib/sanitize";
import { cn } from "@/lib/utils";

const strengthColors = {
  weak: "bg-destructive",
  fair: "bg-orange-500",
  good: "bg-yellow-500",
  strong: "bg-emerald-500",
} as const;

export function ChangePasswordPage() {
  const navigate = useNavigate();
  const { changePassword, user } = useAuth();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<{
    currentPassword?: string;
    newPassword?: string;
    confirmPassword?: string;
  }>({});

  const strength = useMemo(() => evaluatePasswordStrength(newPassword), [newPassword]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const cleanCurrent = sanitizePassword(currentPassword);
    const cleanNew = sanitizePassword(newPassword);
    const cleanConfirm = sanitizePassword(confirmPassword);
    const errors: typeof fieldErrors = {};

    if (!cleanCurrent) errors.currentPassword = "Current password is required";
    if (!cleanNew) errors.newPassword = "New password is required";
    else if (cleanNew.length < 8) errors.newPassword = "Use at least 8 characters";
    if (cleanNew !== cleanConfirm) errors.confirmPassword = "Passwords do not match";

    setFieldErrors(errors);
    if (Object.keys(errors).length > 0) return;

    setIsSubmitting(true);
    try {
      await changePassword(cleanCurrent, cleanNew);
      toast.success("Password updated.");
      navigate(user?.must_change_password ? "/" : "/settings", { replace: true });
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.message
          : "Unable to update password. Please try again.";
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <AuthLayout
      title="Change password"
      description={
        user?.must_change_password
          ? "You must set a new password before accessing the dashboard."
          : "Update your operator password."
      }
    >
      {user?.must_change_password && (
        <div className="mb-4 rounded-md border border-primary/20 bg-primary/5 px-3 py-2 text-sm text-foreground">
          First-time setup: replace the initial password issued by your master.
        </div>
      )}

      <form className="space-y-4" onSubmit={handleSubmit} noValidate>
        <div className="space-y-2">
          <Label htmlFor="current-password">Current password</Label>
          <Input
            id="current-password"
            name="currentPassword"
            type="password"
            autoComplete="current-password"
            autoFocus
            value={currentPassword}
            error={Boolean(fieldErrors.currentPassword)}
            onChange={(event) => setCurrentPassword(sanitizePassword(event.target.value))}
            disabled={isSubmitting}
          />
          {fieldErrors.currentPassword && (
            <p className="text-sm text-destructive">{fieldErrors.currentPassword}</p>
          )}
        </div>

        <div className="space-y-2">
          <Label htmlFor="new-password">New password</Label>
          <Input
            id="new-password"
            name="newPassword"
            type="password"
            autoComplete="new-password"
            value={newPassword}
            error={Boolean(fieldErrors.newPassword)}
            onChange={(event) => setNewPassword(sanitizePassword(event.target.value))}
            disabled={isSubmitting}
          />
          {newPassword.length > 0 && (
            <div className="space-y-2 pt-1">
              <div className="flex items-center justify-between text-xs">
                <span className="text-muted-foreground">Strength</span>
                <span
                  className={cn(
                    "font-medium",
                    strength.score === "strong" && "text-emerald-600 dark:text-emerald-400",
                    strength.score === "good" && "text-yellow-600 dark:text-yellow-400",
                    strength.score === "fair" && "text-orange-600 dark:text-orange-400",
                    strength.score === "weak" && "text-destructive",
                  )}
                >
                  {strength.label}
                </span>
              </div>
              <div className="h-1.5 overflow-hidden rounded-full bg-muted">
                <div
                  className={cn("h-full rounded-full transition-all", strengthColors[strength.score])}
                  style={{ width: `${strength.percent}%` }}
                />
              </div>
              {strength.hints.length > 0 && (
                <ul className="space-y-1 text-xs text-muted-foreground">
                  {strength.hints.slice(0, 3).map((hint) => (
                    <li key={hint}>• {hint}</li>
                  ))}
                </ul>
              )}
            </div>
          )}
          {fieldErrors.newPassword && (
            <p className="text-sm text-destructive">{fieldErrors.newPassword}</p>
          )}
        </div>

        <div className="space-y-2">
          <Label htmlFor="confirm-password">Confirm new password</Label>
          <Input
            id="confirm-password"
            name="confirmPassword"
            type="password"
            autoComplete="new-password"
            value={confirmPassword}
            error={Boolean(fieldErrors.confirmPassword)}
            onChange={(event) => setConfirmPassword(sanitizePassword(event.target.value))}
            disabled={isSubmitting}
          />
          {fieldErrors.confirmPassword && (
            <p className="text-sm text-destructive">{fieldErrors.confirmPassword}</p>
          )}
        </div>

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? "Updating…" : "Update password"}
        </Button>
      </form>
    </AuthLayout>
  );
}
