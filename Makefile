.PHONY: generate test

generate:
	git submodule update --init --recursive
	go generate ./...

test: generate
	go test ./... -count=1
