package service

import (
	"fmt"
	"net/smtp"
	"os"
)

type EmailService interface {
	Send(to []string, message string) error
	CreateInvitationMail(to string, group string, link string) string
	CreateSignupAndInvitationMail(to string) error
	CreateSignupVerification(to string, link string) string
	CreateResetPassword(to string, link string) string
}

type EmailServiceOpts struct{}

type EmailServiceImpl struct {
	email    string
	password string
}

func NewEmailService() *EmailServiceImpl {
	return &EmailServiceImpl{
		email:    os.Getenv("EMAIL_SERVICE_EMAIL"),
		password: os.Getenv("EMAIL_SERVICE_PASSWORD"),
	}
}

// Sends a mail.
func (service *EmailServiceImpl) Send(to []string, message string) error {
	auth := smtp.PlainAuth("", service.email, service.password, "smtp.gmail.com")
	addr := fmt.Sprintf("%s:%d", "smtp.gmail.com", 587)
	if err := smtp.SendMail(addr, auth, service.email, to, []byte(message)); err != nil {
		return err
	}
	return nil
}

// Create a default group invitation mail notification.
func (service *EmailServiceImpl) CreateInvitationMail(to string, group string, link string) string {
	mailHeader := fmt.Sprintf("From:%s\nTo:%s\nSubject: Invitation Link\n\n", service.email, to)
	mailBody := fmt.Sprintf("Hello\nYou have been invited to the group %s.\nFollow this link to accept the invite: %s", group, link)
	return mailHeader + mailBody
}

// Create a group invitation + signup mail.
func (service *EmailServiceImpl) CreateSignupAndInvitationMail(to string) error {
	return nil
}

// Create signup verification email.
func (service *EmailServiceImpl) CreateSignupVerification(to string, link string) string {
	mailHeader := fmt.Sprintf("From:%s\nTo:%s\nSubject: Verification Link\n\n", service.email, to)
	mailBody := fmt.Sprintf("Hej john john x, tryk her din klovn: %s", link)
	return mailHeader + mailBody
}

// Create a reset password link.
func (service *EmailServiceImpl) CreateResetPassword(to string, link string) string {
	mailHeader := fmt.Sprintf("From:%s\nTo:%s\nSubject: Reset password link \n\n", service.email, to)
	mailBody := fmt.Sprintf("Hello, follow this link to reset your password.\n\n%s", link)
	return mailHeader + mailBody
}