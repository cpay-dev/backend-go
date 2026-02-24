package cpay_test

import (
	"context"
	"fmt"
	"log"
	"os"

	cpay "github.com/cpay-dev/cpay-go"
)

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
