# Simple bridge network
resource "docker_network" "app" {
  name   = "app-network"
  driver = "bridge"
}

# Internal network (no external access)
resource "docker_network" "internal" {
  name     = "internal-network"
  driver   = "bridge"
  internal = true
}

# Network with custom IPAM configuration
resource "docker_network" "custom" {
  name   = "custom-network"
  driver = "bridge"

  ipam {
    driver = "default"

    config {
      subnet  = "172.28.0.0/16"
      gateway = "172.28.0.1"
    }
  }

  labels = {
    environment = "development"
    managed_by  = "terraform"
  }
}
