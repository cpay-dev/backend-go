import { createRoot } from "react-dom/client";
import { setBaseUrl } from "./api";
import { injectStyles } from "./styles";
import { App } from "./App";
import type { CheckoutCallbacks } from "./App";

// Detect base URL from the <script> tag that loaded us
function detectBaseUrl(): string {
  const scripts = document.querySelectorAll("script[src]");
  for (let i = scripts.length - 1; i >= 0; i--) {
    const src = (scripts[i] as HTMLScriptElement).src;
    if (src.includes("/sdk/cpay")) {
      try {
        const url = new URL(src);
        return url.origin;
      } catch {
        // ignore invalid URL
      }
    }
  }
  return "";
}

interface RenderOptions {
  product?: string;
  paymentLink?: string;
  subscription?: string;
  invoice?: string;
  container: string | HTMLElement;
  theme?: "light" | "dark";
  walletConnectProjectId?: string;
  onSuccess?: (data: Record<string, unknown>) => void;
  onError?: (err: { code: string; message: string }) => void;
}

interface RenderHandle {
  destroy: () => void;
}

function resolveType(opts: RenderOptions): { type: "product" | "paymentLink" | "subscription" | "invoice"; id: string } | null {
  if (opts.product) return { type: "product", id: opts.product };
  if (opts.paymentLink) return { type: "paymentLink", id: opts.paymentLink };
  if (opts.subscription) return { type: "subscription", id: opts.subscription };
  if (opts.invoice) return { type: "invoice", id: opts.invoice };
  return null;
}

function render(opts: RenderOptions): RenderHandle {
  const resolved = resolveType(opts);
  if (!resolved) {
    console.error("[CPay] One of product, paymentLink, or subscription ID is required");
    return { destroy: () => {} };
  }

  const container =
    typeof opts.container === "string"
      ? document.querySelector(opts.container)
      : opts.container;

  if (!container) {
    console.error(`[CPay] Container not found: ${opts.container}`);
    return { destroy: () => {} };
  }

  injectStyles();

  const callbacks: CheckoutCallbacks = {
    onSuccess: opts.onSuccess,
    onError: opts.onError,
  };

  const root = createRoot(container);
  root.render(
    <App
      type={resolved.type}
      id={resolved.id}
      theme={opts.theme}
      walletConnectProjectId={opts.walletConnectProjectId}
      callbacks={callbacks}
    />
  );

  return {
    destroy: () => {
      root.unmount();
    },
  };
}

interface ButtonOptions extends Omit<RenderOptions, "container"> {
  container: string | HTMLElement;
  label?: string;
}

function button(opts: ButtonOptions): RenderHandle {
  const resolved = resolveType(opts as RenderOptions);
  if (!resolved) {
    console.error("[CPay] One of product, paymentLink, or subscription ID is required");
    return { destroy: () => {} };
  }

  const container =
    typeof opts.container === "string"
      ? document.querySelector(opts.container)
      : opts.container;

  if (!container) {
    console.error(`[CPay] Container not found: ${opts.container}`);
    return { destroy: () => {} };
  }

  injectStyles();

  // Render the full checkout inline
  return render(opts as RenderOptions);
}

// Initialize
const baseUrl = detectBaseUrl();
if (baseUrl) {
  setBaseUrl(baseUrl);
}

// Expose global API
const CPay = { render, button };
(window as unknown as Record<string, unknown>).CPay = CPay;

export default CPay;
