# These variables get inserted into ./build/commit.go
BUILD_TIME=$(shell date)
GIT_REVISION=$(shell git rev-parse --short HEAD)
GIT_DIRTY=$(shell git diff-index --quiet HEAD -- || echo "✗-")

ldflags= -X github.com/NebulousLabs/skynet-accounts/build.GitRevision=${GIT_DIRTY}${GIT_REVISION} \
-X "github.com/NebulousLabs/skynet-accounts/build.BuildTime=${BUILD_TIME}"

racevars= history_size=3 halt_on_error=1 atexit_sleep_ms=2000

# all will build and install release binaries
all: release

# count says how many times to run the tests.
count = 1
# pkgs changes which packages the makefile calls operate on. run changes which
# tests are run during testing.
pkgs = ./ ./api ./build ./database

# integration-pkgs defines the packages which contain integration tests
integration-pkgs = ./test

# fmt calls go fmt on all packages.
fmt:
	gofmt -s -l -w $(pkgs)

# vet calls go vet on all packages.
# We don't check composite literals because we need to use unkeyed fields for
# MongoDB's BSONs and that sets vet off.
# NOTE: go vet requires packages to be built in order to obtain type info.
vet:
	go vet -composites=false $(pkgs)

# markdown-spellcheck runs codespell on all markdown files that are not
# vendored.
markdown-spellcheck:
	git ls-files "*.md" :\!:"vendor/**" | xargs codespell --check-filenames

# lint runs golangci-lint (which includes golint, a spellcheck of the codebase,
# and other linters), the custom analyzers, and also a markdown spellchecker.
lint: markdown-spellcheck lint-analyze
	golangci-lint run -c .golangci.yml

# lint-ci runs golint.
lint-ci:
# golint is skipped on Windows.
ifneq ("$(OS)","Windows_NT")
# Linux
	go get golang.org/x/lint/golint
	golint -min_confidence=1.0 -set_exit_status $(pkgs)
endif

# lint-analyze runs the custom analyzers.
lint-analyze:
	analyze -lockcheck -- $(pkgs)

# start-mongo starts a local mongoDB container with no persistence.
# The first command stops any running testing container but if there is none the
# error is ignored.
start-mongo:
	-docker stop skynet-accounts-mongo-test-db
	docker run \
     --rm \
     --detach \
     --name skynet-accounts-mongo-test-db \
     -p 127.0.0.1:37017:27017 \
     -e MONGO_INITDB_ROOT_USERNAME=admin \
     -e MONGO_INITDB_ROOT_PASSWORD=ivolocalpass \
     mongo

stop-mongo:
	docker stop skynet-accounts-mongo-test-db

# debug builds and installs debug binaries. This will also install the utils.
debug:
	go install -tags='debug profile netgo' -ldflags='$(ldflags)' $(pkgs)
debug-race:
	GORACE='$(racevars)' go install -race -tags='debug profile netgo' -ldflags='$(ldflags)' $(pkgs)

# dev builds and installs developer binaries. This will also install the utils.
dev:
	go install -tags='dev debug profile netgo' -ldflags='$(ldflags)' $(pkgs)
dev-race:
	GORACE='$(racevars)' go install -race -tags='dev debug profile netgo' -ldflags='$(ldflags)' $(pkgs)

# release builds and installs release binaries.
release:
	go install -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs)
release-race:
	GORACE='$(racevars)' go install -race -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs)
release-util:
	go install -tags='netgo' -ldflags='-s -w $(ldflags)' $(release-pkgs) $(util-pkgs)

test:
	go test -short -tags='debug testing netgo' -timeout=5s $(pkgs) -run=. -count=$(count)
test-long: clean fmt vet lint-ci
	@mkdir -p cover
	GORACE='$(racevars)' go test -race --coverprofile='./cover/cover.out' -v -failfast -tags='testing debug netgo' -timeout=3600s $(pkgs) -run=. -count=$(count)

# test-int always returns a zero exit value! Only use it manually!
test-int: clean fmt vet lint-ci start-mongo
	@mkdir -p cover
	GORACE='$(racevars)' go test -race --coverprofile='./cover/cover.out' -v -tags='testing debug netgo' -timeout=3600s $(integration-pkgs) -run=. -count=$(count) ; \
	make stop-mongo

.PHONY: all fmt install release clean test test-int test-long stop-mongo
