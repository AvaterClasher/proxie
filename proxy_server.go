package main

import (
    "net/http"
    "io"
    "fmt"
    "os"
    "bufio"
    "log"
    "strconv"
)

type Proxy struct {
    BlockedSites map[string]string
    port int
}

func CreateProxy(port int) *Proxy {
    rv := new(Proxy)
    rv.BlockedSites = make(map[string]string)
    rv.port = port
    return rv
}

func (p *Proxy) ReadConfig(path string) {
    file, err := os.Open(path)
    if err != nil {
        log.Fatalf("Error opening config file: %v", err)
    }
    defer file.Close()

    scanner := bufio.NewScanner(file)
    for scanner.Scan() {
        site := scanner.Text()
        p.BlockedSites[site] = site
    }

    if err := scanner.Err(); err != nil {
        log.Fatalf("Error reading config file: %v", err)
    }
}

func (p *Proxy) StartServer() {
    http.HandleFunc("/", p.HandleRequest)
    port := fmt.Sprintf(":%d", p.port)
    log.Printf("Starting server on port %d", p.port)
    if err := http.ListenAndServe(port, nil); err != nil {
        log.Fatalf("Error starting server: %v", err)
    }
}

func (p *Proxy) HandleRequest(w http.ResponseWriter, r *http.Request) {
    _, blocked := p.BlockedSites[r.URL.Host]
    if blocked {
        log.Println("Blocked site!")
        http.Error(w, "Blocked site!", http.StatusForbidden)
        return
    }

    requestPath := fmt.Sprintf("%s://%s%s", r.URL.Scheme, r.URL.Host, r.URL.Path)
    newRequest, err := http.NewRequest(r.Method, requestPath, r.Body)
    if err != nil {
        log.Printf("Error creating new request: %v", err)
        http.Error(w, "Internal server error", http.StatusInternalServerError)
        return
    }

    log.Printf("Sending %s request to %s\n", r.Method, requestPath)
    client := &http.Client{}
    res, err := client.Do(newRequest)
    if err != nil {
        log.Printf("Error sending request: %v", err)
        http.Error(w, "Error forwarding request", http.StatusBadGateway)
        return
    }
    defer res.Body.Close()

    for key, slice := range res.Header {
        for _, val := range slice {
            w.Header().Add(key, val)
        }
    }

    if _, err := io.Copy(w, res.Body); err != nil {
        log.Printf("Error copying response body: %v", err)
        http.Error(w, "Error copying response body", http.StatusInternalServerError)
    }
}

func main() {
    args := os.Args
    if len(args) != 2 {
        fmt.Println("Arguments: [port]")
        return
    }

    port, err := strconv.Atoi(args[1])
    if err != nil {
        fmt.Println("[port] must be an integer!")
        return
    }

    p := CreateProxy(port)
    p.ReadConfig("blocked.txt")
    p.StartServer()
}
