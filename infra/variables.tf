variable "region" {
  description = "AWS region. Mumbai is closest to Delhi/Bangalore users."
  type        = string
  default     = "ap-south-1"
}

variable "name" {
  description = "Name prefix for resources."
  type        = string
  default     = "crizah_seo"
}

variable "instance_type" {
  type    = string
  default = "t3.micro"
}

variable "api_domain" {
  description = "Public hostname of the API. Caddy gets a Let's Encrypt cert for it."
  type        = string
  default     = "api.crizah.space"
}

variable "frontend_urls" {
  description = "Browser origin(s) allowed by CORS (FRONTEND_URL), comma-separated, no paths."
  type        = string
  default     = "https://crizah.space"
}

variable "ssh_public_key" {
  description = "Public key for the deploy user. The matching private key goes in the GitHub secret SSH_PRIVATE_KEY."
  type        = string

  validation {
    condition     = can(regex("^ssh-(ed25519|rsa) [^']+$", var.ssh_public_key))
    error_message = "ssh_public_key must be a single-line OpenSSH public key (ssh-ed25519 ... or ssh-rsa ...) with no single quotes."
  }
}

variable "ssh_allowed_cidrs" {
  description = "CIDRs allowed to reach SSH (port 22). GitHub Actions runners have no fixed IPs, so the default is open; access is key-only. Tighten if you deploy from elsewhere."
  type        = list(string)
  default     = ["0.0.0.0/0"]
}

variable "root_volume_gb" {
  type    = number
  default = 8
}
