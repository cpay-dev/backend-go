"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import { Code, Copy, Check } from "lucide-react";

function generateSnippet(type: string, id: string, origin: string): string {
  const sdkUrl = `${origin}/sdk/cpay.js`;
  let containerId: string;
  let optionKey: string;

  switch (type) {
    case "product":
      containerId = `cpay-product-${id.slice(0, 8)}`;
      optionKey = "product";
      break;
    case "paymentLink":
      containerId = `cpay-pay-${id.slice(0, 8)}`;
      optionKey = "paymentLink";
      break;
    case "subscription":
      containerId = `cpay-sub-${id.slice(0, 8)}`;
      optionKey = "subscription";
      break;
    case "invoice":
      containerId = `cpay-inv-${id.slice(0, 8)}`;
      optionKey = "invoice";
      break;
    default:
      containerId = `cpay-${id.slice(0, 8)}`;
      optionKey = "product";
  }

  return `<script src="${sdkUrl}"><\/script>
<div id="${containerId}"></div>
<script>
  CPay.button({
    ${optionKey}: '${id}',
    container: '#${containerId}'
  });
<\/script>`;
}

export function EmbedButton({ onClick }: { onClick: () => void }) {
  return (
    <Button variant="ghost" size="icon" title="Embed code" onClick={onClick}>
      <Code className="h-4 w-4" />
    </Button>
  );
}

export function EmbedPanel({
  type,
  id,
}: {
  type: "product" | "paymentLink" | "subscription" | "invoice";
  id: string;
}) {
  const [copied, setCopied] = useState(false);

  const origin =
    typeof window !== "undefined" ? window.location.origin : "https://cpay.dev";
  const snippet = generateSnippet(type, id, origin);

  const handleCopy = async () => {
    await navigator.clipboard.writeText(snippet);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <div className="mt-3 rounded-lg border bg-muted/40 p-3">
      <div className="flex items-center justify-between mb-2">
        <p className="text-xs font-medium text-muted-foreground">Embed Code</p>
        <Button variant="ghost" size="sm" onClick={handleCopy}>
          {copied ? (
            <Check className="mr-1 h-3 w-3" />
          ) : (
            <Copy className="mr-1 h-3 w-3" />
          )}
          {copied ? "Copied" : "Copy"}
        </Button>
      </div>
      <pre className="overflow-x-auto text-xs font-mono whitespace-pre-wrap break-all">
        {snippet}
      </pre>
    </div>
  );
}
