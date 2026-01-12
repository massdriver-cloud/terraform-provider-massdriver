# Local Development Testing

## Build and Install Provider Locally

1. Build the provider:
```bash
cd /Users/coryodaniel/Workspace/massdriver/terraform-provider-massdriver
go build -o terraform-provider-massdriver
```

2. Create the local provider directory:
```bash
mkdir -p ~/.terraform.d/plugins/local/massdriver/massdriver/0.0.1/darwin_arm64
```

3. Copy the built provider:
```bash
cp terraform-provider-massdriver ~/.terraform.d/plugins/local/massdriver/massdriver/0.0.1/darwin_arm64/
```

## Set Up Authentication

Export your Massdriver credentials:
```bash
export MASSDRIVER_ORG_ID="your-org-id"
export MASSDRIVER_API_KEY="your-api-key"
```

You can find these in your Massdriver account settings or from the CLI config:
```bash
cat ~/.massdriver/config.json
```

## Test the Provider

1. Initialize Terraform:
```bash
cd dev
terraform init
```

2. Plan:
```bash
terraform plan
```

3. Apply:
```bash
terraform apply
```

4. Destroy when done:
```bash
terraform destroy
```

## Quick Script

Save this as `dev/test.sh`:

```bash
#!/bin/bash
set -e

# Build provider
cd ..
go build -o terraform-provider-massdriver
mkdir -p ~/.terraform.d/plugins/local/massdriver/massdriver/0.0.1/darwin_arm64
cp terraform-provider-massdriver ~/.terraform.d/plugins/local/massdriver/massdriver/0.0.1/darwin_arm64/

# Test
cd dev
terraform init -upgrade
terraform plan
```
