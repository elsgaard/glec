.PHONY: build clean deploy start test test-integration

build:
	go build .

clean:
	rm glec

deploy:

start:
	go run .

test:
	go test -coverprofile=cover.out -short ./...

test-integration:
	go test -coverprofile=cover.out -p 1 ./...