package main

import (
	// "errors"
	"fmt"
	"io"
	"net/http"
	// "os"
)

func GetRoot(w http.ResponseWriter, r *http.Request) {	// r is a pointer
	fmt.Printf("got / request\n")
	io.WriteString(w, "This is my website!\n")
}

func GetHello(w http.ResponseWriter, r *http.Request) {
	fmt.Printf("got /hello request\n")
	io.WriteString(w, "Hello, HTTP!\n")
}


