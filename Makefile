.PHONY: init build build-all dev release-provider test tidy clean index

PLUGIN := qiniu-cdn
OUTPUT_DIR := dist
PLUGIN_ROOT ?= $(CURDIR)/$(OUTPUT_DIR)/local-plugins
BINARY := $(shell jq -r .binary $(PLUGIN)/manifest.json 2>/dev/null)
ICON := $(shell jq -r .icon $(PLUGIN)/manifest.json 2>/dev/null)

export PROVIDER_DIR PLUGIN_ROOT REPOSITORY VERSION DRY_RUN

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
	@echo "  1. Edit $(PLUGIN)/schema/config.json to define your provider form"
	@echo "  2. Edit $(PLUGIN)/schema/i18n/zh.json and en.json for translations"
	@echo "  3. If defining a new access type, edit $(PLUGIN)/schema/access.json. Otherwise, delete it and update access_provider_type in manifest.json."
	@echo "  4. Implement Deploy() in $(PLUGIN)/server.go"
	@echo "  5. make build PLUGIN=$(PLUGIN)"

build:
	mkdir -p $(OUTPUT_DIR)/$(PLUGIN)
	go build -o $(OUTPUT_DIR)/$(PLUGIN)/$(BINARY) ./$(PLUGIN)
	cp $(PLUGIN)/manifest.json $(OUTPUT_DIR)/$(PLUGIN)/manifest.json
	@if [ -n "$(ICON)" ] && [ "$(ICON)" != "null" ]; then cp $(PLUGIN)/$(ICON) $(OUTPUT_DIR)/$(PLUGIN)/$(ICON); fi

dev:
	@bash .github/scripts/provider-dev.sh

release-provider:
	@bash .github/scripts/request-provider-release.sh

build-all:
	@set -eu; \
	bin='$(BINARY)'; \
	if [ -z "$$bin" ] || [ "$$bin" = "null" ]; then echo "Error: $(PLUGIN)/manifest.json has no binary field"; exit 1; fi; \
	case "$$bin" in *.exe) ;; *) echo "Error: $(PLUGIN)/manifest.json binary must end in .exe for Windows"; exit 1 ;; esac; \
	icon='$(ICON)'; \
	mkdir -p $(OUTPUT_DIR); \
	for pair in linux/amd64 linux/arm64 linux/arm darwin/amd64 darwin/arm64 windows/amd64 windows/arm64 windows/386; do \
	  os=$${pair%/*}; arch=$${pair#*/}; \
	  goarm=; if [ "$$arch" = arm ]; then goarm=7; fi; \
	  echo "building $(PLUGIN) $$os/$$arch"; \
	  raw=$$(mktemp); \
	  GOOS=$$os GOARCH=$$arch GOARM=$$goarm CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o $$raw ./$(PLUGIN); \
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
	go run ./cmd/genindex -ready "$(READY)" -out-dir "$(INDEX_STAGE)"

clean:
	rm -rf $(OUTPUT_DIR)
