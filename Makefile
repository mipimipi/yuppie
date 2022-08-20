.PHONY: lint release

lint:
	golangci-lint run 

# create an annotated tag with name RELEASE
# syntax: make release RELEASE=vX.Y.Z
release:
	@if ! [ -z $(RELEASE) ]; then \
		REL=$(RELEASE); \
		git commit -a -s -m "release $(RELEASE)"; \
		git push; \
		git tag -a $(RELEASE) -m "release $(RELEASE)"; \
		git push origin $(RELEASE); \
	fi