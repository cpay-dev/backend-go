"use client";

import { use, useEffect, useState } from "react";
import { useAccount, useConnect, useConnectors, useWriteContract, useSwitchChain } from "wagmi";
import { parseUnits } from "viem";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Wallet } from "lucide-react";
import { api } from "@/lib/api";
import type { Product } from "@/lib/api";

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

export default function ProductPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [product, setProduct] = useState<Product | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [payStatus, setPayStatus] = useState<"idle" | "sending" | "success" | "error">("idle");

  const { address, isConnected, chainId } = useAccount();
  const connectors = useConnectors();
  const { connectAsync } = useConnect();
  const { writeContractAsync } = useWriteContract();
  const { switchChainAsync } = useSwitchChain();

  useEffect(() => {
    api.products
      .getPublic(id)
      .then(setProduct)
      .catch(() => setError("Product not found or unavailable."))
      .finally(() => setLoading(false));
  }, [id]);

  // Restore success state if this wallet already paid
  useEffect(() => {
    if (!address || !id) return;
    api.payments.checkForProduct(id, address).then((payment) => {
      if (payment) setPayStatus("success");
    }).catch(() => {});
  }, [id, address]);

  const handleConnect = async () => {
    const connector = connectors[0];
    if (connector) await connectAsync({ connector });
  };

  const handlePay = async () => {
    if (!product || !isConnected || !product.merchant_address || !address) return;
    setPayStatus("sending");
    try {
      // Switch chain first if needed (must be in user gesture context)
      if (chainId !== product.chain_id) {
        try {
          await switchChainAsync({ chainId: product.chain_id });
        } catch {
          // Some wallets handle chain switch internally via writeContractAsync
        }
      }

      // Send to merchant's counterfactual account address
      const cfAddress = product.merchant_address as `0x${string}`;
      const amountInUnits = parseUnits(product.price, 6); // USDC uses 6 decimals

      const hash = await writeContractAsync({
        chainId: product.chain_id,
        address: product.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, amountInUnits],
      });

      // Record payment
      await api.payments.recordForProduct(id, {
        payer_address: address,
        token_address: product.token_address,
        chain_id: product.chain_id,
        amount: product.price,
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

  if (error || !product) {
    return (
      <div className="flex min-h-screen items-center justify-center p-4">
        <Card className="w-full max-w-sm">
          <CardContent className="py-8 text-center">
            <p className="text-destructive">{error || "Product not found."}</p>
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
            <p className="text-muted-foreground">Thank you for purchasing {product.name}.</p>
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          {product.image_url && (
            <img
              src={product.image_url}
              alt={product.name}
              className="mb-3 h-40 w-full rounded-lg object-cover"
            />
          )}
          <CardTitle>{product.name}</CardTitle>
          {product.description && (
            <CardDescription>{product.description}</CardDescription>
          )}
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="rounded-lg border bg-muted/40 p-4 text-center">
            <p className="text-3xl font-bold">{product.price}</p>
            <p className="text-sm text-muted-foreground">{product.currency} on Polygon</p>
          </div>

          {payStatus === "error" && (
            <p className="text-center text-sm text-destructive">
              Transaction failed. Please try again.
            </p>
          )}

          {!product.merchant_address ? (
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
              {payStatus === "sending" ? "Sending..." : `Pay ${product.price} ${product.currency}`}
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
