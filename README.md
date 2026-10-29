# terraform-provider-nemo

Deploys stacks of bundles with [nemo](https://github.com/sntns/nemo), from Terraform: a `nemo_stack` is the manifest nemo reads, its target, its parameters, its credentials and its installations, deployed on the target's Docker with Swarm or Compose.

```hcl
terraform {
  required_providers {
    nemo = { source = "sntns/nemo" }
  }
}

resource "nemo_stack" "shop" {
  name   = "shop"
  remote = "docker://registry.example.com/shop/{{ .Component }}:{{ .Version }}"
  target = { orchestrator = "swarm", host = "ssh://deploy@prod-1.example.com" }
  installations = [
    { name = "api", version = "1.4.0" },
    { name = "frontend", version = "1.4.0" },
  ]
}
```

The stack keeps the records of its installations on its target: from any machine that imported its `manifest` output, `nemo status`, `nemo logs` and `nemo installation runs` read it. nemo writes it only with `--force`, which the next apply undoes.

The resource is documented in [docs](docs), generated from its schema with `go generate ./...`; an example is in [examples](examples).

`make test` runs the unit tests, and `make testacc` the acceptance tests, which deploy a stack on the environment's Docker with Compose.
