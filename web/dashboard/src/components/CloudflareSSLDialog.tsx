import { useCallback, useEffect, useMemo, useState } from "react";
import { Cloud, ExternalLink, Loader2 } from "lucide-react";
import { ErrorState } from "@/components/ErrorState";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { Input } from "@/components/ui/Input";
import { Label } from "@/components/ui/Label";
import { Skeleton } from "@/components/ui/Skeleton";
import {
  ApiError,
  fetchCloudflareSetup,
  provisionSSL,
  updateDeployment,
  type CloudflareSetup,
  type CloudflareSetupStep,
  type DNSRecord,
} from "@/lib/api";
import { toast } from "@/lib/toast";

const CLOUDFLARE_TOKEN_URL = "https://dash.cloudflare.com/profile/api-tokens";

const DEFAULT_TOKEN_STEPS: CloudflareSetupStep[] = [
  {
    title: "Open Cloudflare API tokens",
    description: "Go to your Cloudflare profile → API Tokens → Create Token.",
  },
  {
    title: "Use the Edit zone DNS template",
    description:
      'Choose "Edit zone DNS" or create a custom token with Zone → DNS → Edit for your zone.',
  },
  {
    title: "Restrict to your zone",
    description:
      "Under Zone Resources, select Include → Specific zone → your domain. This limits the token to DNS changes only.",
  },
  {
    title: "Copy the token",
    description:
      "Create the token and copy it immediately — Cloudflare only shows it once. Paste it below to provision SSL.",
  },
];

function apiHostForDomain(domain: string): string {
  const trimmed = domain.trim();
  return trimmed ? `api.${trimmed}` : "api.yourdomain.com";
}

function buildFallbackSetup(domain: string): CloudflareSetup {
  const trimmed = domain.trim();
  const apiHost = apiHostForDomain(trimmed);

  if (!trimmed) {
    return {
      domain: "",
      api_host: apiHost,
      server_ip: "",
      token_url: CLOUDFLARE_TOKEN_URL,
      token_steps: DEFAULT_TOKEN_STEPS,
      dns_records: [],
      summary:
        "Enter a public domain in Settings before provisioning Cloudflare wildcard SSL.",
      renewal_note: "",
    };
  }

  return {
    domain: trimmed,
    api_host: apiHost,
    server_ip: "",
    token_url: CLOUDFLARE_TOKEN_URL,
    token_steps: DEFAULT_TOKEN_STEPS,
    dns_records: [
      {
        type: "A",
        name: apiHost,
        value: "",
        notes: "Required — controller API. Proxy status: Proxied (orange cloud).",
      },
      {
        type: "A",
        name: trimmed,
        value: "",
        notes: "Required — dashboard at apex domain. Proxy status: Proxied (orange cloud).",
      },
    ],
    summary: `Create proxied A records for ${apiHost} and ${trimmed} pointing to your VPS IP before provisioning. Caddy uses Cloudflare DNS-01 to issue a wildcard certificate for *.${trimmed} and ${trimmed}.`,
    renewal_note: "",
  };
}

interface CloudflareSSLDialogProps {
  open: boolean;
  onClose: () => void;
  token: string | null;
  domain: string;
  controllerURL: string;
  onProvisioned: () => void;
}

