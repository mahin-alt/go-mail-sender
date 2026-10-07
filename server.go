package main

import (
	// "errors"
	"fmt"
	"io"
	"net/http"
	// "os"
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

// Middleware
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Printf("started %s %s\n", r.Method, r.URL.Path)

		next.ServeHTTP(w, r)

		fmt.Printf("completed %s %s\n", r.Method, r.URL.Path)
	})
}

