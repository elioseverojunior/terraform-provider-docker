# Docker Compose from file
resource "docker_compose" "app" {
  project_name = "myapp"
  compose_file = "${path.module}/docker-compose.yml"

  # Optional: remove orphan containers
  remove_orphans = true

  # Optional: remove volumes on destroy
  remove_volumes = false
}

# Docker Compose with inline content
resource "docker_compose" "inline" {
  project_name = "inline-app"

  compose_content = <<-YAML
    services:
      web:
        image: nginx:latest
        ports:
          - "8080:80"
      redis:
        image: redis:alpine
  YAML
}

# Docker Compose with build and profiles
resource "docker_compose" "full" {
  project_name   = "full-app"
  compose_file   = "${path.module}/docker-compose.yml"
  env_file       = "${path.module}/.env"
  profiles       = ["development"]
  build          = true
  force_recreate = false
  pull           = "missing"
  remove_orphans = true
}

output "running_services" {
  value = docker_compose.app.running_services
}

output "services" {
  value = docker_compose.app.services
}
