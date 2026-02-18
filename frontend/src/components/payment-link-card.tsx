"use client";

import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Copy, ToggleLeft, ToggleRight, Trash2 } from "lucide-react";
import type { PaymentLink } from "@/lib/api";

interface PaymentLinkCardProps {
  link: PaymentLink;
  onToggleActive: (id: string, active: boolean) => void;
  onDelete: (id: string) => void;
}

export function PaymentLinkCard({ link, onToggleActive, onDelete }: PaymentLinkCardProps) {
  const publicUrl = `${window.location.origin}/pay/${link.id}`;

  const usageLabel = link.max_uses
    ? `${link.use_count} / ${link.max_uses} uses`
    : `${link.use_count} uses`;

  return (
    <Card className={!link.active ? "opacity-60" : ""}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <CardTitle className="truncate text-base">{link.title}</CardTitle>
            {link.description && (
              <p className="mt-0.5 truncate text-sm text-muted-foreground">{link.description}</p>
            )}
          </div>
          <Badge variant={link.active ? "default" : "secondary"} className="shrink-0">
            {link.active ? "Active" : "Inactive"}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xl font-bold">{link.amount} USDC</p>
            <p className="text-xs text-muted-foreground">{usageLabel}</p>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              title="Copy payment link"
              onClick={() => navigator.clipboard.writeText(publicUrl)}
            >
              <Copy className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title={link.active ? "Deactivate" : "Activate"}
              onClick={() => onToggleActive(link.id, !link.active)}
            >
              {link.active
                ? <ToggleRight className="h-4 w-4" />
                : <ToggleLeft className="h-4 w-4" />}
            </Button>
            <Button
              variant="ghost"
              size="icon"
              title="Delete"
              onClick={() => {
                if (confirm("Delete this payment link?")) onDelete(link.id);
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
