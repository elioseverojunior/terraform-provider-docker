# Pull nginx:latest image
resource "docker_image" "nginx" {
  name = "nginx:latest"
}

# Pull a specific image version
resource "docker_image" "alpine" {
  name         = "alpine:3.19"
  keep_locally = true
}

# Pull from private registry with authentication
resource "docker_image" "private" {
  name = "myregistry.com/myimage:v1.0"

  registry_auth {
    address  = "myregistry.com"
    username = var.registry_username
    password = var.registry_password
  }
}

variable "registry_username" {
  type      = string
  sensitive = true
}

variable "registry_password" {
  type      = string
  sensitive = true
}
