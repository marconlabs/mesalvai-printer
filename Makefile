.PHONY: check

# gofmt -l + go vet + go test (tool/check.sh; --fast pula os testes)
check:
	tool/check.sh
