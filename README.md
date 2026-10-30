# terraform-provider-nemo

[![Terraform Registry](https://img.shields.io/badge/terraform-sntns%2Fnemo-7B42BC?logo=terraform)](https://registry.terraform.io/providers/sntns/nemo)
[![OpenTofu Registry](https://img.shields.io/badge/opentofu-sntns%2Fnemo-FFDA18?logo=opentofu)](https://search.opentofu.org/provider/sntns/nemo)
[![Latest release](https://img.shields.io/github/v/release/sntns/terraform-provider-nemo?sort=semver)](https://github.com/sntns/terraform-provider-nemo/releases/latest)
[![Release](https://github.com/sntns/terraform-provider-nemo/actions/workflows/release.yml/badge.svg)](https://github.com/sntns/terraform-provider-nemo/actions/workflows/release.yml)
[![License: Apache-2.0](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

Deploys applications made of containers on any Docker host from Terraform or OpenTofu, with [nemo](https://github.com/sntns/nemo): Compose on one machine, Swarm on many, without Kubernetes. A `nemo_stack` is the manifest nemo reads, its target, its parameters, its credentials and its installations, each a versioned bundle of a compose file.

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

## Documentation

On the [Terraform Registry](https://registry.terraform.io/providers/sntns/nemo/latest/docs) and the [OpenTofu Registry](https://search.opentofu.org/provider/sntns/nemo/latest):

- [Getting started](docs/guides/getting-started.md): from a compose file to a stack deployed on your Docker.
- [Secrets](docs/guides/secrets.md): credentials from a file, the environment, a command, or write-only.
- [Swarm with several managers](docs/guides/swarm-records.md): the stack's records on a shared storage.
- [`nemo_stack`](docs/resources/stack.md): every setting of a stack.

## Development

The docs are generated from the schema and [templates](templates) with `go generate ./...`; the examples are in [examples](examples).

`make test` runs the unit tests, and `make testacc` the acceptance tests, which deploy a stack on the environment's Docker with Compose. They run with OpenTofu with `TF_ACC_TERRAFORM_PATH=$(which tofu) TF_ACC_PROVIDER_HOST=registry.opentofu.org`.

## Maintainers

Made and maintained by [Sentiens](https://www.sentiens.fr) ([LinkedIn](https://www.linkedin.com/company/sntns)), by [@ttnlmn](https://github.com/ttnlmn). Issues and pull requests are welcome.
