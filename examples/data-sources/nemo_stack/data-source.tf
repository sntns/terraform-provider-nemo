# A stack nemo deployed, read on the provider's Docker.
data "nemo_stack" "shop" {
  name = "shop"
}

output "api_version" {
  value = one([for installation in data.nemo_stack.shop.installations : installation.version if installation.name == "api"])
}
