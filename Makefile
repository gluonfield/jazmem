sqlc:
	go run github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1 generate

test:
	go test ./...

install:
	GOBIN=/Users/wins/.local/bin go install ./cmd/jazmem ./cmd/jazmem-server
