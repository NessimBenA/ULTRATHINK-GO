.PHONY: build run clean install

build:
	go build -o ultrathink main.go

run: build
	./ultrathink

clean:
	rm -f ultrathink

install: build
	cp ultrathink /usr/local/bin/

dev:
	go run main.go

test:
	go test ./...

deps:
	go mod tidy
	go mod download