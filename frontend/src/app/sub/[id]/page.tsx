"use client";

import { use, useEffect, useState, useCallback } from "react";
import { useAccount, useConnect, useConnectors, useWriteContract, useSwitchChain } from "wagmi";
import { parseUnits } from "viem";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Wallet, RefreshCw, CheckCircle, XCircle, ArrowLeft } from "lucide-react";
import { api } from "@/lib/api";
import type { Subscription, Subscriber } from "@/lib/api";

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

const ERC20_APPROVE_ABI = [
  {
    name: "approve",
    type: "function",
    inputs: [
      { name: "spender", type: "address" },
      { name: "value", type: "uint256" },
    ],
    outputs: [{ name: "", type: "bool" }],
    stateMutability: "nonpayable",
  },
] as const;

const PERIOD_LABELS: Record<string, string> = {
  daily: "Day",
  weekly: "Week",
  monthly: "Month",
  yearly: "Year",
};

const PERIOD_OPTIONS = [1, 3, 6, 12];

type Step = "details" | "method" | "paying" | "success";
type PayMethod = "prepaid" | "approved" | "manual";

export default function SubPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [sub, setSub] = useState<Subscription | null>(null);
  const [subscriber, setSubscriber] = useState<Subscriber | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  // Form state
  const [email, setEmail] = useState("");
  const [payMethod, setPayMethod] = useState<PayMethod | null>(null);
  const [periods, setPeriods] = useState(1);
  const [step, setStep] = useState<Step>("details");
  const [payError, setPayError] = useState("");
  const [paying, setPaying] = useState(false);

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

  // Check if already subscribed
  useEffect(() => {
    if (!address || !id) return;
    api.subscriptions.getSubscriber(id, address).then((s) => {
      if (s && s.id) {
        setSubscriber(s);
        setStep("success");
      }
    }).catch(() => {});
  }, [id, address]);

  const handleConnect = async () => {
    const connector = connectors[0];
    if (connector) await connectAsync({ connector });
  };

  const ensureChain = useCallback(async (targetChainId: number) => {
    if (chainId !== targetChainId) {
      try {
        await switchChainAsync({ chainId: targetChainId });
      } catch {
        // Some wallets handle chain switch internally
      }
    }
  }, [chainId, switchChainAsync]);

  const handlePrepaid = async () => {
    if (!sub || !address || !sub.merchant_address) return;
    setPaying(true);
    setPayError("");
    try {
      await ensureChain(sub.chain_id);
      const totalAmount = parseUnits(sub.amount, 6) * BigInt(periods);
      const cfAddress = sub.merchant_address as `0x${string}`;

      // Transfer N periods worth
      const hash = await writeContractAsync({
        chainId: sub.chain_id,
        address: sub.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, totalAmount],
      });

      // Record payment
      await api.subscriptions.recordPayment(id, {
        payer_address: address,
        token_address: sub.token_address,
        chain_id: sub.chain_id,
        amount: (parseFloat(sub.amount) * periods).toString(),
        tx_hash: hash,
      });

      // Create subscriber
      const s = await api.subscriptions.subscribe(id, {
        payer_address: address,
        email,
        method: "prepaid",
        periods,
      });
      setSubscriber(s);
      setStep("success");
    } catch (err) {
      console.error("Prepaid payment failed:", err);
      setPayError("Transaction failed. Please try again.");
    } finally {
      setPaying(false);
    }
  };

  const handleApproved = async () => {
    if (!sub || !address || !sub.merchant_address) return;
    setPaying(true);
    setPayError("");
    try {
      await ensureChain(sub.chain_id);
      const cfAddress = sub.merchant_address as `0x${string}`;
      const amountPerPeriod = parseUnits(sub.amount, 6);

      // Get relayer address for approval
      const { address: relayerAddr } = await api.subscriptions.getRelayerAddress();

      // Approve relayer for 12 periods worth of auto-charges
      const approveAmount = amountPerPeriod * BigInt(12);
      await writeContractAsync({
        chainId: sub.chain_id,
        address: sub.token_address as `0x${string}`,
        abi: ERC20_APPROVE_ABI,
        functionName: "approve",
        args: [relayerAddr as `0x${string}`, approveAmount],
      });

      // Transfer first period to CF address
      const hash = await writeContractAsync({
        chainId: sub.chain_id,
        address: sub.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, amountPerPeriod],
      });

      // Record first payment
      await api.subscriptions.recordPayment(id, {
        payer_address: address,
        token_address: sub.token_address,
        chain_id: sub.chain_id,
        amount: sub.amount,
        tx_hash: hash,
      });

      // Create subscriber
      const s = await api.subscriptions.subscribe(id, {
        payer_address: address,
        email,
        method: "approved",
        approved_amount: approveAmount.toString(),
      });
      setSubscriber(s);
      setStep("success");
    } catch (err) {
      console.error("Approved payment failed:", err);
      setPayError("Transaction failed. Please try again.");
    } finally {
      setPaying(false);
    }
  };

  const handleManual = async () => {
    if (!sub || !address || !sub.merchant_address) return;
    setPaying(true);
    setPayError("");
    try {
      await ensureChain(sub.chain_id);
      const cfAddress = sub.merchant_address as `0x${string}`;
      const amountInUnits = parseUnits(sub.amount, 6);

      const hash = await writeContractAsync({
        chainId: sub.chain_id,
        address: sub.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, amountInUnits],
      });

      await api.subscriptions.recordPayment(id, {
        payer_address: address,
        token_address: sub.token_address,
        chain_id: sub.chain_id,
        amount: sub.amount,
        tx_hash: hash,
      });

      setStep("success");
    } catch (err) {
      console.error("Manual payment failed:", err);
      setPayError("Transaction failed. Please try again.");
    } finally {
      setPaying(false);
    }
  };

  const handleCancel = async () => {
    if (!address || !subscriber) return;
    try {
      const s = await api.subscriptions.cancel(id, { payer_address: address });
      setSubscriber(s);
    } catch (err) {
      console.error("Cancel failed:", err);
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

  const periodLabel = PERIOD_LABELS[sub.period] ?? sub.period;

  // Success / active subscriber view
  if (step === "success") {
    const isActive = subscriber?.status === "active";
    const isCancelled = subscriber?.status === "cancelled";
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-sm">
          <CardContent className="py-8 space-y-4">
            <div className="text-center space-y-2">
              {isActive ? (
                <CheckCircle className="h-12 w-12 text-green-500 mx-auto" />
              ) : isCancelled ? (
                <XCircle className="h-12 w-12 text-muted-foreground mx-auto" />
              ) : (
                <CheckCircle className="h-12 w-12 text-green-500 mx-auto" />
              )}
              <p className="text-xl font-bold">
                {subscriber ? (
                  isActive ? "Subscribed" : isCancelled ? "Cancelled" : subscriber.status
                ) : "Payment Sent"}
              </p>
              <p className="text-muted-foreground text-sm">
                {sub.title} — {sub.amount} USDC / {periodLabel.toLowerCase()}
              </p>
            </div>

            {subscriber && (
              <div className="rounded-lg border bg-muted/40 p-3 space-y-1 text-sm">
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Method</span>
                  <span className="capitalize">{subscriber.method}</span>
                </div>
                {subscriber.method === "prepaid" && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Periods</span>
                    <span>{subscriber.periods_used} / {subscriber.periods_paid}</span>
                  </div>
                )}
                {subscriber.next_due && isActive && (
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Next due</span>
                    <span>{new Date(subscriber.next_due).toLocaleDateString()}</span>
                  </div>
                )}
                <div className="flex justify-between">
                  <span className="text-muted-foreground">Status</span>
                  <Badge variant={isActive ? "default" : "secondary"} className="capitalize">
                    {subscriber.status.replace("_", " ")}
                  </Badge>
                </div>
              </div>
            )}

            <div className="flex gap-2">
              {!subscriber && (
                <Button variant="outline" className="flex-1" onClick={() => setStep("details")}>
                  <RefreshCw className="mr-2 h-4 w-4" /> Pay Again
                </Button>
              )}
              {subscriber && isActive && (
                <Button variant="destructive" className="flex-1" onClick={handleCancel}>
                  Cancel Subscription
                </Button>
              )}
            </div>
          </CardContent>
        </Card>
      </div>
    );
  }

  // Details step — email + connect wallet
  if (step === "details") {
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
                {periodLabel}
              </Badge>
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="rounded-lg border bg-muted/40 p-4 text-center">
              <p className="text-3xl font-bold">{sub.amount}</p>
              <p className="text-sm text-muted-foreground">
                USDC / {periodLabel.toLowerCase()} on Polygon
              </p>
            </div>

            {!sub.merchant_address ? (
              <p className="text-center text-sm text-destructive">
                Merchant wallet not configured.
              </p>
            ) : !isConnected ? (
              <Button className="w-full" onClick={handleConnect}>
                <Wallet className="mr-2 h-4 w-4" />
                Connect Wallet
              </Button>
            ) : (
              <>
                <div className="space-y-2">
                  <Label htmlFor="email">Email for receipts</Label>
                  <Input
                    id="email"
                    type="email"
                    placeholder="you@example.com"
                    value={email}
                    onChange={(e) => setEmail(e.target.value)}
                  />
                </div>
                <Button
                  className="w-full"
                  onClick={() => setStep("method")}
                  disabled={!email}
                >
                  Choose Payment Method
                </Button>
                <p className="text-center text-xs text-muted-foreground">
                  {address?.slice(0, 6)}...{address?.slice(-4)}
                </p>
              </>
            )}
          </CardContent>
        </Card>
      </div>
    );
  }

  // Method selection step
  if (step === "method") {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-sm">
          <CardHeader>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="icon" onClick={() => setStep("details")}>
                <ArrowLeft className="h-4 w-4" />
              </Button>
              <CardTitle className="text-lg">Payment Method</CardTitle>
            </div>
            <CardDescription>{sub.title} — {sub.amount} USDC / {periodLabel.toLowerCase()}</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            {payError && (
              <p className="text-center text-sm text-destructive">{payError}</p>
            )}

            {/* Prepaid */}
            <div
              className={`rounded-lg border p-4 cursor-pointer transition-colors ${payMethod === "prepaid" ? "border-primary bg-primary/5" : "hover:bg-muted/40"}`}
              onClick={() => setPayMethod("prepaid")}
            >
              <p className="font-medium">Pay upfront</p>
              <p className="text-sm text-muted-foreground">
                Pay for multiple periods now. No auto-charge.
              </p>
              {payMethod === "prepaid" && (
                <div className="mt-3 flex gap-2 flex-wrap">
                  {PERIOD_OPTIONS.map((n) => (
                    <Button
                      key={n}
                      variant={periods === n ? "default" : "outline"}
                      size="sm"
                      onClick={(e) => { e.stopPropagation(); setPeriods(n); }}
                    >
                      {n} {n === 1 ? periodLabel.toLowerCase() : periodLabel.toLowerCase() + "s"}
                    </Button>
                  ))}
                </div>
              )}
            </div>

            {/* Auto-renew */}
            <div
              className={`rounded-lg border p-4 cursor-pointer transition-colors ${payMethod === "approved" ? "border-primary bg-primary/5" : "hover:bg-muted/40"}`}
              onClick={() => setPayMethod("approved")}
            >
              <p className="font-medium">Auto-renew</p>
              <p className="text-sm text-muted-foreground">
                Approve token spending. Charges are automatic each {periodLabel.toLowerCase()}.
              </p>
            </div>

            {/* One-time */}
            <div
              className={`rounded-lg border p-4 cursor-pointer transition-colors ${payMethod === "manual" ? "border-primary bg-primary/5" : "hover:bg-muted/40"}`}
              onClick={() => setPayMethod("manual")}
            >
              <p className="font-medium">Pay once</p>
              <p className="text-sm text-muted-foreground">
                Single payment. No subscription created.
              </p>
            </div>

            {/* Amount summary */}
            {payMethod && (
              <div className="rounded-lg border bg-muted/40 p-3 text-center">
                <p className="text-lg font-bold">
                  {payMethod === "prepaid"
                    ? `${(parseFloat(sub.amount) * periods).toFixed(2)} USDC`
                    : `${sub.amount} USDC`}
                </p>
                <p className="text-xs text-muted-foreground">
                  {payMethod === "prepaid" && `${periods} ${periodLabel.toLowerCase()}${periods > 1 ? "s" : ""} upfront`}
                  {payMethod === "approved" && "First period + approve 12 periods"}
                  {payMethod === "manual" && "One-time payment"}
                </p>
              </div>
            )}

            <Button
              className="w-full"
              disabled={!payMethod || paying}
              onClick={() => {
                if (payMethod === "prepaid") handlePrepaid();
                else if (payMethod === "approved") handleApproved();
                else handleManual();
              }}
            >
              {paying ? "Processing..." : payMethod === "approved" ? "Approve & Pay" : "Pay Now"}
            </Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  return null;
}
