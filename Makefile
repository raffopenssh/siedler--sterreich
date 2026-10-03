.PHONY: build clean stop start restart test

build:
	go build -o siedler ./cmd/srv

clean:
	rm -f siedler

test:
	go test ./...
