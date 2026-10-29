# A secret that neither the plan nor the state keeps: an ephemeral value, given by value_wo.
variable "database_password" {
  type      = string
  ephemeral = true
}

resource "nemo_stack" "shop" {
  name   = "shop"
  remote = "docker://registry.example.com/shop/{{ .Component }}:{{ .Version }}"

  target = {
    orchestrator = "swarm"
    host         = "tcp://10.1.0.10:2376"
    tls = {
      ca   = { path = "certs/ca.pem" }
      cert = { path = "certs/cert.pem" }
      key  = { env = "DOCKER_KEY" }
    }
    volumes = {
      data = {
        driver  = "local"
        options = { type = "nfs", o = "addr=10.0.0.20,rw,nfsvers=4{{AppendJoin .Options \",\"}}", device = ":/exports/{{.Stack}}/{{.Name}}" }
        provision = {
          directory = "{{.Stack}}/{{.Name}}"
          volume    = { driver = "local", options = { type = "nfs", o = "addr=10.0.0.20,rw,nfsvers=4", device = ":/exports" } }
        }
      }
    }
  }

  parameters = {
    domain    = "shop.example.com"
    log-level = "info"
  }

  credentials = {
    tls-certificate = { path = "certs/shop.pem" }
    tls-key         = { env = "SHOP_TLS_KEY" }
  }

  installations = [
    {
      name    = "database"
      version = "2.1.0"
      credentials = {
        # Bump value_wo_version to give a new password: Terraform can't compare a write-only value.
        database-password = { value_wo = var.database_password, value_wo_version = "1" }
      }
    },
    {
      name       = "api"
      version    = "1.4.0"
      parameters = { log-level = "debug" }
    },
    { name = "frontend", version = "1.4.0" },
  ]

  # nemo bounds each action by nemo-timeout, 5m unless parameters give another: this bounds a whole apply.
  timeouts = {
    create = "30m"
    update = "30m"
  }
}

# For nemo, from where it reaches the target: nemo stack import manifest.yaml
output "manifest" {
  value     = nemo_stack.shop.manifest
  sensitive = true
}
