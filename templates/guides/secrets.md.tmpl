---
page_title: "Secrets"
subcategory: "Guides"
description: |-
  Give the credentials of the bundles from a file, the environment, a command, or as a write-only value that neither the plan nor the state keeps.
---

# Secrets

A bundle declares **credentials**: the secrets of its compose file. The stack gives each a **source**, read where Terraform runs, when an action needs it.

## Where a credential is given

`credentials` on the stack go to every installation whose bundle declares them; an installation's own `credentials` come over them.

```terraform
resource "nemo_stack" "shop" {
  # ...
  credentials = {
    tls-certificate = { path = "certs/shop.pem" }
  }

  installations = [
    {
      name    = "database"
      version = "2.1.0"
      credentials = {
        database-password = { env = "SHOP_DATABASE_PASSWORD" }
      }
    },
  ]
}
```

## Which source to choose

Each credential has one source:

| Source | Read from | The state keeps |
|---|---|---|
| `path` | a file | its path |
| `env` | an environment variable | its name |
| `command` | the output of a command, such as `vault kv get -field=password secret/shop` | the command |
| `value` | the configuration | the value, as a sensitive attribute |
| `value_wo` | the configuration, write-only | nothing |

`path`, `env` and `command` keep the secret out of Terraform altogether: it is read at each action, on the machine that runs it. Prefer them when the secret is there already, in a file a CI job mounts, or behind a command of your secret manager.

`value` is the simplest, from a variable or another resource's attribute, but the state then keeps the secret: keep the state encrypted.

## Write-only values

`value_wo` takes the value from the configuration and keeps it neither in the plan nor in the state. It needs Terraform 1.11 or OpenTofu 1.11, or later. It takes an ephemeral value, such as an ephemeral variable or an ephemeral resource:

```terraform
variable "database_password" {
  type      = string
  ephemeral = true
}

resource "nemo_stack" "shop" {
  # ...
  installations = [
    {
      name    = "database"
      version = "2.1.0"
      credentials = {
        database-password = { value_wo = var.database_password, value_wo_version = "1" }
      }
    },
  ]
}
```

Terraform can't compare a value it doesn't keep: to give a new one, change `value_wo_version` too, and the next apply gives `value_wo` again.

## What nemo sees

The stack's `manifest` output keeps the sources, and redacts `value` and `value_wo`. A nemo that imported it asks for a redacted value when one of its actions needs it, and reads the others from their source, on its own machine.

The TLS of a `tcp://` target, `target.tls`, is read from a source in the same way, but has no `value_wo`: every operation of the provider needs it.
