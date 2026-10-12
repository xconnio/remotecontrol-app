test:
	go test -count=1 ./... -v

lint:
	golangci-lint run

build:
	CGO_ENABLED=0 go build -o bin/remotecontrol-app ./cmd/remotecontrol-app

run:
	go run ./cmd/remotecontrol-app
