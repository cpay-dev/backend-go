import { useEffect, useState } from "react";
import { useAccount, useWriteContract, useSwitchChain } from "wagmi";
import { parseUnits } from "viem";
import { sdk, type InvoiceData } from "../api";
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

interface Props {
  id: string;
  callbacks: CheckoutCallbacks;
}

export function InvoiceCheckout({ id, callbacks }: Props) {
  const [invoice, setInvoice] = useState<InvoiceData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [email, setEmail] = useState("");
  const [payStatus, setPayStatus] = useState<"idle" | "sending" | "success" | "error">("idle");

  const { address, isConnected, chainId } = useAccount();
  const { writeContractAsync } = useWriteContract();
  const { switchChainAsync } = useSwitchChain();

  useEffect(() => {
    sdk
      .getInvoice(id)
      .then((inv) => {
        setInvoice(inv);
        if (inv.status === "paid") setPayStatus("success");
      })
      .catch(() => setError("Invoice not found or unavailable."))
      .finally(() => setLoading(false));
  }, [id]);

  const handlePay = async () => {
    if (!invoice || !isConnected || !invoice.merchant_address || !address || !email.trim()) return;
    setPayStatus("sending");
    try {
      if (chainId !== invoice.chain_id) {
        try {
          await switchChainAsync({ chainId: invoice.chain_id });
        } catch {
          // wallet may handle internally
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

      await sdk.recordInvoicePayment(id, {
        payer_address: address,
        payer_email: email.trim(),
        token_address: invoice.token_address,
        chain_id: invoice.chain_id,
        amount: invoice.amount,
        tx_hash: hash,
      });

      setPayStatus("success");
      callbacks.onSuccess?.({
        type: "invoice",
        id,
        tx_hash: hash,
        payer_address: address,
        amount: invoice.amount,
      });
    } catch (err: unknown) {
      console.error("Payment failed:", err);
      setPayStatus("error");
      callbacks.onError?.({
        code: "PAYMENT_FAILED",
        message: err instanceof Error ? err.message : "Transaction failed",
      });
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

  if (error || !invoice) {
    return (
      <div className="cpay-card">
        <div className="cpay-body">
          <p className="cpay-err">{error || "Invoice not found."}</p>
        </div>
      </div>
    );
  }

  if (invoice.status === "cancelled") {
    return (
      <div className="cpay-card">
        <div className="cpay-body">
          <p className="cpay-err">This invoice has been cancelled.</p>
        </div>
      </div>
    );
  }

  if (payStatus === "success") {
    return (
      <div className="cpay-card">
        <div className="cpay-ok">
          <div className="cpay-ok-icon">&#x2705;</div>
          <p className="cpay-ok-title">Payment Sent!</p>
          <p className="cpay-ok-sub">
            Invoice INV-{String(invoice.invoice_number).padStart(3, "0")} has been paid.
          </p>
        </div>
      </div>
    );
  }

  return (
    <div className="cpay-card">
      <div className="cpay-body">
        <h3 className="cpay-title">{invoice.title}</h3>
        {invoice.memo && <p className="cpay-desc">{invoice.memo}</p>}

        {invoice.line_items && invoice.line_items.length > 0 && (
          <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.85em", margin: "8px 0" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid #e4e4e7" }}>
                <th style={{ textAlign: "left", padding: "4px 8px" }}>Item</th>
                <th style={{ textAlign: "right", padding: "4px 8px" }}>Qty</th>
                <th style={{ textAlign: "right", padding: "4px 8px" }}>Price</th>
                <th style={{ textAlign: "right", padding: "4px 8px" }}>Total</th>
              </tr>
            </thead>
            <tbody>
              {invoice.line_items.map((item, i) => (
                <tr key={i} style={{ borderBottom: "1px solid #e4e4e7" }}>
                  <td style={{ padding: "4px 8px" }}>{item.description}</td>
                  <td style={{ textAlign: "right", padding: "4px 8px" }}>{item.quantity}</td>
                  <td style={{ textAlign: "right", padding: "4px 8px" }}>{item.unit_price}</td>
                  <td style={{ textAlign: "right", padding: "4px 8px" }}>{item.amount}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}

        <div className="cpay-price-box">
          <p className="cpay-price">{invoice.amount}</p>
          <p className="cpay-price-sub">USDC on Polygon</p>
        </div>

        {invoice.due_date && (
          <p style={{ textAlign: "center", fontSize: "0.85em", color: "#71717a" }}>
            Due: {new Date(invoice.due_date).toLocaleDateString()}
          </p>
        )}

        {payStatus === "error" && (
          <p className="cpay-err">Transaction failed. Please try again.</p>
        )}

        {!invoice.merchant_address ? (
          <p className="cpay-err">Merchant wallet not configured.</p>
        ) : !isConnected ? (
          <WalletButton />
        ) : (
          <>
            <div className="cpay-group">
              <label className="cpay-label">Email for receipt</label>
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
              onClick={handlePay}
              disabled={payStatus === "sending" || !email.trim()}
            >
              {payStatus === "sending"
                ? "Sending..."
                : `Pay ${invoice.amount} USDC`}
            </button>
            <p className="cpay-addr">
              {address?.slice(0, 6)}...{address?.slice(-4)}
            </p>
          </>
        )}

        <div className="cpay-powered">
          Powered by <a href="https://cpay.dev" target="_blank" rel="noopener">CPay</a>
        </div>
      </div>
    </div>
  );
}
