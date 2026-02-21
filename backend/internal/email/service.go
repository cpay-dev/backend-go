package email

import (
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/resend/resend-go/v2"
)

type Service struct {
	client *resend.Client
	from   string
}

func NewService(apiKey, from string) *Service {
	return &Service{
		client: resend.NewClient(apiKey),
		from:   from,
	}
}

func (s *Service) send(to, subject, html string) error {
	params := &resend.SendEmailRequest{
		From:    s.from,
		To:      []string{to},
		Subject: subject,
		Html:    html,
	}
	_, err := s.client.Emails.Send(params)
	if err != nil {
		log.Error().Err(err).Str("to", to).Str("subject", subject).Msg("failed to send email")
		return fmt.Errorf("send email to %s: %w", to, err)
	}
	log.Info().Str("to", to).Str("subject", subject).Msg("email sent")
	return nil
}

func (s *Service) SendReceipt(to, subTitle, amount, txHash string, nextDue time.Time) error {
	subject := fmt.Sprintf("Payment receipt — %s", subTitle)
	html := receiptHTML(subTitle, amount, txHash, nextDue)
	return s.send(to, subject, html)
}

func (s *Service) SendUpcomingPayment(to, subTitle, amount string, dueDate time.Time) error {
	subject := fmt.Sprintf("Upcoming payment — %s", subTitle)
	html := upcomingHTML(subTitle, amount, dueDate)
	return s.send(to, subject, html)
}

func (s *Service) SendPaymentFailed(to, subTitle, amount, reason string) error {
	subject := fmt.Sprintf("Payment failed — %s", subTitle)
	html := failedHTML(subTitle, amount, reason)
	return s.send(to, subject, html)
}

func (s *Service) SendCancelled(to, subTitle string) error {
	subject := fmt.Sprintf("Subscription cancelled — %s", subTitle)
	html := cancelledHTML(subTitle)
	return s.send(to, subject, html)
}

func (s *Service) SendExpired(to, subTitle string) error {
	subject := fmt.Sprintf("Subscription expired — %s", subTitle)
	html := expiredHTML(subTitle)
	return s.send(to, subject, html)
}

func (s *Service) SendPaymentReceipt(to, title, amount, txHash string) error {
	subject := fmt.Sprintf("Payment receipt — %s", title)
	html := paymentReceiptHTML(title, amount, txHash)
	return s.send(to, subject, html)
}

func (s *Service) SendMerchantPaymentNotification(to, subTitle, amount, payerAddress string) error {
	subject := fmt.Sprintf("New payment received — %s", subTitle)
	html := merchantPaymentHTML(subTitle, amount, payerAddress)
	return s.send(to, subject, html)
}
