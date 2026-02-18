"use client";

import { use, useEffect, useState } from "react";
import { useAccount, useConnect, useConnectors, useWriteContract, useSwitchChain } from "wagmi";
import { parseUnits } from "viem";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Wallet } from "lucide-react";
import { api } from "@/lib/api";
import type { PaymentLink } from "@/lib/api";

const ERC20_TRANSFER_ABI = [
  {
    name: "transfer",
    type: "function",
    inputs: [
      { name: "to", type: "address" },
      { name: "value", type: "uint256" },
    ],
    outputs: [{ name: "", type: "bool" }],
    stateMutability: "nonpayable",
  },
] as const;

export default function PayPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [link, setLink] = useState<PaymentLink | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [payStatus, setPayStatus] = useState<"idle" | "sending" | "success" | "error">("idle");

  const { address, isConnected, chainId } = useAccount();
  const connectors = useConnectors();
  const { connectAsync } = useConnect();
  const { writeContractAsync } = useWriteContract();
  const { switchChainAsync } = useSwitchChain();

  useEffect(() => {
    api.paymentLinks
      .getPublic(id)
      .then(setLink)
      .catch(() => setError("Payment link not found or expired."))
      .finally(() => setLoading(false));
  }, [id]);

  // Restore success state if this wallet already paid
  useEffect(() => {
    if (!address || !id) return;
    api.paymentLinks.checkPayment(id, address).then((payment) => {
      if (payment) setPayStatus("success");
    }).catch(() => {});
  }, [id, address]);

  const handleConnect = async () => {
    const connector = connectors[0];
    if (connector) await connectAsync({ connector });
  };

  const handlePay = async () => {
    if (!link || !isConnected || !link.merchant_address || !address) return;
    setPayStatus("sending");
    try {
      // Switch chain first if needed (must be in user gesture context)
      if (chainId !== link.chain_id) {
        try {
          await switchChainAsync({ chainId: link.chain_id });
        } catch {
          // Some wallets handle chain switch internally via writeContractAsync
        }
      }

      // Send to merchant's counterfactual account address
      const cfAddress = link.merchant_address as `0x${string}`;
      const amountInUnits = parseUnits(link.amount, 6); // USDC uses 6 decimals

      const hash = await writeContractAsync({
        chainId: link.chain_id,
        address: link.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, amountInUnits],
      });

      // Record payment and link use in parallel
      await Promise.all([
        api.paymentLinks.recordPayment(id, {
          payer_address: address,
          token_address: link.token_address,
          chain_id: link.chain_id,
          amount: link.amount,
          tx_hash: hash,
        }),
        api.paymentLinks.recordUse(id),
      ]);

      setPayStatus("success");
    } catch (err) {
      console.error("Payment failed:", err);
      setPayStatus("error");
    }
  };

  if (loading) {
    return (
      <div className="flex min-h-screen items-center justify-center">
        <p className="text-muted-foreground">Loading...</p>
      </div>
    );
  }

  if (error || !link) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-sm">
          <CardContent className="py-8 text-center">
            <p className="text-destructive">{error || "Payment link not found."}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (payStatus === "success") {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-sm">
          <CardContent className="py-8 text-center space-y-2">
            <p className="text-2xl font-bold">Payment Sent!</p>
            <p className="text-muted-foreground">Thank you for your payment.</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <div className="flex items-start justify-between gap-2">
            <div>
              <CardTitle>{link.title}</CardTitle>
              {link.description && (
                <CardDescription className="mt-1">{link.description}</CardDescription>
              )}
            </div>
            <Badge variant={link.active ? "default" : "secondary"} className="shrink-0">
              {link.active ? "Active" : "Inactive"}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="rounded-lg border bg-muted/40 p-4 text-center">
            <p className="text-3xl font-bold">{link.amount}</p>
            <p className="text-sm text-muted-foreground">USDC on Polygon</p>
          </div>

          {payStatus === "error" && (
            <p className="text-center text-sm text-destructive">
              Transaction failed. Please try again.
            </p>
          )}

          {!link.merchant_address ? (
            <p className="text-center text-sm text-destructive">
              Merchant wallet not configured.
            </p>
          ) : !isConnected ? (
            <Button className="w-full" onClick={handleConnect}>
              <Wallet className="mr-2 h-4 w-4" />
              Connect Wallet to Pay
            </Button>
          ) : (
            <Button
              className="w-full"
              onClick={handlePay}
              disabled={payStatus === "sending"}
            >
              {payStatus === "sending" ? "Sending..." : `Pay ${link.amount} USDC`}
            </Button>
          )}

          {isConnected && (
            <p className="text-center text-xs text-muted-foreground">
              {address?.slice(0, 6)}...{address?.slice(-4)}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
