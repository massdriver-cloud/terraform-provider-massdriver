# Registers an existing cloud asset that is NOT managed by a Massdriver
# bundle so other components can connect to it. Usable anywhere (no
# deployment-token requirement); counterpart to massdriver_resource.

resource "massdriver_imported_resource" "external_network" {
  name          = "Existing Corporate VPC"
  resource_type = "network"

  # JSON-encoded payload conforming to the resource type's schema.
  resource = jsonencode({
    data = {
      infrastructure = {
        cidr       = "10.0.0.0/16"
        network_id = "vpc-0123456789abcdef0"
        subnets = [
          { cidr = "10.0.1.0/24", subnet_id = "subnet-aaa" },
          { cidr = "10.0.2.0/24", subnet_id = "subnet-bbb" },
        ]
      }
    }
    specs = {
      network = {
        cidr = "10.0.0.0/16"
      }
    }
  })
}
