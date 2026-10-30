terraform {
  required_providers {
    nemo = {
      source = "sntns/nemo"
    }
  }
}

# The Docker of every stack whose target gives neither host nor context.
# Without it, the environment's: DOCKER_HOST, the current docker context, or the local socket.
provider "nemo" {
  host = "tcp://10.1.0.10:2376"
  tls = {
    ca   = { path = "certs/ca.pem" }
    cert = { path = "certs/cert.pem" }
    key  = { env = "DOCKER_KEY" }
  }
}
