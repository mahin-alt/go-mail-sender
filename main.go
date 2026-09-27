package main

import (
	"fmt"
	"net/smtp"
	"github.com/joho/godotenv"
	"os"
)


func sendMailSimple(subject string, body string, to []string){
	username := os.Getenv("FROM_EMAIL")
	password := os.Getenv("FROM_EMAIL_PASSWORD")
	host := os.Getenv("SMTP_HOST")
	port := os.Getenv("SMTP_PORT")

	auth := smtp.PlainAuth(
		"",
		username,
		password,
		host,
	)

	msg := "Subject: " + subject + "\n" + body

	err := smtp.SendMail(
		host + ":" + port,
		auth,
		username,
		to,
		[]byte(msg),
	)

	if err != nil {
		fmt.Println(err)
		return
	}

}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Env load error: ", err)
		return
	} 

	sendMailSimple("Arekta subject", "Arekta body", []string{"mahin.zavisoft@gmail.com"})
}