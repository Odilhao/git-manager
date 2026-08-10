PKGNAME := git-manager
VERSION := $(shell git describe)

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
