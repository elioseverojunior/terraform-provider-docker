# Simple local volume
resource "docker_volume" "data" {
  name = "app-data"
}

# Volume with labels
resource "docker_volume" "logs" {
  name   = "app-logs"
  driver = "local"

  labels = {
    environment = "development"
    managed_by  = "terraform"
  }
}

# Volume with driver options (NFS example)
resource "docker_volume" "nfs" {
  name   = "nfs-volume"
  driver = "local"

  driver_opts = {
    type   = "nfs"
    o      = "addr=192.168.1.100,rw"
    device = ":/path/to/share"
  }
}
