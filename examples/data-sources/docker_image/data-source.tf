# Get information about an existing image
data "docker_image" "nginx" {
  name = "nginx:latest"
}

output "nginx_image_id" {
  value = data.docker_image.nginx.image_id
}

output "nginx_repo_digests" {
  value = data.docker_image.nginx.repo_digests
}
