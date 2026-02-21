package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/cpay-dev/backend/internal/auth"
	"github.com/cpay-dev/backend/internal/db"
	"github.com/cpay-dev/backend/internal/events"
	grpcclient "github.com/cpay-dev/backend/internal/grpc"
	"github.com/cpay-dev/backend/internal/storage"
)

type handlers struct {
	queries     *db.Queries
	authSvc     *auth.Service
	storage     *storage.MinIOService
	eventPub    *events.Publisher
	chainClient *grpcclient.ChainClient
}

func RegisterRoutes(
	r chi.Router,
	queries *db.Queries,
	authSvc *auth.Service,
	storageSvc *storage.MinIOService,
	eventPub *events.Publisher,
	chainClient *grpcclient.ChainClient,
) {
	h := &handlers{
		queries:     queries,
		authSvc:     authSvc,
		storage:     storageSvc,
		eventPub:    eventPub,
		chainClient: chainClient,
	}

	r.Route("/api", func(r chi.Router) {
		// Auth (public)
		r.Post("/auth/siwe/nonce", h.generateNonce)
		r.Post("/auth/siwe/verify", h.verifySIWE)

		// Public product page
		r.Get("/p/{id}", h.getPublicProduct)
		r.Get("/p/{id}/payment", h.checkProductPayment)
		r.Post("/p/{id}/payment", h.recordProductPayment)

		// Public payment link page
		r.Get("/pay/{id}", h.getPublicPaymentLink)
		r.Get("/pay/{id}/payment", h.checkPaymentLinkPayment)
		r.Post("/pay/{id}/use", h.recordPaymentLinkUse)
		r.Post("/pay/{id}/payment", h.recordPaymentLinkPayment)

		// Public subscription page
		r.Get("/sub/{id}", h.getPublicSubscription)
		r.Get("/sub/{id}/payment", h.checkSubscriptionPayment)
		r.Post("/sub/{id}/payment", h.recordSubscriptionPayment)
		r.Post("/sub/{id}/subscribe", h.createSubscriber)
		r.Post("/sub/{id}/cancel", h.cancelSubscription)
		r.Get("/sub/{id}/subscriber", h.getSubscriber)

		// Relayer address (public — frontend needs it for approve target)
		r.Get("/relayer-address", h.getRelayerAddress)

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(authSvc.Middleware)

			// User
			r.Get("/user/profile", h.getProfile)
			r.Put("/user/role", h.setRole)
			r.Put("/user/email", h.updateEmail)

			// Shops
			r.Post("/shops", h.createShop)
			r.Get("/shops/me", h.getMyShop)
			r.Put("/shops/{id}", h.updateShop)

			// Products
			r.Post("/products", h.createProduct)
			r.Get("/products", h.listMyProducts)
			r.Get("/products/{id}", h.getProduct)
			r.Put("/products/{id}", h.updateProduct)
			r.Patch("/products/{id}/active", h.toggleProductActive)
			r.Delete("/products/{id}", h.deleteProduct)
			r.Get("/products/{id}/payments", h.listProductPayments)

			// Payment Links
			r.Post("/payment-links", h.createPaymentLink)
			r.Get("/payment-links", h.listMyPaymentLinks)
			r.Patch("/payment-links/{id}/active", h.setPaymentLinkActive)
			r.Delete("/payment-links/{id}", h.deletePaymentLink)

			// Subscriptions
			r.Post("/subscriptions", h.createSubscription)
			r.Get("/subscriptions", h.listMySubscriptions)
			r.Patch("/subscriptions/{id}/active", h.toggleSubscriptionActive)
			r.Delete("/subscriptions/{id}", h.deleteSubscription)
			r.Get("/subscriptions/{id}/payments", h.listSubscriptionPayments)
			r.Get("/subscriptions/{id}/subscribers", h.listSubscribers)

			// Payments ledger
			r.Get("/payments", h.listMyPayments)

			// Counterfactual account
			r.Get("/cf-address", h.getMyCFAddress)
			r.Get("/cf-balance", h.getCFBalance)
			r.Get("/cf-withdraw-info", h.getCFWithdrawInfo)

			// Upload
			r.Post("/upload/avatar", h.uploadAvatar)
			r.Post("/upload/product-image", h.uploadProductImage)
		})
	})
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
