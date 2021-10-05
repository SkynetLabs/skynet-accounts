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
pkgs = ./ ./api ./build ./database ./email ./hash ./jwt ./lib ./metafetcher ./skynet

# integration-pkgs defines the packages which contain integration tests
integration-pkgs = ./test ./test/api ./test/database ./test/email

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
	golint ./...
	golangci-lint run -c .golangci.yml
	go mod tidy

# lint-ci runs golint.
lint-ci:
# golint is skipped on Windows.
ifneq ("$(OS)","Windows_NT")
# Linux
	go get -d golang.org/x/lint/golint
	golint -min_confidence=1.0 -set_exit_status $(pkgs)
	go mod tidy
endif

# lint-analyze runs the custom analyzers.
lint-analyze:
	analyze -lockcheck -- $(pkgs)

# start-mongo starts a local mongoDB container with no persistence.
# We first prepare for the start of the container by making sure the test
# keyfile has the right permissions, then we clear any potential leftover
# containers with the same name. After we start the container we initialise a
# single node replica set.
start-mongo:
	-docker stop skynet-accounts-mongo-test-db
	-docker rm skynet-accounts-mongo-test-db
	chmod 400 $(shell pwd)/test/fixtures/mongo_keyfile
	docker run \
     --rm \
     --detach \
     --name skynet-accounts-mongo-test-db \
     -p 17017:17017 \
     -e MONGO_INITDB_ROOT_USERNAME=admin \
     -e MONGO_INITDB_ROOT_PASSWORD=aO4tV5tC1oU3oQ7u \
     -v $(shell pwd)/test/fixtures/mongo_keyfile:/data/mgkey \
	mongo:4.4.1 mongod --port=17017 --replSet=skynet --keyFile=/data/mgkey
	sleep 3 # wait for mongo to start before we try to configure it
	# Initialise a single node replica set.
	docker exec skynet-accounts-mongo-test-db mongo -u admin -p aO4tV5tC1oU3oQ7u --port 17017 --eval "rs.initiate({_id: \"skynet\", members: [{ _id: 0, host: \"localhost:17017\" }]})"

stop-mongo:
	-docker stop skynet-accounts-mongo-test-db

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

# check is a development helper that ensures all test files at least build
# without actually running the tests.
check:
	go test --exec=true ./...

bench: clean fmt
	go test -tags='debug testing netgo' -timeout=500s -run=XXX -bench=. $(pkgs) -count=$(count)

test:
	go test -short -tags='debug testing netgo' -timeout=5s $(pkgs) -run=. -count=$(count)

test-long: clean fmt vet lint lint-ci
	@mkdir -p cover
	GORACE='$(racevars)' go test -race --coverprofile='./cover/cover.out' -v -failfast -tags='testing debug netgo' -timeout=30s $(pkgs) -run=. -count=$(count)

# test-int always returns a zero exit value! Only use it manually!
# These env var values are for testing only. They can be freely changed.
test-int: export COOKIE_HASH_KEY="7eb32cfab5014d14394648dae1cf4e606727eee2267f6a50213cd842e61c5bce"
test-int: export COOKIE_ENC_KEY="65d31d12b80fc57df16d84c02a9bb62e2bc3b633388b05e49ef8abfdf0d35cf3"
test-int: test-long start-mongo
	GORACE='$(racevars)' go test -race -v -tags='testing debug netgo' -timeout=300s $(integration-pkgs) -run=. -count=$(count)
	-make stop-mongo

# test-single allows us to run a single integration test.
# Make sure to start MongoDB yourself!
# Example: make test-single RUN=TestHandlers
test-single: export COOKIE_HASH_KEY="7eb32cfab5014d14394648dae1cf4e606727eee2267f6a50213cd842e61c5bce"
test-single: export COOKIE_ENC_KEY="65d31d12b80fc57df16d84c02a9bb62e2bc3b633388b05e49ef8abfdf0d35cf3"
test-single:
	GORACE='$(racevars)' go test -race -v -tags='testing debug netgo' -timeout=300s $(integration-pkgs) -run=$(RUN) -count=$(count)

.PHONY: all fmt install release clean check test test-int test-long test-single stop-mongo
