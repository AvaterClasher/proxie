package main

import (
	"fmt"
	"net/http"
)

func Test(w http.ResponseWriter, r *http.Request) {
    if r.Method == "GET"{
        fmt.Fprintf(w, "Received GET request!\n")
    }
}

func main() {
	http.HandleFunc("/test", Test)
    http.ListenAndServe(":8080", nil)
}