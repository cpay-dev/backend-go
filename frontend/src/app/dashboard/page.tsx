"use client";

import { useAuth } from "@/providers/auth-provider";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Plus, ExternalLink } from "lucide-react";
import { useEffect, useState, useCallback } from "react";
import { api } from "@/lib/api";
import { ProductCard } from "@/components/product-card";
import { PaymentLinkCard } from "@/components/payment-link-card";
import { SubscriptionCard } from "@/components/subscription-card";
import Link from "next/link";
import type { Product, PaymentLink, Subscription, Payment } from "@/lib/api";

interface Shop {
  id: string;
  name: string;
  website?: string;
  title?: string;
  avatar_url?: string;
}

export default function DashboardPage() {
  const { user } = useAuth();
  const [shop, setShop] = useState<Shop | null>(null);
  const [products, setProducts] = useState<Product[]>([]);
  const [loadingProducts, setLoadingProducts] = useState(false);
  const [paymentLinks, setPaymentLinks] = useState<PaymentLink[]>([]);
  const [loadingLinks, setLoadingLinks] = useState(false);
  const [subscriptions, setSubscriptions] = useState<Subscription[]>([]);
  const [loadingSubscriptions, setLoadingSubscriptions] = useState(false);
  const [payments, setPayments] = useState<Payment[]>([]);
  const [loadingPayments, setLoadingPayments] = useState(false);
  const [cfAddress, setCfAddress] = useState<string | null>(null);

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

  useEffect(() => {
    if (user?.role === "merchant") {
      api.shops.getMine().then((s) => setShop(s as Shop)).catch(() => {});
      loadProducts();
      loadPaymentLinks();
      loadSubscriptions();
      loadPayments();
      // Load CF address (cached after first call)
      api.cf.getAddress().then((r) => setCfAddress(r.address)).catch(() => {});
    }
  }, [user, loadProducts, loadPaymentLinks, loadSubscriptions, loadPayments]);

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
            {cfAddress && (
              <CardContent>
                <div className="rounded-md bg-muted px-3 py-2">
                  <p className="text-xs text-muted-foreground mb-0.5">Your payment account (counterfactual)</p>
                  <p className="font-mono text-xs break-all">{cfAddress}</p>
                </div>
              </CardContent>
            )}
          </Card>
        ) : (
          <p className="text-muted-foreground">Loading shop details...</p>
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
                        href={`https://polygonscan.com/tx/${payment.tx_hash}`}
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
