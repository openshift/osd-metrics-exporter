FIPS_ENABLED=true
include boilerplate/generated-includes.mk

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update

# Enable additional golangci-lint rules
GOLANGCI_OPTIONAL_CONFIG := .golangci-extras.yml
