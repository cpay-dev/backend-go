package cpay_test

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	cpay "github.com/cpay-dev/cpay-go"
)

// Authenticate with SIWE, create a shop, add a product, and register a
// webhook — the typical merchant onboarding flow.
func Example_merchantFlow() {
	ctx := context.Background()

	// 1. Create an unauthenticated client.
	client := cpay.New(cpay.WithBaseURL("https://cpay.dev"))

	// 2. Get a SIWE nonce.
	nonce, err := client.GetNonce(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Nonce:", nonce)

	// 3. User signs a SIWE message with their wallet (off-SDK).
	//    Then verify the signature to get a JWT.
	resp, err := client.Verify(ctx, &cpay.VerifyRequest{
		Message:   "<SIWE message containing the nonce>",
		Signature: "0x<wallet signature>",
	})
	if err != nil {
		log.Fatal(err)
	}
	client.SetToken(resp.Token)

	// 4. Set role to merchant (one-time).
	roleResp, err := client.SetRole(ctx, "merchant")
	if err != nil && !cpay.IsConflict(err) {
		log.Fatal(err)
	}
	if roleResp != nil {
		client.SetToken(roleResp.Token)
	}

	// 5. Create a shop.
	shop, err := client.CreateShop(ctx, &cpay.CreateShopRequest{
		Name: "My Crypto Store",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Shop:", shop.ID)

	// 6. Create a product.
	product, err := client.CreateProduct(ctx, &cpay.CreateProductRequest{
		Name:  "Premium Access",
		Price: "9.99",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Product:", product.ID)

	// 7. Create a webhook to receive payment notifications.
	wh, err := client.CreateWebhook(ctx, &cpay.CreateWebhookRequest{
		URL:        "https://myserver.com/webhooks/cpay",
		EventTypes: []string{"payment.received", "subscription.payment"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Webhook secret:", wh.Secret)
}

// Fetch a public product, check for an existing payment, and record
// a new on-chain payment — the typical customer checkout flow.
func Example_payerFlow() {
	ctx := context.Background()
	client := cpay.New(cpay.WithBaseURL("https://cpay.dev"))

	// View a public product (no auth).
	product, err := client.GetPublicProduct(ctx, "product-uuid")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Product:", product.Name, "Price:", product.Price)

	// Check if already paid.
	existing, err := client.CheckProductPayment(ctx, product.ID, "0xPayerWallet")
	if err != nil {
		log.Fatal(err)
	}
	if existing != nil {
		fmt.Println("Already paid:", existing.TxHash)
		return
	}

	// Record payment after sending on-chain tx.
	payment, err := client.RecordProductPayment(ctx, product.ID, &cpay.RecordPaymentRequest{
		PayerAddress: "0xPayerWallet",
		TokenAddress: product.TokenAddress,
		ChainID:      int64(product.ChainID),
		Amount:       product.Price,
		TxHash:       "0xTransactionHash",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Payment recorded:", payment.ID)
}

// Upload a product image from disk.
func Example_uploadProductImage() {
	ctx := context.Background()
	client := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithToken("jwt-token"),
	)

	f, err := os.Open("product.png")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	url, err := client.UploadProductImage(ctx, f, "product.png", "product-uuid")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Uploaded:", url)
}

// Create a reusable payment link, then pay it as a customer.
func Example_paymentLinks() {
	ctx := context.Background()
	client := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithToken("merchant-jwt-token"),
	)

	// Merchant creates a payment link.
	maxUses := int32(100)
	link, err := client.CreatePaymentLink(ctx, &cpay.CreatePaymentLinkRequest{
		Title:  "Donate to Project",
		Amount: "5.00",
		MaxUses: &maxUses,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Link:", link.ID)

	// List all links.
	links, err := client.ListPaymentLinks(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Total links:", len(links))

	// Customer pays via the public endpoint (no auth).
	pub := cpay.New(cpay.WithBaseURL("https://cpay.dev"))
	pubLink, err := pub.GetPublicPaymentLink(ctx, link.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Pay", pubLink.Amount, "to", *pubLink.MerchantAddress)

	payment, err := pub.RecordPaymentLinkPayment(ctx, link.ID, &cpay.RecordPaymentRequest{
		PayerAddress: "0xPayerWallet",
		TokenAddress: pubLink.TokenAddress,
		ChainID:      int64(pubLink.ChainID),
		Amount:       pubLink.Amount,
		TxHash:       "0xTxHash",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Payment:", payment.ID)
}

// Create a subscription plan, subscribe a customer, record a payment,
// and list subscribers.
func Example_subscriptions() {
	ctx := context.Background()
	client := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithToken("merchant-jwt-token"),
	)

	// Merchant creates a monthly subscription.
	sub, err := client.CreateSubscription(ctx, &cpay.CreateSubscriptionRequest{
		Title:        "Pro Plan",
		Amount:       "19.99",
		TokenAddress: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359", // USDC on Polygon
		Period:       "monthly",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Subscription:", sub.ID)

	// Customer subscribes via the public endpoint (no auth).
	pub := cpay.New(cpay.WithBaseURL("https://cpay.dev"))
	subscriber, err := pub.CreateSubscriber(ctx, sub.ID, &cpay.CreateSubscriberRequest{
		PayerAddress: "0xPayerWallet",
		Email:        "user@example.com",
		Method:       "crypto",
		Periods:      3, // pre-pay 3 months
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Subscriber:", subscriber.ID, "Status:", subscriber.Status)

	// Record first subscription payment.
	sp, err := pub.RecordSubscriptionPayment(ctx, sub.ID, &cpay.RecordSubscriptionPaymentRequest{
		PayerAddress: "0xPayerWallet",
		TokenAddress: "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359",
		Amount:       "59.97", // 3 × 19.99
		TxHash:       "0xSubTxHash",
		SubscriberID: &subscriber.ID,
		Method:       "crypto",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Subscription payment:", sp.ID)

	// Merchant views subscribers.
	subs, err := client.ListSubscribers(ctx, sub.ID)
	if err != nil {
		log.Fatal(err)
	}
	for _, s := range subs {
		fmt.Printf("  %s — %s (paid %d periods)\n", s.PayerAddress, s.Status, s.PeriodsPaid)
	}

	// Customer cancels.
	cancelled, err := pub.CancelSubscriber(ctx, sub.ID, &cpay.CancelSubscriberRequest{
		PayerAddress: "0xPayerWallet",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Cancelled at:", *cancelled.CancelledAt)
}

// Create an invoice, send it, and record the customer's payment.
func Example_invoices() {
	ctx := context.Background()
	client := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithToken("merchant-jwt-token"),
	)

	email := "client@example.com"
	dueDate := "2026-03-15"
	memo := "Web development services — February 2026"

	// Create a draft invoice with line items.
	inv, err := client.CreateInvoice(ctx, &cpay.CreateInvoiceRequest{
		Title:          "Invoice #42",
		Memo:           &memo,
		Amount:         "2500.00",
		RecipientEmail: &email,
		DueDate:        &dueDate,
		LineItems: []cpay.LineItem{
			{Description: "Frontend development", Quantity: 40, UnitPrice: "50.00", Amount: "2000.00"},
			{Description: "Design review", Quantity: 10, UnitPrice: "50.00", Amount: "500.00"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Invoice:", inv.ID, "Status:", inv.Status) // status: "draft"

	// Send the invoice (draft → sent).
	inv, err = client.SendInvoice(ctx, inv.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Sent, invoice number:", inv.InvoiceNumber)

	// Customer views and pays via the public endpoint (no auth).
	pub := cpay.New(cpay.WithBaseURL("https://cpay.dev"))
	pubInv, err := pub.GetPublicInvoice(ctx, inv.ID)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Pay", pubInv.Amount, "to", *pubInv.MerchantAddress)

	paid, err := pub.RecordInvoicePayment(ctx, inv.ID, &cpay.RecordInvoicePaymentRequest{
		PayerAddress: "0xPayerWallet",
		TokenAddress: pubInv.TokenAddress,
		Amount:       pubInv.Amount,
		TxHash:       "0xInvoiceTxHash",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Invoice paid:", paid.Status) // status: "paid"
}

// Manage webhooks and inspect delivery history.
func Example_webhooks() {
	ctx := context.Background()
	client := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithToken("merchant-jwt-token"),
	)

	// Create a webhook — save the secret for signature verification.
	wh, err := client.CreateWebhook(ctx, &cpay.CreateWebhookRequest{
		URL:        "https://myserver.com/webhooks/cpay",
		EventTypes: []string{"payment.received", "invoice.paid", "subscription.payment"},
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Webhook:", wh.Webhook.ID)
	fmt.Println("Secret:", wh.Secret) // store this — only shown once

	// List all webhooks.
	webhooks, err := client.ListWebhooks(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Total webhooks:", len(webhooks))

	// Update a webhook.
	updated, err := client.UpdateWebhook(ctx, wh.Webhook.ID, &cpay.UpdateWebhookRequest{
		URL:        "https://myserver.com/webhooks/cpay/v2",
		EventTypes: []string{"payment.received"},
		Active:     true,
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Updated URL:", updated.URL)

	// Check delivery history.
	deliveries, err := client.ListWebhookDeliveries(ctx, wh.Webhook.ID)
	if err != nil {
		log.Fatal(err)
	}
	for _, d := range deliveries {
		fmt.Printf("  %s — %s (attempts: %d)\n", d.EventType, d.Status, d.Attempts)
	}
}

// Check merchant balance and record a withdrawal.
func Example_withdrawals() {
	ctx := context.Background()
	client := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithToken("merchant-jwt-token"),
	)

	usdc := "0x3c499c542cEF5E3811e1192ce70d8cC03d5c3359"

	// Get the merchant's counterfactual address.
	addr, err := client.GetCFAddress(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("CF address:", addr)

	// Check USDC balance.
	bal, err := client.GetCFBalance(ctx, usdc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Balance:", bal.Balance)

	// Get withdrawal info (includes deployment status).
	info, err := client.GetCFWithdrawInfo(ctx, usdc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Deployed:", info.IsDeployed)

	// After sending the withdrawal tx on-chain, record it.
	wd, err := client.RecordWithdrawal(ctx, &cpay.RecordWithdrawalRequest{
		TokenAddress: usdc,
		ChainID:      137,
		Amount:       bal.Balance,
		TxHash:       "0xWithdrawTxHash",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Withdrawal:", wd.ID)
}

// Manage products: list, update, toggle active status, and delete.
func Example_productManagement() {
	ctx := context.Background()
	client := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithToken("merchant-jwt-token"),
	)

	// List all products.
	products, err := client.ListProducts(ctx)
	if err != nil {
		log.Fatal(err)
	}
	for _, p := range products {
		fmt.Printf("  %s — %s (%s)\n", p.ID, p.Name, p.Price)
	}

	if len(products) == 0 {
		return
	}
	id := products[0].ID

	// Update a product.
	desc := "Updated description"
	updated, err := client.UpdateProduct(ctx, id, &cpay.UpdateProductRequest{
		Name:        "Premium Access v2",
		Description: &desc,
		Price:       "14.99",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Updated:", updated.Name)

	// Deactivate.
	if err := client.SetProductActive(ctx, id, false); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Deactivated")

	// View payments for the product.
	payments, err := client.ListProductPayments(ctx, id)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Payments:", len(payments))

	// Delete.
	if err := client.DeleteProduct(ctx, id); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Deleted")
}

// Use error helpers to handle specific API errors gracefully.
func Example_errorHandling() {
	ctx := context.Background()
	client := cpay.New(cpay.WithBaseURL("https://cpay.dev"))

	_, err := client.GetPublicProduct(ctx, "nonexistent-id")
	if cpay.IsNotFound(err) {
		fmt.Println("Product not found")
	} else if cpay.IsForbidden(err) {
		fmt.Println("Access denied — check your JWT token")
	} else if err != nil {
		// All API errors implement *cpay.Error.
		if apiErr, ok := err.(*cpay.Error); ok {
			fmt.Printf("API error %d: %s\n", apiErr.StatusCode, apiErr.Message)
		} else {
			fmt.Println("Network error:", err)
		}
	}

	// IsConflict is useful for idempotent operations.
	authedClient := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithToken("jwt-token"),
	)
	_, err = authedClient.SetRole(ctx, "merchant")
	if cpay.IsConflict(err) {
		fmt.Println("Role already set — safe to continue")
	}
}

// Use a custom http.Client for timeouts, retries, or proxies.
func Example_customHTTPClient() {
	client := cpay.New(
		cpay.WithBaseURL("https://cpay.dev"),
		cpay.WithHTTPClient(&http.Client{
			Timeout: 10 * time.Second,
		}),
		cpay.WithToken("jwt-token"),
	)

	ctx := context.Background()
	profile, err := client.GetProfile(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Wallet:", profile.WalletAddress)
	if profile.Email != nil {
		fmt.Println("Email:", *profile.Email)
	}
}
