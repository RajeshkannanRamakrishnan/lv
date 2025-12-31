.PHONY: all build run test clean fmt vet

APP_NAME := lv

all: test build

build:
	go build -o $(APP_NAME) main.go

run:
	go run main.go

test:
	go test -v ./...

clean:
	rm -f $(APP_NAME)

fmt:
	go fmt ./...

vet:
	go vet ./...
