.PHONY: build build-all test tidy clean

PLUGIN := webhook-deployer
OUTPUT_DIR := dist

build:
	go build -o $(OUTPUT_DIR)/$(PLUGIN) ./$(PLUGIN)

build-all:
	GOOS=linux   GOARCH=amd64 go build -o $(OUTPUT_DIR)/$(PLUGIN)-linux-amd64   ./$(PLUGIN)
	GOOS=darwin  GOARCH=amd64 go build -o $(OUTPUT_DIR)/$(PLUGIN)-darwin-amd64  ./$(PLUGIN)
	GOOS=darwin  GOARCH=arm64 go build -o $(OUTPUT_DIR)/$(PLUGIN)-darwin-arm64  ./$(PLUGIN)
	GOOS=windows GOARCH=amd64 go build -o $(OUTPUT_DIR)/$(PLUGIN)-windows-amd64.exe ./$(PLUGIN)

test:
	go test ./...

tidy:
	go mod tidy

clean:
	rm -rf $(OUTPUT_DIR)
