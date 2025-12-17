terraform {
  required_providers {
    docker = {
      source = "registry.terraform.io/elioseverojunior/docker"
    }
  }
}

# Local Docker (default configuration)
provider "docker" {}

# Remote Docker with TLS
# provider "docker" {
#   host       = "tcp://docker-host:2376"
#   tls_verify = true
#   cert_path  = "~/.docker/certs"
# }
