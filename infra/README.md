# Deploying the API (AWS Lightsail + Caddy + GitHub Actions)

Terraform creates one Lightsail instance (Ubuntu 24.04, plan `micro_3_1`: 1 GB RAM, 2 vCPU, 40 GB disk, 1 TB transfer, about $7/month) in `ap-south-1` with a static IP. On first boot it
installs Caddy (HTTPS for `api.crizah.space`), creates a locked-down `foodmap` service user and a
`deploy` user, and sets up the systemd unit. After that, every push to `main` that touches `api/**`
builds the binary in GitHub Actions and ships it **plus `api/data/places.db`** to the server.

The web app (`web/`) is deployed separately by Vercel; it is not part of this.

## One-time setup

1. **Deploy key** (the private half goes to GitHub, the public half to Terraform):
   ```bash
   ssh-keygen -t ed25519 -f ~/.ssh/foodmap_deploy -C deploy@foodmap -N ""
   ```
2. **Create the server** (needs AWS credentials in your shell):
   ```bash
   cd infra
   cp terraform.tfvars.example terraform.tfvars   # paste ~/.ssh/foodmap_deploy.pub into ssh_public_key
   terraform init
   terraform plan
   terraform apply
   ```
3. **DNS:** at your domain provider add the record Terraform prints: `A  api.crizah.space -> <public_ip>`.
   Caddy fetches the certificate by itself once the record resolves.
4. **Host key** (wait ~2 min after apply for first boot to finish):
   ```bash
   ssh-keyscan -t ed25519 <public_ip>
   ```
5. **GitHub → Settings → Secrets and variables → Actions**, add:

   | Secret | Value |
   |---|---|
   | `DEPLOY_HOST` | the `public_ip` output |
   | `SSH_PRIVATE_KEY` | contents of `~/.ssh/foodmap_deploy` |
   | `SSH_KNOWN_HOSTS` | the full line printed by `ssh-keyscan` in step 4 |

6. **Vercel** env var for the web app: `API_URL=https://api.crizah.space`.
7. Push to `main` (or run the workflow manually from the Actions tab). Check: `curl https://api.crizah.space/healthz` → `ok`.

## Notes

- `terraform.tfstate` is local and git-ignored. Back it up, or add an S3 backend, before relying on it.
- The instance ignores AMI/user-data changes on purpose, so later `apply`s never replace it. To rebuild it
  deliberately: `terraform apply -replace=aws_lightsail_instance.api` (the next deploy re-uploads the DB).
- SSH is open to the internet (key-only, no passwords) because GitHub runners have no fixed IPs.
  Tighten `ssh_allowed_cidrs` if you deploy from somewhere else.
- Port 8080 is not exposed; only Caddy can reach the Go server. Caddy's access log is discarded because
  request URLs contain users' coordinates.
- Changing `FRONTEND_URL` later: SSH in as `ubuntu` (same key), edit `/opt/foodmap/.env` with sudo, then `sudo systemctl restart foodmap`.
- Lightsail is a flat monthly price: the public IPv4, disk and transfer are included, and the static IP is free while attached. Plans: `nano_3_1` (0.5 GB, about $5) or `micro_3_1` (1 GB, about $7); change `bundle_id` to switch. List plans with `aws lightsail get-bundles`.
- Lightsail has its own firewall (set in `main.tf`): only ports 22, 80 and 443 are open.
