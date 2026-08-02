import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  KeyRound,
  RefreshCw,
  Settings2,
  Shield,
} from "lucide-react";
import { ErrorState } from "@/components/ErrorState";
import { Button } from "@/components/ui/Button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/Card";
import { Input } from "@/components/ui/Input";
import { Label } from "@/components/ui/Label";
import { CardSkeleton } from "@/components/ui/Skeleton";
import { useAuth } from "@/context/AuthContext";
import {
  ApiError,
  fetchDeployment,
  regenerateAgentKey,
  updateDeployment,
  type DeploymentConfig,
} from "@/lib/api";
import { toast } from "@/lib/toast";
import { sanitizeControllerURL, sanitizeDomain } from "@/lib/sanitize";

export function SettingsPage() {
  const { user, token } = useAuth();
  const [deployment, setDeployment] = useState<DeploymentConfig | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isRotating, setIsRotating] = useState(false);
  const [deploymentError, setDeploymentError] = useState<string | null>(null);

  const [publicDomain, setPublicDomain] = useState("");
  const [controllerURL, setControllerURL] = useState("");

  const loadAll = useCallback(async () => {
    setIsLoading(true);
    setDeploymentError(null);

    try {
      const deploy = await fetchDeployment(token);
      setDeployment(deploy);
      setPublicDomain(deploy.public_domain ?? "");
      setControllerURL(deploy.controller_public_url ?? "");
    } catch (err) {
      setDeployment(null);
      setDeploymentError(
        err instanceof ApiError ? err.message : "Unable to load deployment settings.",
      );
    } finally {
      setIsLoading(false);
    }
  }, [token]);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  async function handleSaveDeployment() {
    if (!deployment) return;

    setIsSaving(true);
    const cleanedDomain = sanitizeDomain(publicDomain);
    const cleanedControllerURL = sanitizeControllerURL(controllerURL);
    try {
      const updated = await updateDeployment(token, {
        public_domain: cleanedDomain,
        controller_public_url: cleanedControllerURL,
        ssl_mode: deployment.ssl_mode,
      });
      setDeployment(updated);
      setPublicDomain(updated.public_domain ?? cleanedDomain);
      setControllerURL(updated.controller_public_url ?? cleanedControllerURL);
      toast.success("Settings saved");
    } catch (err) {
      toast.error(
        err instanceof ApiError ? err.message : "Failed to save deployment settings",
      );
    } finally {
      setIsSaving(false);
    }
  }

  async function handleRotateKey() {
    setIsRotating(true);
    try {
      const result = await regenerateAgentKey(token);
      setDeployment((prev) =>
        prev ? { ...prev, has_agent_key: result.has_agent_key } : prev,
      );
      toast.success(result.message);
    } catch (err) {
      toast.error(
        err instanceof ApiError ? err.message : "Failed to rotate agent key",
      );
    } finally {
      setIsRotating(false);
    }
  }

  return (
    <div className="w-full space-y-6">
      {isLoading && (
        <div className="flex flex-col gap-6 w-full">
          <CardSkeleton lines={3} />
          <CardSkeleton lines={8} />
        </div>
      )}

      {!isLoading && (
        <>
          <div className="flex flex-col gap-6 w-full">
            <Card className="w-full">
              <CardHeader className="flex flex-row items-center justify-between gap-4 border-b border-border/40 pb-4">
                <div className="min-w-0 space-y-1">
                  <CardTitle className="flex items-center gap-2 text-lg">
                    <Settings2 className="h-5 w-5 text-primary" aria-hidden="true" />
                    Account
                  </CardTitle>
                  <CardDescription>
                    Signed in as{" "}
                    <span className="font-medium text-foreground">{user?.username}</span>
                  </CardDescription>
                </div>
                <Link to="/change-password" className="shrink-0">
                  <Button variant="outline">
                    <KeyRound className="h-4 w-4" />
                    Change password
                  </Button>
                </Link>
              </CardHeader>
            </Card>
          </div>

          <div className="flex flex-col gap-6 w-full">
            <Card className="w-full">
              <CardHeader className="border-b border-border/40 pb-4">
                <CardTitle className="flex items-center gap-2 text-lg">
                  <Settings2 className="h-5 w-5 text-primary" aria-hidden="true" />
                  Deployment
                </CardTitle>
                <CardDescription>
                  Tell agents how to reach your controller. SSL and DNS are configured on the
                  server — not in this dashboard.
                </CardDescription>
              </CardHeader>
              <CardContent className="space-y-6 p-6">
                {deploymentError && (
                  <ErrorState
                    message={deploymentError}
                    onRetry={() => void loadAll()}
                    className="py-6"
                  />
                )}

                <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                  <div className="space-y-2">
                    <Label htmlFor="public-domain">Your domain</Label>
                    <Input
                      id="public-domain"
                      placeholder="trinityproxy.local"
                      value={publicDomain}
                      onChange={(e) => setPublicDomain(sanitizeDomain(e.target.value))}
                      sanitize="text"
                    />
                    <p className="text-xs text-muted-foreground">
                      Bare domain only — no https:// (e.g. trinityproxy.local).
                    </p>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="controller-url">Controller address</Label>
                    <Input
                      id="controller-url"
                      placeholder="https://api.trinityproxy.local"
                      value={controllerURL}
                      onChange={(e) => setControllerURL(sanitizeControllerURL(e.target.value))}
                      sanitize="text"
                    />
                    <p className="text-xs text-muted-foreground">
                      URL agents use to register (e.g. https://api.yourdomain.com). Set this
                      after running the server SSL script.
                    </p>
                  </div>
                </div>

                <div className="flex flex-wrap gap-2">
                  <Button onClick={() => void handleSaveDeployment()} disabled={isSaving || !deployment}>
                    {isSaving ? "Saving…" : "Save"}
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => void handleRotateKey()}
                    disabled={isRotating}
                  >
                    <RefreshCw className="h-4 w-4" />
                    {isRotating ? "Generating…" : "Generate enrollment key"}
                  </Button>
                </div>

                {deployment?.has_agent_key && (
                  <p className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Shield className="h-3.5 w-3.5" />
                    You&apos;re all set — copy the install command on{" "}
                    <Link to="/deploy" className="text-primary underline-offset-2 hover:underline">
                      Deploy Agent
                    </Link>
                    . Install scripts use the enrollment key; each agent receives a unique node token on first heartbeat.
                  </p>
                )}
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}
