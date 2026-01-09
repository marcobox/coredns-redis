#!/bin/bash
set -e

echo "=== Running Go Format Check ==="
gofmt -l . | tee /tmp/gofmt.out
if [ -s /tmp/gofmt.out ]; then
    echo "❌ Found unformatted files (run 'go fmt ./...')"
    exit 1
else
    echo "✓ All files properly formatted"
fi

echo ""
echo "=== Running Go Vet ==="
go vet ./...
echo "✓ Go vet passed"

echo ""
echo "=== Running Shadow Analysis ==="
if command -v shadow &> /dev/null; then
    go vet -vettool=$(which shadow) ./... || echo "⚠ Shadow check found potential issues"
else
    echo "ℹ shadow tool not installed (optional)"
fi

echo ""
echo "=== Checking for Ineffectual Assignments ==="
if command -v ineffassign &> /dev/null; then
    ineffassign ./... || echo "⚠ Ineffassign found potential issues"
else
    echo "ℹ ineffassign tool not installed (optional)"
fi

echo ""
echo "=== Running Tests with Race Detector ==="
go test -race -short ./...
echo "✓ Tests passed with race detector"

echo ""
echo "=== Summary ==="
echo "✓ All basic linting checks passed!"
echo "Note: Advanced linters (golangci-lint, staticcheck) require Go 1.25.5 rebuild"