export function CloudflareSSLDialog({
  open,
  onClose,
  token,
  domain,
  controllerURL,
  onProvisioned,
}: CloudflareSSLDialogProps) {
  const [setup, setSetup] = useState<CloudflareSetup | null>(null);
  const [isLoadingSetup, setIsLoadingSetup] = useState(false);
  const [setupError, setSetupError] = useState<string | null>(null);
  const [apiToken, setApiToken] = useState("");
  const [isProvisioning, setIsProvisioning] = useState(false);
  const [provisionError, setProvisionError] = useState<string | null>(null);
  const [provisionSuccess, setProvisionSuccess] = useState(false);

  const hasDomain = domain.trim().length > 0;

  const loadSetup = useCallback(async () => {
    if (!hasDomain) {
      setSetup(null);
      setSetupError(null);
      setIsLoadingSetup(false);
      return;
    }

    setIsLoadingSetup(true);
    setSetupError(null);
    try {
      const data = await fetchCloudflareSetup(token, { domain, ssl_mode: "caddy" });
      setSetup(data);
    } catch (err) {
      setSetup(null);
      setSetupError(
        err instanceof ApiError ? err.message : "Unable to load Cloudflare setup.",
      );
    } finally {
      setIsLoadingSetup(false);
    }
  }, [domain, hasDomain, token]);

  useEffect(() => {
    if (!open) {
      setApiToken("");
      setProvisionError(null);
      setProvisionSuccess(false);
      setSetup(null);
      setSetupError(null);
      return;
    }
    void loadSetup();
  }, [open, loadSetup]);

  const displaySetup = useMemo(
    () => setup ?? buildFallbackSetup(domain),
    [domain, setup],
  );

  const tokenSteps =
    displaySetup.token_steps?.length > 0
      ? displaySetup.token_steps
      : DEFAULT_TOKEN_STEPS;
  const dnsRecords: DNSRecord[] =
    displaySetup.dns_records?.length > 0
      ? displaySetup.dns_records
      : buildFallbackSetup(domain).dns_records;
  const tokenUrl = displaySetup.token_url || CLOUDFLARE_TOKEN_URL;

  function handleClose() {
    setApiToken("");
    setProvisionError(null);
    onClose();
  }

  async function handleProvision() {
    if (!hasDomain) {
      toast.error("Enter a domain first");
      return;
    }
    if (!apiToken.trim()) {
      toast.error("Enter your Cloudflare API token");
      return;
    }

    setIsProvisioning(true);
    setProvisionError(null);
    setProvisionSuccess(false);

    try {
      await updateDeployment(token, {
        public_domain: domain,
        controller_public_url: controllerURL,
        ssl_mode: "caddy",
      });

      await provisionSSL(token, domain, apiToken.trim());
      setProvisionSuccess(true);
      setApiToken("");
      toast.success("Wildcard SSL provisioned successfully");
      onProvisioned();
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Failed to provision SSL";
      setProvisionError(message);
    } finally {
      setIsProvisioning(false);
    }
  }

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      title="Set up Cloudflare SSL"
      description="Wildcard certificate via DNS-01 with proxied (orange cloud) DNS records."
      className="max-w-xl"
      footer={
        <div className="flex flex-wrap items-center justify-end gap-2">
          <Button variant="outline" onClick={handleClose} disabled={isProvisioning}>
            {provisionSuccess ? "Close" : "Cancel"}
          </Button>
          <Button
            onClick={() => void handleProvision()}
            disabled={isProvisioning || !hasDomain || !apiToken.trim()}
          >
            {isProvisioning ? (
              <>
                <Loader2 className="h-4 w-4 animate-spin" />
                Provisioning (30–60s)…
              </>
            ) : (
              "Provision SSL"
            )}
          </Button>
        </div>
      }
    >
      <div className="space-y-6 text-sm">
        {!hasDomain && (
          <div className="rounded-md border border-primary/30 bg-primary/10 px-3 py-2 text-primary">
            Enter your domain in Settings and click Save before setting up Cloudflare SSL.
          </div>
        )}

        {hasDomain && setupError && !isLoadingSetup && (
          <ErrorState
            message={setupError}
            onRetry={() => void loadSetup()}
            className="py-6"
          />
        )}

        {hasDomain && (
          <>
            <div className="space-y-3">
              <div className="flex items-start gap-2">
                <Cloud className="mt-0.5 h-4 w-4 shrink-0 text-primary" aria-hidden="true" />
                {isLoadingSetup ? (
                  <div className="flex-1 space-y-2" aria-busy="true" aria-label="Loading setup summary">
                    <Skeleton className="h-4 w-full" />
                    <Skeleton className="h-4 w-4/5" />
                  </div>
                ) : (
                  <p className="text-muted-foreground">{displaySetup.summary}</p>
                )}
              </div>
            </div>

            <div className="space-y-3">
              <p className="font-medium text-foreground">1. Create a Cloudflare API token</p>
              <p className="text-muted-foreground">
                Required permission: <span className="font-medium text-foreground">Zone → DNS → Edit</span>{" "}
                for your zone.
              </p>
              <ol className="list-decimal space-y-2 pl-5 text-muted-foreground">
                {tokenSteps.map((step) => (
                  <li key={step.title}>
                    <span className="font-medium text-foreground">{step.title}</span>
                    {" — "}
                    {step.description}
                  </li>
                ))}
              </ol>
              <a
                href={tokenUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-1.5 text-primary underline-offset-2 hover:underline"
              >
                Open Cloudflare API tokens
                <ExternalLink className="h-3.5 w-3.5" aria-hidden="true" />
              </a>
            </div>

            <div className="space-y-2">
              <Label htmlFor="cloudflare-api-token">Cloudflare API token</Label>
              <Input
                id="cloudflare-api-token"
                type="password"
                autoComplete="off"
                placeholder="Paste token (shown once by Cloudflare)"
                value={apiToken}
                onChange={(e) => setApiToken(e.target.value)}
                disabled={isProvisioning}
              />
              <p className="text-xs text-muted-foreground">
                Sent only to the provision endpoint and never logged. On success it is written to
                the server for Caddy renewal (not stored in the dashboard database).
              </p>
            </div>

            <div className="space-y-3">
              <p className="font-medium text-foreground">2. DNS records (proxied / orange cloud)</p>
              <p className="text-muted-foreground">
                Add proxied A records in Cloudflare pointing to your VPS IP. Enable the orange cloud
                icon for both records.
              </p>
              {isLoadingSetup ? (
                <div className="grid gap-2" aria-busy="true" aria-label="Loading DNS records">
                  <Skeleton className="h-16 w-full" />
                  <Skeleton className="h-16 w-full" />
                </div>
              ) : (
                <div className="grid gap-2">
                  {dnsRecords.map((record) => (
                    <div
                      key={`${record.type}-${record.name}`}
                      className="rounded-md border border-border bg-muted/30 p-3"
                    >
                      <div className="flex flex-wrap items-center gap-2 font-mono text-xs">
                        <span className="rounded bg-primary/10 px-1.5 py-0.5 font-semibold text-primary">
                          {record.type}
                        </span>
                        <span className="font-medium text-foreground">{record.name}</span>
                        <span className="text-muted-foreground">→</span>
                        <span>{record.value || "YOUR_VPS_IP"}</span>
                        <span className="rounded bg-orange-500/15 px-1.5 py-0.5 text-orange-600 dark:text-orange-400">
                          Proxied
                        </span>
                      </div>
                      {record.notes && (
                        <p className="mt-1.5 text-xs text-muted-foreground">{record.notes}</p>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {provisionError && (
              <pre className="max-h-40 overflow-auto rounded-md border border-destructive/30 bg-destructive/10 p-3 text-xs text-destructive whitespace-pre-wrap">
                {provisionError}
              </pre>
            )}

            {provisionSuccess && (
              <div className="space-y-2">
                <p className="rounded-md border border-primary/30 bg-primary/10 px-3 py-2 text-primary">
                  Wildcard certificate issued for *.{displaySetup.domain} and {displaySetup.domain}.
                  Controller:{" "}
                  <span className="font-mono">https://{displaySetup.api_host}</span>
                </p>
                {displaySetup.renewal_note && (
                  <p className="text-xs text-muted-foreground">{displaySetup.renewal_note}</p>
                )}
              </div>
            )}
          </>
        )}
      </div>
    </Dialog>
  );
}
