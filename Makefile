.PHONY: server client

server: server.go
	go run server.go

client: client.go
	CGO_CFLAGS=-Wno-deprecated-declarations go build -v client.go
	./client 127.0.0.1:4242
