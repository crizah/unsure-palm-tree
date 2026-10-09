output "public_ip" {
  description = "Elastic IP of the API server."
  value       = aws_eip.api.public_ip
}

output "dns_record" {
  description = "Create this record at your DNS provider."
  value       = "A  ${var.api_domain}  ->  ${aws_eip.api.public_ip}"
}

output "github_secrets" {
  description = "Values for the GitHub Actions secrets."
  value = {
    DEPLOY_HOST = aws_eip.api.public_ip
  }
}

output "ssh_command" {
  value = "ssh deploy@${aws_eip.api.public_ip}"
}
