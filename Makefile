export KONFLUX_BUILDS=true
FIPS_ENABLED=true
include boilerplate/generated-includes.mk

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update
