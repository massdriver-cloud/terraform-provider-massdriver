#!/bin/bash
set -e

echo "Building provider..."
cd "$(dirname "$0")/.."
go build -o terraform-provider-massdriver

echo "Installing provider locally..."
mkdir -p ~/.terraform.d/plugins/local/massdriver/massdriver/0.0.1/darwin_arm64
cp terraform-provider-massdriver ~/.terraform.d/plugins/local/massdriver/massdriver/0.0.1/darwin_arm64/

echo "Initializing OpenTofu..."
cd dev
rm -rf .terraform .terraform.lock.hcl
tofu init

echo "Running plan..."
tofu plan

echo ""
echo "✅ Provider built and installed successfully!"
echo "Run 'tofu apply' to create the project"
