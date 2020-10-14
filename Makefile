include boilerplate/generated-includes.mk

.PHONY: boilerplate-update
boilerplate-update:
	@boilerplate/update

.PHONY: build-push
	hack/app_sre_build_deploy.sh
