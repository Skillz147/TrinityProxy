import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  Cloud,
  Globe,
  Info,
  KeyRound,
  RefreshCw,
  Server,
  Settings2,
  Shield,
  Terminal,
} from "lucide-react";
import { CloudflareSSLDialog } from "@/components/CloudflareSSLDialog";
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
  fetchDevSetup,
  fetchDNSHints,
  regenerateAgentKey,
  updateDeployment,
  type DeploymentConfig,
  type DevSetup,
  type DNSHints,
  type SSLMode,
} from "@/lib/api";
import { toast } from "@/lib/toast";
import { sanitizeControllerURL, sanitizeDomain } from "@/lib/sanitize";

const SSL_MODES: { value: SSLMode; label: string; hint: string }[] = [
  {
    value: "caddy",
    label: "Caddy",
    hint: "Caddy reverse proxy with automatic HTTPS via Cloudflare DNS-01.",
  },
  {
    value: "dev-mkcert",
    label: "Dev (mkcert)",
    hint: "Local HTTPS with mkcert for development domains.",
  },
  {
    value: "none",
    label: "VPS IP only",
    hint: "No TLS — use http://YOUR_IP:3100 for the controller.",
  },
];

export function SettingsPage() {
  const { user, token } = useAuth();
  const [deployment, setDeployment] = useState<DeploymentConfig | null>(null);
  const [dnsHints, setDnsHints] = useState<DNSHints | null>(null);
  const [devSetup, setDevSetup] = useState<DevSetup | null>(null);
  const [isLoading, setIsLoading] = useState(true);
  const [isSaving, setIsSaving] = useState(false);
  const [isRotating, setIsRotating] = useState(false);
  const [deploymentError, setDeploymentError] = useState<string | null>(null);
  const [dnsHintsError, setDnsHintsError] = useState<string | null>(null);
  const [devSetupError, setDevSetupError] = useState<string | null>(null);
  const [cloudflareDialogOpen, setCloudflareDialogOpen] = useState(false);

  const [publicDomain, setPublicDomain] = useState("");
  const [controllerURL, setControllerURL] = useState("");
  const [sslMode, setSslMode] = useState<SSLMode>("none");

  const loadAll = useCallback(async () => {
    setIsLoading(true);
    setDeploymentError(null);
    setDnsHintsError(null);
    setDevSetupError(null);

    const [deployResult, dnsResult, devResult] = await Promise.allSettled([
      fetchDeployment(token),
      fetchDNSHints(token),
      fetchDevSetup(token),
    ]);

    if (deployResult.status === "fulfilled") {
      const deploy = deployResult.value;
      setDeployment(deploy);
      setPublicDomain(deploy.public_domain ?? "");
      setControllerURL(deploy.controller_public_url ?? "");
      setSslMode(deploy.ssl_mode ?? "none");
    } else {
      const err = deployResult.reason;
      setDeployment(null);
      setDeploymentError(
        err instanceof ApiError ? err.message : "Unable to load deployment settings.",
      );
    }

    if (dnsResult.status === "fulfilled") {
      setDnsHints(dnsResult.value);
    } else {
      setDnsHints(null);
      const err = dnsResult.reason;
      setDnsHintsError(
        err instanceof ApiError ? err.message : "Unable to load DNS hints.",
      );
    }

    if (devResult.status === "fulfilled") {
      setDevSetup(devResult.value);
    } else {
      setDevSetup(null);
      const err = devResult.reason;
      setDevSetupError(
        err instanceof ApiError ? err.message : "Unable to load dev setup.",
      );
    }

    setIsLoading(false);
  }, [token]);

  useEffect(() => {
    void loadAll();
  }, [loadAll]);

  async function handleSaveDeployment() {
    setIsSaving(true);
    const cleanedDomain = sanitizeDomain(publicDomain);
    const cleanedControllerURL = sanitizeControllerURL(controllerURL);
    try {
      const updated = await updateDeployment(token, {
        public_domain: cleanedDomain,
        controller_public_url: cleanedControllerURL,
        ssl_mode: sslMode,
      });
      setDeployment(updated);
      setPublicDomain(updated.public_domain ?? cleanedDomain);
      setControllerURL(updated.controller_public_url ?? cleanedControllerURL);
      const [dns, dev] = await Promise.all([
        fetchDNSHints(token),
        fetchDevSetup(token),
      ]);
      setDnsHints(dns);
      setDevSetup(dev);
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

  async function handleSSLProvisioned() {
    const [dns, dev, deploy] = await Promise.all([
      fetchDNSHints(token),
      fetchDevSetup(token),
      fetchDeployment(token),
    ]);
    setDnsHints(dns);
    setDevSetup(dev);
    setDeployment(deploy);
  }

  const showDnsHints =
    sslMode !== "caddy" && dnsHints != null && (dnsHints.records?.length ?? 0) > 0;
  const showDevSetup = devSetup != null;
  const cleanedDomain = sanitizeDomain(publicDomain);

  return (
    <div className="w-full space-y-6">
      {isLoading && (
        <div className="flex flex-col gap-6 w-full">
          <CardSkeleton lines={3} />
          <CardSkeleton lines={2} />
          <CardSkeleton lines={12} />
          <CardSkeleton lines={8} />
          <CardSkeleton lines={6} />
        </div>
      )}

      {!isLoading && (
        <>
          {!publicDomain.trim() && (
            <div className="flex items-start gap-3 rounded-lg border border-primary/30 bg-primary/5 p-4">
              <Info className="mt-0.5 h-5 w-5 shrink-0 text-primary" aria-hidden="true" />
              <div className="space-y-1 text-sm">
                <p className="font-medium">Welcome — set up your deployment</p>
                <p className="text-muted-foreground">
                  Enter your domain below and click <strong>Save</strong>. Then go to{" "}
                  <Link to="/deploy" className="text-primary underline-offset-2 hover:underline">
                    Deploy Agent
                  </Link>{" "}
                  for your install command.
                </p>
              </div>
            </div>
          )}

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
                  <Globe className="h-5 w-5 text-primary" aria-hidden="true" />
                  Deployment
                </CardTitle>
                <CardDescription>
                  Tell agents how to reach your network. Save once — we generate the install
                  command for you.
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

                <div className="grid grid-cols-1 md:grid-cols-3 gap-4">
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
                    <Label htmlFor="controller-url">Controller address (optional)</Label>
                    <Input
                      id="controller-url"
                      placeholder="http://api.trinityproxy.local:3100"
                      value={controllerURL}
                      onChange={(e) => setControllerURL(sanitizeControllerURL(e.target.value))}
                      sanitize="text"
                    />
                    <p className="text-xs text-muted-foreground">
                      Auto-derived from your domain when left blank (e.g. http://api.yourdomain.com:3100).
                      Override only for custom setups.
                    </p>
                  </div>

                  <div className="space-y-2">
                    <Label htmlFor="ssl-mode">SSL mode</Label>
                    <select
                      id="ssl-mode"
                      className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                      value={sslMode}
                      onChange={(e) => setSslMode(e.target.value as SSLMode)}
                    >
                      {SSL_MODES.map(({ value, label }) => (
                        <option key={value} value={value}>
                          {label}
                        </option>
                      ))}
                    </select>
                    <p className="text-xs text-muted-foreground">
                      {SSL_MODES.find((m) => m.value === sslMode)?.hint}
                    </p>
                  </div>
                </div>

                {sslMode === "caddy" && (
                  <div className="flex flex-wrap items-center gap-3 rounded-md border border-border bg-muted/30 p-4">
                    <Cloud className="h-5 w-5 shrink-0 text-primary" aria-hidden="true" />
                    <div className="min-w-0 flex-1 space-y-1">
                      <p className="text-sm font-medium">Cloudflare wildcard SSL</p>
                      <p className="text-sm text-muted-foreground">
                        Issue a wildcard certificate for{" "}
                        <span className="font-mono text-foreground">
                          *.{cleanedDomain || "yourdomain.com"}
                        </span>{" "}
                        with proxied DNS records (orange cloud).
                      </p>
                    </div>
                    <Button
                      variant="outline"
                      onClick={() => setCloudflareDialogOpen(true)}
                      disabled={!cleanedDomain}
                    >
                      Set up Cloudflare SSL
                    </Button>
                  </div>
                )}

                <div className="flex flex-wrap gap-2">
                  <Button onClick={() => void handleSaveDeployment()} disabled={isSaving}>
                    {isSaving ? "Saving…" : "Save"}
                  </Button>
                  <Button
                    variant="outline"
                    onClick={() => void handleRotateKey()}
                    disabled={isRotating}
                  >
                    <RefreshCw className="h-4 w-4" />
                    {isRotating ? "Generating…" : "Generate new key"}
                  </Button>
                </div>

                {deployment?.has_agent_key && (
                  <p className="flex items-center gap-2 text-xs text-muted-foreground">
                    <Shield className="h-3.5 w-3.5" />
                    You&apos;re all set — copy the install command on{" "}
                    <Link to="/deploy" className="text-primary underline-offset-2 hover:underline">
                      Deploy Agent
                    </Link>
                    .
                  </p>
                )}
              </CardContent>
            </Card>

            {devSetupError && (
              <ErrorState
                message={devSetupError}
                onRetry={() => void loadAll()}
                className="h-full py-6"
              />
            )}

            {showDevSetup && devSetup && (
              <Card className="w-full">
                <CardHeader className="border-b border-border/40 pb-4">
                  <CardTitle className="flex items-center gap-2 text-lg">
                    <Terminal className="h-5 w-5 text-primary" aria-hidden="true" />
                    Dev setup
                  </CardTitle>
                  <CardDescription>{devSetup.controller_note}</CardDescription>
                </CardHeader>
                <CardContent className="space-y-6 p-6 text-sm">
                  <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
                    <div className="rounded-md border border-border bg-background p-4 shadow-sm">
                      <p className="flex items-center gap-1.5 font-medium text-foreground mb-2">
                        Suggested controller URL
                      </p>
                      <code className="block break-all rounded bg-muted px-3 py-2 text-xs font-mono">
                        {devSetup.suggested_controller_url}
                      </code>
                    </div>
                    <div className="rounded-md border border-border bg-background p-4 shadow-sm">
                      <p className="flex items-center gap-1.5 font-medium text-foreground mb-2">
                        /etc/hosts entry
                      </p>
                      <code className="block break-all rounded bg-muted px-3 py-2 text-xs font-mono">
                        {devSetup.hosts_file_entry}
                      </code>
                    </div>
                  </div>
                  <div className="rounded-md border border-border bg-background p-4 shadow-sm">
                    <p className="flex items-center gap-1.5 font-medium text-foreground mb-2">
                      mkcert (dev-mkcert mode)
                    </p>
                    <pre className="overflow-x-auto rounded bg-zinc-950 px-4 py-3 text-xs leading-relaxed text-zinc-50 shadow-inner">
                      <code>{devSetup.mkcert_instructions}</code>
                    </pre>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>

          {dnsHintsError && (
            <ErrorState
              message={dnsHintsError}
              onRetry={() => void loadAll()}
              className="py-6"
            />
          )}

          {showDnsHints && dnsHints && (
            <Card className="w-full">
              <CardHeader className="border-b border-border/40 pb-4">
                <CardTitle className="flex items-center gap-2 text-lg">
                  <Server className="h-5 w-5 text-primary" aria-hidden="true" />
                  DNS checklist
                </CardTitle>
                <CardDescription>{dnsHints.summary}</CardDescription>
              </CardHeader>
              <CardContent className="p-6">
                <div className="grid gap-3">
                  {(dnsHints.records ?? []).map((record) => (
                    <div
                      key={`${record.type}-${record.name}`}
                      className="rounded-md border border-border bg-background p-4 shadow-sm"
                    >
                      <div className="flex items-center gap-2 font-mono text-xs">
                        <span className="font-semibold text-primary">{record.type}</span>
                        <span className="text-muted-foreground">{record.name}</span>
                        <span className="text-muted-foreground">→</span>
                        <span className="font-medium">{record.value || "YOUR_VPS_IP"}</span>
                      </div>
                      {record.notes && (
                        <p className="mt-2 text-xs text-muted-foreground">{record.notes}</p>
                      )}
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}

          <CloudflareSSLDialog
            open={cloudflareDialogOpen}
            onClose={() => setCloudflareDialogOpen(false)}
            token={token}
            domain={cleanedDomain}
            controllerURL={sanitizeControllerURL(controllerURL)}
            onProvisioned={() => void handleSSLProvisioned()}
          />
        </>
      )}
    </div>
  );
}
