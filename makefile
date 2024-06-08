build:
	go build -o proxie -v

clean:
	go clean
	rm -f proxie

format:
	go fmt ./...