import { type HTMLAttributes } from "react";
import { cn } from "@/lib/utils";

export interface SkeletonProps extends HTMLAttributes<HTMLDivElement> {
  /** When true, disables the shimmer animation. */
  static?: boolean;
}

export function Skeleton({ className, static: isStatic, ...props }: SkeletonProps) {
  return (
    <div
      className={cn(
        "relative overflow-hidden rounded-md bg-muted",
        !isStatic && "before:absolute before:inset-0 before:-translate-x-full before:animate-shimmer before:bg-gradient-to-r before:from-transparent before:via-foreground/5 before:to-transparent",
        className,
      )}
      aria-hidden="true"
      {...props}
    />
  );
}

export interface TableSkeletonProps {
  rows?: number;
  columns?: number;
  className?: string;
}

export function TableSkeleton({ rows = 5, columns = 4, className }: TableSkeletonProps) {
  return (
    <div className={cn("w-full space-y-3", className)} aria-busy="true" aria-label="Loading table">
      <div className="flex gap-4">
        {Array.from({ length: columns }).map((_, i) => (
          <Skeleton key={`header-${i}`} className="h-4 flex-1" />
        ))}
      </div>
      {Array.from({ length: rows }).map((_, row) => (
        <div key={`row-${row}`} className="flex gap-4">
          {Array.from({ length: columns }).map((_, col) => (
            <Skeleton key={`cell-${row}-${col}`} className="h-10 flex-1" />
          ))}
        </div>
      ))}
    </div>
  );
}

export interface CardSkeletonProps {
  lines?: number;
  showHeader?: boolean;
  className?: string;
}

export function CardSkeleton({
  lines = 3,
  showHeader = true,
  className,
}: CardSkeletonProps) {
  return (
    <div
      className={cn(
        "rounded-lg border border-border bg-card p-6 space-y-4",
        className,
      )}
      aria-busy="true"
      aria-label="Loading card"
    >
      {showHeader && (
        <div className="space-y-2">
          <Skeleton className="h-5 w-1/3" />
          <Skeleton className="h-4 w-2/3" />
        </div>
      )}
      <div className="space-y-2">
        {Array.from({ length: lines }).map((_, i) => (
          <Skeleton
            key={`line-${i}`}
            className={cn("h-4", i === lines - 1 ? "w-4/5" : "w-full")}
          />
        ))}
      </div>
    </div>
  );
}

export default Skeleton;
