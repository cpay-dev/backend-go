"use client";

import { use, useEffect, useState } from "react";
import { useAccount, useConnect, useConnectors, useWriteContract, useSwitchChain } from "wagmi";
import { parseUnits } from "viem";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Badge } from "@/components/ui/badge";
import { Wallet } from "lucide-react";
import { api } from "@/lib/api";
import type { Invoice } from "@/lib/api";

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

export default function InvoicePage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [invoice, setInvoice] = useState<Invoice | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [payStatus, setPayStatus] = useState<"idle" | "sending" | "success" | "error">("idle");
  const [email, setEmail] = useState("");

  const { address, isConnected, chainId } = useAccount();
  const connectors = useConnectors();
  const { connectAsync } = useConnect();
  const { writeContractAsync } = useWriteContract();
  const { switchChainAsync } = useSwitchChain();

  useEffect(() => {
    api.invoices
      .getPublic(id)
      .then(setInvoice)
      .catch(() => setError("Invoice not found or unavailable."))
      .finally(() => setLoading(false));
  }, [id]);

  // Restore success state if already paid
  useEffect(() => {
    if (invoice?.status === "paid") {
      setPayStatus("success");
    }
  }, [invoice]);

  const handleConnect = async () => {
    const connector = connectors[0];
    if (connector) await connectAsync({ connector });
  };

  const handlePay = async () => {
    if (!invoice || !isConnected || !invoice.merchant_address || !address || !email.trim()) return;
    setPayStatus("sending");
    try {
      if (chainId !== invoice.chain_id) {
        try {
          await switchChainAsync({ chainId: invoice.chain_id });
        } catch {
          // Some wallets handle chain switch internally
        }
      }

      const cfAddress = invoice.merchant_address as `0x${string}`;
      const amountInUnits = parseUnits(invoice.amount, 6);

      const hash = await writeContractAsync({
        chainId: invoice.chain_id,
        address: invoice.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, amountInUnits],
      });

      await api.invoices.recordPayment(id, {
        payer_address: address,
        payer_email: email.trim(),
        token_address: invoice.token_address,
        chain_id: invoice.chain_id,
        amount: invoice.amount,
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

  if (error || !invoice) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardContent className="py-8 text-center">
            <p className="text-destructive">{error || "Invoice not found."}</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (payStatus === "success") {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardContent className="py-8 text-center space-y-2">
            <p className="text-2xl font-bold">Payment Sent!</p>
            <p className="text-muted-foreground">
              Invoice INV-{String(invoice.invoice_number).padStart(3, "0")} has been paid.
            </p>
          </CardContent>
        </Card>
      </div>
    );
  }

  if (invoice.status === "cancelled") {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-md">
          <CardContent className="py-8 text-center space-y-2">
            <p className="text-lg font-semibold">Invoice Cancelled</p>
            <p className="text-muted-foreground">This invoice is no longer payable.</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="flex items-center justify-between">
            <CardTitle>{invoice.title}</CardTitle>
            <Badge variant="secondary">
              INV-{String(invoice.invoice_number).padStart(3, "0")}
            </Badge>
          </div>
          {invoice.memo && <CardDescription>{invoice.memo}</CardDescription>}
        </CardHeader>
        <CardContent className="space-y-4">
          {invoice.line_items && invoice.line_items.length > 0 && (
            <div className="rounded-lg border">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b bg-muted/40">
                    <th className="px-3 py-2 text-left font-medium">Item</th>
                    <th className="px-3 py-2 text-right font-medium">Qty</th>
                    <th className="px-3 py-2 text-right font-medium">Price</th>
                    <th className="px-3 py-2 text-right font-medium">Total</th>
                  </tr>
                </thead>
                <tbody>
                  {invoice.line_items.map((item, i) => (
                    <tr key={i} className="border-b last:border-0">
                      <td className="px-3 py-2">{item.description}</td>
                      <td className="px-3 py-2 text-right">{item.quantity}</td>
                      <td className="px-3 py-2 text-right">{item.unit_price}</td>
                      <td className="px-3 py-2 text-right">{item.amount}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="rounded-lg border bg-muted/40 p-4 text-center">
            <p className="text-3xl font-bold">{invoice.amount}</p>
            <p className="text-sm text-muted-foreground">USDC on Polygon</p>
          </div>

          {invoice.due_date && (
            <p className="text-sm text-muted-foreground text-center">
              Due: {new Date(invoice.due_date).toLocaleDateString()}
            </p>
          )}

          {invoice.notes && (
            <p className="text-sm text-muted-foreground">{invoice.notes}</p>
          )}

          {payStatus === "error" && (
            <p className="text-center text-sm text-destructive">
              Transaction failed. Please try again.
            </p>
          )}

          {!invoice.merchant_address ? (
            <p className="text-center text-sm text-destructive">
              Merchant wallet not configured.
            </p>
          ) : !isConnected ? (
            <Button className="w-full" onClick={handleConnect}>
              <Wallet className="mr-2 h-4 w-4" />
              Connect Wallet to Pay
            </Button>
          ) : (
            <>
              <div className="space-y-2">
                <Label htmlFor="email">Email for receipt</Label>
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
                onClick={handlePay}
                disabled={payStatus === "sending" || !email.trim()}
              >
                {payStatus === "sending" ? "Sending..." : `Pay ${invoice.amount} USDC`}
              </Button>
            </>
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
