package main

import "net/smtp"

func sendMail(config Config) error {
	auth := smtp.PlainAuth(
		"",
		config.Username,
		config.Password,
		config.Host,
	)

	message := []byte(
		"From: " + config.From + "\r\n" +
			"To: " + config.To + "\r\n" +
			"Subject: bye from go\r\n" +
			"\r\n" +
			"ehlo, sending new email\r\n",
	)

	return smtp.SendMail(
		config.Host+":"+config.Port,
		auth,
		config.From,
		[]string{config.To},
		message,
	)
}
