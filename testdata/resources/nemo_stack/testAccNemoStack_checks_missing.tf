resource "nemo_stack" "test" {
  name   = "%[1]s"
  target = { orchestrator = "compose" }
  installations = [
    { name = "gill", repository = "ghcr.io/sntns/gill", version = "0.0.0-none" },
  ]
}
