"use client";

import { use, useEffect, useState } from "react";
import { useAccount, useConnect, useConnectors, useWriteContract, useSwitchChain } from "wagmi";
import { parseUnits } from "viem";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Wallet, RefreshCw } from "lucide-react";
import { api } from "@/lib/api";
import type { Subscription } from "@/lib/api";

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

const PERIOD_LABELS: Record<string, string> = {
  daily: "Daily",
  weekly: "Weekly",
  monthly: "Monthly",
  yearly: "Yearly",
};

export default function SubPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [sub, setSub] = useState<Subscription | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [payStatus, setPayStatus] = useState<"idle" | "sending" | "success" | "error">("idle");

  const { address, isConnected, chainId } = useAccount();
  const connectors = useConnectors();
  const { connectAsync } = useConnect();
  const { writeContractAsync } = useWriteContract();
  const { switchChainAsync } = useSwitchChain();

  useEffect(() => {
    api.subscriptions
      .getPublic(id)
      .then(setSub)
      .catch(() => setError("Subscription not found or inactive."))
      .finally(() => setLoading(false));
  }, [id]);

  const handleConnect = async () => {
    const connector = connectors[0];
    if (connector) await connectAsync({ connector });
  };

  const handlePay = async () => {
    if (!sub || !isConnected || !sub.merchant_address || !address) return;
    setPayStatus("sending");
    try {
      // Switch chain first if needed (must be in user gesture context)
      if (chainId !== sub.chain_id) {
        try {
          await switchChainAsync({ chainId: sub.chain_id });
        } catch {
          // Some wallets handle chain switch internally via writeContractAsync
        }
      }

      const cfAddress = sub.merchant_address as `0x${string}`;
      const amountInUnits = parseUnits(sub.amount, 6); // USDC 6 decimals

      const hash = await writeContractAsync({
        chainId: sub.chain_id,
        address: sub.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, amountInUnits],
      });

      // Record subscription payment
      await api.subscriptions.recordPayment(id, {
        payer_address: address,
        token_address: sub.token_address,
        chain_id: sub.chain_id,
        amount: sub.amount,
        tx_hash: hash,
      });

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

  if (error || !sub) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-sm">
          <CardContent className="py-8 text-center">
            <p className="text-destructive">{error || "Subscription not found."}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (payStatus === "success") {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-sm">
          <CardContent className="py-8 text-center space-y-3">
            <p className="text-2xl font-bold">Payment Sent!</p>
            <p className="text-muted-foreground">
              Your {PERIOD_LABELS[sub.period]?.toLowerCase() ?? sub.period} payment for{" "}
              <strong>{sub.title}</strong> has been recorded.
            </p>
            <Button variant="outline" onClick={() => setPayStatus("idle")} className="gap-2">
              <RefreshCw className="h-4 w-4" />
              Pay Again
            </Button>
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
              <CardTitle>{sub.title}</CardTitle>
              {sub.description && (
                <CardDescription className="mt-1">{sub.description}</CardDescription>
              )}
            </div>
            <Badge variant="outline" className="shrink-0 gap-1">
              <RefreshCw className="h-3 w-3" />
              {PERIOD_LABELS[sub.period] ?? sub.period}
            </Badge>
          </div>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="rounded-lg border bg-muted/40 p-4 text-center">
            <p className="text-3xl font-bold">{sub.amount}</p>
            <p className="text-sm text-muted-foreground">
              USDC / {PERIOD_LABELS[sub.period]?.toLowerCase() ?? sub.period} on Polygon
            </p>
          </div>

          {payStatus === "error" && (
            <p className="text-center text-sm text-destructive">
              Transaction failed. Please try again.
            </p>
          )}

          {!sub.merchant_address ? (
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
              {payStatus === "sending"
                ? "Sending..."
                : `Pay ${sub.amount} USDC`}
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
