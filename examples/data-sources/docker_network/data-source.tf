# Get information about an existing network
data "docker_network" "bridge" {
  name = "bridge"
}

output "bridge_network_id" {
  value = data.docker_network.bridge.id
}

output "bridge_network_driver" {
  value = data.docker_network.bridge.driver
}
