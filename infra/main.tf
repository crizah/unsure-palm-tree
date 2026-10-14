resource "aws_lightsail_key_pair" "deploy" {
  name       = "${var.name}-deploy"
  public_key = var.ssh_public_key
}

resource "aws_lightsail_instance" "api" {
  name              = var.name
  availability_zone = "${var.region}${var.availability_zone_suffix}"
  blueprint_id      = "ubuntu_24_04"
  bundle_id         = var.bundle_id
  key_pair_name     = aws_lightsail_key_pair.deploy.name
  ip_address_type   = "ipv4"

  user_data = templatefile("${path.module}/user_data.sh.tftpl", {
    api_domain     = var.api_domain
    frontend_urls  = var.frontend_urls
    ssh_public_key = var.ssh_public_key
  })

  tags = {
    Name = var.name
  }

  lifecycle {
    # An edited bootstrap script must not replace the instance: that would wipe
    # the server's data. Replace deliberately with `terraform apply -replace=aws_lightsail_instance.api`.
    ignore_changes = [user_data]
  }
}

# Fixed public IP that survives stop/start and instance replacement. Free while attached.
resource "aws_lightsail_static_ip" "api" {
  name = "${var.name}-ip"
}

resource "aws_lightsail_static_ip_attachment" "api" {
  static_ip_name = aws_lightsail_static_ip.api.name
  instance_name  = aws_lightsail_instance.api.name
}

# Lightsail's firewall: only these ports are open. 8080 (the Go server) is deliberately NOT
# opened: only Caddy, on the same machine, reaches it.
resource "aws_lightsail_instance_public_ports" "api" {
  instance_name = aws_lightsail_instance.api.name

  port_info {
    protocol  = "tcp"
    from_port = 80
    to_port   = 80
    cidrs     = ["0.0.0.0/0"] # certificate challenge + redirect to HTTPS
  }

  port_info {
    protocol  = "tcp"
    from_port = 443
    to_port   = 443
    cidrs     = ["0.0.0.0/0"]
  }

  port_info {
    protocol  = "tcp"
    from_port = 22
    to_port   = 22
    cidrs     = var.ssh_allowed_cidrs # key-only SSH
  }
}
