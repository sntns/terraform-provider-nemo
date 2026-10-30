---
page_title: "Getting started"
subcategory: "Guides"
description: |-
  From a compose file to a stack deployed on your Docker, upgraded, then destroyed.
---

# Getting started

This guide builds a bundle from a compose file, deploys it with Terraform or OpenTofu on the Docker of your machine, upgrades it, and destroys it. It takes about five minutes.

You need:

- Docker running locally: Docker Desktop or Docker Engine.
- Terraform or OpenTofu: the commands below are Terraform's, `tofu` takes the same.
- [nemo](https://github.com/sntns/nemo#install), to build the bundle: `brew install sntns/tap/nemo`. Deploying needs no nemo: the provider embeds it.

## 1. Make a bundle

A bundle is a compose file with an `x-nemo-bundle` block. In an empty directory, write `compose.yaml`:

```yaml
x-nemo-bundle:
  name: hello
  version: 0.1.0

services:
  web:
    image: traefik/whoami:v1.10
    ports: ["8080:80"]
```

Build it into a `bundle.json`:

```shell
nemo bundle build -f compose.yaml --image traefik/whoami:v1.10=docker://traefik/whoami:v1.10
```

`--image` says where the image comes from: nemo reads its digest, and the bundle always deploys exactly that image.

## 2. Deploy it

Next to it, write `main.tf`:

```terraform
terraform {
  required_providers {
    nemo = {
      source = "sntns/nemo"
    }
  }
}

resource "nemo_stack" "hello" {
  name   = "hello"
  target = { orchestrator = "compose" }

  installations = [
    { name = "hello", file = "${path.module}/bundle.json" },
  ]
}

output "manifest" {
  value     = nemo_stack.hello.manifest
  sensitive = true
}
```

With neither `host` nor `context`, the target is the Docker of the environment: `DOCKER_HOST`, the current docker context, or the local socket. Deploy:

```shell
terraform init
terraform apply
```

```console
$ curl -s localhost:8080 | head -1
Hostname: 5e0f9a3c4b1d
```

## 3. Upgrade it

Rebuild the bundle with another version: the provider follows the content of `bundle.json`, so the next plan shows the change.

```shell
nemo bundle build -f compose.yaml --version 0.2.0 --image traefik/whoami:v1.10=docker://traefik/whoami:v1.10
terraform apply
```

The apply upgrades the installation in place. Removing an installation from `installations` uninstalls it; adding one installs it, after the bundles it depends on.

## 4. See it with nemo

The stack's records are kept on its target. The `manifest` output gives them to nemo, from this machine or any other that reaches the target:

```shell
terraform output -raw manifest > hello.yaml
nemo stack import hello.yaml
nemo status
nemo logs hello
```

nemo reads the stack, and refuses to change it without `--force`: Terraform manages it, and its next apply undoes what `--force` did.

## 5. Destroy it

```shell
terraform destroy
```

The volumes of the bundles are kept for a next install, unless the stack sets `purge_on_destroy = true`.

## Next

- Publish bundles to a registry with `nemo bundle build -o docker://ghcr.io/you/hello`, and give the installations a `version` and the stack a `remote`, as in the [`nemo_stack` example](../resources/stack#example-usage).
- Deploy to a remote Docker with `target.host`: `ssh://user@host`, or `tcp://` with [`target.tls`](../resources/stack#nested-schema-for-targettls).
- Give the bundles their [secrets](secrets).
