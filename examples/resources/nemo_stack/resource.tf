resource "nemo_stack" "shop" {
  name   = "shop"
  remote = "docker://registry.example.com/shop/{{ .Component }}:{{ .Version }}"

  target = {
    orchestrator = "swarm"
    host         = "tcp://10.1.0.10:2376"
    tls = {
      ca   = file("pki/ca.pem")
      cert = file("pki/deployer.pem")
      key  = file("pki/deployer-key.pem")
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
    tls-certificate = { value = file("pki/shop.pem") }
    tls-key         = { value = file("pki/shop-key.pem") }
  }

  installations = [
    {
      name    = "database"
      version = "2.1.0"
      credentials = {
        database-password = { env = "SHOP_DATABASE_PASSWORD" }
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
