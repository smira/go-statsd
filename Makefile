all: test check bench

.PHONY: deps
deps:
	go get -v -d -t ./...

.PHONY: test
test: deps
	go test -race -v -coverprofile=coverage.txt -covermode=atomic

.PHONY: bench
bench: deps
	go test -v -bench . -benchmem -run nothing ./...

.PHONY: check
check: deps
	golangci-lint run

.PHONY: deps bench test check
