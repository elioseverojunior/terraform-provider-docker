# Required resources
resource "docker_image" "nginx" {
  name = "nginx:latest"
}

resource "docker_network" "app" {
  name = "app-network"
}

resource "docker_volume" "html" {
  name = "nginx-html"
}

# Simple container
resource "docker_container" "web" {
  name  = "web-server"
  image = docker_image.nginx.image_id

  ports {
    internal = 80
    external = 8080
  }
}

# Container with full configuration
resource "docker_container" "app" {
  name  = "app-container"
  image = docker_image.nginx.image_id

  # Environment variables
  env = {
    "NGINX_HOST" = "localhost"
    "NGINX_PORT" = "80"
  }

  # Labels
  labels = {
    "app"         = "web"
    "environment" = "development"
  }

  # Port mappings
  ports {
    internal = 80
    external = 8081
    protocol = "tcp"
  }

  ports {
    internal = 443
    external = 8443
    protocol = "tcp"
  }

  # Volume mounts
  volumes {
    volume_name    = docker_volume.html.name
    container_path = "/usr/share/nginx/html"
    read_only      = false
  }

  # Network attachment
  networks = [docker_network.app.name]

  # Resource limits
  memory     = 536870912 # 512MB
  cpu_shares = 512

  # Restart policy
  restart = "unless-stopped"

  # Health check
  healthcheck {
    test     = ["CMD", "curl", "-f", "http://localhost/"]
    interval = "30s"
    timeout  = "10s"
    retries  = 3
  }
}
