package api

import (
	"bytes"
	"context"
	"fmt"

	"github.com/jung-kurt/gofpdf/v2"
)

func (s *Server) generateAndStoreInvoice(ctx context.Context, merchantID, paymentIntentID string, amount float64, currency, title string) (string, error) {
	if s.minio == nil {
		return "", nil
	}

	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.AddPage()
	pdf.SetFont("Arial", "B", 18)
	pdf.Cell(40, 10, "CPay Invoice")
	pdf.Ln(14)
	pdf.SetFont("Arial", "", 12)
	pdf.Cell(80, 8, fmt.Sprintf("Payment Intent: %s", paymentIntentID))
	pdf.Ln(8)
	pdf.Cell(80, 8, fmt.Sprintf("Merchant: %s", merchantID))
	pdf.Ln(8)
	pdf.Cell(80, 8, fmt.Sprintf("Title: %s", title))
	pdf.Ln(8)
	pdf.Cell(80, 8, fmt.Sprintf("Amount: %.8f %s", amount, currency))

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return "", err
	}

	objectKey := fmt.Sprintf("invoices/%s/%s.pdf", merchantID, paymentIntentID)
	if err := s.minio.PutObjectBytes(ctx, objectKey, "application/pdf", buf.Bytes()); err != nil {
		return "", err
	}
	return objectKey, nil
}
