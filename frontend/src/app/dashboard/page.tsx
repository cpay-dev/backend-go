"use client";

import { useAuth } from "@/providers/auth-provider";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Plus, ExternalLink, ArrowDownToLine, Loader2 } from "lucide-react";
import { useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import { ProductCard } from "@/components/product-card";
import { PaymentLinkCard } from "@/components/payment-link-card";
import { SubscriptionCard } from "@/components/subscription-card";
import Link from "next/link";
import type { Product, PaymentLink, Subscription, Payment } from "@/lib/api";
import { useAccount, useWriteContract, useSendTransaction, useSwitchChain } from "wagmi";
import { encodeFunctionData } from "viem";

// Universal CREATE2 factory (deployed on all EVM chains)
const CREATE2_FACTORY = "0x4e59b44847b379578588920cA78FbF26c0B4956C" as const;
const CF_CHAIN_ID = 80002; // Polygon Amoy testnet
const CF_TOKEN_ADDRESS = "0x41E94Eb019C0762f9Bfcf9Fb1E58725BfB0e7582" as const;
// MinimalWallet init bytecode (solc 0.8.20)
const MINIMAL_WALLET_BYTECODE =
  "0x60a060405234801561000f575f80fd5b5060405161036883038061036883398101604081905261002e9161003f565b6001600160a01b031660805261006c565b5f6020828403121561004f575f80fd5b81516001600160a01b0381168114610065575f80fd5b9392505050565b6080516102df6100895f395f81816047015260bf01526102df5ff3fe60806040526004361061002b575f3560e01c80638da5cb5b14610036578063b61d27f614610086575f80fd5b3661003257005b5f80fd5b348015610041575f80fd5b506100697f000000000000000000000000000000000000000000000000000000000000000081565b6040516001600160a01b0390911681526020015b60405180910390f35b348015610091575f80fd5b506100a56100a03660046101c3565b6100b2565b60405161007d919061024f565b6060336001600160a01b037f0000000000000000000000000000000000000000000000000000000000000000161461011d5760405162461bcd60e51b81526020600482015260096024820152683737ba1037bbb732b960b91b60448201526064015b60405180910390fd5b5f80866001600160a01b031686868660405161013a92919061029a565b5f6040518083038185875af1925050503d805f8114610174576040519150601f19603f3d011682016040523d82523d5f602084013e610179565b606091505b5091509150816101b95760405162461bcd60e51b815260206004820152600b60248201526a18d85b1b0819985a5b195960aa1b6044820152606401610114565b9695505050505050565b5f805f80606085870312156101d6575f80fd5b84356001600160a01b03811681146101ec575f80fd5b93506020850135925060408501356001600160401b038082111561020e575f80fd5b818701915087601f830112610221575f80fd5b81358181111561022f575f80fd5b886020828501011115610240575f80fd5b95989497505060200194505050565b5f6020808352835180828501525f5b8181101561027a5785810183015185820160400152820161025e565b505f604082860101526040601f19601f8301168501019250505092915050565b818382375f910190815291905056fea2646970667358221220a9640b5094688010a419fc9d0f8507ab64efb2167ad7daa81f98994eefcf3ab464736f6c63430008140033" as const;

const EXECUTE_ABI = [
  {
    name: "execute",
    type: "function",
    inputs: [
      { name: "to", type: "address" },
      { name: "value", type: "uint256" },
      { name: "data", type: "bytes" },
    ],
    outputs: [{ name: "", type: "bytes" }],
    stateMutability: "nonpayable",
  },
] as const;

const ERC20_TRANSFER_ABI = [
  {
    name: "transfer",
    type: "function",
    inputs: [
      { name: "to", type: "address" },
      { name: "amount", type: "uint256" },
    ],
    outputs: [{ name: "", type: "bool" }],
    stateMutability: "nonpayable",
  },
] as const;

interface WithdrawInfo {
  cf_address: string;
  balance: string; // raw token units (U256 string)
  is_deployed: boolean;
  token: string;
  chain_id: number;
}

interface Shop {
  id: string;
  name: string;
  website?: string;
  title?: string;
  avatar_url?: string;
}

export default function DashboardPage() {
  const { user } = useAuth();
  const { address: walletAddress } = useAccount();
  const { writeContractAsync } = useWriteContract();
  const { sendTransactionAsync } = useSendTransaction();
  const { switchChainAsync } = useSwitchChain();
  const [shop, setShop] = useState<Shop | null>(null);
  const [products, setProducts] = useState<Product[]>([]);
  const [loadingProducts, setLoadingProducts] = useState(false);
  const [paymentLinks, setPaymentLinks] = useState<PaymentLink[]>([]);
  const [loadingLinks, setLoadingLinks] = useState(false);
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);
  const [loadingSubscriptions, setLoadingSubscriptions] = useState(false);
  const [payments, setPayments] = useState<Payment[]>([]);
  const [loadingPayments, setLoadingPayments] = useState(false);
  const [withdrawInfo, setWithdrawInfo] = useState<WithdrawInfo | null>(null);
  const [withdrawing, setWithdrawing] = useState(false);
  const [withdrawTxHash, setWithdrawTxHash] = useState<string | null>(null);
  const [withdrawError, setWithdrawError] = useState<string | null>(null);

  const loadProducts = useCallback(async () => {
    setLoadingProducts(true);
    try {
      const list = await api.products.list();
      setProducts(list);
    } catch {
      setProducts([]);
    } finally {
      setLoadingProducts(false);
    }
  }, []);

  const loadPaymentLinks = useCallback(async () => {
    setLoadingLinks(true);
    try {
      const list = await api.paymentLinks.list();
      setPaymentLinks(list);
    } catch {
      setPaymentLinks([]);
    } finally {
      setLoadingLinks(false);
    }
  }, []);

  const loadSubscriptions = useCallback(async () => {
    setLoadingSubscriptions(true);
    try {
      const list = await api.subscriptions.list();
      setSubscriptions(list);
    } catch {
      setSubscriptions([]);
    } finally {
      setLoadingSubscriptions(false);
    }
  }, []);

  const loadPayments = useCallback(async () => {
    setLoadingPayments(true);
    try {
      const list = await api.payments.listMine();
      setPayments(list);
    } catch {
      setPayments([]);
    } finally {
      setLoadingPayments(false);
    }
  }, []);

  const loadWithdrawInfo = useCallback(async () => {
    try {
      const info = await api.cf.getWithdrawInfo(CF_TOKEN_ADDRESS);
      setWithdrawInfo(info);
    } catch {
      // ignore
    }
  }, []);

  useEffect(() => {
    if (user?.role === "merchant") {
      api.shops.getMine().then((s) => setShop(s as Shop)).catch(() => {});
      loadProducts();
      loadPaymentLinks();
      loadSubscriptions();
      loadPayments();
      loadWithdrawInfo();
    }
  }, [user, loadProducts, loadPaymentLinks, loadSubscriptions, loadPayments, loadWithdrawInfo]);

  const handleToggleActive = async (id: string, active: boolean) => {
    try {
      await api.products.toggleActive(id, active);
      setProducts((prev) => prev.map((p) => (p.id === id ? { ...p, active } : p)));
    } catch (err) {
      console.error("Failed to toggle product:", err);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await api.products.delete(id);
      setProducts((prev) => prev.filter((p) => p.id !== id));
    } catch (err) {
      console.error("Failed to delete product:", err);
    }
  };

  const handleToggleLinkActive = async (id: string, active: boolean) => {
    try {
      await api.paymentLinks.toggleActive(id, active);
      setPaymentLinks((prev) => prev.map((l) => (l.id === id ? { ...l, active } : l)));
    } catch (err) {
      console.error("Failed to toggle payment link:", err);
    }
  };

  const handleDeleteLink = async (id: string) => {
    try {
      await api.paymentLinks.delete(id);
      setPaymentLinks((prev) => prev.filter((l) => l.id !== id));
    } catch (err) {
      console.error("Failed to delete payment link:", err);
    }
  };

  const handleToggleSubActive = async (id: string, active: boolean) => {
    try {
      await api.subscriptions.toggleActive(id, active);
      setSubscriptions((prev) => prev.map((s) => (s.id === id ? { ...s, active } : s)));
    } catch (err) {
      console.error("Failed to toggle subscription:", err);
    }
  };

  const handleDeleteSub = async (id: string) => {
    try {
      await api.subscriptions.delete(id);
      setSubscriptions((prev) => prev.filter((s) => s.id !== id));
    } catch (err) {
      console.error("Failed to delete subscription:", err);
    }
  };

  const handleWithdraw = async () => {
    if (!withdrawInfo || !walletAddress) return;

    setWithdrawing(true);
    setWithdrawError(null);
    setWithdrawTxHash(null);

    try {
      // Switch to Amoy if needed
      try {
        await switchChainAsync({ chainId: CF_CHAIN_ID });
      } catch {
        // ignore if already on correct chain
      }

      const cfAddr = withdrawInfo.cf_address as `0x${string}`;
      const balanceRaw = BigInt(withdrawInfo.balance);

      if (balanceRaw === BigInt(0)) {
        setWithdrawError("No balance to withdraw");
        return;
      }

      // Step 1: Deploy MinimalWallet via CREATE2 factory if not deployed
      if (!withdrawInfo.is_deployed) {
        // Build initCode: bytecode + abi.encode(owner)
        const bytecode = MINIMAL_WALLET_BYTECODE.slice(2); // strip 0x
        const ownerPadded = walletAddress.toLowerCase().replace("0x", "").padStart(64, "0");
        const initCode = ("0x" + bytecode + ownerPadded) as `0x${string}`;

        // salt = keccak256(merchant address) — computed off-chain via viem
        const { keccak256: viemKeccak256, toBytes } = await import("viem");
        const salt = viemKeccak256(toBytes(walletAddress)) as `0x${string}`;

        // Call CREATE2 factory: factory(bytes32 salt, bytes initCode)
        // The Nick's factory just takes salt + initCode as calldata: salt ++ initCode
        const factoryCalldata = (salt + initCode.slice(2)) as `0x${string}`;

        await sendTransactionAsync({
          chainId: CF_CHAIN_ID,
          to: CREATE2_FACTORY,
          data: factoryCalldata,
        });
      }

      // Step 2: Call execute(token, 0, transfer(merchantEOA, balance)) on the CF wallet
      const transferCalldata = encodeFunctionData({
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [walletAddress, balanceRaw],
      });

      const hash = await writeContractAsync({
        chainId: CF_CHAIN_ID,
        address: cfAddr,
        abi: EXECUTE_ABI,
        functionName: "execute",
        args: [CF_TOKEN_ADDRESS, BigInt(0), transferCalldata],
      });

      setWithdrawTxHash(hash);

      // Refresh withdraw info
      await loadWithdrawInfo();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err);
      setWithdrawError(msg.length > 120 ? msg.slice(0, 120) + "…" : msg);
    } finally {
      setWithdrawing(false);
    }
  };

  if (user?.role === "merchant") {
    return (
      <div className="space-y-8">
        <div>
          <h2 className="text-2xl font-bold">Dashboard</h2>
          <p className="text-muted-foreground">Welcome back, merchant</p>
        </div>

        {shop ? (
          <Card>
            <CardHeader>
              <div className="flex items-center gap-4">
                <Avatar className="h-12 w-12">
                  <AvatarImage src={shop.avatar_url} />
                  <AvatarFallback>{shop.name[0]}</AvatarFallback>
                </Avatar>
                <div className="flex-1 min-w-0">
                  <CardTitle>{shop.name}</CardTitle>
                  {shop.title && <CardDescription>{shop.title}</CardDescription>}
                </div>
                <Badge variant="secondary">Merchant</Badge>
              </div>
            </CardHeader>
          </Card>
        ) : (
          <p className="text-muted-foreground">Loading shop details...</p>
        )}

        {/* CF Wallet / Withdraw Section */}
        {withdrawInfo && (
          <Card>
            <CardHeader>
              <div className="flex items-center justify-between">
                <div>
                  <CardTitle className="text-base">Payment Account (Counterfactual)</CardTitle>
                  <CardDescription className="font-mono text-xs break-all mt-1">
                    {withdrawInfo.cf_address}
                  </CardDescription>
                </div>
                <Badge variant={withdrawInfo.is_deployed ? "secondary" : "outline"}>
                  {withdrawInfo.is_deployed ? "Deployed" : "Not deployed"}
                </Badge>
              </div>
            </CardHeader>
            <CardContent className="space-y-3">
              <div className="flex items-center justify-between">
                <div>
                  <p className="text-sm text-muted-foreground">USDC Balance</p>
                  <p className="text-xl font-bold">
                    {BigInt(withdrawInfo.balance) === BigInt(0)
                      ? "0"
                      : (Number(BigInt(withdrawInfo.balance)) / 1e6).toFixed(6).replace(/\.?0+$/, "")}{" "}
                    USDC
                  </p>
                </div>
                <div className="flex flex-col items-end gap-2">
                  <Button
                    onClick={handleWithdraw}
                    disabled={withdrawing || BigInt(withdrawInfo.balance) === BigInt(0)}
                    size="sm"
                  >
                    {withdrawing ? (
                      <>
                        <Loader2 className="mr-2 h-4 w-4 animate-spin" />
                        Withdrawing…
                      </>
                    ) : (
                      <>
                        <ArrowDownToLine className="mr-2 h-4 w-4" />
                        Withdraw to wallet
                      </>
                    )}
                  </Button>
                  <Button variant="ghost" size="sm" onClick={loadWithdrawInfo} disabled={withdrawing}>
                    Refresh
                  </Button>
                </div>
              </div>

              {!withdrawInfo.is_deployed && BigInt(withdrawInfo.balance) > BigInt(0) && (
                <p className="text-xs text-muted-foreground">
                  ℹ️ Wallet not yet deployed. Withdrawing will deploy it first (2 transactions).
                </p>
              )}

              {withdrawTxHash && (
                <div className="rounded-md bg-green-50 dark:bg-green-950 px-3 py-2 text-xs">
                  <span className="text-green-700 dark:text-green-300 font-medium">Withdrawn! </span>
                  <a
                    href={`https://amoy.polygonscan.com/tx/${withdrawTxHash}`}
                    target="_blank"
                    rel="noopener noreferrer"
                    className="font-mono text-green-600 dark:text-green-400 hover:underline break-all"
                  >
                    {withdrawTxHash}
                  </a>
                </div>
              )}

              {withdrawError && (
                <div className="rounded-md bg-red-50 dark:bg-red-950 px-3 py-2">
                  <p className="text-xs text-red-700 dark:text-red-300">{withdrawError}</p>
                </div>
              )}
            </CardContent>
          </Card>
        )}

        {/* Products Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Products</h3>
            <Link href="/dashboard/products/new">
              <Button size="sm">
                <Plus className="mr-2 h-4 w-4" />
                Add Product
              </Button>
            </Link>
          </div>

          {loadingProducts ? (
            <p className="text-sm text-muted-foreground">Loading products...</p>
          ) : products.length === 0 ? (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-muted-foreground">No products yet</p>
                <Link href="/dashboard/products/new">
                  <Button variant="outline" size="sm" className="mt-3">
                    <Plus className="mr-2 h-4 w-4" />
                    Create your first product
                  </Button>
                </Link>
              </CardContent>
            </Card>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {products.map((product) => (
                <ProductCard
                  key={product.id}
                  product={product}
                  onToggleActive={handleToggleActive}
                  onDelete={handleDelete}
                />
              ))}
            </div>
          )}
        </div>

        {/* Payment Links Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Payment Links</h3>
            <Link href="/dashboard/payment-links/new">
              <Button size="sm">
                <Plus className="mr-2 h-4 w-4" />
                New Payment Link
              </Button>
            </Link>
          </div>

          {loadingLinks ? (
            <p className="text-sm text-muted-foreground">Loading payment links...</p>
          ) : paymentLinks.length === 0 ? (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-muted-foreground">No payment links yet</p>
                <Link href="/dashboard/payment-links/new">
                  <Button variant="outline" size="sm" className="mt-3">
                    <Plus className="mr-2 h-4 w-4" />
                    Create your first payment link
                  </Button>
                </Link>
              </CardContent>
            </Card>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {paymentLinks.map((link) => (
                <PaymentLinkCard
                  key={link.id}
                  link={link}
                  onToggleActive={handleToggleLinkActive}
                  onDelete={handleDeleteLink}
                />
              ))}
            </div>
          )}
        </div>

        {/* Subscriptions Section */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Subscription Plans</h3>
            <Link href="/dashboard/subscriptions/new">
              <Button size="sm">
                <Plus className="mr-2 h-4 w-4" />
                New Plan
              </Button>
            </Link>
          </div>

          {loadingSubscriptions ? (
            <p className="text-sm text-muted-foreground">Loading subscriptions...</p>
          ) : subscriptions.length === 0 ? (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-muted-foreground">No subscription plans yet</p>
                <Link href="/dashboard/subscriptions/new">
                  <Button variant="outline" size="sm" className="mt-3">
                    <Plus className="mr-2 h-4 w-4" />
                    Create your first plan
                  </Button>
                </Link>
              </CardContent>
            </Card>
          ) : (
            <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {subscriptions.map((sub) => (
                <SubscriptionCard
                  key={sub.id}
                  subscription={sub}
                  onToggleActive={handleToggleSubActive}
                  onDelete={handleDeleteSub}
                />
              ))}
            </div>
          )}
        </div>

        {/* Payments Ledger */}
        <div className="space-y-4">
          <div className="flex items-center justify-between">
            <h3 className="text-lg font-semibold">Payment History</h3>
            <Button variant="ghost" size="sm" onClick={loadPayments} disabled={loadingPayments}>
              {loadingPayments ? "Loading..." : "Refresh"}
            </Button>
          </div>

          {loadingPayments ? (
            <p className="text-sm text-muted-foreground">Loading payments...</p>
          ) : payments.length === 0 ? (
            <Card>
              <CardContent className="py-8 text-center">
                <p className="text-muted-foreground">No payments received yet</p>
              </CardContent>
            </Card>
          ) : (
            <Card>
              <CardContent className="pt-4">
                <div className="divide-y">
                  {payments.map((payment) => (
                    <div key={payment.id} className="flex items-center justify-between py-3">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <Badge variant="outline" className="text-xs capitalize">
                            {payment.kind.replace("_", " ")}
                          </Badge>
                          <span className="text-sm font-medium">{payment.amount} USDC</span>
                        </div>
                        <p className="text-xs text-muted-foreground mt-0.5 font-mono truncate">
                          {payment.payer_address}
                        </p>
                        <p className="text-xs text-muted-foreground">
                          {new Date(payment.created_at).toLocaleString()}
                        </p>
                      </div>
                      <a
                        href={`https://${payment.chain_id === 80002 ? "amoy.polygonscan.com" : "polygonscan.com"}/tx/${payment.tx_hash}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        className="ml-3 shrink-0 text-muted-foreground hover:text-foreground"
                        title="View on Polygonscan"
                      >
                        <ExternalLink className="h-4 w-4" />
                      </a>
                    </div>
                  ))}
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">Dashboard</h2>
        <p className="text-muted-foreground">Welcome to CPay</p>
      </div>
      <Card>
        <CardHeader>
          <CardTitle>Your Account</CardTitle>
        </CardHeader>
        <CardContent className="space-y-2">
          <p className="text-sm font-mono">{user?.wallet_address}</p>
          <Badge variant="secondary">User</Badge>
        </CardContent>
      </Card>
    </div>
  );
}
