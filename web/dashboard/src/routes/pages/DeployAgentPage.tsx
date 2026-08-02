import { useCallback, useEffect, useRef, useState } from "react";
import { Link } from "react-router-dom";
import {
  Box,
  Check,
  Command,
  Copy,
  Globe,
  Info,
  Monitor,
  Rocket,
  Server,
  Settings,
  Terminal,
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
import { Label } from "@/components/ui/Label";
import { CardSkeleton } from "@/components/ui/Skeleton";
import { useAuth } from "@/context/AuthContext";
import {
  ApiError,
  fetchDeployCommands,
  type DeployCommandsResponse,
  type DeployLogLevel,
  type DeployPlatform,
} from "@/lib/api";
import { toast } from "@/lib/toast";
import { cn } from "@/lib/utils";

type DeployEnvironment = "prod" | "dev";

const ENV_PLATFORMS: Record<DeployEnvironment, string[]> = {
  prod: ["linux-vps", "windows"],
  dev: ["macos", "docker", "mac-dev"],
};

const DEFAULT_PLATFORM: Record<DeployEnvironment, string> = {
  prod: "linux-vps",
  dev: "macos",
};

const LOG_LEVEL_OPTIONS: {
  id: DeployLogLevel;
  label: string;
  description: string;
}[] = [
  {
    id: "quiet",
    label: "Quiet",
    description: "Total silent — no installer output",
  },
  {
    id: "silent",
    label: "Silent",
    description: "Errors only, minimal output",
  },
  {
    id: "info",
    label: "Info",
    description: "Normal progress messages (default)",
  },
  {
    id: "debug",
    label: "Debug",
    description: "Verbose output for troubleshooting",
  },
];

const LOG_LEVEL_PLATFORMS = new Set(["linux-vps", "macos", "windows"]);

function supportsLogLevel(platformId: string): boolean {
  return LOG_LEVEL_PLATFORMS.has(platformId);
}

function platformEnvironment(id: string): DeployEnvironment {
  return ENV_PLATFORMS.prod.includes(id) ? "prod" : "dev";
}

function platformsForEnv(
  platforms: DeployPlatform[],
  env: DeployEnvironment,
): DeployPlatform[] {
  const ids = ENV_PLATFORMS[env];
  return platforms.filter((p) => ids.includes(p.id));
}

function PlatformIcon({ id, className }: { id: string; className?: string }) {
  switch (id) {
    case "linux-vps":
      return <Server className={className} />;
    case "windows":
      return <Monitor className={className} />;
    case "macos":
    case "mac-dev":
      return <Command className={className} />;
    case "docker":
      return <Box className={className} />;
    default:
      return <Terminal className={className} />;
  }
}

export function DeployAgentPage() {
  const { token } = useAuth();
  const [data, setData] = useState<DeployCommandsResponse | null>(null);
  const [selectedEnv, setSelectedEnv] = useState<DeployEnvironment>("prod");
  const [selectedId, setSelectedId] = useState<string>("linux-vps");
  const [logLevel, setLogLevel] = useState<DeployLogLevel>("info");
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const isFirstLoad = useRef(true);

  const loadCommands = useCallback(async () => {
    if (isFirstLoad.current) {
      setIsLoading(true);
    }
    setError(null);
    try {
      const response = await fetchDeployCommands(token, logLevel);
      setData(response);
      setSelectedId((current) => {
        const id = response.platforms.some((p) => p.id === current)
          ? current
          : (response.platforms[0]?.id ?? "linux-vps");
        setSelectedEnv(platformEnvironment(id));
        return id;
      });
    } catch (err) {
      const message =
        err instanceof ApiError
          ? err.message
          : "Unable to load deploy commands.";
      setError(message);
      setData(null);
    } finally {
      setIsLoading(false);
      isFirstLoad.current = false;
    }
  }, [token, logLevel]);

  useEffect(() => {
    void loadCommands();
  }, [loadCommands]);

  const envPlatforms = data ? platformsForEnv(data.platforms, selectedEnv) : [];
  const selectedPlatform =
    envPlatforms.find((p) => p.id === selectedId) ?? envPlatforms[0];
  const showLogLevel = selectedPlatform && supportsLogLevel(selectedPlatform.id);
  const displayCommand = selectedPlatform?.command ?? "";

  function handlePlatformChange(id: string) {
    setSelectedId(id);
  }

  function handleEnvChange(env: DeployEnvironment) {
    setSelectedEnv(env);
    if (!data) return;
    const available = platformsForEnv(data.platforms, env);
    const next =
      available.find((p) => p.id === selectedId)?.id ??
      available[0]?.id ??
      DEFAULT_PLATFORM[env];
    setSelectedId(next);
  }

  async function handleCopy(command: string) {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      toast.success("Command copied to clipboard");
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error("Unable to copy — select the command manually");
    }
  }

  return (
    <div className="w-full space-y-6">
      {isLoading && (
        <div className="flex flex-col gap-6 w-full">
          <CardSkeleton lines={4} />
          <CardSkeleton lines={6} />
        </div>
      )}

      {!isLoading && error && (
        <ErrorState message={error} onRetry={() => void loadCommands()} />
      )}

      {!isLoading && !error && data && (
        <>
          {!data.has_agent_key && (
            <div className="flex items-start gap-3 rounded-lg border border-amber-500/40 bg-amber-500/10 p-4">
              <Info
                className="mt-0.5 h-5 w-5 shrink-0 text-amber-600 dark:text-amber-400"
                aria-hidden="true"
              />
              <div className="space-y-2 text-sm">
                <p className="font-medium text-foreground">
                  Almost there — save your domain first
                </p>
                <p className="text-muted-foreground">
                  Go to{" "}
                  <Link
                    to="/settings"
                    className="text-primary underline-offset-2 hover:underline"
                  >
                    Settings
                  </Link>
                  , enter your domain, and click <strong>Save</strong>. Install
                  commands will include your agent key automatically.
                </p>
                <Link to="/settings" className="inline-block mt-2">
                  <Button variant="outline" size="sm">
                    <Settings className="h-4 w-4" />
                    Open Settings
                  </Button>
                </Link>
              </div>
            </div>
          )}

          <div className="flex flex-col gap-6 w-full">
            <Card className="w-full">
              <CardHeader className="pb-4 border-b border-border/40">
                <CardTitle className="flex items-center gap-2 text-lg">
                  <Rocket className="h-5 w-5 text-primary" aria-hidden="true" />
                  Select Platform
                </CardTitle>
                <CardDescription>
                  Pick a target to get the install command.
                </CardDescription>
              </CardHeader>
              <CardContent className="p-6 space-y-6">
                <div className="space-y-3">
                  <Label className="text-muted-foreground uppercase text-xs font-semibold tracking-wider">
                    Environment
                  </Label>
                  <div className="flex flex-col sm:flex-row gap-3 w-full">
                    <Button
                      variant={selectedEnv === "prod" ? "primary" : "outline"}
                      onClick={() => handleEnvChange("prod")}
                      className={cn(
                        "flex-1 h-12 w-full transition-all duration-200",
                        selectedEnv !== "prod" && "text-muted-foreground hover:border-primary/50",
                      )}
                    >
                      <Globe className="h-4 w-4" />
                      Production
                    </Button>
                    <Button
                      variant={selectedEnv === "dev" ? "primary" : "outline"}
                      onClick={() => handleEnvChange("dev")}
                      className={cn(
                        "flex-1 h-12 w-full transition-all duration-200",
                        selectedEnv !== "dev" && "text-muted-foreground hover:border-primary/50",
                      )}
                    >
                      <Terminal className="h-4 w-4" />
                      Local Development
                    </Button>
                  </div>
                </div>

                <div className="space-y-3">
                  <Label className="text-muted-foreground uppercase text-xs font-semibold tracking-wider">
                    Target OS / Runtime
                  </Label>
                  <div className="flex flex-col sm:flex-row flex-wrap gap-3 w-full">
                    {envPlatforms.map((platform) => (
                      <Button
                        key={platform.id}
                        variant={
                          selectedPlatform?.id === platform.id
                            ? "primary"
                            : "outline"
                        }
                        onClick={() => handlePlatformChange(platform.id)}
                        className={cn(
                          "flex-1 h-12 w-full sm:w-auto transition-all duration-200",
                          selectedPlatform?.id !== platform.id &&
                            "text-muted-foreground hover:border-primary/50",
                        )}
                      >
                        <PlatformIcon
                          id={platform.id}
                          className="h-4 w-4 opacity-80"
                        />
                        {platform.label}
                      </Button>
                    ))}
                  </div>
                </div>
              </CardContent>
            </Card>

            {selectedPlatform && (
              <Card className="w-full border-primary/20 shadow-sm">
                <CardHeader className="bg-muted/10 border-b border-border/40 pb-4">
                  <div className="flex items-center gap-3">
                    <div className="rounded-md bg-primary/10 p-2 text-primary">
                      <PlatformIcon
                        id={selectedPlatform.id}
                        className="h-5 w-5"
                      />
                    </div>
                    <div>
                      <CardTitle className="text-lg">
                        {selectedPlatform.label}
                      </CardTitle>
                      {selectedPlatform.description && (
                        <CardDescription className="mt-1">
                          {selectedPlatform.description}
                        </CardDescription>
                      )}
                      {selectedEnv === "dev" && (
                        <p className="mt-1 font-mono text-xs text-muted-foreground break-all">
                          {selectedPlatform.controller_url}
                        </p>
                      )}
                    </div>
                  </div>
                </CardHeader>
                <CardContent className="space-y-6 p-6">
                  {showLogLevel && (
                    <div className="space-y-3">
                      <Label className="text-muted-foreground uppercase text-xs font-semibold tracking-wider">
                        Install Log Level
                      </Label>
                      <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 w-full">
                        {LOG_LEVEL_OPTIONS.map((option) => (
                          <Button
                            key={option.id}
                            variant={
                              logLevel === option.id ? "primary" : "outline"
                            }
                            onClick={() => setLogLevel(option.id)}
                            className={cn(
                              "h-auto min-h-10 flex-col items-start gap-0.5 px-3 py-2 text-left transition-all duration-200",
                              logLevel !== option.id &&
                                "text-muted-foreground hover:border-primary/50",
                            )}
                          >
                            <span className="font-medium">{option.label}</span>
                            <span className="text-xs font-normal opacity-80">
                              {option.description}
                            </span>
                          </Button>
                        ))}
                      </div>
                    </div>
                  )}

                  <div className="space-y-3">
                    <div className="flex items-center justify-between gap-2">
                      <Label className="text-sm font-semibold">Install command</Label>
                      <Button
                        variant="secondary"
                        size="sm"
                        onClick={() => void handleCopy(displayCommand)}
                        className="h-8"
                        disabled={!displayCommand}
                      >
                        {copied ? (
                          <>
                            <Check className="h-3.5 w-3.5" />
                            Copied
                          </>
                        ) : (
                          <>
                            <Copy className="h-3.5 w-3.5" />
                            Copy
                          </>
                        )}
                      </Button>
                    </div>
                    <div className="group relative">
                      <pre className="overflow-x-auto rounded-lg border border-border bg-zinc-950 p-4 text-xs leading-relaxed text-zinc-50 shadow-inner">
                        <code>{displayCommand}</code>
                      </pre>
                    </div>
                  </div>
                </CardContent>
              </Card>
            )}
          </div>
        </>
      )}
    </div>
  );
}
