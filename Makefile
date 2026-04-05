
.PHONY: demo test bench

demo:
	@cd demo && go run main.go

cli:
	@cd demo && go run cli_search.go

test:
	@go test -v

bench:
	@go test -bench=. -benchmem

gen:
	@cd demo/gen_data && go run gen_data.go
