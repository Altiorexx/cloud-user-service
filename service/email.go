package service

import (
	"fmt"
	"net/smtp"
	"os"
	"sync"
)

var (
	email_instance *EmailService
	email_once     sync.Once
)

type EmailService struct {
	email    string
	password string
}

func NewEmailService() *EmailService {

	email_once.Do(func() {

		email_instance = &EmailService{
			email:    os.Getenv("EMAIL_SERVICE_EMAIL"),
			password: os.Getenv("EMAIL_SERVICE_PASSWORD"),
		}

	})

	// return reference
	return email_instance
}

// Create a default organisation invitation mail.
func (service *EmailService) CreateInvitationMail(to string, link string) string {
	mailHeader := fmt.Sprintf("From:%s\nTo:%s\nSubject: Invitation Link\n\n", service.email, to)
	mailBody := fmt.Sprintf("Hej john john x, tryk her din klovn: %s", link)
	return mailHeader + mailBody
}

// Sends a mail.
func (service *EmailService) Send(to []string, message string) error {
	auth := smtp.PlainAuth("", service.email, service.password, "smtp.gmail.com")
	addr := fmt.Sprintf("%s:%d", "smtp.gmail.com", 587)
	if err := smtp.SendMail(addr, auth, service.email, to, []byte(message)); err != nil {
		return err
	}
	return nil
}
