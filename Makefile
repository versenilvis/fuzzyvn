
.PHONY: demo test bench

demo:
	@go run demo/main.go

cli:
	@go run demo/cli_search.go

test:
	@go test -v

bench:
	@go test -bench=. -benchmem

gen:
	@cd demo/gen_data && go run gen_data.go
