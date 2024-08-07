package service

import (
	"fmt"
	"net/smtp"
	"os"
)

type EmailService interface {
	Send(to []string, message string) error
	CreateInvitationMail(to string, group string, link string) string
	CreateSignupAndInvitationMail(to string, group string, link string) string
	CreateSignupVerification(to string, link string) string
	CreateResetPassword(to string, link string) string
	CreateRemovedFromGroup(to string, group string) string
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

// Create a group signup invitation flow  mail.
func (service *EmailServiceImpl) CreateSignupAndInvitationMail(to string, group string, link string) string {
	mailHeader := fmt.Sprintf("From:%s\nTo:%s\nSubject: Invitation Link\n\n", service.email, to)
	mailBody := fmt.Sprintf("Hello\nYou have been invited to the group %s, but you are not a user yet!\nFollow this link to sign up and accept the invite: %s", group, link)
	return mailHeader + mailBody
}

// Create signup verification email.
func (service *EmailServiceImpl) CreateSignupVerification(to string, link string) string {
	mailHeader := fmt.Sprintf("From:%s\nTo:%s\nSubject: Verification Link\n\n", service.email, to)
	mailBody := fmt.Sprintf("Hello\nClick here to verify your account: %s", link)
	return mailHeader + mailBody
}

// Create a reset password link.
func (service *EmailServiceImpl) CreateResetPassword(to string, link string) string {
	mailHeader := fmt.Sprintf("From:%s\nTo:%s\nSubject: Reset password link \n\n", service.email, to)
	mailBody := fmt.Sprintf("Hello\nfollow this link to reset your password.\n\n%s", link)
	return mailHeader + mailBody
}

// Create a removed from group email notification.
func (service *EmailServiceImpl) CreateRemovedFromGroup(to string, group string) string {
	mailHeader := fmt.Sprintf("From:%s\nTo:%s\nSubject: Removed from group\n\n", service.email, to)
	mailBody := fmt.Sprintf("Hello\n\n, This is a message to notify you, that you've been removed from the group\t%s\n\n", group)
	return mailHeader + mailBody
}
