IMAGE_REPOSITORY?=app-sre

include boilerplate/generated-includes.mk

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update
