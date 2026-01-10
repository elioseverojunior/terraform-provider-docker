# List all Docker networks
data "docker_networks" "all" {}

# Output all network names
output "network_names" {
  value = [for n in data.docker_networks.all.networks : n.name]
}

# Conditional resource creation - only create if network doesn't exist
locals {
  my_network_exists = length([
    for n in data.docker_networks.all.networks : n
    if n.name == "my-app-network"
  ]) > 0
}

resource "docker_network" "my_app" {
  for_each = toset(local.my_network_exists ? [] : ["enabled"])
  name     = "my-app-network"
}
