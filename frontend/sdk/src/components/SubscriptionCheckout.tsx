import { useEffect, useState, useCallback } from "react";
import { useAccount, useWriteContract, useSwitchChain } from "wagmi";
import { parseUnits } from "viem";
import { sdk, type SubscriptionData, type SubscriberData } from "../api";
import { WalletButton } from "./WalletButton";
import type { CheckoutCallbacks } from "../App";

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

type Step = "details" | "method" | "success";
type PayMethod = "prepaid" | "approved" | "manual";

interface Props {
  id: string;
  callbacks: CheckoutCallbacks;
}

export function SubscriptionCheckout({ id, callbacks }: Props) {
  const [sub, setSub] = useState<SubscriptionData | null>(null);
  const [subscriber, setSubscriber] = useState<SubscriberData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const [email, setEmail] = useState("");
  const [payMethod, setPayMethod] = useState<PayMethod | null>(null);
  const [periods, setPeriods] = useState(1);
  const [step, setStep] = useState<Step>("details");
  const [payError, setPayError] = useState("");
  const [paying, setPaying] = useState(false);

  const { address, isConnected, chainId } = useAccount();
  const { writeContractAsync } = useWriteContract();
  const { switchChainAsync } = useSwitchChain();

  useEffect(() => {
    sdk
      .getSubscription(id)
      .then(setSub)
      .catch(() => setError("Subscription not found or inactive."))
      .finally(() => setLoading(false));
  }, [id]);

  // Check if already subscribed
  useEffect(() => {
    if (!address || !id) return;
    sdk
      .getSubscriber(id, address)
      .then((s) => {
        if (s && s.id) {
          setSubscriber(s);
          setStep("success");
        }
      })
      .catch(() => {});
  }, [id, address]);

  const ensureChain = useCallback(
    async (targetChainId: number) => {
      if (chainId !== targetChainId) {
        try {
          await switchChainAsync({ chainId: targetChainId });
        } catch {
          // wallet may handle internally
        }
      }
    },
    [chainId, switchChainAsync]
  );

  const handlePrepaid = async () => {
    if (!sub || !address || !sub.merchant_address) return;
    setPaying(true);
    setPayError("");
    try {
      await ensureChain(sub.chain_id);
      const totalAmount = parseUnits(sub.amount, 6) * BigInt(periods);
      const cfAddress = sub.merchant_address as `0x${string}`;

      const hash = await writeContractAsync({
        chainId: sub.chain_id,
        address: sub.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, totalAmount],
      });

      await sdk.recordSubPayment(id, {
        payer_address: address,
        token_address: sub.token_address,
        chain_id: sub.chain_id,
        amount: (parseFloat(sub.amount) * periods).toString(),
        tx_hash: hash,
      });

      const s = await sdk.subscribe(id, {
        payer_address: address,
        email,
        method: "prepaid",
        periods,
      });
      setSubscriber(s);
      setStep("success");
      callbacks.onSuccess?.({
        type: "subscription",
        id,
        tx_hash: hash,
        payer_address: address,
        method: "prepaid",
        subscriber_id: s.id,
      });
    } catch (err: unknown) {
      console.error("Prepaid payment failed:", err);
      setPayError("Transaction failed. Please try again.");
      callbacks.onError?.({
        code: "PAYMENT_FAILED",
        message: err instanceof Error ? err.message : "Transaction failed",
      });
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

      const { address: relayerAddr } = await sdk.getRelayerAddress();

      // Approve relayer for 12 periods
      const approveAmount = amountPerPeriod * BigInt(12);
      await writeContractAsync({
        chainId: sub.chain_id,
        address: sub.token_address as `0x${string}`,
        abi: ERC20_APPROVE_ABI,
        functionName: "approve",
        args: [relayerAddr as `0x${string}`, approveAmount],
      });

      // Transfer first period
      const hash = await writeContractAsync({
        chainId: sub.chain_id,
        address: sub.token_address as `0x${string}`,
        abi: ERC20_TRANSFER_ABI,
        functionName: "transfer",
        args: [cfAddress, amountPerPeriod],
      });

      await sdk.recordSubPayment(id, {
        payer_address: address,
        token_address: sub.token_address,
        chain_id: sub.chain_id,
        amount: sub.amount,
        tx_hash: hash,
      });

      const s = await sdk.subscribe(id, {
        payer_address: address,
        email,
        method: "approved",
        approved_amount: approveAmount.toString(),
      });
      setSubscriber(s);
      setStep("success");
      callbacks.onSuccess?.({
        type: "subscription",
        id,
        tx_hash: hash,
        payer_address: address,
        method: "approved",
        subscriber_id: s.id,
      });
    } catch (err: unknown) {
      console.error("Approved payment failed:", err);
      setPayError("Transaction failed. Please try again.");
      callbacks.onError?.({
        code: "PAYMENT_FAILED",
        message: err instanceof Error ? err.message : "Transaction failed",
      });
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

      await sdk.recordSubPayment(id, {
        payer_address: address,
        token_address: sub.token_address,
        chain_id: sub.chain_id,
        amount: sub.amount,
        tx_hash: hash,
      });

      setStep("success");
      callbacks.onSuccess?.({
        type: "subscription",
        id,
        tx_hash: hash,
        payer_address: address,
        method: "manual",
      });
    } catch (err: unknown) {
      console.error("Manual payment failed:", err);
      setPayError("Transaction failed. Please try again.");
      callbacks.onError?.({
        code: "PAYMENT_FAILED",
        message: err instanceof Error ? err.message : "Transaction failed",
      });
    } finally {
      setPaying(false);
    }
  };

  const handleCancel = async () => {
    if (!address || !subscriber) return;
    try {
      const s = await sdk.cancelSubscription(id, address);
      setSubscriber(s);
    } catch (err) {
      console.error("Cancel failed:", err);
    }
  };

  if (loading) {
    return (
      <div className="cpay-card">
        <div className="cpay-body">
          <p style={{ textAlign: "center", color: "#71717a" }}>Loading...</p>
        </div>
      </div>
    );
  }

  if (error || !sub) {
    return (
      <div className="cpay-card">
        <div className="cpay-body">
          <p className="cpay-err">{error || "Subscription not found."}</p>
        </div>
      </div>
    );
  }

  const periodLabel = PERIOD_LABELS[sub.period] ?? sub.period;

  // Success / active subscriber view
  if (step === "success") {
    const isActive = subscriber?.status === "active";
    const isCancelled = subscriber?.status === "cancelled";
    return (
      <div className="cpay-card">
        <div className="cpay-ok">
          <div className="cpay-ok-icon">{isCancelled ? "\u274C" : "\u2705"}</div>
          <p className="cpay-ok-title">
            {subscriber
              ? isActive
                ? "Subscribed"
                : isCancelled
                  ? "Cancelled"
                  : subscriber.status
              : "Payment Sent"}
          </p>
          <p className="cpay-ok-sub">
            {sub.title} &mdash; {sub.amount} USDC / {periodLabel.toLowerCase()}
          </p>
        </div>

        {subscriber && (
          <div className="cpay-body" style={{ paddingTop: 0 }}>
            <div className="cpay-sub-info">
              <div className="cpay-sub-row">
                <span className="cpay-sub-label">Method</span>
                <span style={{ textTransform: "capitalize" }}>{subscriber.method}</span>
              </div>
              {subscriber.method === "prepaid" && (
                <div className="cpay-sub-row">
                  <span className="cpay-sub-label">Periods</span>
                  <span>
                    {subscriber.periods_used} / {subscriber.periods_paid}
                  </span>
                </div>
              )}
              {subscriber.next_due && isActive && (
                <div className="cpay-sub-row">
                  <span className="cpay-sub-label">Next due</span>
                  <span>{new Date(subscriber.next_due).toLocaleDateString()}</span>
                </div>
              )}
              <div className="cpay-sub-row">
                <span className="cpay-sub-label">Status</span>
                <span
                  className={`cpay-badge ${isActive ? "cpay-badge-ok" : "cpay-badge-muted"}`}
                >
                  {subscriber.status.replace("_", " ")}
                </span>
              </div>
            </div>

            {!subscriber && (
              <button
                className="cpay-btn cpay-btn-outline"
                onClick={() => setStep("details")}
              >
                Pay Again
              </button>
            )}
            {subscriber && isActive && (
              <button className="cpay-btn cpay-btn-danger" onClick={handleCancel}>
                Cancel Subscription
              </button>
            )}
          </div>
        )}

        <div className="cpay-body" style={{ paddingTop: subscriber ? 0 : undefined }}>
          <div className="cpay-powered">
            Powered by{" "}
            <a href="https://cpay.dev" target="_blank" rel="noopener">
              CPay
            </a>
          </div>
        </div>
      </div>
    );
  }

  // Details step
  if (step === "details") {
    return (
      <div className="cpay-card">
        <div className="cpay-body">
          <h3 className="cpay-title">{sub.title}</h3>
          {sub.description && <p className="cpay-desc">{sub.description}</p>}

          <div className="cpay-price-box">
            <p className="cpay-price">{sub.amount}</p>
            <p className="cpay-price-sub">
              USDC / {periodLabel.toLowerCase()} on Polygon
            </p>
          </div>

          {!sub.merchant_address ? (
            <p className="cpay-err">Merchant wallet not configured.</p>
          ) : !isConnected ? (
            <WalletButton />
          ) : (
            <>
              <div className="cpay-group">
                <label className="cpay-label">Email for receipts</label>
                <input
                  className="cpay-input"
                  type="email"
                  placeholder="you@example.com"
                  value={email}
                  onChange={(e) => setEmail(e.target.value)}
                />
              </div>
              <button
                className="cpay-btn cpay-btn-primary"
                onClick={() => setStep("method")}
                disabled={!email}
              >
                Choose Payment Method
              </button>
              <p className="cpay-addr">
                {address?.slice(0, 6)}...{address?.slice(-4)}
              </p>
            </>
          )}

          <div className="cpay-powered">
            Powered by{" "}
            <a href="https://cpay.dev" target="_blank" rel="noopener">
              CPay
            </a>
          </div>
        </div>
      </div>
    );
  }

  // Method selection step
  return (
    <div className="cpay-card">
      <div className="cpay-body">
        <button className="cpay-back" onClick={() => setStep("details")}>
          &larr; Back
        </button>
        <h3 className="cpay-title">Payment Method</h3>
        <p className="cpay-desc">
          {sub.title} &mdash; {sub.amount} USDC / {periodLabel.toLowerCase()}
        </p>

        {payError && <p className="cpay-err">{payError}</p>}

        <div className="cpay-methods">
          {/* Prepaid */}
          <div
            className={`cpay-method${payMethod === "prepaid" ? " cpay-selected" : ""}`}
            onClick={() => setPayMethod("prepaid")}
          >
            <p className="cpay-method-title">Pay upfront</p>
            <p className="cpay-method-desc">
              Pay for multiple periods now. No auto-charge.
            </p>
            {payMethod === "prepaid" && (
              <div className="cpay-periods">
                {PERIOD_OPTIONS.map((n) => (
                  <button
                    key={n}
                    className={`cpay-period-btn${periods === n ? " cpay-selected" : ""}`}
                    onClick={(e) => {
                      e.stopPropagation();
                      setPeriods(n);
                    }}
                  >
                    {n} {n === 1 ? periodLabel.toLowerCase() : periodLabel.toLowerCase() + "s"}
                  </button>
                ))}
              </div>
            )}
          </div>

          {/* Auto-renew */}
          <div
            className={`cpay-method${payMethod === "approved" ? " cpay-selected" : ""}`}
            onClick={() => setPayMethod("approved")}
          >
            <p className="cpay-method-title">Auto-renew</p>
            <p className="cpay-method-desc">
              Approve token spending. Charges are automatic each{" "}
              {periodLabel.toLowerCase()}.
            </p>
          </div>

          {/* One-time */}
          <div
            className={`cpay-method${payMethod === "manual" ? " cpay-selected" : ""}`}
            onClick={() => setPayMethod("manual")}
          >
            <p className="cpay-method-title">Pay once</p>
            <p className="cpay-method-desc">
              Single payment. No subscription created.
            </p>
          </div>
        </div>

        {/* Amount summary */}
        {payMethod && (
          <div className="cpay-price-box">
            <p className="cpay-price">
              {payMethod === "prepaid"
                ? `${(parseFloat(sub.amount) * periods).toFixed(2)} USDC`
                : `${sub.amount} USDC`}
            </p>
            <p className="cpay-price-sub">
              {payMethod === "prepaid" &&
                `${periods} ${periodLabel.toLowerCase()}${periods > 1 ? "s" : ""} upfront`}
              {payMethod === "approved" && "First period + approve 12 periods"}
              {payMethod === "manual" && "One-time payment"}
            </p>
          </div>
        )}

        <button
          className="cpay-btn cpay-btn-primary"
          disabled={!payMethod || paying}
          onClick={() => {
            if (payMethod === "prepaid") handlePrepaid();
            else if (payMethod === "approved") handleApproved();
            else handleManual();
          }}
        >
          {paying
            ? "Processing..."
            : payMethod === "approved"
              ? "Approve & Pay"
              : "Pay Now"}
        </button>

        <div className="cpay-powered">
          Powered by{" "}
          <a href="https://cpay.dev" target="_blank" rel="noopener">
            CPay
          </a>
        </div>
      </div>
    </div>
  );
}
