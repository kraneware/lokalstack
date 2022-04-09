SHELL := /bin/bash

TEST_PACKAGES = $(shell find . -name "*_test.go" | sort | rev | cut -d'/' -f2- | rev | uniq)
CURDIR = $(shell pwd)

.DEFAULT_GOAL := package

.PHONY: clean init displavars test

clean:
	@rm -Rf target

init: clean
	@mkdir target
	@mkdir target/testing
	@mkdir target/bin
	@mkdir target/deploy
	@mkdir target/tools

deps: init
	go get -d && go mod tidy

displayvars:
	@for package in $(TEST_PACKAGES); do \
		echo $${package:2}; \
	done

cleanup:
	gofmt -w .
	$(GOPATH)/bin/goimports -w .

test: init
	@for package in $(TEST_PACKAGES); do \
	  echo Testing package $$package ; \
	  cd $(CURDIR)/$$package ; \
	  mkdir -p ${CURDIR}/target/testing/$$package ; \
	  go test -v -race -covermode=atomic -coverprofile ${CURDIR}/target/testing/$$package/coverage.out | tee ${CURDIR}/target/testing/$$package/target.txt ; \
	  if [ "$${PIPESTATUS[0]}" -ne "0" ]; then exit 1; fi; \
	  grep "FAIL!" ${CURDIR}/target/testing/$$package/target.txt ; \
	  if [ "$$?" -ne "1" ]; then exit 1; fi; \
	  cat ${CURDIR}/target/testing/$$package/coverage.out >> ${CURDIR}/target/coverage_profile.out ; \
	done

build: coverage-checks
	@if [ -f lambda-deploy.json ]; then \
	  echo Building lambda target/bin/`cat lambda-deploy.json | python3 -c 'import json,sys;print(json.load(sys.stdin)["handler"])'` ... ; \
	  env GOOS=linux GOARCH=amd64 go build -o target/bin/`cat lambda-deploy.json | python3 -c 'import json,sys;print(json.load(sys.stdin)["handler"])'` ; \
	fi

package: build
	@if [ -f lambda-deploy.json ]; then \
	  echo Packaging lambda target/deploy/`cat lambda-deploy.json | python3 -c 'import json,sys;print(json.load(sys.stdin)["handler"])'`.zip ... ; \
	  zip -D target/deploy/`cat lambda-deploy.json | python3 -c 'import json,sys;print(json.load(sys.stdin)["handler"])'`.zip lambda-deploy.json ; \
	  cd target/bin && zip -Du ../deploy/`cat ../../lambda-deploy.json | python3 -c 'import json,sys;print(json.load(sys.stdin)["handler"])'`.zip * ; \
	fi