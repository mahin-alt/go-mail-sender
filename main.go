package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/mail"
	"os"
	"strings"

	"github.com/joho/godotenv"
	"github.com/mailtrap/mailtrap-go"
)

type mailtrapConfig struct {
	token     string
	fromEmail string
}

func loadMailtrapConfig() (mailtrapConfig, error) {
	token := strings.TrimSpace(os.Getenv("MAILTRAP_API_TOKEN"))
	if token == "" || token == "your_mailtrap_api_token" {
		return mailtrapConfig{}, errors.New("MAILTRAP_API_TOKEN is missing; set it in .env")
	}

	fromEmail := strings.TrimSpace(os.Getenv("FROM_EMAIL"))
	parsedEmail, err := mail.ParseAddress(fromEmail)
	if err != nil || parsedEmail.Address != fromEmail {
		return mailtrapConfig{}, errors.New("FROM_EMAIL must be a valid email address")
	}

	return mailtrapConfig{token: token, fromEmail: fromEmail}, nil
}

func sendMailSimple(ctx context.Context, subject string, body string, to []string) error {
	config, err := loadMailtrapConfig()
	if err != nil {
		return err
	}

	client, err := mailtrap.NewClient(config.token)
	if err != nil {
		return fmt.Errorf("create Mailtrap client: %w", err)
	}

	recipients := make([]mailtrap.Address, 0, len(to))
	for _, address := range to {
		recipients = append(recipients, mailtrap.Address{Email: address})
	}

	_, _, err = client.Send(ctx, &mailtrap.SendRequest{
		From:     mailtrap.Address{Email: config.fromEmail, Name: "Mailtrap Test"},
		To:       recipients,
		Subject:  subject,
		Text:     body,
		Category: "Integration Test",
	})
	if err != nil {
		return fmt.Errorf("send email with Mailtrap: %w", err)
	}

	return nil
}

func main() {
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Env load error: ", err)
		return
	}
	if _, err := loadMailtrapConfig(); err != nil {
		fmt.Printf("Mailtrap config error: %v\n", err)
		return
	}

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
		Addr:    ":3333",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			ctx = context.WithValue(ctx, keyServerAddr, l.Addr().String())
			return ctx
		},
	}

	// Server two
	serverTwo := &http.Server{
		Addr:    ":4444",
		Handler: mux,
		BaseContext: func(l net.Listener) context.Context {
			ctx = context.WithValue(ctx, keyServerAddr, l.Addr().String())
			return ctx
		},
	}

	go func() {
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
