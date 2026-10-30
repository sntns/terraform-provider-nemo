# The latest 1.x of the api bundle, deployed at each apply that finds a newer one.
data "nemo_bundle" "api" {
  repository         = "registry.example.com/shop/api"
  version_constraint = "~> 1.4"
}

resource "nemo_stack" "shop" {
  name   = "shop"
  remote = "docker://registry.example.com/shop/{{ .Component }}:{{ .Version }}"
  target = { orchestrator = "swarm", host = "ssh://deploy@prod-1.example.com" }

  installations = [
    { name = "api", version = data.nemo_bundle.api.version },
  ]
}
