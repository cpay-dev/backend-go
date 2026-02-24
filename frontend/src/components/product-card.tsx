"use client";

import { useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Copy, Pencil, ToggleLeft, ToggleRight, Trash2 } from "lucide-react";
import Link from "next/link";
import type { Product } from "@/lib/api";
import { EmbedButton, EmbedPanel } from "@/components/embed-snippet";

interface ProductCardProps {
  product: Product;
  onToggleActive: (id: string, active: boolean) => void;
  onDelete: (id: string) => void;
}

export function ProductCard({ product, onToggleActive, onDelete }: ProductCardProps) {
  const [showEmbed, setShowEmbed] = useState(false);
  const publicUrl = typeof window !== "undefined"
    ? `${window.location.origin}/p/${product.id}`
    : `/p/${product.id}`;

  return (
    <Card className={!product.active ? "opacity-60" : ""}>
      <CardHeader className="pb-3">
        <div className="flex items-start justify-between">
          <div className="flex items-start gap-3">
            {product.image_url ? (
              <img
                src={product.image_url}
                alt={product.name}
                className="h-12 w-12 rounded-lg border object-cover"
              />
            ) : (
              <div className="flex h-12 w-12 items-center justify-center rounded-lg border bg-muted text-lg font-bold text-muted-foreground">
                {product.name[0]?.toUpperCase()}
              </div>
            )}
            <div>
              <CardTitle className="text-base">{product.name}</CardTitle>
              {product.description && (
                <p className="mt-0.5 text-sm text-muted-foreground line-clamp-1">
                  {product.description}
                </p>
              )}
            </div>
          </div>
          <Badge variant={product.active ? "default" : "secondary"}>
            {product.active ? "Active" : "Inactive"}
          </Badge>
        </div>
      </CardHeader>
      <CardContent>
        <div className="flex items-center justify-between">
          <div>
            <p className="text-xl font-bold">
              {product.price} {product.currency}
            </p>
          </div>
          <div className="flex items-center gap-1">
            <Button
              variant="ghost"
              size="icon"
              title="Copy product link"
              onClick={() => navigator.clipboard.writeText(publicUrl)}
            >
              <Copy className="h-4 w-4" />
            </Button>
            <EmbedButton onClick={() => setShowEmbed(!showEmbed)} />
            <Button
              variant="ghost"
              size="icon"
              title={product.active ? "Deactivate" : "Activate"}
              onClick={() => onToggleActive(product.id, !product.active)}
            >
              {product.active ? (
                <ToggleRight className="h-4 w-4" />
              ) : (
                <ToggleLeft className="h-4 w-4" />
              )}
            </Button>
            <Link href={`/dashboard/products/${product.id}/edit`}>
              <Button variant="ghost" size="icon" title="Edit">
                <Pencil className="h-4 w-4" />
              </Button>
            </Link>
            <Button
              variant="ghost"
              size="icon"
              title="Delete"
              onClick={() => {
                if (confirm("Delete this product?")) {
                  onDelete(product.id);
                }
              }}
            >
              <Trash2 className="h-4 w-4 text-destructive" />
            </Button>
          </div>
        </div>
        {showEmbed && <EmbedPanel type="product" id={product.id} />}
      </CardContent>
    </Card>
  );
}
