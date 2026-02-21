package email

import (
	"fmt"
	"time"
)

func wrap(body string) string {
	return fmt.Sprintf(`<!DOCTYPE html>
<html><head><meta charset="utf-8"><style>
body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,sans-serif;max-width:480px;margin:40px auto;padding:0 16px;color:#1a1a1a}
.card{border:1px solid #e5e7eb;border-radius:12px;padding:24px;margin-top:16px}
.amount{font-size:28px;font-weight:700;text-align:center;margin:16px 0}
.label{font-size:13px;color:#6b7280;text-transform:uppercase;letter-spacing:0.5px}
.mono{font-family:monospace;font-size:13px;word-break:break-all;color:#6b7280}
.footer{margin-top:24px;font-size:12px;color:#9ca3af;text-align:center}
hr{border:none;border-top:1px solid #f3f4f6;margin:16px 0}
</style></head><body>%s
<div class="footer">CPay — Crypto payments on Polygon</div>
</body></html>`, body)
}

func receiptHTML(subTitle, amount, txHash string, nextDue time.Time) string {
	nextDueStr := "N/A"
	if !nextDue.IsZero() {
		nextDueStr = nextDue.Format("Jan 2, 2006")
	}
	return wrap(fmt.Sprintf(`
<div class="card">
  <p class="label">Payment Receipt</p>
  <h2>%s</h2>
  <div class="amount">%s USDC</div>
  <hr>
  <p class="label">Transaction</p>
  <p class="mono">%s</p>
  <hr>
  <p class="label">Next payment due</p>
  <p><strong>%s</strong></p>
</div>`, subTitle, amount, txHash, nextDueStr))
}

func upcomingHTML(subTitle, amount string, dueDate time.Time) string {
	return wrap(fmt.Sprintf(`
<div class="card">
  <p class="label">Upcoming Payment</p>
  <h2>%s</h2>
  <div class="amount">%s USDC</div>
  <hr>
  <p>Your next payment of <strong>%s USDC</strong> is due on <strong>%s</strong>.</p>
  <p>Make sure your wallet has sufficient funds and allowance.</p>
</div>`, subTitle, amount, amount, dueDate.Format("Jan 2, 2006")))
}

func failedHTML(subTitle, amount, reason string) string {
	return wrap(fmt.Sprintf(`
<div class="card">
  <p class="label">Payment Failed</p>
  <h2>%s</h2>
  <div class="amount">%s USDC</div>
  <hr>
  <p><strong>Reason:</strong> %s</p>
  <p>Please ensure your wallet has sufficient USDC balance and that the approval is still active. Your subscription will be paused until the payment succeeds.</p>
</div>`, subTitle, amount, reason))
}

func cancelledHTML(subTitle string) string {
	return wrap(fmt.Sprintf(`
<div class="card">
  <p class="label">Subscription Cancelled</p>
  <h2>%s</h2>
  <hr>
  <p>Your subscription has been cancelled. If you prepaid for future periods, they will remain active until exhausted.</p>
  <p>No further automatic charges will be made.</p>
</div>`, subTitle))
}

func expiredHTML(subTitle string) string {
	return wrap(fmt.Sprintf(`
<div class="card">
  <p class="label">Subscription Expired</p>
  <h2>%s</h2>
  <hr>
  <p>Your subscription has expired. All prepaid periods have been used or your approval has been exhausted.</p>
  <p>Visit the subscription page to renew.</p>
</div>`, subTitle))
}

func paymentReceiptHTML(title, amount, txHash string) string {
	return wrap(fmt.Sprintf(`
<div class="card">
  <p class="label">Payment Receipt</p>
  <h2>%s</h2>
  <div class="amount">%s USDC</div>
  <hr>
  <p class="label">Transaction</p>
  <p class="mono">%s</p>
</div>`, title, amount, txHash))
}

func merchantPaymentHTML(subTitle, amount, payerAddress string) string {
	short := payerAddress
	if len(short) > 10 {
		short = short[:6] + "..." + short[len(short)-4:]
	}
	return wrap(fmt.Sprintf(`
<div class="card">
  <p class="label">Payment Received</p>
  <h2>%s</h2>
  <div class="amount">%s USDC</div>
  <hr>
  <p><strong>From:</strong> <span class="mono">%s</span></p>
</div>`, subTitle, amount, short))
}
