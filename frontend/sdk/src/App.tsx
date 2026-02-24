import { useState, useMemo } from "react";
import { WagmiProvider, createConfig, http } from "wagmi";
import { polygonAmoy } from "wagmi/chains";
import { injected, walletConnect } from "wagmi/connectors";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ProductCheckout } from "./components/ProductCheckout";
import { PaymentLinkCheckout } from "./components/PaymentLinkCheckout";
import { SubscriptionCheckout } from "./components/SubscriptionCheckout";
import { InvoiceCheckout } from "./components/InvoiceCheckout";

export interface CheckoutCallbacks {
  onSuccess?: (data: Record<string, unknown>) => void;
  onError?: (err: { code: string; message: string }) => void;
}

interface AppProps {
  type: "product" | "paymentLink" | "subscription" | "invoice";
  id: string;
  theme?: "light" | "dark";
  walletConnectProjectId?: string;
  callbacks: CheckoutCallbacks;
}

export function App({ type, id, theme, walletConnectProjectId, callbacks }: AppProps) {
  const [queryClient] = useState(() => new QueryClient());

  const config = useMemo(() => {
    const connectors = [
      injected({ shimDisconnect: true }),
      ...(walletConnectProjectId
        ? [
            walletConnect({
              projectId: walletConnectProjectId,
              showQrModal: true,
              metadata: {
                name: "CPay",
                description: "Crypto Payments",
                url: "https://cpay.dev",
                icons: [],
              },
            }),
          ]
        : []),
    ];

    return createConfig({
      chains: [polygonAmoy],
      connectors,
      transports: {
        [polygonAmoy.id]: http("https://rpc-amoy.polygon.technology"),
      },
    });
  }, [walletConnectProjectId]);

  const dark = theme === "dark";

  return (
    <div className={`cpay-root${dark ? " cpay-dark" : ""}`}>
      <WagmiProvider config={config}>
        <QueryClientProvider client={queryClient}>
          {type === "product" && (
            <ProductCheckout id={id} callbacks={callbacks} />
          )}
          {type === "paymentLink" && (
            <PaymentLinkCheckout id={id} callbacks={callbacks} />
          )}
          {type === "subscription" && (
            <SubscriptionCheckout id={id} callbacks={callbacks} />
          )}
          {type === "invoice" && (
            <InvoiceCheckout id={id} callbacks={callbacks} />
          )}
        </QueryClientProvider>
      </WagmiProvider>
    </div>
  );
}
