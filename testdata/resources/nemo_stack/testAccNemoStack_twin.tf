resource "nemo_stack" "test" {
  name       = "%[1]s"
  target     = { orchestrator = "compose" }
  parameters = { nemo-timeout = "6m" }
  installations = [
    { name = "hello", file = "%[2]s" },
  ]
}

resource "nemo_stack" "twin" {
  name   = "%[1]s"
  target = { orchestrator = "compose" }
  installations = [
    { name = "hello", file = "%[2]s" },
  ]
}
