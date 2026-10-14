output "public_ip" {
  description = "Static IP of the API server."
  value       = aws_lightsail_static_ip.api.ip_address
}

output "dns_record" {
  description = "Create this record at your DNS provider (host is just the subdomain, e.g. 'api')."
  value       = "A  ${var.api_domain}  ->  ${aws_lightsail_static_ip.api.ip_address}"
}

output "github_secrets" {
  description = "Values for the GitHub Actions secrets."
  value = {
    DEPLOY_HOST = aws_lightsail_static_ip.api.ip_address
  }
}

output "ssh_command" {
  value = "ssh ubuntu@${aws_lightsail_static_ip.api.ip_address}"
}
