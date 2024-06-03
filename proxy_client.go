package main

import (
    "net/http"
    "bufio"
    "io"
    "fmt"
    "os"
    "strings"
)

// ParseReplaceBody takes a body reader and a map of keywords to replace with value strings
// and returns the modified body as a byte array.
func ParseReplaceBody(bodyReader io.Reader, keyWords map[string]string) []byte {
    body, err := io.ReadAll(bodyReader)
    if err != nil {
        fmt.Println("Error reading body:", err)
        return []byte{}
    }
    bodyStr := string(body)
    for key, value := range keyWords {
        bodyStr = strings.ReplaceAll(bodyStr, key, value)
    }
    return []byte(bodyStr)
}

func main() {
    scanner := bufio.NewScanner(os.Stdin)
    fmt.Println("Enter URLs to process. Press Ctrl+D (EOF) to stop.")
    for scanner.Scan() {
        line := scanner.Text()

        // Construct the proxy path
        proxyPath := fmt.Sprintf("http://localhost:8080/proxy/%s", line)

        // Make an HTTP GET request
        res, err := http.Get(proxyPath)
        if err != nil {
            fmt.Println("Error making request:", err)
            continue
        }

        // Ensure the response body is closed after reading
        defer res.Body.Close()

        // Testing parse and replace
        keywords := map[string]string{
            "Google": "TESTING VALUE",
            "google": "SECONDARY TESTING VALUE",
        }
        modifiedBody := ParseReplaceBody(res.Body, keywords)
        fmt.Println(string(modifiedBody))
    }

    if err := scanner.Err(); err != nil {
        fmt.Println("Error reading input:", err)
    }
}
