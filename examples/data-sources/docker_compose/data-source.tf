# Get information about an existing Docker Compose stack
data "docker_compose" "myapp" {
  project_name = "myapp"
}

output "running_services" {
  value = data.docker_compose.myapp.running_services
}

output "total_services" {
  value = data.docker_compose.myapp.total_services
}

output "services" {
  value = data.docker_compose.myapp.services
}
