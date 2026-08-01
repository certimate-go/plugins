.PHONY: init build build-all test tidy clean index

PLUGIN := qiniu-cdn
OUTPUT_DIR := dist

init:
	@# Validate plugin name
	@if [ "$(origin PLUGIN)" = "file" ]; then echo "Error: PLUGIN is required. Usage: make init PLUGIN=<name>"; exit 1; fi
	@if ! echo "$(PLUGIN)" | grep -qE '^[a-z][a-z0-9-]+$$'; then echo "Error: PLUGIN must match ^[a-z][a-z0-9-]+$$ (kebab-case, starts with letter)"; exit 1; fi
	@if [ -d "$(PLUGIN)" ]; then echo "Error: directory '$(PLUGIN)' already exists"; exit 1; fi
	@# Copy template
	cp -r _template $(PLUGIN)
	@# Substitute placeholders
	find $(PLUGIN) -type f \( -name '*.json' -o -name '*.go' \) -exec sed -i '' 's/__PLUGIN_NAME__/$(PLUGIN)/g' {} +
	@echo "Plugin scaffolded at $(PLUGIN)/"
	@echo "Next steps:"
	@echo "  1. Edit $(PLUGIN)/schema/deploy.json to define your deploy form"
	@echo "  2. Edit $(PLUGIN)/schema/i18n/zh.json and en.json for translations"
	@echo "  3. If defining a new access type, edit $(PLUGIN)/schema/access.json. Otherwise, delete it and update access_provider_type in manifest.json."
	@echo "  4. Implement Deploy() in $(PLUGIN)/server.go"
	@echo "  5. make build PLUGIN=$(PLUGIN)"

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

index:
	go run ./cmd/genindex -o index.json

clean:
	rm -rf $(OUTPUT_DIR)
