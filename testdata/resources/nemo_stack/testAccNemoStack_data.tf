resource "nemo_stack" "test" {
  name   = "%[1]s"
  target = { orchestrator = "compose" }
  installations = [
    { name = "hello", file = "%[2]s" },
  ]
}

data "nemo_stack" "read" {
  name = nemo_stack.test.name
}
