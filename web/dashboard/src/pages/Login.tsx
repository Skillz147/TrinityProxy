import { FormEvent, useState } from "react";
import toast from "react-hot-toast";
import { AuthLayout } from "@/components/AuthLayout";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Label } from "@/components/ui/Label";
import { useAuth } from "@/context/AuthContext";
import { ApiError } from "@/lib/api";
import { sanitizePassword, sanitizeUsername } from "@/lib/sanitize";

export function LoginPage() {
  const { login } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<{ username?: string; password?: string }>({});

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();

    const cleanUsername = sanitizeUsername(username);
    const cleanPassword = sanitizePassword(password);
    const errors: { username?: string; password?: string } = {};

    if (!cleanUsername) errors.username = "Username is required";
    if (!cleanPassword) errors.password = "Password is required";

    setFieldErrors(errors);
    if (Object.keys(errors).length > 0) return;

    setIsSubmitting(true);
    try {
      const user = await login(cleanUsername, cleanPassword);
      if (user.must_change_password) {
        toast.success("Signed in. Please set a new password.");
      } else {
        toast.success("Welcome back.");
      }
    } catch (error) {
      const message =
        error instanceof ApiError
          ? error.message
          : "Unable to sign in. Check your credentials and try again.";
      toast.error(message);
    } finally {
      setIsSubmitting(false);
    }
  }

  return (
    <AuthLayout
      title="Sign in"
      description="Enter the operator credentials provided by your master."
    >
      <form className="space-y-4" onSubmit={handleSubmit} noValidate>
        <div className="space-y-2">
          <Label htmlFor="username">Username</Label>
          <Input
            id="username"
            name="username"
            autoComplete="username"
            autoFocus
            value={username}
            error={Boolean(fieldErrors.username)}
            onChange={(event) => setUsername(sanitizeUsername(event.target.value))}
            placeholder="operator"
            disabled={isSubmitting}
          />
          {fieldErrors.username && (
            <p className="text-sm text-destructive">{fieldErrors.username}</p>
          )}
        </div>

        <div className="space-y-2">
          <Label htmlFor="password">Password</Label>
          <Input
            id="password"
            name="password"
            type="password"
            autoComplete="current-password"
            value={password}
            error={Boolean(fieldErrors.password)}
            onChange={(event) => setPassword(sanitizePassword(event.target.value))}
            placeholder="••••••••"
            disabled={isSubmitting}
          />
          {fieldErrors.password && (
            <p className="text-sm text-destructive">{fieldErrors.password}</p>
          )}
        </div>

        <Button type="submit" className="w-full" disabled={isSubmitting}>
          {isSubmitting ? "Signing in…" : "Sign in"}
        </Button>
      </form>

      <p className="mt-4 text-center text-xs text-muted-foreground">
        Master URL example:{" "}
        <code className="rounded bg-muted px-1.5 py-0.5 text-[11px]">http://master:8080</code>
      </p>
    </AuthLayout>
  );
}
