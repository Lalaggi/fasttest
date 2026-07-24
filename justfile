build:
    go build -o fasttest .

install:
    go install .

test:
    go test ./...

lint:
    golangci-lint run

run:
    go run .

clean:
    rm -f fasttest
