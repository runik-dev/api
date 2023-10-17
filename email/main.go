package email

import (
	"fmt"
	"net/smtp"
)

type EmailSender struct {
	Host     string
	Port     string
	Username string
	Password string
	Email    string
	Auth     smtp.Auth
}

func NewEmailSender(host, port, username, password, email string) *EmailSender {
	auth := smtp.PlainAuth("", username, password, host)
	return &EmailSender{
		Host:     host,
		Port:     port,
		Username: username,
		Password: password,
		Email:    email,
		Auth:     auth,
	}
}

func (sender *EmailSender) SendEmail(recipient, subject, message string) error {
	emailBody := "To: " + recipient + "\r\n" +
		"Subject: " + subject + "\r\n\r\n" +
		message

	if sender == nil {
		return fmt.Errorf("sender is nil")
	}
	err := smtp.SendMail(sender.Host+":"+sender.Port, sender.Auth, sender.Email, []string{recipient}, []byte(emailBody))
	if err != nil {
		fmt.Println("err" + err.Error())
		return err
	}

	return nil
}
