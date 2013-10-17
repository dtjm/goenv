cover: cover/profile
	go tool cover -func=cover/profile

cover/profile: src/**/*.go
	mkdir -p cover
	for pkg in `go list ./...`; do \
		echo "go test -coverprofile=cover/$$pkg.out $$pkg"; \
		mkdir -p cover/$$pkg; \
		go test -coverprofile=cover/$$pkg.out $$pkg; \
	done
	echo "mode: count" > cover/profile
	for i in `find cover -name *.out`; do \
		tail -n +2 $$i >> cover/profile; \
		done

cover/index.html: cover/profile
	go tool cover -html cover/profile -o cover/index.html

test:
	go test ./...

clean:
	rm -rf cover pkg bin

.PHONY: cover
