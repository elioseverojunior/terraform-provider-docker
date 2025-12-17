# Terraform Provider for Docker

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go)](https://go.dev/)
[![Terraform](https://img.shields.io/badge/Terraform-1.0+-7B42BC?style=flat&logo=terraform)](https://www.terraform.io/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

A comprehensive Terraform provider for managing Docker resources including Docker Engine, Docker Swarm, and Docker Hub. This provider combines features from multiple Docker providers to offer a unified infrastructure-as-code experience.

## Features

### Docker Engine

- **Images**: Pull, manage, and tag Docker images
- **Containers**: Create and manage Docker containers with full configuration support
- **Networks**: Create and manage Docker networks (bridge, overlay, macvlan)
- **Volumes**: Create and manage Docker volumes
- **Compose**: Deploy Docker Compose stacks using the Compose SDK

### Docker Swarm

- **Services**: Deploy and manage Swarm services with rolling updates
- **Secrets**: Manage Swarm secrets securely
- **Configs**: Manage Swarm configs

### Docker Registry

- **Registry Images**: Push images to registries with authentication
- **Tags**: Create and manage image tags

### Docker Hub

- **Repositories**: Create and manage Docker Hub repositories
- **Organizations**: Manage organization teams and members
- **Access Tokens**: Manage Personal Access Tokens (PATs)
- **Permissions**: Manage repository team permissions

## Requirements

- [Terraform](https://www.terraform.io/downloads.html) >= 1.0
- [Go](https://golang.org/doc/install) >= 1.24 (for building)
- [Docker](https://docs.docker.com/get-docker/) (running)

## Installation

### From Source

```bash
git clone https://github.com/elioseverojunior/terraform-provider-docker.git
cd terraform-provider-docker
make install
```

### Manual Installation

```bash
go build -o terraform-provider-docker
mkdir -p ~/.terraform.d/plugins/registry.terraform.io/elioseverojunior/docker/0.1.0/$(go env GOOS)_$(go env GOARCH)
cp terraform-provider-docker ~/.terraform.d/plugins/registry.terraform.io/elioseverojunior/docker/0.1.0/$(go env GOOS)_$(go env GOARCH)/
```

## Quick Start

### Basic Usage

```hcl
terraform {
  required_providers {
    docker = {
      source = "elioseverojunior/docker"
    }
  }
}

provider "docker" {}

# Pull an image
resource "docker_image" "nginx" {
  name = "nginx:latest"
}

# Create a container
resource "docker_container" "nginx" {
  name  = "nginx-server"
  image = docker_image.nginx.name

  ports {
    internal = 80
    external = 8080
  }
}

output "container_id" {
  value = docker_container.nginx.id
}
```

```bash
terraform init
terraform apply
```

## Provider Configuration

### Docker Engine (Local)

```hcl
provider "docker" {
  # Uses default Docker socket
  # host = "unix:///var/run/docker.sock"
}
```

### Docker Engine (Remote with TLS)

```hcl
provider "docker" {
  host       = "tcp://docker-host:2376"
  tls_verify = true
  cert_path  = "/path/to/certs"
}

# Or with inline certificates
provider "docker" {
  host       = "tcp://docker-host:2376"
  tls_verify = true
  ca_cert    = file("ca.pem")
  cert       = file("cert.pem")
  key        = file("key.pem")
}
```

### Docker Hub

```hcl
provider "docker" {
  # Docker Engine configuration
  host = "unix:///var/run/docker.sock"

  # Docker Hub configuration
  hub_username = "your-username"
  hub_password = "your-password"
  # Or use a Personal Access Token
  # hub_token = "your-pat"
}
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `DOCKER_HOST` | Docker daemon socket |
| `DOCKER_TLS_VERIFY` | Enable TLS verification |
| `DOCKER_CERT_PATH` | Path to TLS certificates |
| `DOCKER_HUB_USERNAME` | Docker Hub username |
| `DOCKER_HUB_PASSWORD` | Docker Hub password |
| `DOCKER_HUB_TOKEN` | Docker Hub Personal Access Token |

## Examples

### Docker Compose Stack

```hcl
resource "docker_compose" "app" {
  project_name = "myapp"

  compose_file = file("docker-compose.yml")

  environment = {
    APP_ENV = "production"
  }
}
```

### Docker Network with Containers

```hcl
resource "docker_network" "app_network" {
  name   = "app-network"
  driver = "bridge"

  ipam {
    config {
      subnet  = "172.20.0.0/16"
      gateway = "172.20.0.1"
    }
  }
}

resource "docker_container" "app" {
  name    = "app"
  image   = "myapp:latest"
  network = docker_network.app_network.name
}
```

### Docker Swarm Service

```hcl
resource "docker_secret" "app_secret" {
  name = "app-secret"
  data = base64encode("secret-value")
}

resource "docker_service" "app" {
  name = "myapp"

  task_spec {
    container_spec {
      image = "myapp:latest"

      secrets {
        secret_id   = docker_secret.app_secret.id
        secret_name = docker_secret.app_secret.name
        file_name   = "/run/secrets/app_secret"
      }
    }

    resources {
      limits {
        memory_bytes = 536870912  # 512MB
      }
    }
  }

  mode {
    replicated {
      replicas = 3
    }
  }

  update_config {
    parallelism    = 1
    delay          = "10s"
    failure_action = "rollback"
  }

  endpoint_spec {
    ports {
      target_port    = 8080
      published_port = 80
    }
  }
}
```

### Docker Hub Repository

```hcl
resource "docker_hub_repository" "app" {
  namespace   = "myorg"
  name        = "myapp"
  description = "My application image"
  private     = true
}

resource "docker_org_team" "developers" {
  org_name         = "myorg"
  team_name        = "developers"
  team_description = "Development team"
}

resource "docker_hub_repository_team_permission" "dev_access" {
  namespace  = docker_hub_repository.app.namespace
  repository = docker_hub_repository.app.name
  team_name  = docker_org_team.developers.team_name
  permission = "write"
}
```

### Image Tag and Push

```hcl
resource "docker_tag" "app" {
  source_image = "myapp:latest"
  target_image = "registry.example.com/myapp:v1.0.0"
}

resource "docker_registry_image" "app" {
  name = docker_tag.app.target_image

  auth_config {
    address  = "registry.example.com"
    username = var.registry_username
    password = var.registry_password
  }
}
```

## Resources

### Docker Engine

| Resource | Description |
|----------|-------------|
| `docker_image` | Manages Docker images |
| `docker_container` | Manages Docker containers |
| `docker_network` | Manages Docker networks |
| `docker_volume` | Manages Docker volumes |
| `docker_compose` | Manages Docker Compose stacks |

### Docker Swarm

| Resource | Description |
|----------|-------------|
| `docker_service` | Manages Swarm services |
| `docker_secret` | Manages Swarm secrets |
| `docker_config` | Manages Swarm configs |

### Docker Registry

| Resource | Description |
|----------|-------------|
| `docker_tag` | Creates image tags |
| `docker_registry_image` | Pushes images to registries |

### Docker Hub

| Resource | Description |
|----------|-------------|
| `docker_hub_repository` | Manages Docker Hub repositories |
| `docker_hub_repository_team_permission` | Manages team permissions on repositories |
| `docker_org_team` | Manages organization teams |
| `docker_org_member` | Manages organization members |
| `docker_org_team_member` | Manages team memberships |
| `docker_access_token` | Manages Personal Access Tokens |

## Data Sources

### Docker Engine

| Data Source | Description |
|-------------|-------------|
| `docker_image` | Reads image information |
| `docker_container` | Reads container information |
| `docker_network` | Reads network information |
| `docker_compose` | Reads Compose stack information |
| `docker_logs` | Reads container logs |
| `docker_plugin` | Reads plugin information |
| `docker_registry_image` | Reads registry image digest |

### Docker Hub

| Data Source | Description |
|-------------|-------------|
| `docker_hub_repository` | Reads repository information |
| `docker_hub_repositories` | Lists repositories in a namespace |
| `docker_hub_repository_tags` | Lists repository tags |
| `docker_org` | Reads organization information |
| `docker_org_members` | Lists organization members |
| `docker_org_team` | Reads team information |
| `docker_access_tokens` | Lists Personal Access Tokens |

## Data Source Examples

### Read Container Logs

```hcl
data "docker_logs" "app" {
  name       = "my-container"
  tail       = "100"
  timestamps = true
}

output "logs" {
  value = data.docker_logs.app.logs_raw
}
```

### List Docker Hub Repositories

```hcl
data "docker_hub_repositories" "myorg" {
  namespace = "myorg"
}

output "repositories" {
  value = data.docker_hub_repositories.myorg.repositories[*].name
}
```

### Get Repository Tags

```hcl
data "docker_hub_repository_tags" "nginx" {
  namespace = "library"
  name      = "nginx"
}

output "latest_tags" {
  value = [for tag in data.docker_hub_repository_tags.nginx.tags : tag.name]
}
```

## Development

```bash
# Build
make build

# Install locally
make install

# Run tests
make test

# Run acceptance tests
make testacc

# Generate documentation
make docs

# Format code
make fmt

# Lint
make lint

# Clean build artifacts
make clean
```

## Documentation

Full documentation is available in the [docs](./docs) directory:

- [Provider Configuration](./docs/index.md)
- [Resources](./docs/resources/)
- [Data Sources](./docs/data-sources/)

## Architecture

```
internal/
├── docker/           # Docker Engine client wrapper
│   └── client.go
├── dockerhub/        # Docker Hub API client
│   └── client.go
└── provider/
    ├── provider.go   # Provider implementation
    │
    ├── # Docker Engine resources
    ├── image_resource.go
    ├── container_resource.go
    ├── network_resource.go
    ├── volume_resource.go
    ├── compose_resource.go
    │
    ├── # Swarm resources
    ├── service_resource.go
    ├── secret_resource.go
    ├── config_resource.go
    │
    ├── # Registry resources
    ├── tag_resource.go
    ├── registry_image_resource.go
    │
    ├── # Docker Hub resources
    ├── hub_repository_resource.go
    ├── hub_repository_team_permission_resource.go
    ├── org_team_resource.go
    ├── org_member_resource.go
    ├── org_team_member_resource.go
    ├── access_token_resource.go
    │
    └── # Data sources
        ├── *_data_source.go
```

## Notes

- **Docker Swarm features** require Docker to be running in Swarm mode (`docker swarm init`)
- **Docker Hub features** require authentication via `hub_username`/`hub_password` or `hub_token`
- **Registry push** requires appropriate authentication for the target registry

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

MIT License - see [LICENSE](LICENSE) for details.

## Related Projects

- [Docker](https://www.docker.com/) - Container platform
- [Docker Hub](https://hub.docker.com/) - Container registry
- [Terraform](https://www.terraform.io/) - Infrastructure as Code
- [terraform-plugin-framework](https://github.com/hashicorp/terraform-plugin-framework) - Terraform Provider SDK
