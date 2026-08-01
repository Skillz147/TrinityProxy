import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, Copy, Eye, RefreshCw } from "lucide-react";
import { Button } from "@/components/ui/Button";
import { Dialog } from "@/components/ui/Dialog";
import { Skeleton } from "@/components/ui/Skeleton";
import {
  ApiError,
  fetchNodeCredentials,
  type NodeCredentials,
  type ProxyNode,
} from "@/lib/api";
import { toast } from "@/lib/toast";

interface SocksCredentialsDialogProps {
  node: ProxyNode | null;
  token: string | null;
  onClose: () => void;
}

async function copyText(value: string, successMessage: string) {
  try {
    await navigator.clipboard.writeText(value);
    toast.success(successMessage);
  } catch {
    toast.error("Unable to copy — check browser permissions");
  }
}

function formatAllCredentials(creds: NodeCredentials): string {
  return [
    `Host: ${creds.ip}`,
    `Port: ${creds.port}`,
    `Username: ${creds.username}`,
    `Password: ${creds.password}`,
    `Connection string: ${creds.connection_string}`,
  ].join("\n");
}

function CredentialField({
  label,
  value,
  mono = true,
}: {
  label: string;
  value: string;
  mono?: boolean;
}) {
  return (
    <div className="space-y-1.5">
      <dt className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        {label}
      </dt>
      <dd className="flex items-center gap-2">
        <span
          className={`min-w-0 flex-1 break-all rounded-md border border-border bg-muted/50 px-3 py-2 text-sm ${
            mono ? "font-mono" : ""
          }`}
        >
          {value}
        </span>
        <Button
          variant="outline"
          size="icon"
          aria-label={`Copy ${label.toLowerCase()}`}
          onClick={() => void copyText(value, `${label} copied`)}
        >
          <Copy className="h-4 w-4" />
        </Button>
      </dd>
    </div>
  );
}

export function SocksCredentialsDialog({
  node,
  token,
  onClose,
}: SocksCredentialsDialogProps) {
  const [credentials, setCredentials] = useState<NodeCredentials | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadCredentials = useCallback(async () => {
    if (!node) return;

    setIsLoading(true);
    setError(null);

    try {
      const creds = await fetchNodeCredentials(token, node.id);
      setCredentials(creds);
    } catch (err) {
      const message =
        err instanceof ApiError ? err.message : "Unable to load credentials.";
      setError(message);
      setCredentials(null);
    } finally {
      setIsLoading(false);
    }
  }, [node, token]);

  useEffect(() => {
    if (!node) {
      setCredentials(null);
      setError(null);
      setIsLoading(false);
      return;
    }

    void loadCredentials();
  }, [node, loadCredentials]);

  const handleCopyAll = () => {
    if (!credentials) return;
    void copyText(formatAllCredentials(credentials), "All credentials copied");
  };

  return (
    <Dialog
      open={node !== null}
      onClose={onClose}
      title="SOCKS credentials"
      description={
        node ? `${node.ip}:${node.port} · ${node.username}` : undefined
      }
      footer={
        credentials ? (
          <div className="flex justify-end">
            <Button variant="outline" leftIcon={<Copy className="h-4 w-4" />} onClick={handleCopyAll}>
              Copy all
            </Button>
          </div>
        ) : undefined
      }
    >
      {isLoading && (
        <div className="space-y-4" aria-busy="true" aria-label="Loading credentials">
          {Array.from({ length: 5 }).map((_, index) => (
            <div key={`field-${index}`} className="space-y-2">
              <Skeleton className="h-3 w-24" />
              <Skeleton className="h-10 w-full" />
            </div>
          ))}
        </div>
      )}

      {!isLoading && error && (
        <div
          className="flex flex-col items-center rounded-lg border border-destructive/30 bg-destructive/5 px-4 py-8 text-center"
          role="alert"
        >
          <AlertTriangle className="mb-3 h-8 w-8 text-destructive" aria-hidden="true" />
          <p className="text-sm text-foreground">{error}</p>
          <Button
            variant="outline"
            className="mt-4"
            leftIcon={<RefreshCw className="h-4 w-4" />}
            onClick={() => void loadCredentials()}
          >
            Try again
          </Button>
        </div>
      )}

      {!isLoading && !error && credentials && (
        <dl className="space-y-4">
          <CredentialField label="Host / IP" value={credentials.ip} />
          <CredentialField label="Port" value={String(credentials.port)} />
          <CredentialField label="Username" value={credentials.username} />
          <CredentialField label="Password" value={credentials.password} />
          <div className="border-t border-border pt-4">
            <CredentialField label="Connection string" value={credentials.connection_string} />
          </div>
        </dl>
      )}

      {!isLoading && !error && !credentials && node && (
        <div className="flex flex-col items-center py-8 text-center text-sm text-muted-foreground">
          <Eye className="mb-2 h-8 w-8 opacity-50" aria-hidden="true" />
          No credentials available.
        </div>
      )}
    </Dialog>
  );
}
