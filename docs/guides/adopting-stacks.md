---
page_title: "Adopting stacks"
subcategory: "Guides"
description: |-
  Find the stacks a Docker has, deployed with nemo or elsewhere, and bring them under Terraform: with terraform query, or an import block.
---

# Adopting stacks

A stack deployed with `nemo up`, or by another Terraform configuration, keeps its records on its target. The provider finds it there, and imports it with its installations.

## Import one stack

By its name, on the provider's Docker or the environment's:

```terraform
import {
  to = nemo_stack.shop
  id = "shop"
}
```

On another Docker, its address then its name, `tcp://10.1.0.10:2376/shop`. With Terraform 1.12 or later, an `identity` gives them apart:

```terraform
import {
  to       = nemo_stack.shop
  identity = { name = "shop", host = "tcp://10.1.0.10:2376" }
}
```

`terraform plan -generate-config-out=shop.tf` then writes its configuration.

## Find every stack, with `terraform query`

With Terraform 1.14 or later, the list resource `nemo_stack` finds every stack a Docker has the records of. In a `.tfquery.hcl` file:

```terraform
list "nemo_stack" "here" {
  provider         = nemo
  include_resource = true

  # config {
  #   host = "ssh://deploy@prod-1.example.com"
  # }
}
```

```shell
terraform query -generate-config-out=stacks.tf
```

```console
list.nemo_stack.here   host=<null>,name=shop    shop
list.nemo_stack.here   host=<null>,name=hello   hello
```

`stacks.tf` gets a `nemo_stack` and an `import` block for each: move them to the configuration, and the next apply imports them. The `config` block takes the `host` or `context` of the Docker, as a stack's `target` does; without it, the provider's.

## What an imported stack keeps

An imported stack gets its target, its installer and its installations from its records. The credentials and the parameters the records keep are those nemo resolved: give them in the configuration, as the stack needs them. The first apply then marks the stack as Terraform's, and nemo changes it only with `--force`.
