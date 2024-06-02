package main;

import (
    "net/http"
    "bufio"
    "io"
    "fmt"
    "os"
)

func main() {
    scanner := bufio.NewScanner(os.Stdin)
    for scanner.Scan() {
        line := scanner.Text()

        // localhost:8080/proxy/[TARGET URL]
        proxy_path := fmt.Sprintf("http://localhost:8080/proxy/%s", line)

        res, err := http.Get(proxy_path)
        if err != nil {
            fmt.Println(err)
            continue
        }

        body, err := io.ReadAll(res.Body)
        if err != nil {
            fmt.Println(err)
            return;
        }
        fmt.Println(string(body))
    }
}