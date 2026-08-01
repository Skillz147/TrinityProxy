import { type ReactNode } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/Card";

interface AuthLayoutProps {
  title: string;
  description: string;
  children: ReactNode;
}

function TrinityLogo() {
  return (
    <div className="mx-auto mb-6 flex h-14 w-14 items-center justify-center rounded-2xl bg-primary/10 ring-1 ring-primary/20">
      <svg
        viewBox="0 0 32 32"
        aria-hidden="true"
        className="h-8 w-8 text-primary"
        fill="none"
        xmlns="http://www.w3.org/2000/svg"
      >
        <path
          d="M16 4L28 10V22L16 28L4 22V10L16 4Z"
          stroke="currentColor"
          strokeWidth="1.75"
          strokeLinejoin="round"
        />
        <path
          d="M16 10L22 13.5V20.5L16 24L10 20.5V13.5L16 10Z"
          fill="currentColor"
          fillOpacity="0.2"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinejoin="round"
        />
        <circle cx="16" cy="16" r="2.5" fill="currentColor" />
      </svg>
    </div>
  );
}

export function AuthLayout({ title, description, children }: AuthLayoutProps) {
  return (
    <div className="relative min-h-screen auth-glow auth-grid">
      <div className="pointer-events-none absolute inset-0 overflow-hidden">
        <div className="absolute -left-24 top-1/4 h-72 w-72 rounded-full bg-primary/10 blur-3xl" />
        <div className="absolute -right-24 bottom-1/4 h-72 w-72 rounded-full bg-primary/5 blur-3xl" />
      </div>

      <div className="relative flex min-h-screen items-center justify-center px-4 py-12 sm:px-6">
        <div className="w-full max-w-md">
          <div className="mb-8 text-center">
            <TrinityLogo />
            <p className="text-xs font-semibold uppercase tracking-[0.2em] text-primary">
              TrinityProxy Master
            </p>
          </div>

          <Card className="border-border/80 bg-card/95 shadow-xl backdrop-blur-sm">
            <CardHeader className="text-center">
              <CardTitle>{title}</CardTitle>
              <CardDescription>{description}</CardDescription>
            </CardHeader>
            <CardContent>{children}</CardContent>
          </Card>

          <p className="mt-6 text-center text-xs text-muted-foreground">
            Secure operator access · Controller dashboard
          </p>
        </div>
      </div>
    </div>
  );
}
