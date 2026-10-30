# Two bundles of a registry, deployed with Swarm on a Docker reached over SSH.
resource "nemo_stack" "shop" {
  name   = "shop"
  remote = "docker://registry.example.com/shop/{{ .Component }}:{{ .Version }}"
  target = { orchestrator = "swarm", host = "ssh://deploy@prod-1.example.com" }

  installations = [
    { name = "api", version = "1.4.0" },
    { name = "frontend", version = "1.4.0" },
  ]
}
