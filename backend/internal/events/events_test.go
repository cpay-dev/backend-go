package events

import (
	"encoding/json"
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	data := map[string]string{
		"product_id": "prod-1",
		"shop_id":    "shop-1",
		"name":       "Widget",
	}

	evt, err := NewEvent(EventProductCreated, SubjectProducts, data)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	if evt.ID == "" {
		t.Error("event ID is empty")
	}
	if evt.Type != EventProductCreated {
		t.Errorf("Type = %q, want %q", evt.Type, EventProductCreated)
	}
	if evt.Subject != SubjectProducts {
		t.Errorf("Subject = %q, want %q", evt.Subject, SubjectProducts)
	}
	if evt.Timestamp.IsZero() {
		t.Error("Timestamp is zero")
	}
	if time.Since(evt.Timestamp) > 5*time.Second {
		t.Error("Timestamp is too old")
	}

	// Verify data is marshaled correctly
	var decoded map[string]string
	if err := json.Unmarshal(evt.Data, &decoded); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if decoded["product_id"] != "prod-1" {
		t.Errorf("product_id = %q, want prod-1", decoded["product_id"])
	}
	if decoded["shop_id"] != "shop-1" {
		t.Errorf("shop_id = %q, want shop-1", decoded["shop_id"])
	}
	if decoded["name"] != "Widget" {
		t.Errorf("name = %q, want Widget", decoded["name"])
	}
}

func TestNewEvent_UniqueIDs(t *testing.T) {
	evt1, err := NewEvent(EventProductCreated, SubjectProducts, map[string]string{})
	if err != nil {
		t.Fatalf("NewEvent 1: %v", err)
	}
	evt2, err := NewEvent(EventProductCreated, SubjectProducts, map[string]string{})
	if err != nil {
		t.Fatalf("NewEvent 2: %v", err)
	}

	if evt1.ID == evt2.ID {
		t.Error("two events have the same ID")
	}
}

func TestNewEvent_InvalidData(t *testing.T) {
	// Functions can't be marshaled to JSON
	_, err := NewEvent(EventProductCreated, SubjectProducts, func() {})
	if err == nil {
		t.Error("expected error for unmarshalable data")
	}
}

func TestSubjectConstants(t *testing.T) {
	subjects := map[string]string{
		"SubjectUsers":         SubjectUsers,
		"SubjectShops":         SubjectShops,
		"SubjectProducts":      SubjectProducts,
		"SubjectPaymentLinks":  SubjectPaymentLinks,
		"SubjectPayments":      SubjectPayments,
		"SubjectSubscriptions": SubjectSubscriptions,
		"SubjectSubscribers":   SubjectSubscribers,
		"SubjectEmails":        SubjectEmails,
		"SubjectWithdrawals":   SubjectWithdrawals,
	}

	for name, value := range subjects {
		if value == "" {
			t.Errorf("%s is empty", name)
		}
	}

	// Verify no duplicates
	seen := make(map[string]string)
	for name, value := range subjects {
		if prev, exists := seen[value]; exists {
			t.Errorf("duplicate subject value %q: %s and %s", value, prev, name)
		}
		seen[value] = name
	}
}

func TestEventTypeConstants(t *testing.T) {
	// Verify the new withdrawal event type
	if EventWithdrawalCompleted != "withdrawal.completed" {
		t.Errorf("EventWithdrawalCompleted = %q, want withdrawal.completed", EventWithdrawalCompleted)
	}
	if SubjectWithdrawals != "withdrawals" {
		t.Errorf("SubjectWithdrawals = %q, want withdrawals", SubjectWithdrawals)
	}
}

func TestNewEvent_WithdrawalEvent(t *testing.T) {
	data := map[string]string{
		"withdrawal_id": "wd-1",
		"shop_id":       "shop-1",
		"token_address": "0xToken",
		"chain_id":      "80002",
		"amount":        "1000000",
		"tx_hash":       "0xabc123",
	}

	evt, err := NewEvent(EventWithdrawalCompleted, SubjectWithdrawals, data)
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}

	if evt.Type != EventWithdrawalCompleted {
		t.Errorf("Type = %q, want %q", evt.Type, EventWithdrawalCompleted)
	}
	if evt.Subject != SubjectWithdrawals {
		t.Errorf("Subject = %q, want %q", evt.Subject, SubjectWithdrawals)
	}

	var decoded map[string]string
	if err := json.Unmarshal(evt.Data, &decoded); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if decoded["withdrawal_id"] != "wd-1" {
		t.Errorf("withdrawal_id = %q, want wd-1", decoded["withdrawal_id"])
	}
	if decoded["tx_hash"] != "0xabc123" {
		t.Errorf("tx_hash = %q, want 0xabc123", decoded["tx_hash"])
	}
}

func TestEmailQueuedData_JSON(t *testing.T) {
	data := EmailQueuedData{
		EmailType:    EmailTypePaymentReceipt,
		To:           "user@example.com",
		Title:        "Widget Purchase",
		Amount:       "100.00",
		TxHash:       "0xabc",
		PayerAddress: "0xPayer",
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded EmailQueuedData
	if err := json.Unmarshal(bytes, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.EmailType != EmailTypePaymentReceipt {
		t.Errorf("EmailType = %q, want %q", decoded.EmailType, EmailTypePaymentReceipt)
	}
	if decoded.To != "user@example.com" {
		t.Errorf("To = %q, want user@example.com", decoded.To)
	}
}

func TestEmailQueuedData_OmitEmpty(t *testing.T) {
	data := EmailQueuedData{
		EmailType: EmailTypeCancelled,
		To:        "user@example.com",
		Title:     "Sub Cancelled",
	}

	bytes, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(bytes, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// These fields have omitempty and should not be present
	for _, field := range []string{"amount", "tx_hash", "next_due", "reason", "payer_address"} {
		if _, exists := raw[field]; exists {
			t.Errorf("field %q should be omitted when empty", field)
		}
	}
}
