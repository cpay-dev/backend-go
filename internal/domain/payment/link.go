package payment

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

type LinkInput struct {
	Title            string
	PricingMode      string
	Amount           *float64
	Currency         string
	AllowedTokens    int
	Reusable         bool
	MaxPayments      *int
	ExpiresAt        *time.Time
	AfterPaymentType string
	RedirectURL      string
	MinAmount        *float64
	MaxAmount        *float64
}

func ValidateLinkInput(in LinkInput) error {
	if strings.TrimSpace(in.Title) == "" {
		return fmt.Errorf("title is required")
	}
	mode := strings.ToLower(strings.TrimSpace(in.PricingMode))
	if mode != "fixed" && mode != "open" {
		return fmt.Errorf("pricing_mode must be fixed or open")
	}
	if strings.TrimSpace(in.Currency) == "" {
		return fmt.Errorf("currency is required")
	}
	if in.AllowedTokens <= 0 {
		return fmt.Errorf("at least one allowed token is required")
	}
	if mode == "fixed" {
		if in.Amount == nil || *in.Amount <= 0 {
			return fmt.Errorf("amount is required for fixed pricing")
		}
	}
	if mode == "open" {
		if in.MinAmount != nil && *in.MinAmount < 0 {
			return fmt.Errorf("min_amount cannot be negative")
		}
		if in.MaxAmount != nil && *in.MaxAmount <= 0 {
			return fmt.Errorf("max_amount must be positive")
		}
		if in.MinAmount != nil && in.MaxAmount != nil && *in.MinAmount > *in.MaxAmount {
			return fmt.Errorf("min_amount cannot be greater than max_amount")
		}
	}
	if in.MaxPayments != nil && *in.MaxPayments <= 0 {
		return fmt.Errorf("max_payments must be positive")
	}
	if in.ExpiresAt != nil && in.ExpiresAt.Before(time.Now().UTC()) {
		return fmt.Errorf("expires_at must be in the future")
	}
	after := strings.ToLower(strings.TrimSpace(in.AfterPaymentType))
	if after != "" && after != "confirmation_page" && after != "redirect" {
		return fmt.Errorf("after_payment_type must be confirmation_page or redirect")
	}
	if after == "redirect" {
		if strings.TrimSpace(in.RedirectURL) == "" {
			return fmt.Errorf("redirect_url is required when after_payment_type is redirect")
		}
		if _, err := url.ParseRequestURI(in.RedirectURL); err != nil {
			return fmt.Errorf("redirect_url is invalid")
		}
	}
	return nil
}
