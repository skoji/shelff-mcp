.PHONY: generate test

generate:
	go generate ./...

test: generate
	go test ./... -count=1
