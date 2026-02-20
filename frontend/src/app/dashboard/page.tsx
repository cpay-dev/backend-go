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
import { useAccount, useWriteContract, useSendTransaction, useSwitchChain, usePublicClient } from "wagmi";
import { keccak256, toBytes, getContractAddress, type Hex } from "viem";

// Universal CREATE2 factory (deployed on all EVM chains)
const CREATE2_FACTORY = "0x4e59b44847b379578588920cA78FbF26c0B4956C" as const;
const CF_CHAIN_ID = 80002; // Polygon Amoy testnet
const CF_TOKEN_ADDRESS = "0x41E94Eb019C0762f9Bfcf9Fb1E58725BfB0e7582" as const;
// PaymentWallet init bytecode (solc 0.8.20)
// withdraw(token) permissionless, withdrawNative() permissionless, execute(to,value,data) onlyOwner
const PAYMENT_WALLET_BYTECODE =
  "0x60a060405234801561000f575f80fd5b5060405161070b38038061070b83398101604081905261002e9161003f565b6001600160a01b031660805261006c565b5f6020828403121561004f575f80fd5b81516001600160a01b0381168114610065575f80fd5b9392505050565b6080516106736100985f395f8181609201528181610120015281816102b101526103bc01526106735ff3fe608060405260043610610041575f3560e01c806350431ce41461004c57806351cff8d9146100625780638da5cb5b14610081578063b61d27f6146100ca575f80fd5b3661004857005b5f80fd5b348015610057575f80fd5b506100606100f6565b005b34801561006d575f80fd5b5061006061007c3660046104d6565b61020c565b34801561008c575f80fd5b506100b47f000000000000000000000000000000000000000000000000000000000000000081565b6040516100c191906104f6565b60405180910390f35b3480156100d5575f80fd5b506100e96100e436600461050a565b6103af565b6040516100c19190610589565b478061011d5760405162461bcd60e51b8152600401610114906105d4565b60405180910390fd5b5f7f00000000000000000000000000000000000000000000000000000000000000006001600160a01b0316826040515f6040518083038185875af1925050503d805f8114610186576040519150601f19603f3d011682016040523d82523d5f602084013e61018b565b606091505b50509050806101d55760405162461bcd60e51b81526020600482015260166024820152751b985d1a5d99481d1c985b9cd9995c8819985a5b195960521b6044820152606401610114565b6040518281527fe1abb8128ba4c3d7694c033a0a48ca4f62651a7e8a6356204348ea23841e6db69060200160405180910390a15050565b6040516370a0823160e01b81525f906001600160a01b038316906370a082319061023a9030906004016104f6565b602060405180830381865afa158015610255573d5f803e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061027991906105f8565b90505f811161029a5760405162461bcd60e51b8152600401610114906105d4565b60405163a9059cbb60e01b81526001600160a01b037f0000000000000000000000000000000000000000000000000000000000000000811660048301526024820183905283169063a9059cbb906044016020604051808303815f875af1158015610306573d5f803e3d5ffd5b505050506040513d601f19601f8201168201806040525081019061032a919061060f565b6103685760405162461bcd60e51b815260206004820152600f60248201526e1d1c985b9cd9995c8819985a5b1959608a1b6044820152606401610114565b816001600160a01b03167f7084f5476618d8e60b11ef0d7d3f06914655adb8793e28ff7f018d4c76d505d5826040516103a391815260200190565b60405180910390a25050565b6060336001600160a01b037f000000000000000000000000000000000000000000000000000000000000000016146104155760405162461bcd60e51b81526020600482015260096024820152683737ba1037bbb732b960b91b6044820152606401610114565b5f80866001600160a01b031686868660405161043292919061062e565b5f6040518083038185875af1925050503d805f811461046c576040519150601f19603f3d011682016040523d82523d5f602084013e610471565b606091505b5091509150816104b15760405162461bcd60e51b815260206004820152600b60248201526a18d85b1b0819985a5b195960aa1b6044820152606401610114565b9695505050505050565b80356001600160a01b03811681146104d1575f80fd5b919050565b5f602082840312156104e6575f80fd5b6104ef826104bb565b9392505050565b6001600160a01b0391909116815260200190565b5f805f806060858703121561051d575f80fd5b610526856104bb565b93506020850135925060408501356001600160401b0380821115610548575f80fd5b818701915087601f83011261055b575f80fd5b813581811115610569575f80fd5b88602082850101111561057a575f80fd5b95989497505060200194505050565b5f6020808352835180828501525f5b818110156105b457858101830151858201604001528201610598565b505f604082860101526040601f19601f8301168501019250505092915050565b6020808252600a90820152696e6f2062616c616e636560b01b604082015260600190565b5f60208284031215610608575f80fd5b5051919050565b5f6020828403121561061f575f80fd5b815180151581146104ef575f80fd5b818382375f910190815291905056fea2646970667358221220f3431f894c4699a0abeaae1ac6b7a442c6f9bc4158bcf5b0308f4b6d2434e31164736f6c63430008140033" as const;

