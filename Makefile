# These variables get inserted into ./build/commit.go
BUILD_TIME=$(shell date)
GIT_REVISION=$(shell git rev-parse --short HEAD)
GIT_DIRTY=$(shell git diff-index --quiet HEAD -- || echo "✗-")

ldflags= -X github.com/SkynetLabs/skynet-accounts/build.GitRevision=${GIT_DIRTY}${GIT_REVISION} \
-X "github.com/SkynetLabs/skynet-accounts/build.BuildTime=${BUILD_TIME}"

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
	pip install codespell 1>/dev/null 2>&1
	git ls-files "*.md" :\!:"vendor/**" | xargs codespell --check-filenames

# lint runs golangci-lint (which includes golint, a spellcheck of the codebase,
# and other linters), the custom analyzers, and also a markdown spellchecker.
lint: fmt markdown-spellcheck vet
	golint ./...
	golangci-lint run -c .golangci.yml
	go mod tidy
	analyze -lockcheck -- $(pkgs)

# lint-ci runs golint.
lint-ci:
# golint is skipped on Windows.
ifneq ("$(OS)","Windows_NT")
# Linux
	go get -d golang.org/x/lint/golint
	golint -min_confidence=1.0 -set_exit_status $(pkgs)
	go mod tidy
endif

# Credentials and port we are going to use for our test MongoDB instance.
MONGO_USER=admin
MONGO_PASSWORD=aO4tV5tC1oU3oQ7u
MONGO_PORT=17017

# call_mongo is a helper function that executes a query in an `eval` call to the
# test mongo instance.
define call_mongo
    docker exec skynet-accounts-mongo-test-db mongo -u $(MONGO_USER) -p $(MONGO_PASSWORD) --port $(MONGO_PORT) --eval $(1)
endef

# start-mongo starts a local mongoDB container with no persistence.
# We first prepare for the start of the container by making sure the test
# keyfile has the right permissions, then we clear any potential leftover
# containers with the same name. After we start the container we initialise a
# single node replica set. All the output is discarded because it's noisy and
# if it causes a failure we'll immediately know where it is even without it.
start-mongo:
	-docker stop skynet-accounts-mongo-test-db 1>/dev/null 2>&1
	-docker rm skynet-accounts-mongo-test-db 1>/dev/null 2>&1
	chmod 400 $(shell pwd)/test/fixtures/mongo_keyfile
	docker run \
     --rm \
     --detach \
     --name skynet-accounts-mongo-test-db \
     -p $(MONGO_PORT):$(MONGO_PORT) \
     -e MONGO_INITDB_ROOT_USERNAME=$(MONGO_USER) \
     -e MONGO_INITDB_ROOT_PASSWORD=$(MONGO_PASSWORD) \
     -v $(shell pwd)/test/fixtures/mongo_keyfile:/data/mgkey \
	mongo:4.4.1 mongod --port=$(MONGO_PORT) --replSet=skynet --keyFile=/data/mgkey 1>/dev/null 2>&1
	# wait for mongo to start before we try to configure it
	status=1 ; while [[ $$status -gt 0 ]]; do \
		sleep 1 ; \
		$(call call_mongo,"") 1>/dev/null 2>&1 ; \
		status=$$? ; \
	done
	# Initialise a single node replica set.
	$(call call_mongo,"rs.initiate({_id: \"skynet\", members: [{ _id: 0, host: \"localhost:$(MONGO_PORT)\" }]})") 1>/dev/null 2>&1

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

bench: fmt
	go test -tags='debug testing netgo' -timeout=500s -run=XXX -bench=. $(pkgs) -count=$(count)

test:
	go test -short -tags='debug testing netgo' -timeout=5s $(pkgs) -run=. -count=$(count)

test-long: lint lint-ci
	@mkdir -p cover
	GORACE='$(racevars)' go test -race --coverprofile='./cover/cover.out' -v -failfast -tags='testing debug netgo' -timeout=30s $(pkgs) -run=. -count=$(count)

# These env var values are for testing only. They can be freely changed.
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

.PHONY: all fmt install release clean check test test-int test-long test-single start-mongo stop-mongo
