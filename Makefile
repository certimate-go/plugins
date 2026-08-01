.PHONY: init build build-all test tidy clean index

PLUGIN := qiniu-cdn
OUTPUT_DIR := dist
BINARY := $(shell jq -r .binary $(PLUGIN)/manifest.json 2>/dev/null)
ICON := $(shell jq -r .icon $(PLUGIN)/manifest.json 2>/dev/null)

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
	@set -eu; \
	bin='$(BINARY)'; \
	if [ -z "$$bin" ] || [ "$$bin" = "null" ]; then echo "Error: $(PLUGIN)/manifest.json has no binary field"; exit 1; fi; \
	icon='$(ICON)'; \
	mkdir -p $(OUTPUT_DIR); \
	for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do \
	  os=$${pair%/*}; arch=$${pair#*/}; \
	  echo "building $(PLUGIN) $$os/$$arch"; \
	  raw=$$(mktemp); \
	  GOOS=$$os GOARCH=$$arch CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o $$raw ./$(PLUGIN); \
	  stage=$$(mktemp -d); \
	  cp $$raw "$$stage/$$bin"; \
	  rm -f $$raw; \
	  cp $(PLUGIN)/manifest.json "$$stage/manifest.json"; \
	  files="$$bin manifest.json"; \
	  if [ -n "$$icon" ] && [ "$$icon" != "null" ] && [ -f "$(PLUGIN)/$$icon" ]; then cp "$(PLUGIN)/$$icon" "$$stage/$$icon"; files="$$files $$icon"; fi; \
	  (cd "$$stage" && zip -q -X "$(CURDIR)/$(OUTPUT_DIR)/$(PLUGIN)_$${os}_$${arch}.zip" $$files); \
	  rm -rf "$$stage"; \
	done

test:
	go test ./...

tidy:
	go mod tidy

index:
	go run ./cmd/genindex -o index.json

clean:
	rm -rf $(OUTPUT_DIR)
