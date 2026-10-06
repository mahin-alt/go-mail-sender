package main

import (
	"encoding/json"
	// "errors"
	"fmt"
	"io"
	"net/http"
	// "os"
	"strings"
)

const keyServerAddr = "serverAddr"

func GetRoot(w http.ResponseWriter, r *http.Request) {	// r is a pointer
	ctx := r.Context()

	fmt.Printf("%s: got / request\n", ctx.Value(keyServerAddr))
	io.WriteString(w, "This is my website!\n")
}

func GetHello(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	fmt.Printf("%s: got /hello request\n", ctx.Value(keyServerAddr))
	io.WriteString(w, "Hello, HTTP!\n")
}

// SendMail accepts a JSON request and sends the email using the SMTP settings
// from the environment.
func SendMail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "Only POST is supported", http.StatusMethodNotAllowed)
		return
	}

	// The JSON shape mirrors the arguments needed by sendMailSimple.
	var request struct {
		Subject string   `json:"subject"`
		Body    string   `json:"body"`
		To      []string `json:"to"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid JSON request", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(request.Subject) == "" ||
		strings.TrimSpace(request.Body) == "" ||
		len(request.To) == 0 {
		http.Error(w, "subject, body, and at least one to address are required", http.StatusBadRequest)
		return
	}

	// Subject is written into an email header, so reject newlines to prevent
	// callers from adding extra headers.
	if strings.ContainsAny(request.Subject, "\r\n") {
		http.Error(w, "subject must not contain a newline", http.StatusBadRequest)
		return
	}

	for _, address := range request.To {
		if strings.TrimSpace(address) == "" {
			http.Error(w, "to addresses must not be empty", http.StatusBadRequest)
			return
		}
	}

	if err := sendMailSimple(request.Subject, request.Body, request.To); err != nil {
		fmt.Printf("email send error: %v\n", err)
		http.Error(w, "Could not send email", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"message": "Email sent"}); err != nil {
		fmt.Printf("email response error: %v\n", err)
	}
}
