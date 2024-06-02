package main

import (
    "net/http"
    "io"
    "fmt"
)

func main() {
    s := "www.google.com"
    res, err := http.Post(fmt.Sprintf("http://localhost:8080/proxy/%s", s), "application/json", nil)
    if err != nil {
        fmt.Println(err)
        return;
    }
    body, err := io.ReadAll(res.Body)
    if err != nil {
        fmt.Println(err)
        return;
    }
    fmt.Printf(string(body))
}