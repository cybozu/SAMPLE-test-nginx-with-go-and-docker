build:
	docker build -t sample-nginx:latest sample-nginx

test: build
	test/run

.PHONY: build
