.PHONY: test build lint dashboard-build

test:
	go test ./...

build:
	go build ./...

lint:
	go vet ./...
	cd dashboard && npm ci && npm run lint

dashboard-build:
	cd dashboard && npm ci && npm run build