const PAYMENT_WALLET_ABI = [
  {
    name: "withdraw",
    type: "function",
    inputs: [{ name: "token", type: "address" }],
    outputs: [],
    stateMutability: "nonpayable",
  },
  {
    name: "withdrawNative",
    type: "function",
    inputs: [],
    outputs: [],
    stateMutability: "nonpayable",
  },
] as const;

/** Compute the CREATE2 counterfactual address locally from the frontend bytecode. */
function computeLocalCFAddress(owner: `0x${string}`): `0x${string}` {
  const ownerPadded = owner.toLowerCase().replace("0x", "").padStart(64, "0");
  const initCode = ("0x" + PAYMENT_WALLET_BYTECODE.slice(2) + ownerPadded) as Hex;
  const salt = keccak256(toBytes(owner));
  return getContractAddress({ bytecode: initCode, from: CREATE2_FACTORY, opcode: "CREATE2", salt });
}

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
  const publicClient = usePublicClient({ chainId: CF_CHAIN_ID });
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
      // Compute CF address locally from frontend bytecode — this is the
      // address the CREATE2 factory will actually deploy to.
      const localCFAddr = computeLocalCFAddress(walletAddress);

      // Safety check: if backend derived a different address, the bytecodes
      // are out of sync and withdraw would silently no-op on an empty address.
      if (localCFAddr.toLowerCase() !== withdrawInfo.cf_address.toLowerCase()) {
        setWithdrawError(
          `CF address mismatch: backend=${withdrawInfo.cf_address.slice(0, 10)}… local=${localCFAddr.slice(0, 10)}…. Rebuild chain service.`
        );
        return;
      }

      // Switch to Amoy if needed
      try {
        await switchChainAsync({ chainId: CF_CHAIN_ID });
      } catch {
        // ignore if already on correct chain
      }

      const balanceRaw = BigInt(withdrawInfo.balance);

      if (balanceRaw === BigInt(0)) {
        setWithdrawError("No balance to withdraw");
        return;
      }

      // Step 1: Deploy PaymentWallet via CREATE2 factory if not deployed
      if (!withdrawInfo.is_deployed) {
        const bytecode = PAYMENT_WALLET_BYTECODE.slice(2); // strip 0x
        const ownerPadded = walletAddress.toLowerCase().replace("0x", "").padStart(64, "0");
        const initCode = ("0x" + bytecode + ownerPadded) as `0x${string}`;
        const salt = keccak256(toBytes(walletAddress));
        const factoryCalldata = (salt + initCode.slice(2)) as `0x${string}`;

        const deployHash = await sendTransactionAsync({
          chainId: CF_CHAIN_ID,
          to: CREATE2_FACTORY,
          data: factoryCalldata,
        });

        if (publicClient) {
          await publicClient.waitForTransactionReceipt({ hash: deployHash });
        }
      }

      // Step 2: Call withdraw(token) on the locally-computed address
      const hash = await writeContractAsync({
        chainId: CF_CHAIN_ID,
        address: localCFAddr,
        abi: PAYMENT_WALLET_ABI,
        functionName: "withdraw",
        args: [CF_TOKEN_ADDRESS],
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
