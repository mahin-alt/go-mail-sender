package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/smtp"
	"os"

	"github.com/joho/godotenv"
)


func sendMailSimple(subject string, body string, to []string) error {
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

	// A blank line separates the email headers from the message body.
	msg := "Subject: " + subject + "\r\n\r\n" + body

	err := smtp.SendMail(
		net.JoinHostPort(host, port),
		auth,
		username,
		to,
		[]byte(msg),
	)

	if err != nil {
		return err
	}

	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Env load error: ", err)
		return
	} 

	// sendMailSimple("Arekta subject", "Arekta body", []string{"mahin.zavisoft@gmail.com"})

	// Default server multiplexer and default HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/", GetRoot)
	mux.HandleFunc("/hello", GetHello)
	// Example: POST /send-mail with {"subject":"Hi","body":"Hello","to":["you@example.com"]}.
	mux.HandleFunc("/send-mail", SendMail)

	// err = http.ListenAndServe(":3333", mux)

	// if errors.Is(err, http.ErrServerClosed) {
	// 	fmt.Printf("Server closed\n")
	// } else if err != nil {
	// 	fmt.Printf("Error starting server: %s\n", err)
	// 	os.Exit(1)
	// }

	// This context is application/server level context and background is empty initial context with no deadlines, cancel, cause etc
	//   
	ctx, cancelCtx := context.WithCancel(context.Background())
	// Server one
	serverOne := &http.Server{
		Addr: ":3333",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			ctx = context.WithValue(ctx, keyServerAddr, l.Addr().String())
			return ctx
		},
	}

	// Server two
	serverTwo := &http.Server{
		Addr: ":4444",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context{
			ctx = context.WithValue(ctx, keyServerAddr, l.Addr().String())
			return ctx
		},
	}

	go func ()  {
		err := serverOne.ListenAndServe()

		if errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("Server one closed\n")
		} else if err != nil {
			fmt.Printf("error listening for server one: %s\n", err)
		}

		cancelCtx()
	}()

	  go func() {
        err := serverTwo.ListenAndServe()
        if errors.Is(err, http.ErrServerClosed) {
            fmt.Printf("server two closed\n")
        } else if err != nil {
            fmt.Printf("error listening for server two: %s\n", err)
        }
        cancelCtx()
    }()
	<-ctx.Done()

}
