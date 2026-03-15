build:
    go build -o bin/imago ./cmd/imago

install: build
    cp bin/imago ~/.local/bin/imago

test:
    go test ./...

vet:
    go vet ./...
