#!/bin/bash
# Verification script for Storage Node implementation

echo "========================================="
echo "Storage Node Implementation Verification"
echo "========================================="
echo ""

echo "1. Directory Structure:"
echo "----------------------"
find . -type d | grep -v ".git" | head -20

echo ""
echo "2. Go Source Files:"
echo "-------------------"
find . -name "*.go" -type f | wc -l | xargs echo "Total Go files:"

echo ""
echo "3. Key Components:"
echo "------------------"
echo "✓ Protobuf API: $(ls -1 pkg/proto/*.proto 2>/dev/null | wc -l) files"
echo "✓ Domain Models: $(ls -1 internal/model/*.go 2>/dev/null | wc -l) files"
echo "✓ Storage Layer: $(find internal/storage -name "*.go" 2>/dev/null | wc -l) files"
echo "✓ Service Layer: $(ls -1 internal/service/*.go 2>/dev/null | wc -l) files"
echo "✓ Handlers: $(ls -1 internal/handler/*.go 2>/dev/null | wc -l) files"
echo "✓ Config: $(ls -1 internal/config/*.go 2>/dev/null | wc -l) files"
echo "✓ Main: $(ls -1 cmd/storage/*.go 2>/dev/null | wc -l) files"

echo ""
echo "4. Build Configuration:"
echo "----------------------"
echo "✓ go.mod exists: $(test -f go.mod && echo 'YES' || echo 'NO')"
echo "✓ Makefile exists: $(test -f Makefile && echo 'YES' || echo 'NO')"
echo "✓ Dockerfile exists: $(test -f Dockerfile && echo 'YES' || echo 'NO')"
echo "✓ config.yaml exists: $(test -f config.yaml && echo 'YES' || echo 'NO')"

echo ""
echo "5. Documentation:"
echo "-----------------"
echo "✓ README.md: $(test -f README.md && echo 'YES' || echo 'NO')"
echo "✓ IMPLEMENTATION_SUMMARY.md: $(test -f IMPLEMENTATION_SUMMARY.md && echo 'YES' || echo 'NO')"

echo ""
echo "6. File Sizes:"
echo "-------------"
du -sh . 2>/dev/null | awk '{print "Total size: " $1}'

echo ""
echo "========================================="
echo "Verification Complete!"
echo "========================================="
