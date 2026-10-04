package main

import (
	"errors"
	"fmt"
	"net/http"
	"net/smtp"
	"os"

	"github.com/joho/godotenv"
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

	// sendMailSimple("Arekta subject", "Arekta body", []string{"mahin.zavisoft@gmail.com"})
	http.HandleFunc("/", GetRoot)
	http.HandleFunc("/hello", GetHello)

	err = http.ListenAndServe(":3333", nil)

	if errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("Server closed\n")
	} else if err != nil {
		fmt.Printf("Error starting server: %s\n", err)
		os.Exit(1)
	} 
}
