package main

import (
	"fmt"
	"net/http"
	"io"
)

func main() {
	res, err := http.Get("http://localhost:8080/test")
	if err != nil {
		fmt.Println(err)
		return
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(string(body))
}