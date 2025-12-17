# Get information about an existing container
data "docker_container" "my_container" {
  name = "my-container"
}

output "container_id" {
  value = data.docker_container.my_container.container_id
}

output "container_status" {
  value = data.docker_container.my_container.status
}

output "container_ip" {
  value = data.docker_container.my_container.ip_address
}
