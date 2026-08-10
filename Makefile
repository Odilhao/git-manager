PKGNAME := git-manager
VERSION := $(shell git describe)

build:
	CGO_ENABLED=0 go build -o $(PKGNAME) ./cmd/git-manager

dist: $(PKGNAME)-$(VERSION).tar.gz
	echo $(PKGNAME)-$(VERSION)

$(PKGNAME)-$(VERSION).tar.gz:
	go mod vendor
	tar --create \
		--gzip \
		--file /tmp/$(PKGNAME)-$(VERSION).tar.gz \
		--exclude=.git \
		--exclude=.vscode \
		--exclude=.github \
		--exclude=.gitignore \
		--exclude=.copr \
		--exclude=.packit.yml \
		--transform s/^\./$(PKGNAME)-$(VERSION)/ \
		. && mv /tmp/$(PKGNAME)-$(VERSION).tar.gz .
	rm -rf ./vendor
	@echo $(PKGNAME)-$(VERSION).tar.gz

new-release:
	@if [ -z "$(VERSION)" ]; then \
		echo "Usage: make new-release VERSION=X.Y.Z"; \
		exit 1; \
	fi
	@if [ -n "$$(git status --porcelain)" ]; then \
		echo "Error: working tree is dirty. Commit or stash changes first."; \
		exit 1; \
	fi
	@if [ "$$(git rev-parse --abbrev-ref HEAD)" != "main" ]; then \
		echo "Error: not on main branch. Switch to main before releasing."; \
		exit 1; \
	fi
	@sed -i 's/^\(Version:\).*$$/\1        $(VERSION)/' .rpm/git-manager.spec
	@git add .rpm/git-manager.spec
	@git commit -m "chore(release): bump rpm spec version to $(VERSION)"
	@git tag $(VERSION)
	@git push origin main $(VERSION)
