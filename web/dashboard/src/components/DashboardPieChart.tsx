import {
  Cell,
  Legend,
  Pie,
  PieChart,
  ResponsiveContainer,
  Tooltip,
} from "recharts";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/Card";
import { Skeleton } from "@/components/ui/Skeleton";

export interface PieChartDatum {
  name: string;
  value: number;
  color: string;
}

interface DashboardPieChartProps {
  title: string;
  data: PieChartDatum[];
  emptyLabel?: string;
}

function ChartTooltip({
  active,
  payload,
}: {
  active?: boolean;
  payload?: Array<{ name: string; value: number; payload: PieChartDatum }>;
}) {
  if (!active || !payload?.length) return null;
  const item = payload[0];
  return (
    <div className="rounded-md border border-border bg-popover px-3 py-2 text-sm shadow-md">
      <p className="font-medium text-foreground">{item.name}</p>
      <p className="text-muted-foreground tabular-nums">{item.value} agents</p>
    </div>
  );
}

export function DashboardPieChart({ title, data, emptyLabel = "No data" }: DashboardPieChartProps) {
  const filtered = data.filter((d) => d.value > 0);
  const total = filtered.reduce((sum, d) => sum + d.value, 0);

  return (
    <Card className="w-full">
      <CardHeader className="pb-2">
        <CardTitle className="text-sm font-medium">{title}</CardTitle>
      </CardHeader>
      <CardContent>
        {total === 0 ? (
          <div className="flex h-[220px] items-center justify-center text-sm text-muted-foreground">
            {emptyLabel}
          </div>
        ) : (
          <ResponsiveContainer width="100%" height={220}>
            <PieChart>
              <Pie
                data={filtered}
                dataKey="value"
                nameKey="name"
                cx="50%"
                cy="50%"
                innerRadius={52}
                outerRadius={78}
                paddingAngle={2}
              >
                {filtered.map((entry) => (
                  <Cell key={entry.name} fill={entry.color} stroke="transparent" />
                ))}
              </Pie>
              <Tooltip content={<ChartTooltip />} />
              <Legend
                verticalAlign="bottom"
                iconType="circle"
                formatter={(value) => (
                  <span className="text-xs text-muted-foreground">{value}</span>
                )}
              />
            </PieChart>
          </ResponsiveContainer>
        )}
      </CardContent>
    </Card>
  );
}

export function DashboardPieChartSkeleton({ title }: { title: string }) {
  return (
    <Card className="w-full" aria-busy="true" aria-label={`Loading ${title}`}>
      <CardHeader className="pb-2">
        <Skeleton className="h-4 w-32" />
      </CardHeader>
      <CardContent>
        <Skeleton className="mx-auto h-[220px] w-[220px] rounded-full" />
      </CardContent>
    </Card>
  );
}
