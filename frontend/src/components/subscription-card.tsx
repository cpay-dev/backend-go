"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Copy, ToggleLeft, ToggleRight, Trash2, RefreshCw } from "lucide-react";
import type { Subscription } from "@/lib/api";

const PERIOD_LABELS: Record<string, string> = {
  daily: "Daily",
  weekly: "Weekly",
  monthly: "Monthly",
  yearly: "Yearly",
};

interface SubscriptionCardProps {
  subscription: Subscription;
  onToggleActive: (id: string, active: boolean) => void;
  onDelete: (id: string) => void;
}

export function SubscriptionCard({ subscription: sub, onToggleActive, onDelete }: SubscriptionCardProps) {
  const publicUrl = `${window.location.origin}/sub/${sub.id}`;

  return (
    <Card className={!sub.active ? "opacity-60" : ""}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <CardTitle className="truncate text-base">{sub.title}</CardTitle>
            {sub.description && (
              <p className="mt-0.5 truncate text-sm text-muted-foreground">{sub.description}</p>
            )}
          </div>
          <div className="flex shrink-0 items-center gap-1.5">
            <Badge variant="outline" className="gap-1 text-xs">
              <RefreshCw className="h-3 w-3" />
              {PERIOD_LABELS[sub.period] ?? sub.period}
            </Badge>
            <Badge variant={sub.active ? "default" : "secondary"}>
              {sub.active ? "Active" : "Inactive"}
            </Badge>
          </div>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xl font-bold">{sub.amount} USDC</p>
            <p className="text-xs text-muted-foreground">
              per {PERIOD_LABELS[sub.period]?.toLowerCase() ?? sub.period}
            </p>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              title="Copy subscription link"
              onClick={() => navigator.clipboard.writeText(publicUrl)}
            >
              <Copy className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title={sub.active ? "Deactivate" : "Activate"}
              onClick={() => onToggleActive(sub.id, !sub.active)}
            >
              {sub.active ? (
                <ToggleRight className="h-4 w-4" />
              ) : (
                <ToggleLeft className="h-4 w-4" />
              )}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title="Delete"
              onClick={() => {
                if (confirm("Delete this subscription plan?")) onDelete(sub.id);
              }}
            >
              <Trash2 className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
