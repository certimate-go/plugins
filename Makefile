.PHONY: init dev release-provider test tidy clean index

PLUGIN :=
OUTPUT_DIR := dist
PLUGIN_ROOT ?= $(CURDIR)/$(OUTPUT_DIR)/local-plugins

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
	@echo "  5. make dev PROVIDER_DIR=$(PLUGIN)"

dev:
	@bash .github/scripts/provider-dev.sh

release-provider:
	@bash .github/scripts/request-provider-release.sh

test:
	go test ./...

tidy:
	go mod tidy

index:
	go run ./cmd/genindex -ready "$(READY)" -out-dir "$(INDEX_STAGE)"

clean:
	rm -rf $(OUTPUT_DIR)
